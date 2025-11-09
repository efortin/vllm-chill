// Package proxy provides the HTTP proxy and autoscaling logic for vLLM deployments.
package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"sync"
	"time"

	"github.com/efortin/vllm-chill/pkg/kubernetes"
	"github.com/efortin/vllm-chill/pkg/operation"
	"github.com/efortin/vllm-chill/pkg/stats"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	k8sclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	defaultScaleUpTimeout = 2 * time.Minute
	defaultCheckInterval  = 10 * time.Second
)

// AutoScaler manages automatic scaling of vLLM deployments
type AutoScaler struct {
	clientset    *k8sclient.Clientset
	crdClient    *kubernetes.CRDClient
	k8sManager   *kubernetes.K8sManager
	config       *Config
	targetURL    *url.URL
	lastActivity time.Time
	mu           sync.RWMutex
	isScalingUp  bool
	scaleUpCond  *sync.Cond
	metrics      *stats.MetricsRecorder
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
	restConfig, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get in-cluster config: %w", err)
	}

	clientset, err := k8sclient.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create clientset: %w", err)
	}

	// Create dynamic client for CRD operations
	dynamicClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %w", err)
	}

	// For Unix socket, we use a placeholder URL scheme
	// The actual socket path is in config.TargetSocket
	targetURL, err := url.Parse("http://unix")
	if err != nil {
		return nil, fmt.Errorf("invalid target URL: %w", err)
	}

	k8sManagerConfig := &kubernetes.Config{
		Namespace:     config.Namespace,
		Deployment:    config.Deployment,
		ConfigMapName: config.ConfigMapName,
	}

	as := &AutoScaler{
		clientset:    clientset,
		crdClient:    kubernetes.NewCRDClient(dynamicClient),
		k8sManager:   kubernetes.NewK8sManager(clientset, k8sManagerConfig),
		config:       config,
		targetURL:    targetURL,
		lastActivity: time.Now(),
		metrics:      stats.NewMetricsRecorder(),
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

// SetTargetURL sets the target URL for the autoscaler (used for testing)
func (as *AutoScaler) SetTargetURL(url *url.URL) {
	as.mu.Lock()
	defer as.mu.Unlock()
	as.targetURL = url
}

// SetMetrics sets the metrics recorder for the autoscaler (used for testing)
func (as *AutoScaler) SetMetrics(m *stats.MetricsRecorder) {
	as.mu.Lock()
	defer as.mu.Unlock()
	as.metrics = m
}

// GetMetrics returns the metrics recorder
func (as *AutoScaler) GetMetrics() *stats.MetricsRecorder {
	as.mu.RLock()
	defer as.mu.RUnlock()
	return as.metrics
}

// newUnixSocketTransport creates an HTTP transport that connects via Unix socket
func (as *AutoScaler) newUnixSocketTransport() *http.Transport {
	socketPath := as.config.GetTargetSocket()
	return &http.Transport{
		DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
			return net.Dial("unix", socketPath)
		},
	}
}

// newUnixSocketClient creates an HTTP client that connects via Unix socket
func (as *AutoScaler) newUnixSocketClient() *http.Client {
	return &http.Client{
		Transport: as.newUnixSocketTransport(),
		Timeout:   30 * time.Second,
	}
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
		as.metrics.SetVLLMState(3) // stopping
	} else {
		as.metrics.SetVLLMState(1) // starting
	}

	dep, err := as.clientset.AppsV1().Deployments(as.config.Namespace).Get(
		ctx,
		as.config.Deployment,
		metav1.GetOptions{},
	)
	if err != nil {
		as.metrics.RecordScaleOp(direction, false, time.Since(start))
		if replicas == 0 {
			as.metrics.SetVLLMState(2) // failed to stop, keep as running
		} else {
			as.metrics.SetVLLMState(0) // failed to start, mark as stopped
		}
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
		if replicas == 0 {
			as.metrics.SetVLLMState(2) // failed to stop, keep as running
		} else {
			as.metrics.SetVLLMState(0) // failed to start, mark as stopped
		}
		return err
	}

	as.metrics.RecordScaleOp(direction, true, time.Since(start))
	as.metrics.UpdateReplicas(replicas)

	// Record shutdown duration when scaling to 0
	if replicas == 0 {
		shutdownDuration := time.Since(start)
		as.metrics.RecordVLLMShutdown(shutdownDuration)
		as.metrics.SetVLLMState(0) // stopped
		log.Printf("Scaled %s/%s to 0 replicas (shutdown took %v)", as.config.Namespace, as.config.Deployment, shutdownDuration)
	} else {
		log.Printf("Scaled %s/%s to %d replicas", as.config.Namespace, as.config.Deployment, replicas)
	}

	return nil
}

