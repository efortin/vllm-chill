// Package proxy provides the HTTP proxy and autoscaling logic for vLLM deployments.
package proxy

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/efortin/vllm-chill/pkg/kubernetes"
	"github.com/efortin/vllm-chill/pkg/stats"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"k8s.io/client-go/dynamic"
	k8sclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	defaultScaleUpTimeout    = 2 * time.Minute
	defaultCheckInterval     = 10 * time.Second
	configDriftCheckInterval = 30 * time.Second // Check for config drift every 30s
)

// AutoScaler manages automatic scaling of vLLM deployments
type AutoScaler struct {
	clientset    *k8sclient.Clientset
	crdClient    *kubernetes.CRDClient
	k8sManager   *kubernetes.K8sManager
	config       *Config
	targetURL    *url.URL
	lastActivity time.Time
	activeModel  string // Currently active model ID
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

	// Construct target URL from environment variables
	targetHost := os.Getenv("VLLM_TARGET")
	targetPort := os.Getenv("VLLM_PORT")
	if targetHost == "" {
		targetHost = "vllm"
	}
	if targetPort == "" {
		targetPort = "80"
	}
	targetURL, err := url.Parse(fmt.Sprintf("http://%s:%s", targetHost, targetPort))
	if err != nil {
		return nil, fmt.Errorf("invalid target URL: %w", err)
	}

	k8sManagerConfig := &kubernetes.Config{
		Namespace:     config.Namespace,
		Deployment:    config.Deployment,
		ConfigMapName: config.ConfigMapName,
		GPUCount:      config.GPUCount,
		CPUOffloadGB:  config.CPUOffloadGB,
	}

	as := &AutoScaler{
		clientset:    clientset,
		crdClient:    kubernetes.NewCRDClient(dynamicClient),
		k8sManager:   kubernetes.NewK8sManager(clientset, k8sManagerConfig),
		config:       config,
		targetURL:    targetURL,
		lastActivity: time.Now(),
		activeModel:  config.ModelID,
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

	// Start watching the active model for changes
	as.startModelWatch(ctx)

	// Start periodic config drift check
	go as.startConfigDriftCheck(ctx)

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

// podExists checks if the vLLM pod exists
func (as *AutoScaler) podExists(ctx context.Context) (bool, error) {
	return as.k8sManager.PodExists(ctx)
}

// managePod creates or deletes the pod based on the desired state
func (as *AutoScaler) managePod(ctx context.Context, create bool) error {
	start := time.Now()
	direction := "up"
	if !create {
		direction = "down"
		as.metrics.SetVLLMState(3) // stopping
	} else {
		as.metrics.SetVLLMState(1) // starting
	}

	var err error
	if create {
		// Get model config for the currently active model
		var modelConfig *kubernetes.ModelConfig
		activeModelID := as.activeModel
		modelConfig, err = as.crdClient.GetModel(ctx, activeModelID)
		if err != nil {
			as.metrics.RecordScaleOp(direction, false, time.Since(start))
			as.metrics.SetVLLMState(0) // failed to start, mark as stopped
			return fmt.Errorf("failed to get model config for '%s': %w", activeModelID, err)
		}

		log.Printf("Creating pod with model: %s (%s)", activeModelID, modelConfig.ModelName)
		err = as.k8sManager.CreatePod(ctx, modelConfig)
	} else {
		err = as.k8sManager.DeletePod(ctx)
	}

	if err != nil {
		as.metrics.RecordScaleOp(direction, false, time.Since(start))
		if !create {
			as.metrics.SetVLLMState(2) // failed to stop, keep as running
		} else {
			as.metrics.SetVLLMState(0) // failed to start, mark as stopped
		}
		return err
	}

	as.metrics.RecordScaleOp(direction, true, time.Since(start))
	if create {
		as.metrics.UpdateReplicas(1)
		log.Printf("Created pod %s/%s", as.config.Namespace, as.config.Deployment)
	} else {
		as.metrics.UpdateReplicas(0)
		shutdownDuration := time.Since(start)
		as.metrics.RecordVLLMShutdown(shutdownDuration)
		as.metrics.SetVLLMState(0) // stopped
		log.Printf("Deleted pod %s/%s (shutdown took %v)", as.config.Namespace, as.config.Deployment, shutdownDuration)
	}

	return nil
}

// waitForReady waits for the pod to be ready
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
			return fmt.Errorf("timeout waiting for pod to be ready")
		case <-ticker.C:
			pod, err := as.k8sManager.GetPod(ctx)
			if err != nil {
				continue
			}
			// Check if pod is ready
			for _, cond := range pod.Status.Conditions {
				if cond.Type == "Ready" && cond.Status == "True" {
					startupDuration := time.Since(startupStart)
					as.metrics.RecordVLLMStartup(startupDuration)
					as.metrics.SetVLLMState(2) // running
					log.Printf("Pod %s/%s is ready (startup took %v)", as.config.Namespace, as.config.Deployment, startupDuration)
					return nil
				}
			}
		}
	}
}

