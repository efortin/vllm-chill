// Package stats provides Prometheus metrics collection and GPU statistics monitoring.
package stats

import (
	"strconv"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// Request metrics
	requestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "vllm_chill_requests_total",
			Help: "Total number of requests received",
		},
		[]string{"method", "path", "status"},
	)

	requestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "vllm_chill_request_duration_seconds",
			Help:    "Request duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path", "status"},
	)

	requestPayloadSize = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "vllm_chill_request_payload_bytes",
			Help:    "Request payload size in bytes",
			Buckets: []float64{100, 1000, 10000, 100000, 1000000, 10000000},
		},
		[]string{"method", "path"},
	)

	responsePayloadSize = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "vllm_chill_response_payload_bytes",
			Help:    "Response payload size in bytes",
			Buckets: []float64{100, 1000, 10000, 100000, 1000000, 10000000},
		},
		[]string{"method", "path", "status"},
	)

	// Managed operations metrics
	managedOperations = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "vllm_chill_managed_operations_total",
			Help: "Total number of managed operations (model switches)",
		},
		[]string{"from_model", "to_model", "status"},
	)

	managedOperationDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "vllm_chill_managed_operation_duration_seconds",
			Help:    "Managed operation duration in seconds",
			Buckets: []float64{10, 30, 60, 120, 300, 600},
		},
		[]string{"from_model", "to_model"},
	)

	// Scaling metrics
	scaleOps = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "vllm_chill_scale_operations_total",
			Help: "Total number of scale operations",
		},
		[]string{"direction", "status"},
	)

	scaleOpDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "vllm_chill_scale_operation_duration_seconds",
			Help:    "Scale operation duration in seconds",
			Buckets: []float64{1, 5, 10, 30, 60, 120},
		},
		[]string{"direction"},
	)

	// Current state metrics
	currentReplicas = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "vllm_chill_current_replicas",
			Help: "Current number of replicas",
		},
	)

	idleTimeSeconds = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "vllm_chill_idle_time_seconds",
			Help: "Time since last activity in seconds",
		},
	)

	currentModel = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "vllm_chill_current_model",
			Help: "Current model loaded (1 if loaded, 0 otherwise)",
		},
		[]string{"model_name"},
	)

	// Proxy latency metrics
	proxyLatency = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "vllm_chill_proxy_latency_seconds",
			Help:    "Latency added by the proxy before forwarding to vLLM",
			Buckets: []float64{0.001, 0.005, 0.010, 0.025, 0.050, 0.100, 0.250, 0.500, 1.000},
		},
		[]string{"operation"},
	)

	// vLLM lifecycle metrics
	vllmStartupDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "vllm_chill_vllm_startup_duration_seconds",
			Help:    "Time taken for vLLM to start and become ready",
			Buckets: []float64{10, 20, 30, 45, 60, 90, 120, 180, 240, 300, 420, 600},
		},
	)

	vllmShutdownDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "vllm_chill_vllm_shutdown_duration_seconds",
			Help:    "Time taken for vLLM to shut down completely",
			Buckets: []float64{1, 2, 5, 10, 15, 30, 60},
		},
	)

	vllmState = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "vllm_chill_vllm_state",
			Help: "Current vLLM state: 0=stopped, 1=starting, 2=running, 3=stopping",
		},
	)
)

// MetricsRecorder handles recording metrics
type MetricsRecorder struct {
	mu               sync.RWMutex
	currentModelName string
	lastActivityTime time.Time
	idleTimeUpdater  *time.Ticker
	stopIdleUpdater  chan struct{}
}

// NewMetricsRecorder creates a new metrics recorder
func NewMetricsRecorder() *MetricsRecorder {
	mr := &MetricsRecorder{
		lastActivityTime: time.Now(),
		idleTimeUpdater:  time.NewTicker(10 * time.Second),
		stopIdleUpdater:  make(chan struct{}),
	}

	// Start idle time updater
	go mr.updateIdleTime()

	return mr
}

// Stop stops the metrics recorder
func (mr *MetricsRecorder) Stop() {
	close(mr.stopIdleUpdater)
	mr.idleTimeUpdater.Stop()
}

