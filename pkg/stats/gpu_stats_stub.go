//go:build !cgo

package stats

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// GPUStatsHandler provides GPU statistics (stub version without NVML)
type GPUStatsHandler struct{}

// NewGPUStatsHandler creates a new GPU stats handler
func NewGPUStatsHandler() *GPUStatsHandler {
	return &GPUStatsHandler{}
}

// ServeHTTP implements http.Handler interface
func (h *GPUStatsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusServiceUnavailable)
	w.Write([]byte(`{"error":"GPU stats not available (compiled without CGO/NVML support)"}`))
}

// Shutdown is a no-op for the stub version
func (h *GPUStatsHandler) Shutdown() error {
	return nil
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
	c.JSON(http.StatusServiceUnavailable, gin.H{
		"error": "GPU stats not available (compiled without CGO/NVML support)",
	})
}

// Handler returns the Gin handler function (alias for Handle)
func (h *GinGPUStatsHandler) Handler(c *gin.Context) {
	h.Handle(c)
}

// Shutdown is a no-op for the stub version
func (h *GinGPUStatsHandler) Shutdown() error {
	return nil
}
