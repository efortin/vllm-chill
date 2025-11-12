package proxy

import (
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResponseWriter_Status(t *testing.T) {
	rw := &responseWriter{
		statusCode: 404,
	}

	assert.Equal(t, 404, rw.Status())
}

func TestResponseWriter_Size(t *testing.T) {
	rw := &responseWriter{
		bytesWritten: 42,
	}

	assert.Equal(t, int64(42), rw.Size())
}

func TestResponseWriter_BodyNil(t *testing.T) {
	rw := &responseWriter{}

	assert.Nil(t, rw.Body())
}

func TestNewBodyReader(t *testing.T) {
	testData := "test data"
	rc := io.NopCloser(strings.NewReader(testData))

	br := newBodyReader(rc)

	assert.NotNil(t, br)
	assert.Equal(t, int64(0), br.bytesRead)

	// Read some data
	buf := make([]byte, 4)
	n, err := br.Read(buf)
	assert.NoError(t, err)
	assert.Equal(t, 4, n)
	assert.Equal(t, int64(4), br.bytesRead)
}

func TestBodyReader_BytesRead(t *testing.T) {
	testData := "hello world"
	rc := io.NopCloser(strings.NewReader(testData))

	br := newBodyReader(rc)

	// Read all data
	buf := make([]byte, len(testData))
	n, err := br.Read(buf)
	assert.NoError(t, err)
	assert.Equal(t, len(testData), n)
	assert.Equal(t, int64(len(testData)), br.bytesRead)
}
