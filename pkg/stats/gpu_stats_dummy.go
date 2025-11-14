//go:build (dummy_gpu || test) && !cgo

package stats

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// GPUStatsHandler serves GPU statistics from dummy NVML
type GPUStatsHandler struct {
	deviceCount int
}

// NewGPUStatsHandler creates a new GPU stats handler and initializes mock NVML
func NewGPUStatsHandler() *GPUStatsHandler {
	h := &GPUStatsHandler{}

	// Initialize mock NVML
	if err := nvmlInit(); err != nil {
		log.Printf("[GPU-STATS] Warning: Failed to initialize mock NVML: %v", err)
		h.deviceCount = 0
		return h
	}

	// Get device count
	count, err := nvmlDeviceGetCount()
	if err != nil {
		log.Printf("[GPU-STATS] Warning: Failed to get device count: %v", err)
		h.deviceCount = 0
	} else {
		h.deviceCount = count
		log.Printf("[GPU-STATS] Mock NVML initialized with %d GPU(s)", count)
	}

	return h
}

// Shutdown cleanly shuts down mock NVML
func (h *GPUStatsHandler) Shutdown() error {
	return nvmlShutdown()
}

// ServeHTTP serves GPU statistics as JSON
func (h *GPUStatsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	stats, err := h.getStats()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-GPU-Type", "mock")
	if err := json.NewEncoder(w).Encode(stats); err != nil {
		log.Printf("[GPU-STATS] Failed to encode response: %v", err)
	}
}

// getStats collects GPU statistics from all devices
func (h *GPUStatsHandler) getStats() (*GPUStats, error) {
	stats := &GPUStats{
		Timestamp: time.Now(),
		GPUs:      make([]GPU, 0, h.deviceCount),
	}

	for i := 0; i < h.deviceCount; i++ {
		device, err := nvmlDeviceGetHandleByIndex(i)
		if err != nil {
			log.Printf("[GPU-STATS] Failed to get device %d: %v", i, err)
			continue
		}

		gpu := GPU{Index: i}

		// Get device name
		if name, err := nvmlDeviceGetName(device); err == nil {
			gpu.Name = name
		}

		// Get device UUID
		if uuid, err := nvmlDeviceGetUUID(device); err == nil {
			gpu.UUID = uuid
		}

		// Get temperature
		if temp, err := nvmlDeviceGetTemperature(device); err == nil {
			gpu.Temperature = int(temp)
		}

		// Get utilization
		if gpuUtil, memUtil, err := nvmlDeviceGetUtilizationRates(device); err == nil {
			gpu.Utilization = float64(gpuUtil)
			gpu.MemoryUtil = float64(memUtil)
		}

		// Get memory info
		if used, _, total, err := nvmlDeviceGetMemoryInfo(device); err == nil {
			gpu.MemoryUsed = int64(used) / (1024 * 1024)   // Convert to MB
			gpu.MemoryTotal = int64(total) / (1024 * 1024) // Convert to MB
		}

		// Get power usage
		if power, err := nvmlDeviceGetPowerUsage(device); err == nil {
			gpu.PowerDraw = float64(power) / 1000.0 // Convert mW to W
		}

		stats.GPUs = append(stats.GPUs, gpu)
	}

	return stats, nil
}

// GinGPUStatsHandler wraps GPUStatsHandler for Gin
type GinGPUStatsHandler struct {
	handler *GPUStatsHandler
}

// NewGinGPUStatsHandler creates a new Gin-compatible GPU stats handler
func NewGinGPUStatsHandler() *GinGPUStatsHandler {
	return &GinGPUStatsHandler{
		handler: NewGPUStatsHandler(),
	}
}

// Handle is the Gin handler function
func (h *GinGPUStatsHandler) Handle(c *gin.Context) {
	stats, err := h.handler.getStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Header("X-GPU-Type", "mock")
	c.JSON(http.StatusOK, stats)
}

// Handler returns the Gin handler function (alias for Handle)
func (h *GinGPUStatsHandler) Handler(c *gin.Context) {
	h.Handle(c)
}

// Shutdown cleanly shuts down the handler
func (h *GinGPUStatsHandler) Shutdown() error {
	if h.handler != nil {
		return h.handler.Shutdown()
	}
	return nil
}
