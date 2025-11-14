package proxy

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test exact duplicate chunk detection
func TestDeduplicateExactDuplicateChunks(t *testing.T) {
	recorder := httptest.NewRecorder()
	rw := newResponseWriter(recorder, true, nil)
	rw.toolCallsDetected = true

	// Create identical chunks (simulating vLLM tensor parallelism)
	chunk := map[string]interface{}{
		"id":      "chatcmpl-123",
		"object":  "chat.completion.chunk",
		"created": 1234567890,
		"model":   "qwen3-coder-30b",
		"choices": []interface{}{
			map[string]interface{}{
				"index": 0,
				"delta": map[string]interface{}{
					"tool_calls": []interface{}{
						map[string]interface{}{
							"index": 0,
							"id":    "call_abc123",
							"type":  "function",
							"function": map[string]interface{}{
								"name":      "get_weather",
								"arguments": "",
							},
						},
					},
				},
			},
		},
	}

	chunkJSON, _ := json.Marshal(chunk)
	sseData := "data: " + string(chunkJSON) + "\n\n"

	// Write same chunk twice (duplicate)
	duplicateData := sseData + sseData

	_, err := rw.Write([]byte(duplicateData))
	require.NoError(t, err)

	// Should only have one chunk in output
	output := recorder.Body.String()
	chunkCount := strings.Count(output, "data: {")
	assert.Equal(t, 1, chunkCount, "Should filter out exact duplicate chunk")
}

// Test tool call argument deduplication
func TestDeduplicateToolCallArguments(t *testing.T) {
	recorder := httptest.NewRecorder()
	rw := newResponseWriter(recorder, true, nil)
	rw.toolCallsDetected = true

	// First chunk with arguments
	chunk1 := map[string]interface{}{
		"choices": []interface{}{
			map[string]interface{}{
				"index": 0,
				"delta": map[string]interface{}{
					"tool_calls": []interface{}{
						map[string]interface{}{
							"index": 0,
							"function": map[string]interface{}{
								"arguments": "{\"location\":",
							},
						},
					},
				},
			},
		},
	}

	// Duplicate arguments chunk
	chunk2 := map[string]interface{}{
		"choices": []interface{}{
			map[string]interface{}{
				"index": 0,
				"delta": map[string]interface{}{
					"tool_calls": []interface{}{
						map[string]interface{}{
							"index": 0,
							"function": map[string]interface{}{
								"arguments": "{\"location\":",
							},
						},
					},
				},
			},
		},
	}

	// New arguments (should pass through)
	chunk3 := map[string]interface{}{
		"choices": []interface{}{
			map[string]interface{}{
				"index": 0,
				"delta": map[string]interface{}{
					"tool_calls": []interface{}{
						map[string]interface{}{
							"index": 0,
							"function": map[string]interface{}{
								"arguments": " \"Paris\"}",
							},
						},
					},
				},
			},
		},
	}

	// Write chunks
	for _, chunk := range []map[string]interface{}{chunk1, chunk2, chunk3} {
		chunkJSON, _ := json.Marshal(chunk)
		sseData := "data: " + string(chunkJSON) + "\n\n"
		_, err := rw.Write([]byte(sseData))
		require.NoError(t, err)
	}

	// Should have 2 chunks (chunk2 filtered as duplicate)
	output := recorder.Body.String()
	lines := strings.Split(output, "\n")
	dataLines := 0
	for _, line := range lines {
		if strings.HasPrefix(line, "data: {") {
			dataLines++
		}
	}
	assert.Equal(t, 2, dataLines, "Should filter duplicate arguments chunk")
}

// Test tool call ID deduplication (content_block_start)
func TestDeduplicateToolCallIDs(t *testing.T) {
	recorder := httptest.NewRecorder()
	rw := newResponseWriter(recorder, true, nil)
	rw.toolCallsDetected = true

	// First tool call start event
	chunk1 := map[string]interface{}{
		"choices": []interface{}{
			map[string]interface{}{
				"index": 0,
				"delta": map[string]interface{}{
					"tool_calls": []interface{}{
						map[string]interface{}{
							"index": 0,
							"id":    "call_xyz789",
							"type":  "function",
							"function": map[string]interface{}{
								"name":      "calculate",
								"arguments": "",
							},
						},
					},
				},
			},
		},
	}

	// Duplicate start event (same ID, no args)
	chunk2 := map[string]interface{}{
		"choices": []interface{}{
			map[string]interface{}{
				"index": 0,
				"delta": map[string]interface{}{
					"tool_calls": []interface{}{
						map[string]interface{}{
							"index": 0,
							"id":    "call_xyz789",
							"type":  "function",
							"function": map[string]interface{}{
								"name":      "calculate",
								"arguments": "",
							},
						},
					},
				},
			},
		},
	}

	// Arguments chunk (should pass through)
	chunk3 := map[string]interface{}{
		"choices": []interface{}{
			map[string]interface{}{
				"index": 0,
				"delta": map[string]interface{}{
					"tool_calls": []interface{}{
						map[string]interface{}{
							"index": 0,
							"id":    "call_xyz789",
							"function": map[string]interface{}{
								"arguments": "{\"x\": 5}",
							},
						},
					},
				},
			},
		},
	}

	// Write chunks
	for _, chunk := range []map[string]interface{}{chunk1, chunk2, chunk3} {
		chunkJSON, _ := json.Marshal(chunk)
		sseData := "data: " + string(chunkJSON) + "\n\n"
		_, err := rw.Write([]byte(sseData))
		require.NoError(t, err)
	}

	// Should have 2 chunks (chunk2 filtered as duplicate tool ID)
	output := recorder.Body.String()
	lines := strings.Split(output, "\n")
	dataLines := 0
	for _, line := range lines {
		if strings.HasPrefix(line, "data: {") {
			dataLines++
		}
	}
	assert.Equal(t, 2, dataLines, "Should filter duplicate tool call ID start event")
}