// ensureScaledUp ensures the pod is created and ready
func (as *AutoScaler) ensureScaledUp(ctx context.Context) error {
	as.mu.Lock()
	defer as.mu.Unlock()

	// Check if pod already exists
	exists, err := as.podExists(ctx)
	if err != nil {
		return err
	}

	if exists {
		// Pod already exists, just wait for ready
		// Use background context so request timeout doesn't cancel pod startup
		as.mu.Unlock()
		bgCtx, cancel := context.WithTimeout(context.Background(), defaultScaleUpTimeout)
		defer cancel()
		err := as.waitForReady(bgCtx, defaultScaleUpTimeout)
		as.mu.Lock()
		return err
	}

	// If another goroutine is already scaling up, wait
	if as.isScalingUp {
		log.Printf("Waiting for ongoing pod creation...")
		as.scaleUpCond.Wait()
		return nil
	}

	// We're the one creating the pod
	as.isScalingUp = true
	defer func() {
		as.isScalingUp = false
		as.scaleUpCond.Broadcast()
	}()

	log.Printf("Creating pod %s/%s...", as.config.Namespace, as.config.Deployment)

	as.mu.Unlock()
	err = as.managePod(ctx, true)
	if err != nil {
		as.mu.Lock()
		return err
	}

	// Use background context so request timeout doesn't cancel pod startup
	bgCtx, cancel := context.WithTimeout(context.Background(), defaultScaleUpTimeout)
	defer cancel()
	err = as.waitForReady(bgCtx, defaultScaleUpTimeout)
	as.mu.Lock()
	return err
}

// updateActivity updates the last activity timestamp
func (as *AutoScaler) updateActivity() {
	as.mu.Lock()
	defer as.mu.Unlock()
	as.lastActivity = time.Now()
	if as.metrics != nil {
		as.metrics.UpdateActivity()
	}
}

