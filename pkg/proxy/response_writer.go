package proxy

import (
	"bufio"
	"bytes"
	"io"
	"log"
	"net"
	"net/http"
)

// responseWriter wraps http.ResponseWriter to capture status code and response size
type responseWriter struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int64
	body         *bytes.Buffer
	captureBody  bool
}

// newResponseWriter creates a new response writer wrapper
func newResponseWriter(w http.ResponseWriter, captureBody bool) *responseWriter {
	rw := &responseWriter{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
		captureBody:    captureBody,
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
	// Check if response contains XML tool calls
	content := string(b)
	if hasXMLToolCalls(content) {
		log.Printf("[XML-PARSER] Detected XML tool calls in response (length: %d bytes)", len(b))
		log.Printf("[XML-PARSER] Content preview: %s", content[:minInt(len(content), 200)])
	}

	// Convert XML tool calls if present
	converted := convertXMLToolCallsInResponse(b)

	if len(converted) != len(b) {
		log.Printf("[XML-PARSER] Converted XML to JSON (original: %d bytes, converted: %d bytes)", len(b), len(converted))
	}

	n, err := rw.ResponseWriter.Write(converted)
	rw.bytesWritten += int64(n)

	if rw.captureBody {
		rw.body.Write(converted)
	}

	return len(b), err // Return original length for consistency
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
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