// waitForReady waits for the deployment to have at least one ready replica
func (as *AutoScaler) waitForReady(ctx context.Context, timeout time.Duration) error {
	startupStart := time.Now()
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			as.metrics.SetVLLMState(0) // failed to start, mark as stopped
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
				startupDuration := time.Since(startupStart)
				as.metrics.RecordVLLMStartup(startupDuration)
				as.metrics.SetVLLMState(2) // running
				log.Printf("Deployment %s/%s is ready (startup took %v)", as.config.Namespace, as.config.Deployment, startupDuration)
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
	rw := newResponseWriter(w, as.config.LogOutput, as.metrics)
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

	// Proxy the request via Unix socket
	proxy := httputil.NewSingleHostReverseProxy(as.targetURL)
	proxy.Transport = as.newUnixSocketTransport()
	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, err error) {
		log.Printf("Proxy error: %v", err)
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
	}

	proxy.ServeHTTP(rw, r)
}

// healthHandler handles health check requests
func (as *AutoScaler) healthHandler(c *gin.Context) {
	c.String(http.StatusOK, "OK")
}

// versionHandler handles version information requests
func (as *AutoScaler) versionHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"version":    as.version,
		"commit":     as.commit,
		"build_date": as.buildDate,
	})
}

// MetricsHandler combines vLLM metrics with proxy metrics
func (as *AutoScaler) MetricsHandler(c *gin.Context) {
	// Fetch vLLM metrics via Unix socket
	client := as.newUnixSocketClient()
	vllmMetricsURL := fmt.Sprintf("%s/metrics", as.targetURL.String())
	resp, err := client.Get(vllmMetricsURL)

	var vllmMetrics string
	if err != nil {
		log.Printf("Warning: Failed to fetch vLLM metrics: %v", err)
		vllmMetrics = fmt.Sprintf("# vLLM metrics unavailable: %v\n", err)
	} else {
		defer func() {
			if err := resp.Body.Close(); err != nil {
				log.Printf("Warning: Failed to close response body: %v", err)
			}
		}()
		if resp.StatusCode == http.StatusOK {
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				vllmMetrics = fmt.Sprintf("# Failed to read vLLM metrics: %v\n", err)
			} else {
				vllmMetrics = string(body)
			}
		} else {
			vllmMetrics = fmt.Sprintf("# vLLM metrics returned status %d\n", resp.StatusCode)
		}
	}

	// Get proxy metrics from Prometheus handler
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/metrics", nil)
	promhttp.Handler().ServeHTTP(w, req)
	proxyMetrics := w.Body.String()

	// Combine metrics
	combined := fmt.Sprintf("# vLLM Metrics\n%s\n# Proxy Metrics\n%s", vllmMetrics, proxyMetrics)

	c.Header("Content-Type", "text/plain; version=0.0.4")
	c.String(http.StatusOK, combined)
}

// ginProxyHandler wraps the proxyHandler for Gin
func (as *AutoScaler) ginProxyHandler(c *gin.Context) {
	as.proxyHandler(c.Writer, c.Request)
}

// Start implements operation.Manager interface for manual start
func (as *AutoScaler) Start(ctx context.Context) error {
	return as.ensureScaledUp(ctx)
}

// Stop implements operation.Manager interface for manual stop
func (as *AutoScaler) Stop(ctx context.Context) error {
	return as.scaleDeployment(ctx, 0)
}

// UpdateActivity implements operation.Manager interface
func (as *AutoScaler) UpdateActivity() {
	as.updateActivity()
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

// Run starts the HTTP server and idle checker
func (as *AutoScaler) Run() error {
	// Start idle checker
	go as.startIdleChecker()

	// Setup Gin router
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())

	// Health endpoints
	router.GET("/health", as.healthHandler)
	router.GET("/readyz", as.healthHandler)

	// Proxy group
	proxyGroup := router.Group("/proxy")
	{
		// Metrics endpoint - combines vLLM metrics + proxy metrics
		proxyGroup.GET("/metrics", as.MetricsHandler)
		proxyGroup.GET("/version", as.versionHandler)

		// GPU stats endpoint
		gpuStatsHandler := stats.NewGinGPUStatsHandler()
		proxyGroup.GET("/stats", gpuStatsHandler.Handler)

		// Manual operation endpoints
		operationHandler := operation.NewGinHandler(as)
		proxyGroup.POST("/operations/start", operationHandler.StartHandler)
		proxyGroup.POST("/operations/stop", operationHandler.StopHandler)
	}

	// Default proxy handler for all other routes
	router.NoRoute(as.ginProxyHandler)

	log.Printf("   Metrics endpoint: http://0.0.0.0:%s/proxy/metrics", as.config.Port)
	log.Printf("   GPU stats endpoint: http://0.0.0.0:%s/proxy/stats", as.config.Port)
	log.Printf("   Version endpoint: http://0.0.0.0:%s/proxy/version", as.config.Port)
	log.Printf("   Manual start endpoint: http://0.0.0.0:%s/proxy/operations/start", as.config.Port)
	log.Printf("   Manual stop endpoint: http://0.0.0.0:%s/proxy/operations/stop", as.config.Port)

	return router.Run(":" + as.config.Port)
}
