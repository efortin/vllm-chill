// Package proxy provides the HTTP proxy and autoscaling logic for vLLM deployments.
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

	"github.com/prometheus/client_golang/prometheus/promhttp"
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
	k8sManager       *K8sManager
	config           *Config
	targetURL        *url.URL
	lastActivity     time.Time
	mu               sync.RWMutex
	isScalingUp      bool
	scaleUpCond      *sync.Cond
	isSwitchingModel bool
	switchingToModel string
	modelSwitchCond  *sync.Cond
	metrics          *MetricsRecorder
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
		k8sManager:   NewK8sManager(clientset, config),
		config:       config,
		targetURL:    targetURL,
		lastActivity: time.Now(),
		metrics:      NewMetricsRecorder(),
	}
	as.scaleUpCond = sync.NewCond(&as.mu)
	as.modelSwitchCond = sync.NewCond(&as.mu)

	// Verify prerequisites at startup
	ctx := context.Background()
	if err := as.verifyPrerequisites(ctx); err != nil {
		return nil, fmt.Errorf("prerequisite check failed: %w", err)
	}

	// Ensure K8s resources exist (managed mode is always enabled)
	// Get the first available model from CRD as initial model
	models, err := as.crdClient.ListModels(ctx)
	if err != nil || len(models) == 0 {
		log.Printf("Warning: No VLLMModels found. Resources will be created when first model is requested.")
	} else {
		// Use the first model as initial configuration
		initialModel, err := as.crdClient.GetModel(ctx, models[0].Spec.ServedModelName)
		if err != nil {
			log.Printf("Warning: Failed to get initial model config: %v", err)
		} else {
			if err := as.k8sManager.EnsureVLLMResources(ctx, initialModel); err != nil {
				log.Printf("Warning: Failed to ensure vLLM resources: %v", err)
			}
		}
	}

	return as, nil
}

// verifyPrerequisites checks that all required permissions and CRDs are available
func (as *AutoScaler) verifyPrerequisites(ctx context.Context) error {
	log.Printf("Verifying prerequisites...")

	// Check if VLLMModel CRD exists
	log.Printf("Checking if VLLMModel CRD is installed...")
	_, err := as.crdClient.dynamicClient.Resource(vllmModelGVR).Namespace(as.config.Namespace).List(ctx, metav1.ListOptions{Limit: 1})
	if err != nil {
		return fmt.Errorf("VLLMModel CRD not found or not accessible. Please install it with: kubectl apply -f manifests/crds/vllmmodel.yaml. Error: %w", err)
	}
	log.Printf("✓ VLLMModel CRD is installed")

	// Check deployment permissions (get, list, create, update)
	log.Printf("Checking deployment permissions...")
	_, err = as.clientset.AppsV1().Deployments(as.config.Namespace).List(ctx, metav1.ListOptions{Limit: 1})
	if err != nil {
		return fmt.Errorf("cannot list deployments in namespace %s. Check RBAC permissions. Error: %w", as.config.Namespace, err)
	}
	log.Printf("✓ Deployment list permission verified")

	// Check service permissions
	log.Printf("Checking service permissions...")
	_, err = as.clientset.CoreV1().Services(as.config.Namespace).List(ctx, metav1.ListOptions{Limit: 1})
	if err != nil {
		return fmt.Errorf("cannot list services in namespace %s. Check RBAC permissions. Error: %w", as.config.Namespace, err)
	}
	log.Printf("✓ Service list permission verified")

	// Check configmap permissions
	log.Printf("Checking configmap permissions...")
	_, err = as.clientset.CoreV1().ConfigMaps(as.config.Namespace).List(ctx, metav1.ListOptions{Limit: 1})
	if err != nil {
		return fmt.Errorf("cannot list configmaps in namespace %s. Check RBAC permissions. Error: %w", as.config.Namespace, err)
	}
	log.Printf("✓ ConfigMap list permission verified")

	log.Printf("✓ All prerequisites verified successfully")
	return nil
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
	start := time.Now()
	direction := "up"
	if replicas == 0 {
		direction = "down"
	}

	dep, err := as.clientset.AppsV1().Deployments(as.config.Namespace).Get(
		ctx,
		as.config.Deployment,
		metav1.GetOptions{},
	)
	if err != nil {
		// If deployment doesn't exist and we're scaling up, try to create resources first
		if replicas > 0 {
			log.Printf("Deployment not found, attempting to create resources...")
			if createErr := as.ensureResourcesExist(ctx); createErr != nil {
				log.Printf("Failed to create resources: %v", createErr)
				as.metrics.RecordScaleOp(direction, false, time.Since(start))
				return fmt.Errorf("deployment not found and failed to create: %w", createErr)
			}
			// Wait briefly for Kubernetes to process the resource creation (eventually consistent)
			log.Printf("Waiting for deployment to be registered in Kubernetes...")
			time.Sleep(2 * time.Second)

			// Try to get deployment again after creation
			dep, err = as.clientset.AppsV1().Deployments(as.config.Namespace).Get(
				ctx,
				as.config.Deployment,
				metav1.GetOptions{},
			)
			if err != nil {
				as.metrics.RecordScaleOp(direction, false, time.Since(start))
				return fmt.Errorf("deployment still not found after creation: %w", err)
			}
		} else {
			as.metrics.RecordScaleOp(direction, false, time.Since(start))
			return err
		}
	}

	dep.Spec.Replicas = &replicas
	_, err = as.clientset.AppsV1().Deployments(as.config.Namespace).Update(
		ctx,
		dep,
		metav1.UpdateOptions{},
	)
	if err != nil {
		as.metrics.RecordScaleOp(direction, false, time.Since(start))
		return err
	}

	as.metrics.RecordScaleOp(direction, true, time.Since(start))
	as.metrics.UpdateReplicas(replicas)

	log.Printf("Scaled %s/%s to %d replicas", as.config.Namespace, as.config.Deployment, replicas)
	return nil
}