// Test DONE marker passthrough
func TestDeduplicateDoneMarker(t *testing.T) {
	recorder := httptest.NewRecorder()
	rw := newResponseWriter(recorder, true, nil)
	rw.toolCallsDetected = true

	chunk := map[string]interface{}{
		"choices": []interface{}{
			map[string]interface{}{
				"index": 0,
				"delta": map[string]interface{}{
					"content": "test",
				},
			},
		},
	}

	chunkJSON, _ := json.Marshal(chunk)
	sseData := "data: " + string(chunkJSON) + "\n\ndata: [DONE]\n\n"

	_, err := rw.Write([]byte(sseData))
	require.NoError(t, err)

	output := recorder.Body.String()
	assert.Contains(t, output, "data: [DONE]", "Should preserve [DONE] marker")
}

// Test no deduplication when tool calls not detected
func TestNoDeduplicationWithoutToolCalls(t *testing.T) {
	recorder := httptest.NewRecorder()
	rw := newResponseWriter(recorder, true, nil)
	// Note: toolCallsDetected = false (default)

	chunk := map[string]interface{}{
		"choices": []interface{}{
			map[string]interface{}{
				"index": 0,
				"delta": map[string]interface{}{
					"content": "Hello",
				},
			},
		},
	}

	chunkJSON, _ := json.Marshal(chunk)
	sseData := "data: " + string(chunkJSON) + "\n\n"

	// Write same chunk twice
	duplicateData := sseData + sseData

	_, err := rw.Write([]byte(duplicateData))
	require.NoError(t, err)

	// Should have both chunks (no deduplication when toolCallsDetected=false)
	output := recorder.Body.String()
	chunkCount := strings.Count(output, "data: {")
	assert.Equal(t, 2, chunkCount, "Should not deduplicate when tool calls not detected")
}

// Test mixed content and tool calls
func TestDeduplicateMixedContent(t *testing.T) {
	recorder := httptest.NewRecorder()
	rw := newResponseWriter(recorder, true, nil)
	rw.toolCallsDetected = true

	// Regular content chunk
	chunk1 := map[string]interface{}{
		"choices": []interface{}{
			map[string]interface{}{
				"index": 0,
				"delta": map[string]interface{}{
					"content": "Thinking...",
				},
			},
		},
	}

	// Tool call chunk
	chunk2 := map[string]interface{}{
		"choices": []interface{}{
			map[string]interface{}{
				"index": 0,
				"delta": map[string]interface{}{
					"tool_calls": []interface{}{
						map[string]interface{}{
							"index": 0,
							"id":    "call_123",
							"function": map[string]interface{}{
								"name":      "search",
								"arguments": "",
							},
						},
					},
				},
			},
		},
	}

	// Duplicate tool call (should be filtered)
	chunk3 := chunk2

	for _, chunk := range []map[string]interface{}{chunk1, chunk2, chunk3} {
		chunkJSON, _ := json.Marshal(chunk)
		sseData := "data: " + string(chunkJSON) + "\n\n"
		_, err := rw.Write([]byte(sseData))
		require.NoError(t, err)
	}

	output := recorder.Body.String()
	lines := strings.Split(output, "\n")
	dataLines := 0
	for _, line := range lines {
		if strings.HasPrefix(line, "data: {") {
			dataLines++
		}
	}
	assert.Equal(t, 2, dataLines, "Should filter duplicate tool call but keep content")
}