// proxyHandler handles incoming HTTP requests
func (as *AutoScaler) proxyHandler(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx := r.Context()

	// Wrap request body to capture size and check for model parameter
	var requestSize int64
	var requestedModel string
	if r.Body != nil {
		bodyReader := newBodyReader(r.Body)
		r.Body = bodyReader
		defer func() {
			requestSize = bodyReader.BytesRead()
		}()

		// Extract model from request body if this is a /v1/* endpoint
		if len(r.URL.Path) >= 3 && r.URL.Path[:3] == "/v1" {
			requestedModel = as.extractModelFromRequest(r)
		}
	}

	// Wrap response writer to capture status and size
	rw := newResponseWriter(w, as.config.LogOutput, as.metrics)
	defer func() {
		duration := time.Since(start)
		if as.metrics != nil {
			as.metrics.RecordRequest(r.Method, r.URL.Path, rw.Status(), duration, requestSize, rw.Size())
		}

		// Log output if enabled
		if as.config.LogOutput && len(rw.Body()) > 0 {
			log.Printf("Response body for %s %s: %s", r.Method, r.URL.Path, string(rw.Body()))
		}
	}()

	// Update activity
	as.updateActivity()

	// Handle automatic model switching for /v1/* endpoints
	var modelSwitched bool
	if requestedModel != "" {
		if err := as.handleModelSwitch(ctx, requestedModel); err != nil {
			// Check if this is a model not found error
			if modelNotFoundErr, ok := err.(*ModelNotFoundError); ok {
				log.Printf("Model not found: %s, returning available models", modelNotFoundErr.RequestedModel)
				as.returnAvailableModels(ctx, rw, modelNotFoundErr.RequestedModel)
				return
			}

			// Other errors
			log.Printf("Failed to switch model to %s: %v", requestedModel, err)
			rw.Header().Set("Content-Type", "application/json")
			rw.WriteHeader(http.StatusServiceUnavailable)
			response := map[string]interface{}{
				"error": map[string]interface{}{
					"message": fmt.Sprintf("Failed to switch to model %s: %v", requestedModel, err),
					"type":    "model_switch_error",
					"code":    "model_unavailable",
				},
			}
			if err := json.NewEncoder(rw).Encode(response); err != nil {
				log.Printf("Failed to encode response: %v", err)
			}
			return
		}

		// Check if we actually switched models
		as.mu.RLock()
		currentModel := as.activeModel
		as.mu.RUnlock()
		modelSwitched = (requestedModel == currentModel)
	}

	// Ensure deployment is scaled up
	if err := as.ensureScaledUp(ctx); err != nil {
		log.Printf("Failed to scale up: %v", err)

		// Determine if this is a model loading scenario
		isChat := r.URL.Path == "/v1/chat/completions"
		if isChat && (modelSwitched || requestedModel != "") {
			// Send loading message for chat completions
			as.sendLoadingMessage(rw, r, requestedModel)
			return
		}

		// Standard error response for non-chat endpoints
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

	// Proxy the request via HTTP
	proxy := httputil.NewSingleHostReverseProxy(as.targetURL)
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
	// Fetch vLLM metrics via HTTP
	client := &http.Client{Timeout: 30 * time.Second}
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
// It handles Anthropic API format transformation for Claude Code compatibility
func (as *AutoScaler) ginProxyHandler(c *gin.Context) {
	// Check if this is an Anthropic API request (/v1/messages)
	if strings.HasPrefix(c.Request.URL.Path, "/v1/messages") {
		as.handleAnthropicFormatRequest(c)
		return
	}

	// Fall back to original proxy handler for OpenAI format requests
	as.proxyHandler(c.Writer, c.Request)
}

// handleAnthropicFormatRequest transforms Anthropic format to OpenAI for vLLM
func (as *AutoScaler) handleAnthropicFormatRequest(c *gin.Context) {
	// Parse Anthropic format request body
	var anthropicBody map[string]interface{}
	if err := c.ShouldBindJSON(&anthropicBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Invalid request body: %v", err)})
		return
	}

	log.Printf("Received Anthropic format request, transforming to OpenAI format for vLLM")

	// Transform Anthropic format to OpenAI format
	openAIBody := transformAnthropicToOpenAI(anthropicBody)

	// Marshal to JSON
	openAIBytes, err := json.Marshal(openAIBody)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to marshal OpenAI request: %v", err)})
		return
	}

	// Create new request for vLLM with OpenAI format
	// Change path from /v1/messages to /v1/chat/completions
	newReq := c.Request.Clone(c.Request.Context())
	newReq.URL.Path = "/v1/chat/completions"
	newReq.Body = io.NopCloser(bytes.NewReader(openAIBytes))
	newReq.ContentLength = int64(len(openAIBytes))

	// Create a response recorder to capture vLLM response
	recorder := httptest.NewRecorder()

	// Call the existing proxy handler with transformed request
	as.proxyHandler(recorder, newReq)

	// Check if response is streaming
	isStreaming := strings.Contains(recorder.Header().Get("Content-Type"), "text/event-stream")

	if isStreaming {
		// Handle streaming response
		as.transformOpenAIStreamToAnthropic(c, recorder)
	} else {
		// Transform OpenAI response to Anthropic format
		anthropicResp, err := transformOpenAIResponseToAnthropic(recorder.Body.Bytes())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to transform response: %v", err)})
			return
		}

		// Copy response headers from vLLM
		for key, values := range recorder.Header() {
			for _, value := range values {
				c.Header(key, value)
			}
		}

		// Send Anthropic format response
		c.JSON(recorder.Code, anthropicResp)
	}
}

// Start implements operation.Manager interface for manual start
func (as *AutoScaler) Start(ctx context.Context) error {
	return as.ensureScaledUp(ctx)
}

// Stop implements operation.Manager interface for manual stop
func (as *AutoScaler) Stop(ctx context.Context) error {
	return as.managePod(ctx, false)
}

// UpdateActivity implements operation.Manager interface
func (as *AutoScaler) UpdateActivity() {
	as.updateActivity()
}

// GetActiveModel returns the currently active model ID
func (as *AutoScaler) GetActiveModel() string {
	as.mu.RLock()
	defer as.mu.RUnlock()
	return as.activeModel
}

