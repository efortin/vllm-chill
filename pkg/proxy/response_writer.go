package proxy

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"

	"github.com/efortin/vllm-chill/pkg/stats"
)

// responseWriter wraps http.ResponseWriter to capture status code and response size
type responseWriter struct {
	http.ResponseWriter
	statusCode        int
	bytesWritten      int64
	body              *bytes.Buffer
	captureBody       bool
	sseBuffer         *bytes.Buffer          // Buffer for accumulating SSE chunks
	toolCallsDetected bool                   // Whether native tool calls were detected
	metrics           *stats.MetricsRecorder // Metrics recorder for tracking operations
	// Deduplication fields for native tool calls (vLLM tensor parallelism workaround)
	seenChunks       map[string]bool // Track seen SSE chunks by hash
	lastToolCallArgs map[int]string  // Track last arguments per tool call index
	toolCallIDs      map[string]bool // Track which tool call IDs we've sent start events for
}

// newResponseWriter creates a new response writer wrapper
func newResponseWriter(w http.ResponseWriter, captureBody bool, metrics *stats.MetricsRecorder) *responseWriter {
	rw := &responseWriter{
		ResponseWriter:   w,
		statusCode:       http.StatusOK,
		captureBody:      captureBody,
		sseBuffer:        &bytes.Buffer{},
		metrics:          metrics,
		seenChunks:       make(map[string]bool),
		lastToolCallArgs: make(map[int]string),
		toolCallIDs:      make(map[string]bool),
	}
	if captureBody {
		rw.body = &bytes.Buffer{}
	}

	return rw
}

// WriteHeader captures the status code
func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// Write captures the response size and deduplicates tool call chunks
func (rw *responseWriter) Write(b []byte) (int, error) {
	// Accumulate all data in SSE buffer
	rw.sseBuffer.Write(b)

	// Try to process complete lines
	content := rw.sseBuffer.String()

	// Check if we have SSE data chunks
	if !strings.HasPrefix(content, "data: ") {
		// Not SSE format, pass through
		n, err := rw.ResponseWriter.Write(b)
		rw.bytesWritten += int64(n)
		if rw.captureBody {
			rw.body.Write(b)
		}
		return len(b), err
	}

	// Parse only NEW SSE chunks (everything in current write)
	lines := strings.Split(string(b), "\n")

	for _, line := range lines {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		jsonData := strings.TrimPrefix(line, "data: ")
		if jsonData == "[DONE]" {
			continue
		}

		var chunk map[string]interface{}
		if err := json.Unmarshal([]byte(jsonData), &chunk); err != nil {
			continue
		}

		// Check for native tool calls
		if choices, ok := chunk["choices"].([]interface{}); ok && len(choices) > 0 {
			if choice, ok := choices[0].(map[string]interface{}); ok {
				if delta, ok := choice["delta"].(map[string]interface{}); ok {
					if toolCalls, ok := delta["tool_calls"].([]interface{}); ok && len(toolCalls) > 0 {
						if !rw.toolCallsDetected {
							rw.toolCallsDetected = true
							log.Printf("[TOOL-CALLS] Native tool calls detected - enabling deduplication")
						}
					}
				}
			}
		}
	}

	// If native tool calls detected, deduplicate chunks from vLLM tensor parallelism
	if rw.toolCallsDetected {
		dedupedData, bytesFiltered := rw.deduplicateToolCallChunks(b)
		if bytesFiltered > 0 {
			log.Printf("[DEDUP] Filtered %d duplicate bytes from vLLM tensor parallelism", bytesFiltered)
		}
		if len(dedupedData) == 0 {
			// All chunks were duplicates
			return len(b), nil
		}
		n, err := rw.ResponseWriter.Write(dedupedData)
		rw.bytesWritten += int64(n)
		if rw.captureBody {
			rw.body.Write(dedupedData)
		}
		return len(b), err
	}

	// Normal pass-through (no tool calls)
	n, err := rw.ResponseWriter.Write(b)
	rw.bytesWritten += int64(n)
	if rw.captureBody {
		rw.body.Write(b)
	}
	return len(b), err
}