// Test incremental argument building
func TestDeduplicateIncrementalArguments(t *testing.T) {
	recorder := httptest.NewRecorder()
	rw := newResponseWriter(recorder, true, nil)
	rw.toolCallsDetected = true

	// Simulate incremental JSON argument streaming
	argChunks := []string{
		"{\"city",
		"\": \"",
		"New York",
		"\"}",
	}

	for _, arg := range argChunks {
		chunk := map[string]interface{}{
			"choices": []interface{}{
				map[string]interface{}{
					"index": 0,
					"delta": map[string]interface{}{
						"tool_calls": []interface{}{
							map[string]interface{}{
								"index": 0,
								"function": map[string]interface{}{
									"arguments": arg,
								},
							},
						},
					},
				},
			},
		}

		chunkJSON, _ := json.Marshal(chunk)
		sseData := "data: " + string(chunkJSON) + "\n\n"
		_, err := rw.Write([]byte(sseData))
		require.NoError(t, err)
	}

	// All chunks should pass through (different arguments each time)
	output := recorder.Body.String()
	lines := strings.Split(output, "\n")
	dataLines := 0
	for _, line := range lines {
		if strings.HasPrefix(line, "data: {") {
			dataLines++
		}
	}
	assert.Equal(t, 4, dataLines, "Should allow incremental argument building")
}

// Test empty lines handling
func TestDeduplicateEmptyLines(t *testing.T) {
	recorder := httptest.NewRecorder()
	rw := newResponseWriter(recorder, true, nil)
	rw.toolCallsDetected = true

	chunk := map[string]interface{}{
		"choices": []interface{}{
			map[string]interface{}{
				"index": 0,
				"delta": map[string]interface{}{
					"content": "test",
				},
			},
		},
	}

	chunkJSON, _ := json.Marshal(chunk)
	// SSE format with double newline separator
	sseData := "data: " + string(chunkJSON) + "\n\n"

	_, err := rw.Write([]byte(sseData))
	require.NoError(t, err)

	output := recorder.Body.String()
	// Output should maintain SSE format
	assert.Contains(t, output, "\n\n", "Should preserve SSE double newline format")
}

// Benchmark deduplication performance
func BenchmarkDeduplication(b *testing.B) {
	recorder := httptest.NewRecorder()
	rw := newResponseWriter(recorder, false, nil)
	rw.toolCallsDetected = true

	chunk := map[string]interface{}{
		"choices": []interface{}{
			map[string]interface{}{
				"index": 0,
				"delta": map[string]interface{}{
					"tool_calls": []interface{}{
						map[string]interface{}{
							"index": 0,
							"function": map[string]interface{}{
								"arguments": "{\"test\": \"data\"}",
							},
						},
					},
				},
			},
		},
	}

	chunkJSON, _ := json.Marshal(chunk)
	sseData := []byte("data: " + string(chunkJSON) + "\n\n")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rw.Write(sseData)
	}
}

// Test multiple tool calls in same request
func TestDeduplicateMultipleToolCalls(t *testing.T) {
	recorder := httptest.NewRecorder()
	rw := newResponseWriter(recorder, true, nil)
	rw.toolCallsDetected = true

	// First tool call
	chunk1 := map[string]interface{}{
		"choices": []interface{}{
			map[string]interface{}{
				"index": 0,
				"delta": map[string]interface{}{
					"tool_calls": []interface{}{
						map[string]interface{}{
							"index": 0,
							"id":    "call_1",
							"function": map[string]interface{}{
								"name":      "tool1",
								"arguments": "",
							},
						},
					},
				},
			},
		},
	}

	// Second tool call (different index)
	chunk2 := map[string]interface{}{
		"choices": []interface{}{
			map[string]interface{}{
				"index": 0,
				"delta": map[string]interface{}{
					"tool_calls": []interface{}{
						map[string]interface{}{
							"index": 1,
							"id":    "call_2",
							"function": map[string]interface{}{
								"name":      "tool2",
								"arguments": "",
							},
						},
					},
				},
			},
		},
	}

	// Duplicate of first tool call
	chunk3 := chunk1

	for _, chunk := range []map[string]interface{}{chunk1, chunk2, chunk3} {
		chunkJSON, _ := json.Marshal(chunk)
		sseData := "data: " + string(chunkJSON) + "\n\n"
		_, err := rw.Write([]byte(sseData))
		require.NoError(t, err)
	}

	output := recorder.Body.String()
	lines := strings.Split(output, "\n")
	dataLines := 0
	for _, line := range lines {
		if strings.HasPrefix(line, "data: {") {
			dataLines++
		}
	}
	assert.Equal(t, 2, dataLines, "Should filter duplicate tool call while keeping different indices")
}

// Test bytes filtered metric
func TestDeduplicationBytesFiltered(t *testing.T) {
	recorder := httptest.NewRecorder()
	rw := newResponseWriter(recorder, true, nil)

	chunk := map[string]interface{}{
		"choices": []interface{}{
			map[string]interface{}{
				"index": 0,
				"delta": map[string]interface{}{
					"content": "test",
				},
			},
		},
	}

	chunkJSON, _ := json.Marshal(chunk)
	sseData := []byte("data: " + string(chunkJSON) + "\n\n")

	// Test the deduplicateToolCallChunks method directly
	duplicateData := bytes.Repeat(sseData, 2)
	deduped, bytesFiltered := rw.deduplicateToolCallChunks(duplicateData)

	assert.Greater(t, bytesFiltered, 0, "Should report filtered bytes")
	assert.Less(t, len(deduped), len(duplicateData), "Deduplicated data should be smaller")
}
