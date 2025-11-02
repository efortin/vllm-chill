package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	defaultScaleUpTimeout = 2 * time.Minute
	defaultCheckInterval  = 10 * time.Second
)

// AutoScaler manages automatic scaling of vLLM deployments
type AutoScaler struct {
	clientset        *kubernetes.Clientset
	crdClient        *CRDClient
	config           *Config
	targetURL        *url.URL
	lastActivity     time.Time
	mu               sync.RWMutex
	isScalingUp      bool
	scaleUpCond      *sync.Cond
	isSwitchingModel bool
	switchingToModel string
	modelSwitchCond  *sync.Cond
}

// NewAutoScaler creates a new AutoScaler instance
func NewAutoScaler(config *Config) (*AutoScaler, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	// In-cluster config
	k8sConfig, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get in-cluster config: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(k8sConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create clientset: %w", err)
	}

	// Create dynamic client for CRD operations
	dynamicClient, err := dynamic.NewForConfig(k8sConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %w", err)
	}

	targetURL, err := url.Parse(config.GetTargetURL())
	if err != nil {
		return nil, fmt.Errorf("invalid target URL: %w", err)
	}

	as := &AutoScaler{
		clientset:    clientset,
		crdClient:    NewCRDClient(dynamicClient, config.Namespace),
		config:       config,
		targetURL:    targetURL,
		lastActivity: time.Now(),
	}
	as.scaleUpCond = sync.NewCond(&as.mu)
	as.modelSwitchCond = sync.NewCond(&as.mu)

	return as, nil
}

// getReplicas returns the current number of replicas for the deployment
func (as *AutoScaler) getReplicas(ctx context.Context) (int32, error) {
	dep, err := as.clientset.AppsV1().Deployments(as.config.Namespace).Get(
		ctx,
		as.config.Deployment,
		metav1.GetOptions{},
	)
	if err != nil {
		return 0, err
	}
	if dep.Spec.Replicas == nil {
		return 0, nil
	}
	return *dep.Spec.Replicas, nil
}

// scaleDeployment scales the deployment to the specified number of replicas
func (as *AutoScaler) scaleDeployment(ctx context.Context, replicas int32) error {
	dep, err := as.clientset.AppsV1().Deployments(as.config.Namespace).Get(
		ctx,
		as.config.Deployment,
		metav1.GetOptions{},
	)
	if err != nil {
		return err
	}

	dep.Spec.Replicas = &replicas
	_, err = as.clientset.AppsV1().Deployments(as.config.Namespace).Update(
		ctx,
		dep,
		metav1.UpdateOptions{},
	)
	if err != nil {
		return err
	}

	log.Printf("✅ Scaled %s/%s to %d replicas", as.config.Namespace, as.config.Deployment, replicas)
	return nil
}

// waitForReady waits for the deployment to have at least one ready replica
func (as *AutoScaler) waitForReady(ctx context.Context, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for deployment to be ready")
		case <-ticker.C:
			dep, err := as.clientset.AppsV1().Deployments(as.config.Namespace).Get(
				ctx,
				as.config.Deployment,
				metav1.GetOptions{},
			)
			if err != nil {
				continue
			}
			if dep.Status.ReadyReplicas > 0 {
				log.Printf("✅ Deployment %s/%s is ready", as.config.Namespace, as.config.Deployment)
				return nil
			}
		}
	}
}

// ensureScaledUp ensures the deployment is scaled up and ready
func (as *AutoScaler) ensureScaledUp(ctx context.Context) error {
	as.mu.Lock()
	defer as.mu.Unlock()

	// Check if already scaled up
	replicas, err := as.getReplicas(ctx)
	if err != nil {
		return err
	}

	if replicas > 0 {
		// Already scaled up, just wait for ready
		as.mu.Unlock()
		err := as.waitForReady(ctx, defaultScaleUpTimeout)
		as.mu.Lock()
		return err
	}

	// If another goroutine is already scaling up, wait
	if as.isScalingUp {
		log.Printf("⏳ Waiting for ongoing scale-up...")
		as.scaleUpCond.Wait()
		return nil
	}

	// We're the one scaling up
	as.isScalingUp = true
	defer func() {
		as.isScalingUp = false
		as.scaleUpCond.Broadcast()
	}()

	log.Printf("🔄 Scaling up %s/%s...", as.config.Namespace, as.config.Deployment)

	as.mu.Unlock()
	err = as.scaleDeployment(ctx, 1)
	if err != nil {
		as.mu.Lock()
		return err
	}

	err = as.waitForReady(ctx, defaultScaleUpTimeout)
	as.mu.Lock()
	return err
}

