package proxy

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/efortin/vllm-chill/pkg/stats"
	"github.com/stretchr/testify/assert"
)

func TestExtractModelFromRequest(t *testing.T) {
	as := &AutoScaler{
		metrics: stats.NewMetricsRecorder(),
	}

	tests := []struct {
		name          string
		body          string
		expectedModel string
	}{
		{
			name:          "valid request with model",
			body:          `{"model": "gpt-4", "messages": [{"role": "user", "content": "hello"}]}`,
			expectedModel: "gpt-4",
		},
		{
			name:          "request without model",
			body:          `{"messages": [{"role": "user", "content": "hello"}]}`,
			expectedModel: "",
		},
		{
			name:          "invalid JSON",
			body:          `{invalid json}`,
			expectedModel: "",
		},
		{
			name:          "empty body",
			body:          ``,
			expectedModel: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewBufferString(tt.body))
			model := as.extractModelFromRequest(req)
			assert.Equal(t, tt.expectedModel, model)
		})
	}
}

func TestModelNotFoundError(t *testing.T) {
	err := &ModelNotFoundError{RequestedModel: "gpt-4"}
	assert.Equal(t, "model 'gpt-4' not found", err.Error())
}

func TestGetModelNames(t *testing.T) {
	models := []ModelInfo{
		{Name: "model1"},
		{Name: "model2"},
		{Name: "model3"},
	}

	names := getModelNames(models)
	assert.Equal(t, []string{"model1", "model2", "model3"}, names)
}

func TestSendLoadingMessageStreaming(t *testing.T) {
	as := &AutoScaler{
		metrics: stats.NewMetricsRecorder(),
	}

	// Create request with streaming enabled
	reqBody := map[string]interface{}{
		"model":    "gpt-4",
		"messages": []map[string]string{{"role": "user", "content": "hello"}},
		"stream":   true,
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewBuffer(bodyBytes))
	w := httptest.NewRecorder()

	as.sendLoadingMessage(w, req, "gpt-4")

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "text/event-stream", w.Header().Get("Content-Type"))
	assert.Contains(t, w.Body.String(), "data:")
	assert.Contains(t, w.Body.String(), "[DONE]")
	assert.Contains(t, w.Body.String(), "Model 'gpt-4' is loading")
}

func TestSendLoadingMessageNonStreaming(t *testing.T) {
	as := &AutoScaler{
		metrics: stats.NewMetricsRecorder(),
	}

	// Create request with streaming disabled
	reqBody := map[string]interface{}{
		"model":    "gpt-4",
		"messages": []map[string]string{{"role": "user", "content": "hello"}},
		"stream":   false,
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewBuffer(bodyBytes))
	w := httptest.NewRecorder()

	as.sendLoadingMessage(w, req, "gpt-4")

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var response map[string]interface{}
	err := json.NewDecoder(w.Body).Decode(&response)
	assert.NoError(t, err)

	assert.Equal(t, "chat.completion", response["object"])
	assert.Equal(t, "gpt-4", response["model"])

	choices, ok := response["choices"].([]interface{})
	assert.True(t, ok)
	assert.Len(t, choices, 1)

	choice := choices[0].(map[string]interface{})
	message := choice["message"].(map[string]interface{})
	assert.Contains(t, message["content"], "Model 'gpt-4' is loading")
}

func TestNewBodyReaderFromBytes(t *testing.T) {
	data := []byte("test data")
	reader := newBodyReaderFromBytes(data)

	assert.NotNil(t, reader)

	// Read the data back
	buf := make([]byte, len(data))
	n, err := reader.Read(buf)
	assert.NoError(t, err)
	assert.Equal(t, len(data), n)
	assert.Equal(t, data, buf)

	// Check bytes read counter
	assert.Equal(t, int64(len(data)), reader.BytesRead())

	// Close should not error
	err = reader.Close()
	assert.NoError(t, err)
}

func TestGetActiveModel(t *testing.T) {
	as := &AutoScaler{
		activeModel: "test-model",
	}

	model := as.GetActiveModel()
	assert.Equal(t, "test-model", model)
}
