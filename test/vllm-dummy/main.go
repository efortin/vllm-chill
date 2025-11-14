package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync/atomic"
	"time"
)

// ModelsResponse represents the /v1/models API response
type ModelsResponse struct {
	Object string  `json:"object"`
	Data   []Model `json:"data"`
}

// Model represents a single model in the API
type Model struct {
	ID      string      `json:"id"`
	Object  string      `json:"object"`
	Created int64       `json:"created"`
	OwnedBy string      `json:"owned_by"`
	Root    string      `json:"root"`
	Parent  interface{} `json:"parent"`
}

var (
	// isReady tracks if the server has finished "loading"
	isReady atomic.Bool
)

func main() {
	modelName := os.Getenv("MODEL_NAME")
	if modelName == "" {
		modelName = "test-model"
	}

	servedModelName := os.Getenv("SERVED_MODEL_NAME")
	if servedModelName == "" {
		servedModelName = "test-model"
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8000"
	}

	// STARTUP_DELAY simulates vLLM model loading time (in seconds)
	startupDelay := 0
	if delayStr := os.Getenv("STARTUP_DELAY"); delayStr != "" {
		if delay, err := strconv.Atoi(delayStr); err == nil && delay > 0 {
			startupDelay = delay
		}
	}

	addr := fmt.Sprintf(":%s", port)
	log.Printf("Starting vLLM dummy server on %s", addr)
	log.Printf("Model: %s (served as: %s)", modelName, servedModelName)

	if startupDelay > 0 {
		log.Printf("Simulating model loading with %d second startup delay", startupDelay)
		// Start server but mark as not ready
		isReady.Store(false)

		// Start the loading simulation in the background
		go func() {
			time.Sleep(time.Duration(startupDelay) * time.Second)
			isReady.Store(true)
			log.Printf("Model loaded, server is now ready to accept requests")
		}()
	} else {
		// No startup delay, immediately ready
		isReady.Store(true)
	}

	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/v1/models", modelsHandler(modelName, servedModelName))
	http.HandleFunc("/v1/chat/completions", chatCompletionsHandler)
	http.HandleFunc("/v1/completions", completionsHandler)

	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatal(err)
	}
}

func healthHandler(w http.ResponseWriter, _ *http.Request) {
	if !isReady.Load() {
		// Server is still loading, return 503 Service Unavailable
		w.WriteHeader(http.StatusServiceUnavailable)
		if _, err := fmt.Fprintln(w, "Loading"); err != nil {
			log.Printf("Failed to write response: %v", err)
		}
		return
	}
	w.WriteHeader(http.StatusOK)
	if _, err := fmt.Fprintln(w, "OK"); err != nil {
		log.Printf("Failed to write response: %v", err)
	}
}

func modelsHandler(modelName, servedModelName string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		response := ModelsResponse{
			Object: "list",
			Data: []Model{
				{
					ID:      servedModelName,
					Object:  "model",
					Created: 1700000000,
					OwnedBy: "vllm",
					Root:    modelName,
					Parent:  nil,
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil {
			log.Printf("Failed to encode models response: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
	}
}

func chatCompletionsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Simple mock response
	response := map[string]interface{}{
		"id":      "chatcmpl-123",
		"object":  "chat.completion",
		"created": 1700000000,
		"model":   "test-model",
		"choices": []map[string]interface{}{
			{
				"index": 0,
				"message": map[string]string{
					"role":    "assistant",
					"content": "This is a fake response from the mock vLLM server.",
				},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]int{
			"prompt_tokens":     10,
			"completion_tokens": 15,
			"total_tokens":      25,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Failed to encode chat completions response: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func completionsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Simple mock response
	response := map[string]interface{}{
		"id":      "cmpl-123",
		"object":  "text_completion",
		"created": 1700000000,
		"model":   "test-model",
		"choices": []map[string]interface{}{
			{
				"index":         0,
				"text":          "This is a fake completion response.",
				"finish_reason": "stop",
			},
		},
		"usage": map[string]int{
			"prompt_tokens":     5,
			"completion_tokens": 10,
			"total_tokens":      15,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Failed to encode completions response: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}
