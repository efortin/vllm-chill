package operation

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
)

// Manager defines the interface for vLLM operations
type Manager interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	UpdateActivity()
}

// Handler handles manual operation requests
type Handler struct {
	manager Manager
}

// NewHandler creates a new operation handler
func NewHandler(manager Manager) *Handler {
	return &Handler{
		manager: manager,
	}
}

// StartHandler handles manual start requests
func (h *Handler) StartHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()
	log.Printf("Manual start requested")

	// Update activity to prevent immediate scale-down
	h.manager.UpdateActivity()

	// Start vLLM
	if err := h.manager.Start(ctx); err != nil {
		log.Printf("Failed to start vLLM: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		response := map[string]interface{}{
			"error": map[string]string{
				"message": fmt.Sprintf("Failed to start vLLM: %v", err),
				"type":    "start_failed",
			},
		}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			log.Printf("Failed to encode response: %v", err)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	response := map[string]string{
		"status":  "success",
		"message": "vLLM started successfully",
	}
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Failed to encode response: %v", err)
	}
}

// StopHandler handles manual stop requests
func (h *Handler) StopHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()
	log.Printf("Manual stop requested")

	// Stop vLLM
	if err := h.manager.Stop(ctx); err != nil {
		log.Printf("Failed to stop vLLM: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		response := map[string]interface{}{
			"error": map[string]string{
				"message": fmt.Sprintf("Failed to stop vLLM: %v", err),
				"type":    "stop_failed",
			},
		}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			log.Printf("Failed to encode response: %v", err)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	response := map[string]string{
		"status":  "success",
		"message": "vLLM stopped successfully",
	}
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Failed to encode response: %v", err)
	}
}
