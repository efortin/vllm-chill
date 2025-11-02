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