// RecordRequest records a request with its metrics
func (mr *MetricsRecorder) RecordRequest(method, path string, status int, duration time.Duration, requestSize, responseSize int64) {
	statusStr := strconv.Itoa(status)

	requestsTotal.WithLabelValues(method, path, statusStr).Inc()
	requestDuration.WithLabelValues(method, path, statusStr).Observe(duration.Seconds())

	if requestSize > 0 {
		requestPayloadSize.WithLabelValues(method, path).Observe(float64(requestSize))
	}
	if responseSize > 0 {
		responsePayloadSize.WithLabelValues(method, path, statusStr).Observe(float64(responseSize))
	}
}

// RecordManagedOperation records a managed operation (model switch)
func (mr *MetricsRecorder) RecordManagedOperation(fromModel, toModel string, success bool, duration time.Duration) {
	status := "success"
	if !success {
		status = "failure"
	}

	managedOperations.WithLabelValues(fromModel, toModel, status).Inc()
	if success {
		managedOperationDuration.WithLabelValues(fromModel, toModel).Observe(duration.Seconds())
	}

	// Update current model
	mr.mu.Lock()
	if mr.currentModelName != "" && mr.currentModelName != toModel {
		currentModel.WithLabelValues(mr.currentModelName).Set(0)
	}
	if success {
		mr.currentModelName = toModel
		currentModel.WithLabelValues(toModel).Set(1)
	}
	mr.mu.Unlock()
}

// RecordScaleOp records a scale operation
func (mr *MetricsRecorder) RecordScaleOp(direction string, success bool, duration time.Duration) {
	status := "success"
	if !success {
		status = "failure"
	}

	scaleOps.WithLabelValues(direction, status).Inc()
	if success {
		scaleOpDuration.WithLabelValues(direction).Observe(duration.Seconds())
	}
}

// UpdateReplicas updates the current replica count
func (mr *MetricsRecorder) UpdateReplicas(replicas int32) {
	currentReplicas.Set(float64(replicas))
}

// UpdateActivity updates the last activity time
func (mr *MetricsRecorder) UpdateActivity() {
	mr.mu.Lock()
	mr.lastActivityTime = time.Now()
	mr.mu.Unlock()
}

// updateIdleTime periodically updates the idle time metric
func (mr *MetricsRecorder) updateIdleTime() {
	for {
		select {
		case <-mr.idleTimeUpdater.C:
			mr.mu.RLock()
			idle := time.Since(mr.lastActivityTime).Seconds()
			mr.mu.RUnlock()
			idleTimeSeconds.Set(idle)
		case <-mr.stopIdleUpdater:
			return
		}
	}
}

// SetCurrentModel sets the current model name
func (mr *MetricsRecorder) SetCurrentModel(modelName string) {
	mr.mu.Lock()
	defer mr.mu.Unlock()

	if mr.currentModelName != "" && mr.currentModelName != modelName {
		currentModel.WithLabelValues(mr.currentModelName).Set(0)
	}
	mr.currentModelName = modelName
	currentModel.WithLabelValues(modelName).Set(1)
}

// RecordProxyLatency records the latency added by the proxy
func (mr *MetricsRecorder) RecordProxyLatency(operation string, duration time.Duration) {
	proxyLatency.WithLabelValues(operation).Observe(duration.Seconds())
}

// RecordVLLMStartup records the time taken for vLLM to start
func (mr *MetricsRecorder) RecordVLLMStartup(duration time.Duration) {
	vllmStartupDuration.Observe(duration.Seconds())
}

// RecordVLLMShutdown records the time taken for vLLM to shut down
func (mr *MetricsRecorder) RecordVLLMShutdown(duration time.Duration) {
	vllmShutdownDuration.Observe(duration.Seconds())
}

// SetVLLMState sets the current vLLM state
// States: 0=stopped, 1=starting, 2=running, 3=stopping
func (mr *MetricsRecorder) SetVLLMState(state int) {
	vllmState.Set(float64(state))
}
