package stats

import (
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// GinGPUStatsHandler serves GPU statistics from NVML using Gin
type GinGPUStatsHandler struct {
	handler *GPUStatsHandler
}

// NewGinGPUStatsHandler creates a new Gin GPU stats handler
func NewGinGPUStatsHandler() *GinGPUStatsHandler {
	return &GinGPUStatsHandler{
		handler: NewGPUStatsHandler(),
	}
}

// Shutdown cleanly shuts down the GPU stats handler
func (h *GinGPUStatsHandler) Shutdown() error {
	return h.handler.Shutdown()
}

// Handler serves GPU statistics as JSON
func (h *GinGPUStatsHandler) Handler(c *gin.Context) {
	// Check cache
	if h.handler.cache != nil && time.Since(h.handler.cacheTime) < h.handler.cacheTTL {
		c.Header("X-Cache", "HIT")
		c.JSON(http.StatusOK, h.handler.cache)
		return
	}

	// Query GPU stats
	stats, err := h.handler.queryGPUStats()
	if err != nil {
		log.Printf("[GPU-STATS] Failed to query GPU stats: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to query GPU: " + err.Error(),
		})
		return
	}

	// Update cache
	h.handler.cache = stats
	h.handler.cacheTime = time.Now()

	// Return JSON
	c.Header("X-Cache", "MISS")
	c.JSON(http.StatusOK, stats)
}
