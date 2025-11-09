package stats

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

// GPUStats represents GPU statistics
type GPUStats struct {
	Timestamp time.Time `json:"timestamp"`
	GPUs      []GPU     `json:"gpus"`
}

// GPU represents a single GPU's metrics
type GPU struct {
	Index       int     `json:"index"`
	Name        string  `json:"name"`
	UUID        string  `json:"uuid"`
	Utilization float64 `json:"utilization_percent"`
	MemoryUsed  int64   `json:"memory_used_mb"`
	MemoryTotal int64   `json:"memory_total_mb"`
	MemoryUtil  float64 `json:"memory_util_percent"`
	Temperature int     `json:"temperature_c"`
	PowerDraw   float64 `json:"power_draw_w"`
	PowerLimit  float64 `json:"power_limit_w"`
	FanSpeed    int     `json:"fan_speed_percent"`
	EncoderUtil int     `json:"encoder_util_percent"`
	DecoderUtil int     `json:"decoder_util_percent"`
}

// GPUStatsHandler serves GPU statistics from NVML
type GPUStatsHandler struct {
	// Cache for 1 second to avoid excessive NVML calls
	cache           *GPUStats
	cacheTime       time.Time
	cacheTTL        time.Duration
	nvmlInitialized bool
}

// NewGPUStatsHandler creates a new GPU stats handler and initializes NVML
func NewGPUStatsHandler() *GPUStatsHandler {
	h := &GPUStatsHandler{
		cacheTTL: 1 * time.Second,
	}

	// Initialize NVML
	if err := h.initNVML(); err != nil {
		log.Printf("[GPU-STATS] Warning: Failed to initialize NVML: %v", err)
	}

	return h
}

// initNVML initializes the NVML library
func (h *GPUStatsHandler) initNVML() error {
	ret := nvml.Init()
	if ret != nvml.SUCCESS {
		return fmt.Errorf("NVML Init failed: %v", nvml.ErrorString(ret))
	}
	h.nvmlInitialized = true
	log.Printf("[GPU-STATS] NVML initialized successfully")
	return nil
}

// Shutdown cleanly shuts down NVML
func (h *GPUStatsHandler) Shutdown() error {
	if !h.nvmlInitialized {
		return nil
	}
	ret := nvml.Shutdown()
	if ret != nvml.SUCCESS {
		return fmt.Errorf("NVML Shutdown failed: %v", nvml.ErrorString(ret))
	}
	h.nvmlInitialized = false
	return nil
}

// ServeHTTP serves GPU statistics as JSON
func (h *GPUStatsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check cache
	if h.cache != nil && time.Since(h.cacheTime) < h.cacheTTL {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Cache", "HIT")
		if err := json.NewEncoder(w).Encode(h.cache); err != nil {
			log.Printf("[GPU-STATS] Failed to encode cached response: %v", err)
		}
		return
	}

	// Query GPU stats
	stats, err := h.queryGPUStats()
	if err != nil {
		log.Printf("[GPU-STATS] Failed to query GPU stats: %v", err)
		http.Error(w, fmt.Sprintf("Failed to query GPU: %v", err), http.StatusInternalServerError)
		return
	}

	// Update cache
	h.cache = stats
	h.cacheTime = time.Now()

	// Return JSON
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Cache", "MISS")
	if err := json.NewEncoder(w).Encode(stats); err != nil {
		log.Printf("[GPU-STATS] Failed to encode response: %v", err)
	}
}

// queryGPUStats queries GPU statistics using NVML
func (h *GPUStatsHandler) queryGPUStats() (*GPUStats, error) {
	if !h.nvmlInitialized {
		return nil, fmt.Errorf("NVML not initialized")
	}

	// Get device count
	count, ret := nvml.DeviceGetCount()
	if ret != nvml.SUCCESS {
		return nil, fmt.Errorf("failed to get device count: %v", nvml.ErrorString(ret))
	}

	if count == 0 {
		return nil, fmt.Errorf("no GPUs found")
	}

	stats := &GPUStats{
		Timestamp: time.Now(),
		GPUs:      make([]GPU, 0, count),
	}

	// Query each GPU
	for i := 0; i < count; i++ {
		device, ret := nvml.DeviceGetHandleByIndex(i)
		if ret != nvml.SUCCESS {
			log.Printf("[GPU-STATS] Warning: Failed to get device %d: %v", i, nvml.ErrorString(ret))
			continue
		}

		gpu, err := h.queryDeviceStats(i, device)
		if err != nil {
			log.Printf("[GPU-STATS] Warning: Failed to query device %d: %v", i, err)
			continue
		}

		stats.GPUs = append(stats.GPUs, *gpu)
	}

	if len(stats.GPUs) == 0 {
		return nil, fmt.Errorf("failed to query any GPUs")
	}

	return stats, nil
}

// queryDeviceStats queries statistics for a single GPU device
func (h *GPUStatsHandler) queryDeviceStats(index int, device nvml.Device) (*GPU, error) {
	gpu := &GPU{Index: index}

	// Get name
	name, ret := device.GetName()
	if ret == nvml.SUCCESS {
		gpu.Name = name
	}

	// Get UUID
	uuid, ret := device.GetUUID()
	if ret == nvml.SUCCESS {
		gpu.UUID = uuid
	}

	// Get utilization rates
	utilization, ret := device.GetUtilizationRates()
	if ret == nvml.SUCCESS {
		gpu.Utilization = float64(utilization.Gpu)
	}

	// Get memory info
	memory, ret := device.GetMemoryInfo()
	if ret == nvml.SUCCESS {
		gpu.MemoryUsed = int64(memory.Used / 1024 / 1024)   // Convert bytes to MiB
		gpu.MemoryTotal = int64(memory.Total / 1024 / 1024) // Convert bytes to MiB
		if gpu.MemoryTotal > 0 {
			gpu.MemoryUtil = (float64(gpu.MemoryUsed) / float64(gpu.MemoryTotal)) * 100
		}
	}

	// Get temperature
	temp, ret := device.GetTemperature(nvml.TEMPERATURE_GPU)
	if ret == nvml.SUCCESS {
		gpu.Temperature = int(temp)
	}

	// Get power usage
	power, ret := device.GetPowerUsage()
	if ret == nvml.SUCCESS {
		gpu.PowerDraw = float64(power) / 1000.0 // Convert mW to W
	}

	// Get power limit
	powerLimit, ret := device.GetPowerManagementLimit()
	if ret == nvml.SUCCESS {
		gpu.PowerLimit = float64(powerLimit) / 1000.0 // Convert mW to W
	}

	// Get fan speed
	fanSpeed, ret := device.GetFanSpeed()
	if ret == nvml.SUCCESS {
		gpu.FanSpeed = int(fanSpeed)
	}

	// Get encoder utilization
	encoderUtil, _, ret := device.GetEncoderUtilization()
	if ret == nvml.SUCCESS {
		gpu.EncoderUtil = int(encoderUtil)
	}

	// Get decoder utilization
	decoderUtil, _, ret := device.GetDecoderUtilization()
	if ret == nvml.SUCCESS {
		gpu.DecoderUtil = int(decoderUtil)
	}

	return gpu, nil
}
