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
	chunkBuffer        []map[string]interface{} // Store parsed chunks
	processedChunks    int                      // Number of chunks already processed
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

	// Parse SSE chunks and extract content
	lines := strings.Split(content, "\n")
	chunkIndex := 0
	for _, line := range lines {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		jsonData := strings.TrimPrefix(line, "data: ")
		if jsonData == "[DONE]" {
			continue
		}

		// Skip already processed chunks
		if chunkIndex < rw.processedChunks {
			chunkIndex++
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
					if deltaContent, ok := delta["content"].(string); ok && deltaContent != "" {
						rw.accumulatedContent.WriteString(deltaContent)

						// Detect XML
						accumulated := rw.accumulatedContent.String()
						if !rw.xmlDetectionMode && strings.Contains(accumulated, "<function=") {
							rw.xmlDetectionMode = true
							log.Printf("[XML-PARSER] XML detection mode activated")
						}
					}
				}
			}
		}

		// Store chunk for later processing
		rw.chunkBuffer = append(rw.chunkBuffer, chunk)
		rw.processedChunks++
		chunkIndex++
	}

	// Check if we have complete XML
	accumulated := rw.accumulatedContent.String()
	if rw.xmlDetectionMode && strings.Contains(accumulated, "</tool_call>") {
		log.Printf("[XML-PARSER] Complete XML tool call detected, converting...")

		// Parse XML
		toolCalls := parseXMLToolCalls(accumulated)
		if len(toolCalls) > 0 {
			log.Printf("[XML-PARSER] Parsed %d tool calls", len(toolCalls))

			// Convert chunks to tool_calls format
			convertedData := rw.convertChunksToToolCalls(toolCalls[0])

			// Write converted data
			n, err := rw.ResponseWriter.Write(convertedData)
			rw.bytesWritten += int64(n)
			if rw.captureBody {
				rw.body.Write(convertedData)
			}

			// Reset state
			rw.sseBuffer.Reset()
			rw.accumulatedContent.Reset()
			rw.xmlDetectionMode = false
			rw.chunkBuffer = nil
			rw.processedChunks = 0

			return len(b), err
		}
	}

	// Not complete yet or no XML - pass through original data
	n, err := rw.ResponseWriter.Write(b)
	rw.bytesWritten += int64(n)
	if rw.captureBody {
		rw.body.Write(b)
	}

	return len(b), err
}

// convertChunksToToolCalls converts accumulated chunks to tool_calls format
func (rw *responseWriter) convertChunksToToolCalls(toolCall ToolCall) []byte {
	var result bytes.Buffer

	// Find the first chunk with content and replace it with tool_calls
	foundContent := false
	for _, chunk := range rw.chunkBuffer {
		if choices, ok := chunk["choices"].([]interface{}); ok && len(choices) > 0 {
			if choice, ok := choices[0].(map[string]interface{}); ok {
				if delta, ok := choice["delta"].(map[string]interface{}); ok {
					if !foundContent {
						// First chunk with content - replace with tool_calls
						delete(delta, "content")
						delta["tool_calls"] = []map[string]interface{}{
							{
								"index": 0,
								"id":    toolCall.ID,
								"type":  toolCall.Type,
								"function": map[string]interface{}{
									"name":      toolCall.Function.Name,
									"arguments": toolCall.Function.Arguments,
								},
							},
						}
						foundContent = true
					} else {
						// Subsequent chunks - remove content
						delete(delta, "content")
					}
				}
			}
		}

		// Re-encode chunk
		chunkJSON, _ := json.Marshal(chunk)
		result.WriteString("data: ")
		result.Write(chunkJSON)
		result.WriteString("\n\n")
	}

	log.Printf("[XML-PARSER] Converted XML to JSON tool calls")
	return result.Bytes()
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