// ensureResourcesExist ensures that the deployment, service, and configmap exist
func (as *AutoScaler) ensureResourcesExist(ctx context.Context) error {
	log.Printf("Ensuring vLLM resources exist...")

	// Try to get the first available model from CRD
	models, err := as.crdClient.ListModels(ctx)
	if err != nil {
		return fmt.Errorf("failed to list VLLMModels from CRD: %w", err)
	}
	if len(models) == 0 {
		return fmt.Errorf("no VLLMModels found in CRD, cannot create resources")
	}

	// Use the first model as initial configuration
	initialModel, err := as.crdClient.GetModel(ctx, models[0].Spec.ServedModelName)
	if err != nil {
		return fmt.Errorf("failed to get initial model config: %w", err)
	}

	// Ensure all resources exist
	if err := as.k8sManager.EnsureVLLMResources(ctx, initialModel); err != nil {
		return fmt.Errorf("failed to ensure vLLM resources: %w", err)
	}

	log.Printf("Successfully ensured vLLM resources exist")
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
				log.Printf("Deployment %s/%s is ready", as.config.Namespace, as.config.Deployment)
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
		log.Printf("Waiting for ongoing scale-up...")
		as.scaleUpCond.Wait()
		return nil
	}

	// We're the one scaling up
	as.isScalingUp = true
	defer func() {
		as.isScalingUp = false
		as.scaleUpCond.Broadcast()
	}()

	log.Printf("Scaling up %s/%s...", as.config.Namespace, as.config.Deployment)

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
	as.metrics.UpdateActivity()
}

