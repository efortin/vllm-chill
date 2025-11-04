package proxy

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"
)

// responseWriter wraps http.ResponseWriter to capture status code and response size
type responseWriter struct {
	http.ResponseWriter
	statusCode         int
	bytesWritten       int64
	body               *bytes.Buffer
	captureBody        bool
	sseBuffer          *bytes.Buffer // Buffer for accumulating SSE chunks
	accumulatedContent strings.Builder
	xmlDetectionMode   bool
	xmlDetectionStart  time.Time                // When XML detection was activated
	chunkBuffer        []map[string]interface{} // Store parsed chunks for template
	toolCallsDetected  bool                     // Whether native tool calls were detected
}

// newResponseWriter creates a new response writer wrapper
func newResponseWriter(w http.ResponseWriter, captureBody bool) *responseWriter {
	rw := &responseWriter{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
		captureBody:    captureBody,
		sseBuffer:      &bytes.Buffer{},
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

// Write captures the response size and converts XML tool calls
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
	hasDoneMarker := false

	for _, line := range lines {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		jsonData := strings.TrimPrefix(line, "data: ")
		if jsonData == "[DONE]" {
			hasDoneMarker = true
			continue
		}

		var chunk map[string]interface{}
		if err := json.Unmarshal([]byte(jsonData), &chunk); err != nil {
			continue
		}

		// Extract content from delta
		if choices, ok := chunk["choices"].([]interface{}); ok && len(choices) > 0 {
			if choice, ok := choices[0].(map[string]interface{}); ok {
				if delta, ok := choice["delta"].(map[string]interface{}); ok {
					// Check for native tool calls (pass through immediately)
					if toolCalls, ok := delta["tool_calls"].([]interface{}); ok && len(toolCalls) > 0 {
						if !rw.toolCallsDetected {
							rw.toolCallsDetected = true
							log.Printf("[TOOL-CALLS] Native tool calls detected - passing through")
						}
					}

					// Extract text content
					if deltaContent, ok := delta["content"].(string); ok && deltaContent != "" {
						rw.accumulatedContent.WriteString(deltaContent)

						// Detect XML mode
						if !rw.xmlDetectionMode && strings.Contains(rw.accumulatedContent.String(), "<function=") {
							rw.xmlDetectionMode = true
							rw.xmlDetectionStart = time.Now()
							log.Printf("[XML-PARSER] XML detection mode activated - buffering until [DONE]")
						}
					}
				}
			}
		}

		// Store chunk for potential conversion (keep only first chunk for template)
		if len(rw.chunkBuffer) == 0 {
			rw.chunkBuffer = append(rw.chunkBuffer, chunk)
		}
	}

	// If we detected XML and stream is done, convert to single tool call response
	if rw.xmlDetectionMode && hasDoneMarker {
		accumulated := rw.accumulatedContent.String()
		log.Printf("[XML-PARSER] Stream complete, parsing XML (length: %d)", len(accumulated))

		toolCalls := parseXMLToolCalls(accumulated)
		if len(toolCalls) > 0 {
			log.Printf("[XML-PARSER] Successfully parsed %d tool calls, sending as single SSE chunk", len(toolCalls))

			// Build a single SSE chunk with the complete tool call
			singleChunk := rw.buildSingleToolCallChunk(toolCalls[0])

			// Write the single chunk
			_, err := rw.ResponseWriter.Write([]byte("data: "))
			if err != nil {
				return 0, err
			}
			_, err = rw.ResponseWriter.Write(singleChunk)
			if err != nil {
				return 0, err
			}
			_, err = rw.ResponseWriter.Write([]byte("\n\n"))
			if err != nil {
				return 0, err
			}

			// Write [DONE] marker
			_, err = rw.ResponseWriter.Write([]byte("data: [DONE]\n\n"))
			if err != nil {
				return 0, err
			}

			rw.bytesWritten += int64(len(singleChunk) + 20)
			if rw.captureBody {
				rw.body.Write(singleChunk)
			}

			// Reset state
			rw.sseBuffer.Reset()
			rw.accumulatedContent.Reset()
			rw.xmlDetectionMode = false
			rw.chunkBuffer = nil

			return len(b), nil
		}

		// XML parsing failed, flush buffered chunks as-is
		log.Printf("[XML-PARSER] Failed to parse XML, flushing %d buffered chunks", len(rw.chunkBuffer))
		for _, chunk := range rw.chunkBuffer {
			chunkJSON, _ := json.Marshal(chunk)
			_, _ = rw.ResponseWriter.Write([]byte("data: "))
			_, _ = rw.ResponseWriter.Write(chunkJSON)
			_, _ = rw.ResponseWriter.Write([]byte("\n\n"))
		}
		_, _ = rw.ResponseWriter.Write([]byte("data: [DONE]\n\n"))

		// Reset state
		rw.xmlDetectionMode = false
		rw.chunkBuffer = nil
		rw.sseBuffer.Reset()
		rw.accumulatedContent.Reset()
		return len(b), nil
	}

	// If NOT in XML mode, pass through immediately
	if !rw.xmlDetectionMode {
		n, err := rw.ResponseWriter.Write(b)
		rw.bytesWritten += int64(n)
		if rw.captureBody {
			rw.body.Write(b)
		}
		return len(b), err
	}

	// XML mode active, buffering until [DONE]
	log.Printf("[XML-PARSER] Buffering chunks... (elapsed: %v)", time.Since(rw.xmlDetectionStart))
	return len(b), nil
}

// buildSingleToolCallChunk builds a single SSE chunk with the complete tool call
func (rw *responseWriter) buildSingleToolCallChunk(toolCall ToolCall) []byte {
	// Use the first chunk as template (to get id, model, created, etc.)
	var templateChunk map[string]interface{}
	if len(rw.chunkBuffer) > 0 {
		templateChunk = rw.chunkBuffer[0]
	} else {
		// Fallback: create minimal chunk
		templateChunk = map[string]interface{}{
			"id":      "chatcmpl-" + toolCall.ID,
			"object":  "chat.completion.chunk",
			"created": time.Now().Unix(),
			"model":   "unknown",
		}
	}

	// Build the complete tool call chunk
	chunk := make(map[string]interface{})
	for k, v := range templateChunk {
		chunk[k] = v
	}

	// Set the delta with complete tool call
	chunk["choices"] = []map[string]interface{}{
		{
			"index": 0,
			"delta": map[string]interface{}{
				"tool_calls": []map[string]interface{}{
					{
						"index": 0,
						"id":    toolCall.ID,
						"type":  toolCall.Type,
						"function": map[string]interface{}{
							"name":      toolCall.Function.Name,
							"arguments": toolCall.Function.Arguments,
						},
					},
				},
			},
			"finish_reason": "tool_calls",
		},
	}

	chunkJSON, _ := json.Marshal(chunk)
	return chunkJSON
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
