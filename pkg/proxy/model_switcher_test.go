package proxy

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

func TestExtractModelFromRequest(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		expected string
		wantErr  bool
	}{
		{
			name:     "valid request",
			body:     `{"model": "qwen3-coder-30b-fp8", "messages": []}`,
			expected: "qwen3-coder-30b-fp8",
			wantErr:  false,
		},
		{
			name:     "different model",
			body:     `{"model": "deepseek-r1-fp8", "messages": []}`,
			expected: "deepseek-r1-fp8",
			wantErr:  false,
		},
		{
			name:     "missing model field",
			body:     `{"messages": []}`,
			expected: "",
			wantErr:  false,
		},
		{
			name:     "invalid json",
			body:     `{invalid json}`,
			expected: "",
			wantErr:  true,
		},
		{
			name:     "empty body",
			body:     ``,
			expected: "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &http.Request{
				Body: io.NopCloser(bytes.NewBufferString(tt.body)),
			}

			got, err := extractModelFromRequest(req)
			if (err != nil) != tt.wantErr {
				t.Errorf("extractModelFromRequest() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.expected {
				t.Errorf("extractModelFromRequest() = %v, want %v", got, tt.expected)
			}

			// Verify body can be read again
			body, err := io.ReadAll(req.Body)
			if err != nil {
				t.Errorf("Failed to read body again: %v", err)
			}
			if string(body) != tt.body {
				t.Errorf("Body not restored correctly, got %v, want %v", string(body), tt.body)
			}
		})
	}
}

func TestOpenAIRequest_Parsing(t *testing.T) {
	tests := []struct {
		name     string
		json     string
		expected string
		wantErr  bool
	}{
		{
			name:     "simple model",
			json:     `{"model": "test-model"}`,
			expected: "test-model",
			wantErr:  false,
		},
		{
			name:     "with additional fields",
			json:     `{"model": "test-model", "temperature": 0.7, "max_tokens": 100}`,
			expected: "test-model",
			wantErr:  false,
		},
		{
			name:     "empty model",
			json:     `{"model": ""}`,
			expected: "",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req OpenAIRequest
			err := json.Unmarshal([]byte(tt.json), &req)
			if (err != nil) != tt.wantErr {
				t.Errorf("json.Unmarshal() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if req.Model != tt.expected {
				t.Errorf("Model = %v, want %v", req.Model, tt.expected)
			}
		})
	}
}

func TestExtractModelFromRequest_BodyRestoration(t *testing.T) {
	body := `{"model": "test-model", "messages": [{"role": "user", "content": "hello"}]}`
	req := &http.Request{
		Body: io.NopCloser(bytes.NewBufferString(body)),
	}

	model, err := extractModelFromRequest(req)
	if err != nil {
		t.Fatalf("extractModelFromRequest() error = %v", err)
	}

	if model != "test-model" {
		t.Errorf("model = %v, want test-model", model)
	}

	// Read body again to verify restoration
	restoredBody, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("Failed to read restored body: %v", err)
	}

	if string(restoredBody) != body {
		t.Errorf("Body not properly restored.\nGot:  %s\nWant: %s", string(restoredBody), body)
	}
}

func TestExtractModelFromRequest_LargePayload(t *testing.T) {
	// Test with a large payload to ensure body restoration works
	largeMessages := make([]map[string]string, 100)
	for i := range largeMessages {
		largeMessages[i] = map[string]string{
			"role":    "user",
			"content": "This is a test message with some content",
		}
	}

	payload := map[string]interface{}{
		"model":    "test-model",
		"messages": largeMessages,
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Failed to marshal payload: %v", err)
	}

	req := &http.Request{
		Body: io.NopCloser(bytes.NewBuffer(bodyBytes)),
	}

	model, err := extractModelFromRequest(req)
	if err != nil {
		t.Fatalf("extractModelFromRequest() error = %v", err)
	}

	if model != "test-model" {
		t.Errorf("model = %v, want test-model", model)
	}

	// Verify body can still be read
	restoredBody, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("Failed to read restored body: %v", err)
	}

	if len(restoredBody) != len(bodyBytes) {
		t.Errorf("Body length mismatch. Got %d, want %d", len(restoredBody), len(bodyBytes))
	}
}