// SwitchModel switches to a different model
func (as *AutoScaler) SwitchModel(ctx context.Context, modelID string) error {
	as.mu.Lock()
	defer as.mu.Unlock()

	// Check if pod is running - if so, delete it first
	exists, err := as.podExists(ctx)
	if err != nil {
		return fmt.Errorf("failed to check pod existence: %w", err)
	}

	if exists {
		log.Printf("Stopping current pod before switching to model: %s", modelID)
		as.mu.Unlock()
		if err := as.managePod(ctx, false); err != nil {
			as.mu.Lock()
			return fmt.Errorf("failed to stop current pod: %w", err)
		}
		as.mu.Lock()
	}

	// Update active model
	as.activeModel = modelID
	log.Printf("Switched active model to: %s", modelID)

	return nil
}

// GetModelConfig retrieves model configuration from CRD
func (as *AutoScaler) GetModelConfig(ctx context.Context, modelID string) (*kubernetes.ModelConfig, error) {
	return as.crdClient.GetModel(ctx, modelID)
}

// ListModels returns all available models from CRDs
func (as *AutoScaler) ListModels(ctx context.Context) ([]ModelInfo, error) {
	models, err := as.crdClient.ListModels(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]ModelInfo, 0, len(models))
	for _, model := range models {
		result = append(result, ModelInfo{
			Name:            model.Name,
			ServedModelName: model.Spec.ServedModelName,
			ModelName:       model.Spec.ModelName,
			MaxModelLen:     fmt.Sprintf("%d", model.Spec.MaxModelLen),
		})
	}

	return result, nil
}

// IsRunning returns whether the vLLM pod is currently running
func (as *AutoScaler) IsRunning() bool {
	ctx := context.Background()
	exists, err := as.podExists(ctx)
	if err != nil {
		return false
	}
	return exists
}

// ModelInfo represents basic model information
type ModelInfo struct {
	Name            string `json:"name"`
	ServedModelName string `json:"servedModelName"`
	ModelName       string `json:"modelName"`
	MaxModelLen     string `json:"maxModelLen"`
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
			exists, err := as.podExists(ctx)
			if err != nil {
				log.Printf("Failed to check pod existence: %v", err)
				continue
			}

			if exists {
				log.Printf("Idle for %v, deleting pod...", idleTime.Round(time.Second))
				if err := as.managePod(ctx, false); err != nil {
					log.Printf("Failed to delete pod: %v", err)
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
	}

	// Default proxy handler for all other routes
	router.NoRoute(as.ginProxyHandler)

	// Log all registered endpoints dynamically
	as.logRegisteredRoutes(router)

	return router.Run(":" + as.config.Port)
}

// logRegisteredRoutes dynamically logs all registered routes from the Gin router
func (as *AutoScaler) logRegisteredRoutes(router *gin.Engine) {
	// Determine base URL for endpoint logging
	baseURL := as.config.PublicEndpoint
	if baseURL == "" {
		baseURL = "http://0.0.0.0:" + as.config.Port
	}

	// Get all routes from the router
	routes := router.Routes()

	// Filter and log only the proxy routes (excluding health and readyz)
	for _, route := range routes {
		// Skip health check endpoints and internal routes
		if route.Path == "/health" || route.Path == "/readyz" {
			continue
		}

		// Only log routes that start with /proxy
		if len(route.Path) >= 6 && route.Path[:6] == "/proxy" {
			log.Printf("   %-4s %s%s", route.Method, baseURL, route.Path)
		}
	}
}

// startModelWatch starts watching the active model for configuration changes
func (as *AutoScaler) startModelWatch(ctx context.Context) {
	as.mu.RLock()
	activeModel := as.activeModel
	as.mu.RUnlock()

	// Watch the active model CRD
	err := as.crdClient.WatchModel(ctx, activeModel, func() {
		log.Printf("Model %s configuration changed, restarting vLLM pod...", activeModel)
		as.restartVLLMPod()
	})
	if err != nil {
		log.Printf("Warning: Failed to start watching model %s: %v", activeModel, err)
	}
}

// restartVLLMPod deletes the vLLM pod to force a restart with new configuration
func (as *AutoScaler) restartVLLMPod() {
	ctx := context.Background()

	// Check if pod exists
	exists, err := as.k8sManager.PodExists(ctx)
	if err != nil {
		log.Printf("Error checking if pod exists: %v", err)
		return
	}

	if !exists {
		log.Printf("vLLM pod doesn't exist, no restart needed")
		return
	}

	// Delete the pod - it will be recreated on next request with new config
	if err := as.k8sManager.DeletePod(ctx); err != nil {
		log.Printf("Error restarting vLLM pod: %v", err)
		return
	}

	log.Printf("Successfully deleted vLLM pod, will be recreated with new configuration on next request")
}

// startConfigDriftCheck periodically verifies that the running vLLM pod matches the CRD config
func (as *AutoScaler) startConfigDriftCheck(ctx context.Context) {
	ticker := time.NewTicker(configDriftCheckInterval)
	defer ticker.Stop()

	log.Printf("Started periodic config drift check (every %v)", configDriftCheckInterval)

	for {
		select {
		case <-ctx.Done():
			log.Printf("Stopped config drift check")
			return
		case <-ticker.C:
			as.checkConfigDrift(ctx)
		}
	}
}

// checkConfigDrift checks if the running pod config matches the CRD and restarts if needed
func (as *AutoScaler) checkConfigDrift(ctx context.Context) {
	as.mu.RLock()
	activeModel := as.activeModel
	as.mu.RUnlock()

	// Get current model config from CRD
	modelConfig, err := as.crdClient.GetModel(ctx, activeModel)
	if err != nil {
		log.Printf("Warning: Failed to get model config for drift check: %v", err)
		return
	}

	// Verify pod config matches
	matches, err := as.k8sManager.VerifyPodConfig(ctx, modelConfig)
	if err != nil {
		log.Printf("Warning: Failed to verify pod config: %v", err)
		return
	}

	if !matches {
		log.Printf("Config drift detected! vLLM pod config doesn't match CRD. Restarting pod...")
		as.restartVLLMPod()
	}
}

// extractModelFromRequest extracts the model parameter from the request body
func (as *AutoScaler) extractModelFromRequest(r *http.Request) string {
	// Read the body
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		return ""
	}

	// Restore the body for subsequent reads
	r.Body = newBodyReaderFromBytes(bodyBytes)

	// Parse JSON to extract model field
	var reqBody map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &reqBody); err != nil {
		return ""
	}

	// Extract model field
	if model, ok := reqBody["model"].(string); ok {
		return model
	}

	return ""
}