// proxyHandler handles incoming HTTP requests
func (as *AutoScaler) proxyHandler(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx := r.Context()

	// Wrap request body to capture size
	var requestSize int64
	if r.Body != nil {
		bodyReader := newBodyReader(r.Body)
		r.Body = bodyReader
		defer func() {
			requestSize = bodyReader.BytesRead()
		}()
	}

	// Wrap response writer to capture status and size
	rw := newResponseWriter(w, as.config.LogOutput)
	defer func() {
		duration := time.Since(start)
		as.metrics.RecordRequest(r.Method, r.URL.Path, rw.Status(), duration, requestSize, rw.Size())

		// Log output if enabled
		if as.config.LogOutput && len(rw.Body()) > 0 {
			log.Printf("Response body for %s %s: %s", r.Method, r.URL.Path, string(rw.Body()))
		}
	}()

	// Update activity
	as.updateActivity()

	// Check if a managed operation is in progress
	as.mu.RLock()
	if as.isSwitchingModel {
		switchingTo := as.switchingToModel
		as.mu.RUnlock()

		// Return a user-friendly message
		rw.Header().Set("Content-Type", "application/json")
		rw.Header().Set("Retry-After", "30")
		rw.WriteHeader(http.StatusServiceUnavailable)
		response := map[string]interface{}{
			"error": map[string]interface{}{
				"message": fmt.Sprintf("Model '%s' is currently loading. Please wait and retry in a few moments.", switchingTo),
				"type":    "model_loading",
				"code":    "managed_operation_in_progress",
			},
		}
		if err := json.NewEncoder(rw).Encode(response); err != nil {
			log.Printf("Failed to encode response: %v", err)
		}
		return
	}
	as.mu.RUnlock()

	// Handle model management for chat completion requests (managed mode is always enabled)
	if r.URL.Path == "/v1/chat/completions" && r.Method == "POST" {
		// Extract the requested model from the request
		requestedModel, err := extractModelFromRequest(r)
		if err != nil {
			log.Printf("Failed to extract model from request: %v", err)
			// Continue without model management
		} else if requestedModel != "" {
			// Try to switch to the requested model
			if err := as.switchModelWithLock(ctx, requestedModel); err != nil {
				log.Printf("Failed to switch model: %v", err)
				rw.Header().Set("Content-Type", "application/json")
				rw.WriteHeader(http.StatusServiceUnavailable)
				response := map[string]interface{}{
					"error": map[string]interface{}{
						"message": fmt.Sprintf("Failed to switch to model '%s': %v", requestedModel, err),
						"type":    "managed_operation_error",
						"code":    "managed_operation_failed",
					},
				}
				if err := json.NewEncoder(rw).Encode(response); err != nil {
					log.Printf("Failed to encode response: %v", err)
				}
				return
			}
		}
	}

	// Ensure deployment is scaled up
	if err := as.ensureScaledUp(ctx); err != nil {
		log.Printf("Failed to scale up: %v", err)
		rw.Header().Set("Content-Type", "application/json")
		rw.Header().Set("Retry-After", "10")
		rw.WriteHeader(http.StatusServiceUnavailable)
		response := map[string]interface{}{
			"error": map[string]interface{}{
				"message": "Service is starting up. Please wait and retry in a few moments.",
				"type":    "service_unavailable",
				"code":    "scaling_up",
			},
		}
		if err := json.NewEncoder(rw).Encode(response); err != nil {
			log.Printf("Failed to encode response: %v", err)
		}
		return
	}

	// Proxy the request
	proxy := httputil.NewSingleHostReverseProxy(as.targetURL)
	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, err error) {
		log.Printf("Proxy error: %v", err)
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
	}

	proxy.ServeHTTP(rw, r)
}

// healthHandler handles health check requests
func (as *AutoScaler) healthHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	if _, err := io.WriteString(w, "OK"); err != nil {
		log.Printf("Failed to write health response: %v", err)
	}
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
				log.Printf("Failed to get replicas: %v", err)
				continue
			}

			if replicas > 0 {
				log.Printf("Idle for %v, scaling to 0...", idleTime.Round(time.Second))
				if err := as.scaleDeployment(ctx, 0); err != nil {
					log.Printf("Failed to scale down: %v", err)
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
	mux := http.NewServeMux()
	mux.HandleFunc("/health", as.healthHandler)
	mux.HandleFunc("/readyz", as.healthHandler)

	// Add metrics endpoint
	if as.config.EnableMetrics {
		mux.Handle("/metrics", promhttp.Handler())
		log.Printf("   Metrics endpoint: http://0.0.0.0:%s/metrics", as.config.Port)
	}

	mux.HandleFunc("/", as.proxyHandler)

	return http.ListenAndServe(":"+as.config.Port, mux)
}
