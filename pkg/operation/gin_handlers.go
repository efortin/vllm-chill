// Package operation provides HTTP handlers for manual vLLM operations (start/stop).
package operation

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
)

// GinHandler handles manual operation requests using Gin
type GinHandler struct {
	manager Manager
}

// NewGinHandler creates a new Gin operation handler
func NewGinHandler(manager Manager) *GinHandler {
	return &GinHandler{
		manager: manager,
	}
}

// StartHandler handles manual start requests
func (h *GinHandler) StartHandler(c *gin.Context) {
	ctx := c.Request.Context()
	log.Printf("Manual start requested")

	// Update activity to prevent immediate scale-down
	h.manager.UpdateActivity()

	// Start vLLM
	if err := h.manager.Start(ctx); err != nil {
		log.Printf("Failed to start vLLM: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"message": err.Error(),
				"type":    "start_failed",
			},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "vLLM started successfully",
	})
}

// StopHandler handles manual stop requests
func (h *GinHandler) StopHandler(c *gin.Context) {
	ctx := c.Request.Context()
	log.Printf("Manual stop requested")

	// Stop vLLM
	if err := h.manager.Stop(ctx); err != nil {
		log.Printf("Failed to stop vLLM: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"message": err.Error(),
				"type":    "stop_failed",
			},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "vLLM stopped successfully",
	})
}