// handleModelSwitch checks if the requested model differs from the active model and switches if needed
func (as *AutoScaler) handleModelSwitch(ctx context.Context, requestedModel string) error {
	as.mu.RLock()
	currentModel := as.activeModel
	as.mu.RUnlock()

	// If the requested model is the same as the active model, no action needed
	if requestedModel == currentModel {
		return nil
	}

	log.Printf("Model switch detected: requested=%s, current=%s", requestedModel, currentModel)

	// Verify the requested model exists in CRDs
	if _, err := as.crdClient.GetModel(ctx, requestedModel); err != nil {
		// Model not found - return special error with available models
		return &ModelNotFoundError{
			RequestedModel: requestedModel,
		}
	}

	// Perform the model switch
	if err := as.SwitchModel(ctx, requestedModel); err != nil {
		return fmt.Errorf("failed to switch model: %w", err)
	}

	log.Printf("Successfully switched to model: %s", requestedModel)
	return nil
}

// ModelNotFoundError represents an error when a requested model is not found
type ModelNotFoundError struct {
	RequestedModel string
}

func (e *ModelNotFoundError) Error() string {
	return fmt.Sprintf("model '%s' not found", e.RequestedModel)
}

// returnAvailableModels returns available models in OpenAI /v1/models format
func (as *AutoScaler) returnAvailableModels(ctx context.Context, w http.ResponseWriter, requestedModel string) {
	// Get all available models from CRDs
	models, err := as.ListModels(ctx)
	if err != nil {
		log.Printf("Failed to list models: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		response := map[string]interface{}{
			"error": map[string]interface{}{
				"message": "Failed to retrieve available models",
				"type":    "internal_error",
				"code":    "model_list_error",
			},
		}
		_ = json.NewEncoder(w).Encode(response)
		return
	}

	// Build OpenAI-compatible model list response
	modelList := make([]map[string]interface{}, 0, len(models))
	for _, model := range models {
		modelList = append(modelList, map[string]interface{}{
			"id":       model.Name,
			"object":   "model",
			"created":  time.Now().Unix(),
			"owned_by": "vllm-chill",
		})
	}

	// Build error response with available models
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotFound)
	response := map[string]interface{}{
		"error": map[string]interface{}{
			"message": fmt.Sprintf("Model '%s' not found. Available models: %v", requestedModel, getModelNames(models)),
			"type":    "invalid_request_error",
			"code":    "model_not_found",
			"param":   "model",
		},
		"available_models": map[string]interface{}{
			"object": "list",
			"data":   modelList,
		},
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Failed to encode response: %v", err)
	}
}

