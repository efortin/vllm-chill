package proxy

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAnthropicStreamingRealTime tests that streaming is sent in real-time without buffering
func TestAnthropicStreamingRealTime(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Create mock vLLM server that streams slowly
	vllmServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify OpenAI format request
		assert.Equal(t, "/v1/chat/completions", r.URL.Path)
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))

		// Verify streaming is requested
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		assert.True(t, body["stream"].(bool))

		// Send SSE response with delays to simulate real streaming
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		flusher := w.(http.Flusher)

		// Send chunks with delays
		chunks := []string{
			`{"id":"test-1","model":"qwen3-coder-30b-fp8","choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}`,
			`{"id":"test-1","model":"qwen3-coder-30b-fp8","choices":[{"index":0,"delta":{"content":" World"},"finish_reason":null}]}`,
			`{"id":"test-1","model":"qwen3-coder-30b-fp8","choices":[{"index":0,"delta":{"content":"!"},"finish_reason":"stop"}]}`,
		}

		for i, chunk := range chunks {
			_, _ = fmt.Fprintf(w, "data: %s\n\n", chunk)
			flusher.Flush()

			// Add delay between chunks to ensure we're testing real-time streaming
			if i < len(chunks)-1 {
				time.Sleep(50 * time.Millisecond)
			}
		}

		_, _ = fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer vllmServer.Close()

	// Create autoscaler with mock vLLM server
	as := createTestAutoscaler(t, vllmServer.URL)

	// Create test request
	anthropicReq := map[string]interface{}{
		"model":      "qwen3-coder-30b-fp8",
		"max_tokens": 100,
		"messages": []map[string]interface{}{
			{"role": "user", "content": "Say hello"},
		},
		"stream": true,
	}
	reqBody, _ := json.Marshal(anthropicReq)

	req := httptest.NewRequest("POST", "/v1/messages", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", "test-key")
	req.Header.Set("anthropic-version", "2023-06-01")

	// Create response recorder
	w := httptest.NewRecorder()

	// Create gin context
	router := gin.New()
	router.POST("/v1/messages", as.handleAnthropicFormatRequest)

	// Track when each chunk is received
	receiveTimes := []time.Time{}
	startTime := time.Now()

	// Execute request in goroutine to allow reading stream
	go func() {
		router.ServeHTTP(w, req)
	}()

	// Give server time to start
	time.Sleep(10 * time.Millisecond)

	// Read stream and track timing
	scanner := bufio.NewScanner(w.Body)
	chunkCount := 0
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") && !strings.Contains(line, "message_start") && !strings.Contains(line, "content_block_start") && !strings.Contains(line, "content_block_stop") && !strings.Contains(line, "message_stop") {
			receiveTimes = append(receiveTimes, time.Now())
			chunkCount++
		}
	}

	// Verify we received chunks in real-time (not all at once)
	assert.GreaterOrEqual(t, chunkCount, 3, "Should receive at least 3 content chunks")

	// If streaming is working correctly, there should be delays between chunks
	if len(receiveTimes) >= 2 {
		timeBetweenChunks := receiveTimes[1].Sub(receiveTimes[0])
		assert.Greater(t, timeBetweenChunks, 30*time.Millisecond, "Chunks should arrive with delays (real-time streaming)")
	}

	totalTime := time.Since(startTime)
	assert.Greater(t, totalTime, 100*time.Millisecond, "Total streaming time should reflect delays in mock server")
}