// deduplicateToolCallChunks removes duplicate SSE chunks from vLLM tensor parallelism
// Returns deduplicated data and number of bytes filtered
func (rw *responseWriter) deduplicateToolCallChunks(b []byte) ([]byte, int) {
	lines := strings.Split(string(b), "\n")
	var output bytes.Buffer
	originalSize := len(b)

	for _, line := range lines {
		// Pass through non-data lines
		if !strings.HasPrefix(line, "data: ") {
			if line != "" || output.Len() > 0 {
				output.WriteString(line)
				output.WriteString("\n")
			}
			continue
		}

		jsonData := strings.TrimPrefix(line, "data: ")

		// Pass through [DONE] marker
		if jsonData == "[DONE]" {
			output.WriteString(line)
			output.WriteString("\n")
			continue
		}

		// Hash the entire chunk for exact duplicate detection
		hash := fmt.Sprintf("%x", sha256.Sum256([]byte(jsonData)))
		if rw.seenChunks[hash] {
			// Skip duplicate chunk
			continue
		}
		rw.seenChunks[hash] = true

		// Parse chunk for tool call deduplication
		var chunk map[string]interface{}
		if err := json.Unmarshal([]byte(jsonData), &chunk); err != nil {
			// Can't parse, pass through
			output.WriteString(line)
			output.WriteString("\n")
			continue
		}

		// Check for tool calls in delta
		if choices, ok := chunk["choices"].([]interface{}); ok && len(choices) > 0 {
			if choice, ok := choices[0].(map[string]interface{}); ok {
				if delta, ok := choice["delta"].(map[string]interface{}); ok {
					if toolCalls, ok := delta["tool_calls"].([]interface{}); ok && len(toolCalls) > 0 {
						// Process each tool call for argument deduplication
						shouldSkip := false
						for _, tc := range toolCalls {
							if toolCall, ok := tc.(map[string]interface{}); ok {
								idx := 0
								if index, ok := toolCall["index"].(float64); ok {
									idx = int(index)
								}

								// Check for tool call ID (used for content_block_start dedup)
								toolID := ""
								if id, ok := toolCall["id"].(string); ok && id != "" {
									toolID = id
								}

								// Get function arguments if present
								args := ""
								if fn, ok := toolCall["function"].(map[string]interface{}); ok {
									if arguments, ok := fn["arguments"].(string); ok {
										args = arguments
									}
								}

								// Skip if we've seen this exact tool ID start event
								if toolID != "" && args == "" {
									// This is a tool_call start (has ID but no args yet)
									if rw.toolCallIDs[toolID] {
										shouldSkip = true
										break
									}
									rw.toolCallIDs[toolID] = true
								}

								// Skip if same arguments as last time for this index
								if args != "" && rw.lastToolCallArgs[idx] == args {
									shouldSkip = true
									break
								}
								if args != "" {
									rw.lastToolCallArgs[idx] = args
								}
							}
						}

						if shouldSkip {
							// Skip this chunk
							continue
						}
					}
				}
			}
		}

		// Write non-duplicate chunk
		output.WriteString(line)
		output.WriteString("\n")
	}

	deduped := output.Bytes()
	bytesFiltered := originalSize - len(deduped)
	return deduped, bytesFiltered
}

// Hijack implements http.Hijacker
func (rw *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hijacker, ok := rw.ResponseWriter.(http.Hijacker); ok {
		return hijacker.Hijack()
	}
	return nil, nil, http.ErrNotSupported
}

// Flush implements http.Flusher
func (rw *responseWriter) Flush() {
	if flusher, ok := rw.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// Status returns the captured status code
func (rw *responseWriter) Status() int {
	return rw.statusCode
}

// Size returns the number of bytes written
func (rw *responseWriter) Size() int64 {
	return rw.bytesWritten
}

// Body returns the captured body
func (rw *responseWriter) Body() []byte {
	if rw.body != nil {
		return rw.body.Bytes()
	}
	return nil
}

// bodyReader wraps the request body to capture its size
type bodyReader struct {
	io.ReadCloser
	bytesRead int64
}

// newBodyReader creates a new body reader wrapper
func newBodyReader(rc io.ReadCloser) *bodyReader {
	return &bodyReader{ReadCloser: rc}
}

// Read captures the number of bytes read
func (br *bodyReader) Read(p []byte) (int, error) {
	n, err := br.ReadCloser.Read(p)
	br.bytesRead += int64(n)
	return n, err
}

// BytesRead returns the total number of bytes read
func (br *bodyReader) BytesRead() int64 {
	return br.bytesRead
}

// newBodyReaderFromBytes creates a new body reader from a byte slice
func newBodyReaderFromBytes(data []byte) *bodyReader {
	return &bodyReader{
		ReadCloser: io.NopCloser(bytes.NewReader(data)),
		bytesRead:  0,
	}
}