// getModelNames extracts model names from ModelInfo slice
func getModelNames(models []ModelInfo) []string {
	names := make([]string, 0, len(models))
	for _, model := range models {
		names = append(names, model.Name)
	}
	return names
}

// sendLoadingMessage sends a streaming chat completion message indicating model is loading
func (as *AutoScaler) sendLoadingMessage(w http.ResponseWriter, r *http.Request, modelName string) {
	// Check if request expects streaming response
	var reqBody map[string]interface{}
	if r.Body != nil {
		bodyBytes, err := io.ReadAll(r.Body)
		if err == nil {
			_ = json.Unmarshal(bodyBytes, &reqBody)
		}
	}

	stream, _ := reqBody["stream"].(bool)

	if stream {
		// Send SSE streaming response
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)

		flusher, _ := w.(http.Flusher)

		// Send loading message chunk
		message := fmt.Sprintf("Model '%s' is loading, please wait...", modelName)
		chunk := map[string]interface{}{
			"id":      "chatcmpl-loading",
			"object":  "chat.completion.chunk",
			"created": time.Now().Unix(),
			"model":   modelName,
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"delta": map[string]interface{}{
						"role":    "assistant",
						"content": message,
					},
					"finish_reason": nil,
				},
			},
		}

		data, _ := json.Marshal(chunk)
		_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
		if flusher != nil {
			flusher.Flush()
		}

		// Send finish chunk
		finishChunk := map[string]interface{}{
			"id":      "chatcmpl-loading",
			"object":  "chat.completion.chunk",
			"created": time.Now().Unix(),
			"model":   modelName,
			"choices": []map[string]interface{}{
				{
					"index":         0,
					"delta":         map[string]interface{}{},
					"finish_reason": "stop",
				},
			},
		}

		finishData, _ := json.Marshal(finishChunk)
		_, _ = fmt.Fprintf(w, "data: %s\n\n", finishData)
		_, _ = fmt.Fprintf(w, "data: [DONE]\n\n")
		if flusher != nil {
			flusher.Flush()
		}
	} else {
		// Send non-streaming response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		message := fmt.Sprintf("Model '%s' is loading, please wait...", modelName)
		response := map[string]interface{}{
			"id":      "chatcmpl-loading",
			"object":  "chat.completion",
			"created": time.Now().Unix(),
			"model":   modelName,
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": message,
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]interface{}{
				"prompt_tokens":     0,
				"completion_tokens": 0,
				"total_tokens":      0,
			},
		}

		_ = json.NewEncoder(w).Encode(response)
	}
}