// updateActivity updates the last activity timestamp
func (as *AutoScaler) updateActivity() {
	as.mu.Lock()
	defer as.mu.Unlock()
	as.lastActivity = time.Now()
}

// proxyHandler handles incoming HTTP requests
func (as *AutoScaler) proxyHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Update activity
	as.updateActivity()

	// Check if a model switch is in progress
	as.mu.RLock()
	if as.isSwitchingModel {
		switchingTo := as.switchingToModel
		as.mu.RUnlock()

		// Return a user-friendly message
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusServiceUnavailable)
		response := map[string]interface{}{
			"error": map[string]interface{}{
				"message": fmt.Sprintf("Model '%s' is currently loading. Please wait and retry in a few moments.", switchingTo),
				"type":    "model_loading",
				"code":    "model_switching_in_progress",
			},
		}
		json.NewEncoder(w).Encode(response)
		return
	}
	as.mu.RUnlock()

	// If model switching is enabled and this is a chat completion request, handle model switching
	if as.config.EnableModelSwitch && r.URL.Path == "/v1/chat/completions" && r.Method == "POST" {
		// Extract the requested model from the request
		requestedModel, err := extractModelFromRequest(r)
		if err != nil {
			log.Printf("⚠️  Failed to extract model from request: %v", err)
			// Continue without model switching
		} else if requestedModel != "" {
			// Try to switch to the requested model
			if err := as.switchModelWithLock(ctx, requestedModel); err != nil {
				log.Printf("❌ Failed to switch model: %v", err)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusServiceUnavailable)
				response := map[string]interface{}{
					"error": map[string]interface{}{
						"message": fmt.Sprintf("Failed to switch to model '%s': %v", requestedModel, err),
						"type":    "model_switch_error",
						"code":    "model_switch_failed",
					},
				}
				json.NewEncoder(w).Encode(response)
				return
			}
		}
	}

	// Ensure deployment is scaled up
	if err := as.ensureScaledUp(ctx); err != nil {
		log.Printf("❌ Failed to scale up: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Retry-After", "10")
		w.WriteHeader(http.StatusServiceUnavailable)
		response := map[string]interface{}{
			"error": map[string]interface{}{
				"message": "Service is starting up. Please wait and retry in a few moments.",
				"type":    "service_unavailable",
				"code":    "scaling_up",
			},
		}
		json.NewEncoder(w).Encode(response)
		return
	}

	// Proxy the request
	proxy := httputil.NewSingleHostReverseProxy(as.targetURL)
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Printf("❌ Proxy error: %v", err)
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
	}

	proxy.ServeHTTP(w, r)
}

// healthHandler handles health check requests
func (as *AutoScaler) healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	io.WriteString(w, "OK")
}

// startIdleChecker starts a background goroutine that checks for idle time
func (as *AutoScaler) startIdleChecker() {
	ticker := time.NewTicker(defaultCheckInterval)
	defer ticker.Stop()

	for range ticker.C {
		as.mu.RLock()
		idleTime := time.Since(as.lastActivity)
		as.mu.RUnlock()

		if idleTime > as.config.GetIdleTimeout() {
			ctx := context.Background()
			replicas, err := as.getReplicas(ctx)
			if err != nil {
				log.Printf("❌ Failed to get replicas: %v", err)
				continue
			}

			if replicas > 0 {
				log.Printf("💤 Idle for %v, scaling to 0...", idleTime.Round(time.Second))
				if err := as.scaleDeployment(ctx, 0); err != nil {
					log.Printf("❌ Failed to scale down: %v", err)
				}
			}
		}
	}
}

// Start starts the HTTP server and idle checker
func (as *AutoScaler) Start() error {
	// Start idle checker
	go as.startIdleChecker()

	// Setup HTTP handlers
	http.HandleFunc("/health", as.healthHandler)
	http.HandleFunc("/readyz", as.healthHandler)
	http.HandleFunc("/", as.proxyHandler)

	return http.ListenAndServe(":"+as.config.Port, nil)
}
