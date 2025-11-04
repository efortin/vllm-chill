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
	clientset    *kubernetes.Clientset
	crdClient    *CRDClient
	k8sManager   *K8sManager
	config       *Config
	targetURL    *url.URL
	lastActivity time.Time
	mu           sync.RWMutex
	isScalingUp  bool
	scaleUpCond  *sync.Cond
	metrics      *MetricsRecorder
	version      string
	commit       string
	buildDate    string
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
		crdClient:    NewCRDClient(dynamicClient),
		k8sManager:   NewK8sManager(clientset, config),
		config:       config,
		targetURL:    targetURL,
		lastActivity: time.Now(),
		metrics:      NewMetricsRecorder(),
		version:      "dev",
		commit:       "none",
		buildDate:    "unknown",
	}
	as.scaleUpCond = sync.NewCond(&as.mu)

	// Ensure K8s resources exist with the configured model
	ctx := context.Background()
	modelConfig, err := as.crdClient.GetModel(ctx, config.ModelID)
	if err != nil {
		return nil, fmt.Errorf("failed to get model '%s' from CRD: %w", config.ModelID, err)
	}
	if err := as.k8sManager.EnsureVLLMResources(ctx, modelConfig); err != nil {
		return nil, fmt.Errorf("failed to ensure vLLM resources: %w", err)
	}
	log.Printf("Loaded model configuration: %s", config.ModelID)

	return as, nil
}

// SetVersion sets the version information for the autoscaler
func (as *AutoScaler) SetVersion(version, commit, buildDate string) {
	as.version = version
	as.commit = commit
	as.buildDate = buildDate
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
		as.metrics.RecordScaleOp(direction, false, time.Since(start))
		return err
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

// versionHandler handles version information requests
func (as *AutoScaler) versionHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	response := map[string]string{
		"version":    as.version,
		"commit":     as.commit,
		"build_date": as.buildDate,
	}
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Failed to encode version response: %v", err)
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

	// Add metrics endpoint (always enabled) - use /proxy/metrics to avoid conflict with vLLM's /metrics
	mux.Handle("/proxy/metrics", promhttp.Handler())
	mux.HandleFunc("/proxy/version", as.versionHandler)

	log.Printf("   Metrics endpoint: http://0.0.0.0:%s/proxy/metrics", as.config.Port)
	log.Printf("   Version endpoint: http://0.0.0.0:%s/proxy/version", as.config.Port)

	mux.HandleFunc("/", as.proxyHandler)

	return http.ListenAndServe(":"+as.config.Port, mux)
}