// transformAnthropicToOpenAI transforms Anthropic request format to OpenAI chat completion format
func transformAnthropicToOpenAI(anthropicBody map[string]interface{}) map[string]interface{} {
	openAIBody := make(map[string]interface{})

	// Extract model from Anthropic request
	if model, ok := anthropicBody["model"].(string); ok {
		openAIBody["model"] = model
	}

	// Build messages array
	openAIMessages := make([]map[string]interface{}, 0)

	// Add system message if present
	if system, ok := anthropicBody["system"].(string); ok && system != "" {
		openAIMessages = append(openAIMessages, map[string]interface{}{
			"role":    "system",
			"content": system,
		})
	}

	// Convert Anthropic messages to OpenAI format
	if messages, ok := anthropicBody["messages"].([]interface{}); ok {
		for _, msg := range messages {
			if msgMap, ok := msg.(map[string]interface{}); ok {
				openAIMsg := make(map[string]interface{})
				openAIMsg["role"] = msgMap["role"]

				// Handle content - can be string or array
				if content, ok := msgMap["content"].(string); ok {
					openAIMsg["content"] = content
				} else if contentArray, ok := msgMap["content"].([]interface{}); ok {
					// For complex content (images, etc), extract text parts
					var textParts []string
					for _, part := range contentArray {
						if partMap, ok := part.(map[string]interface{}); ok {
							if partType, ok := partMap["type"].(string); ok && partType == "text" {
								if text, ok := partMap["text"].(string); ok {
									textParts = append(textParts, text)
								}
							}
						}
					}
					openAIMsg["content"] = strings.Join(textParts, "\n")
				}

				openAIMessages = append(openAIMessages, openAIMsg)
			}
		}
	}

	openAIBody["messages"] = openAIMessages

	// Copy common parameters
	if maxTokens, ok := anthropicBody["max_tokens"]; ok {
		openAIBody["max_tokens"] = maxTokens
	}
	if temperature, ok := anthropicBody["temperature"]; ok {
		openAIBody["temperature"] = temperature
	}
	if stream, ok := anthropicBody["stream"]; ok {
		openAIBody["stream"] = stream
	}
	if topP, ok := anthropicBody["top_p"]; ok {
		openAIBody["top_p"] = topP
	}

	// Transform stop_sequences to stop
	if stopSeqs, ok := anthropicBody["stop_sequences"].([]interface{}); ok {
		openAIBody["stop"] = stopSeqs
	}

	// Transform tools to functions (for tool calling support)
	if tools, ok := anthropicBody["tools"].([]interface{}); ok {
		functions := make([]map[string]interface{}, 0, len(tools))
		for _, tool := range tools {
			if toolMap, ok := tool.(map[string]interface{}); ok {
				fn := make(map[string]interface{})
				if name, ok := toolMap["name"].(string); ok {
					fn["name"] = name
				}
				if desc, ok := toolMap["description"].(string); ok {
					fn["description"] = desc
				}
				// Transform input_schema to parameters
				if schema, ok := toolMap["input_schema"].(map[string]interface{}); ok {
					fn["parameters"] = schema
				}
				functions = append(functions, fn)
			}
		}
		if len(functions) > 0 {
			openAIBody["functions"] = functions
		}
	}

	// Transform tool_choice to function_call
	if toolChoice, ok := anthropicBody["tool_choice"].(map[string]interface{}); ok {
		if choiceType, ok := toolChoice["type"].(string); ok {
			switch choiceType {
			case "auto":
				openAIBody["function_call"] = "auto"
			case "any":
				openAIBody["function_call"] = "auto" // OpenAI doesn't have "any", use "auto"
			case "tool":
				if toolName, ok := toolChoice["name"].(string); ok {
					openAIBody["function_call"] = map[string]interface{}{
						"name": toolName,
					}
				}
			}
		}
	}

	return openAIBody
}

// transformOpenAIResponseToAnthropic transforms OpenAI response to Anthropic format
func transformOpenAIResponseToAnthropic(openAIBytes []byte) (map[string]interface{}, error) {
	var openAIResp map[string]interface{}
	if err := json.Unmarshal(openAIBytes, &openAIResp); err != nil {
		return nil, fmt.Errorf("failed to parse OpenAI response: %w", err)
	}

	anthropicResp := make(map[string]interface{})
	anthropicResp["id"] = openAIResp["id"]
	anthropicResp["type"] = "message"
	anthropicResp["role"] = "assistant"
	anthropicResp["model"] = openAIResp["model"]

	// Extract content from choices
	contentBlocks := make([]map[string]interface{}, 0)

	if choices, ok := openAIResp["choices"].([]interface{}); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]interface{}); ok {
			if message, ok := choice["message"].(map[string]interface{}); ok {
				// Handle text content
				if content, ok := message["content"].(string); ok && content != "" {
					contentBlocks = append(contentBlocks, map[string]interface{}{
						"type": "text",
						"text": content,
					})
				}

				// Handle function_call → tool_use transformation
				if functionCall, ok := message["function_call"].(map[string]interface{}); ok {
					toolUseBlock := map[string]interface{}{
						"type": "tool_use",
						"id":   fmt.Sprintf("toolu_%s", openAIResp["id"]), // Generate tool use ID
					}

					if name, ok := functionCall["name"].(string); ok {
						toolUseBlock["name"] = name
					}

					// Parse arguments JSON string to object
					if argsStr, ok := functionCall["arguments"].(string); ok {
						var argsObj map[string]interface{}
						if err := json.Unmarshal([]byte(argsStr), &argsObj); err == nil {
							toolUseBlock["input"] = argsObj
						} else {
							// If parsing fails, use empty object
							toolUseBlock["input"] = map[string]interface{}{}
						}
					}

					contentBlocks = append(contentBlocks, toolUseBlock)
				}
			}

			// Add stop reason
			if finishReason, ok := choice["finish_reason"].(string); ok {
				switch finishReason {
				case "stop":
					anthropicResp["stop_reason"] = "end_turn"
				case "length":
					anthropicResp["stop_reason"] = "max_tokens"
				case "function_call":
					anthropicResp["stop_reason"] = "tool_use"
				default:
					anthropicResp["stop_reason"] = finishReason
				}
			}
		}
	}

	// Set content blocks (or empty array if none)
	if len(contentBlocks) > 0 {
		anthropicResp["content"] = contentBlocks
	} else {
		anthropicResp["content"] = []map[string]interface{}{}
	}

	// Transform usage
	if usage, ok := openAIResp["usage"].(map[string]interface{}); ok {
		anthropicResp["usage"] = map[string]interface{}{
			"input_tokens":  usage["prompt_tokens"],
			"output_tokens": usage["completion_tokens"],
		}
	}

	return anthropicResp, nil
}