// TestAnthropicStreamingFormat tests that OpenAI SSE is correctly transformed to Anthropic format
func TestAnthropicStreamingFormat(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Create mock vLLM server
	vllmServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		// Send OpenAI format stream
		_, _ = fmt.Fprintf(w, "data: %s\n\n", `{"id":"test-1","model":"qwen3-coder-30b-fp8","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":null}]}`)
		_, _ = fmt.Fprintf(w, "data: %s\n\n", `{"id":"test-1","model":"qwen3-coder-30b-fp8","choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}`)
		_, _ = fmt.Fprintf(w, "data: %s\n\n", `{"id":"test-1","model":"qwen3-coder-30b-fp8","choices":[{"index":0,"delta":{"content":" World"},"finish_reason":"stop"}]}`)
		_, _ = fmt.Fprintf(w, "data: [DONE]\n\n")
	}))
	defer vllmServer.Close()

	// Create autoscaler
	as := createTestAutoscaler(t, vllmServer.URL)

	// Create request
	anthropicReq := map[string]interface{}{
		"model":      "qwen3-coder-30b-fp8",
		"max_tokens": 100,
		"messages": []map[string]interface{}{
			{"role": "user", "content": "Test"},
		},
		"stream": true,
	}
	reqBody, _ := json.Marshal(anthropicReq)

	req := httptest.NewRequest("POST", "/v1/messages", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", "test-key")

	w := httptest.NewRecorder()

	router := gin.New()
	router.POST("/v1/messages", as.handleAnthropicFormatRequest)
	router.ServeHTTP(w, req)

	// Parse SSE events
	events := parseSSEEvents(t, w.Body)

	// Verify event sequence
	require.GreaterOrEqual(t, len(events), 5, "Should have at least 5 events")

	// Check message_start
	assert.Equal(t, "message_start", events[0].Event)
	var msgStart map[string]interface{}
	_ = json.Unmarshal([]byte(events[0].Data), &msgStart)
	assert.Equal(t, "message_start", msgStart["type"])

	// Check content_block_start
	assert.Equal(t, "content_block_start", events[1].Event)

	// Check content_block_delta events
	foundDelta := false
	for _, event := range events {
		if event.Event == "content_block_delta" {
			foundDelta = true
			var delta map[string]interface{}
			_ = json.Unmarshal([]byte(event.Data), &delta)
			assert.Equal(t, "content_block_delta", delta["type"])
			assert.Contains(t, delta, "delta")
		}
	}
	assert.True(t, foundDelta, "Should have content_block_delta events")

	// Check content_block_stop
	foundStop := false
	for _, event := range events {
		if event.Event == "content_block_stop" {
			foundStop = true
		}
	}
	assert.True(t, foundStop, "Should have content_block_stop event")

	// Check message_stop
	assert.Equal(t, "message_stop", events[len(events)-1].Event)
}

// TestAnthropicNonStreamingStillWorks tests that non-streaming requests still work correctly
func TestAnthropicNonStreamingStillWorks(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Create mock vLLM server
	vllmServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify non-streaming request
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		assert.False(t, body["stream"].(bool))

		// Send OpenAI format response
		resp := map[string]interface{}{
			"id":      "test-1",
			"model":   "qwen3-coder-30b-fp8",
			"object":  "chat.completion",
			"created": 1234567890,
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": "Hello World",
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]interface{}{
				"prompt_tokens":     10,
				"completion_tokens": 5,
				"total_tokens":      15,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer vllmServer.Close()

	// Create autoscaler
	as := createTestAutoscaler(t, vllmServer.URL)

	// Create non-streaming request
	anthropicReq := map[string]interface{}{
		"model":      "qwen3-coder-30b-fp8",
		"max_tokens": 100,
		"messages": []map[string]interface{}{
			{"role": "user", "content": "Test"},
		},
		"stream": false,
	}
	reqBody, _ := json.Marshal(anthropicReq)

	req := httptest.NewRequest("POST", "/v1/messages", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", "test-key")

	w := httptest.NewRecorder()

	router := gin.New()
	router.POST("/v1/messages", as.handleAnthropicFormatRequest)
	router.ServeHTTP(w, req)

	// Verify response
	assert.Equal(t, http.StatusOK, w.Code)

	var anthropicResp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &anthropicResp)
	require.NoError(t, err)

	assert.Equal(t, "message", anthropicResp["type"])
	assert.Equal(t, "assistant", anthropicResp["role"])
	assert.Contains(t, anthropicResp, "content")

	content := anthropicResp["content"].([]interface{})
	assert.GreaterOrEqual(t, len(content), 1)
}

// Helper types and functions

type SSEEvent struct {
	Event string
	Data  string
}

func parseSSEEvents(t *testing.T, body io.Reader) []SSEEvent {
	var events []SSEEvent
	scanner := bufio.NewScanner(body)
	var currentEvent string

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "event: ") {
			currentEvent = strings.TrimPrefix(line, "event: ")
		} else if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			if data != "" && data != "{}" {
				events = append(events, SSEEvent{
					Event: currentEvent,
					Data:  data,
				})
			}
			currentEvent = ""
		}
	}

	return events
}

func createTestAutoscaler(t *testing.T, vllmURL string) *AutoScaler {
	config := &Config{
		Port:        "8080",
		Namespace:   "test",
		Deployment:  "test",
		IdleTimeout: "5m",
		ModelID:     "qwen3-coder-30b-fp8",
	}

	as, err := NewAutoScaler(config)
	require.NoError(t, err)

	// Override targetURL for testing
	as.targetURL, _ = url.Parse(vllmURL)

	return as
}
