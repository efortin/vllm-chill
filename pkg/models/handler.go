package models

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/efortin/vllm-chill/pkg/kubernetes"
	"github.com/gin-gonic/gin"
)

// Manager defines the interface for model switching operations
type Manager interface {
	GetActiveModel() string
	SwitchModel(ctx context.Context, modelID string) error
	GetModelConfig(ctx context.Context, modelID string) (*kubernetes.ModelConfig, error)
	ListModels(ctx context.Context) ([]ModelInfo, error)
	IsRunning() bool
}

// ModelInfo represents basic model information
type ModelInfo struct {
	Name            string `json:"name"`
	ServedModelName string `json:"servedModelName"`
	ModelName       string `json:"modelName"`
	MaxModelLen     string `json:"maxModelLen"`
}

// Handler handles HTTP requests for model management
type Handler struct {
	manager Manager
}

// NewHandler creates a new model management handler
func NewHandler(manager Manager) *Handler {
	return &Handler{
		manager: manager,
	}
}

// AvailableHandler returns all available models from CRDs
func (h *Handler) AvailableHandler(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	models, err := h.manager.ListModels(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Failed to list models: %v", err),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"models": models,
		"count":  len(models),
	})
}

// RunningHandler returns the currently active model
func (h *Handler) RunningHandler(c *gin.Context) {
	activeModel := h.manager.GetActiveModel()
	isRunning := h.manager.IsRunning()

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	// Get full model config
	modelConfig, err := h.manager.GetModelConfig(ctx, activeModel)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Failed to get model config: %v", err),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"active_model": activeModel,
		"running":      isRunning,
		"config": gin.H{
			"modelName":       modelConfig.ModelName,
			"servedModelName": modelConfig.ServedModelName,
			"maxModelLen":     modelConfig.MaxModelLen,
			"toolCallParser":  modelConfig.ToolCallParser,
			"reasoningParser": modelConfig.ReasoningParser,
		},
	})
}

// SwitchRequest represents a model switch request
type SwitchRequest struct {
	ModelID string `json:"model_id" binding:"required"`
}

// SwitchHandler switches to a different model
func (h *Handler) SwitchHandler(c *gin.Context) {
	var req SwitchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("Invalid request: %v", err),
		})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	// Verify model exists before switching
	_, err := h.manager.GetModelConfig(ctx, req.ModelID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": fmt.Sprintf("Model '%s' not found: %v", req.ModelID, err),
		})
		return
	}

	// Perform the switch
	if err := h.manager.SwitchModel(ctx, req.ModelID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Failed to switch model: %v", err),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":      "Model switched successfully",
		"active_model": req.ModelID,
		"note":         "vLLM pod will be recreated with the new model on next request",
	})
}