// transformOpenAIStreamToAnthropic transforms OpenAI streaming response to Anthropic format
func (as *AutoScaler) transformOpenAIStreamToAnthropic(c *gin.Context, recorder *httptest.ResponseRecorder) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Status(200)

	c.Stream(func(w io.Writer) bool {
		// Read the OpenAI stream from recorder
		scanner := bufio.NewScanner(recorder.Body)
		messageStartSent := false

		for scanner.Scan() {
			line := scanner.Text()

			if !strings.HasPrefix(line, "data: ") {
				continue
			}

			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				// Send message_stop event
				if _, err := fmt.Fprintf(w, "event: message_stop\ndata: {}\n\n"); err != nil {
					log.Printf("Error writing message_stop event: %v", err)
				}
				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}
				return false
			}

			// Parse OpenAI chunk
			var chunk map[string]interface{}
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue
			}

			// Send message_start event on first chunk
			if !messageStartSent {
				startEvent := map[string]interface{}{
					"type": "message_start",
					"message": map[string]interface{}{
						"id":    chunk["id"],
						"type":  "message",
						"role":  "assistant",
						"model": chunk["model"],
					},
				}
				startJSON, _ := json.Marshal(startEvent)
				if _, err := fmt.Fprintf(w, "event: message_start\ndata: %s\n\n", startJSON); err != nil {
					log.Printf("Error writing message_start event: %v", err)
				}
				messageStartSent = true

				// Send content_block_start
				contentBlockStart := map[string]interface{}{
					"type":  "content_block_start",
					"index": 0,
					"content_block": map[string]interface{}{
						"type": "text",
						"text": "",
					},
				}
				cbsJSON, _ := json.Marshal(contentBlockStart)
				if _, err := fmt.Fprintf(w, "event: content_block_start\ndata: %s\n\n", cbsJSON); err != nil {
					log.Printf("Error writing content_block_start event: %v", err)
				}

				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}
			}

			// Transform chunk to Anthropic format
			if choices, ok := chunk["choices"].([]interface{}); ok && len(choices) > 0 {
				if choice, ok := choices[0].(map[string]interface{}); ok {
					if delta, ok := choice["delta"].(map[string]interface{}); ok {
						if content, ok := delta["content"].(string); ok && content != "" {
							deltaEvent := map[string]interface{}{
								"type":  "content_block_delta",
								"index": 0,
								"delta": map[string]interface{}{
									"type": "text_delta",
									"text": content,
								},
							}
							deltaJSON, _ := json.Marshal(deltaEvent)
							if _, err := fmt.Fprintf(w, "event: content_block_delta\ndata: %s\n\n", deltaJSON); err != nil {
								log.Printf("Error writing content_block_delta event: %v", err)
							}

							if f, ok := w.(http.Flusher); ok {
								f.Flush()
							}
						}
					}

					// Check for finish
					if finishReason, ok := choice["finish_reason"].(string); ok && finishReason != "" {
						// Send content_block_stop
						contentBlockStop := map[string]interface{}{
							"type":  "content_block_stop",
							"index": 0,
						}
						cbstopJSON, _ := json.Marshal(contentBlockStop)
						if _, err := fmt.Fprintf(w, "event: content_block_stop\ndata: %s\n\n", cbstopJSON); err != nil {
							log.Printf("Error writing content_block_stop event: %v", err)
						}

						if f, ok := w.(http.Flusher); ok {
							f.Flush()
						}
					}
				}
			}
		}

		return false
	})
}
