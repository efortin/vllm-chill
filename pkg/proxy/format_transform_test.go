package proxy

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTransformAnthropicToOpenAI(t *testing.T) {
	tests := []struct {
		name          string
		anthropicBody map[string]interface{}
		wantModel     string
		wantMessages  int
		wantSystem    bool
	}{
		{
			name: "basic transformation with string content",
			anthropicBody: map[string]interface{}{
				"model": "claude-3-5-sonnet-20241022",
				"messages": []interface{}{
					map[string]interface{}{
						"role":    "user",
						"content": "Hello, how are you?",
					},
				},
				"max_tokens": 1024,
			},
			wantModel:    "claude-3-5-sonnet-20241022",
			wantMessages: 1,
			wantSystem:   false,
		},
		{
			name: "with system message",
			anthropicBody: map[string]interface{}{
				"model":  "claude-3-haiku-20241022",
				"system": "You are a helpful assistant",
				"messages": []interface{}{
					map[string]interface{}{
						"role":    "user",
						"content": "Hi",
					},
				},
			},
			wantModel:    "claude-3-haiku-20241022",
			wantMessages: 2, // system + user
			wantSystem:   true,
		},
		{
			name: "with array content",
			anthropicBody: map[string]interface{}{
				"model": "claude-3-sonnet-20241022",
				"messages": []interface{}{
					map[string]interface{}{
						"role": "user",
						"content": []interface{}{
							map[string]interface{}{
								"type": "text",
								"text": "First part",
							},
							map[string]interface{}{
								"type": "text",
								"text": "Second part",
							},
						},
					},
				},
			},
			wantModel:    "claude-3-sonnet-20241022",
			wantMessages: 1,
			wantSystem:   false,
		},
		{
			name: "with temperature and stream parameters",
			anthropicBody: map[string]interface{}{
				"model": "claude-3-opus-20240229",
				"messages": []interface{}{
					map[string]interface{}{
						"role":    "user",
						"content": "Test",
					},
				},
				"temperature": 0.7,
				"stream":      true,
				"top_p":       0.9,
			},
			wantModel:    "claude-3-opus-20240229",
			wantMessages: 1,
			wantSystem:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := transformAnthropicToOpenAI(tt.anthropicBody)

			// Check model
			assert.Equal(t, tt.wantModel, result["model"])

			// Check messages
			messages, ok := result["messages"].([]map[string]interface{})
			require.True(t, ok, "messages should be a slice of maps")
			assert.Equal(t, tt.wantMessages, len(messages))

			// Check system message if expected
			if tt.wantSystem {
				assert.Equal(t, "system", messages[0]["role"])
				assert.NotEmpty(t, messages[0]["content"])
			}

			// Check parameters are copied
			if temp, ok := tt.anthropicBody["temperature"]; ok {
				assert.Equal(t, temp, result["temperature"])
			}
			if stream, ok := tt.anthropicBody["stream"]; ok {
				assert.Equal(t, stream, result["stream"])
			}
			if topP, ok := tt.anthropicBody["top_p"]; ok {
				assert.Equal(t, topP, result["top_p"])
			}
		})
	}
}

func TestTransformAnthropicToOpenAI_ArrayContent(t *testing.T) {
	anthropicBody := map[string]interface{}{
		"model": "test-model",
		"messages": []interface{}{
			map[string]interface{}{
				"role": "user",
				"content": []interface{}{
					map[string]interface{}{
						"type": "text",
						"text": "Hello",
					},
					map[string]interface{}{
						"type": "text",
						"text": "World",
					},
				},
			},
		},
	}

	result := transformAnthropicToOpenAI(anthropicBody)
	messages := result["messages"].([]map[string]interface{})

	require.Len(t, messages, 1)
	assert.Equal(t, "Hello\nWorld", messages[0]["content"])
}

func TestTransformOpenAIResponseToAnthropic(t *testing.T) {
	tests := []struct {
		name            string
		openAIResponse  map[string]interface{}
		wantType        string
		wantRole        string
		wantStopReason  string
		wantContentText string
	}{
		{
			name: "successful response",
			openAIResponse: map[string]interface{}{
				"id":    "chatcmpl-123",
				"model": "gpt-4",
				"choices": []interface{}{
					map[string]interface{}{
						"message": map[string]interface{}{
							"role":    "assistant",
							"content": "Hello! How can I help you?",
						},
						"finish_reason": "stop",
					},
				},
				"usage": map[string]interface{}{
					"prompt_tokens":     10,
					"completion_tokens": 20,
					"total_tokens":      30,
				},
			},
			wantType:        "message",
			wantRole:        "assistant",
			wantStopReason:  "end_turn",
			wantContentText: "Hello! How can I help you?",
		},
		{
			name: "length finish reason",
			openAIResponse: map[string]interface{}{
				"id":    "chatcmpl-456",
				"model": "gpt-3.5-turbo",
				"choices": []interface{}{
					map[string]interface{}{
						"message": map[string]interface{}{
							"content": "Response text",
						},
						"finish_reason": "length",
					},
				},
			},
			wantType:        "message",
			wantRole:        "assistant",
			wantStopReason:  "max_tokens",
			wantContentText: "Response text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			openAIBytes, err := json.Marshal(tt.openAIResponse)
			require.NoError(t, err)

			result, err := transformOpenAIResponseToAnthropic(openAIBytes)
			require.NoError(t, err)

			assert.Equal(t, tt.wantType, result["type"])
			assert.Equal(t, tt.wantRole, result["role"])

			if tt.wantStopReason != "" {
				assert.Equal(t, tt.wantStopReason, result["stop_reason"])
			}

			// Check content structure
			content, ok := result["content"].([]map[string]interface{})
			require.True(t, ok, "content should be an array")
			require.Len(t, content, 1)
			assert.Equal(t, "text", content[0]["type"])
			assert.Equal(t, tt.wantContentText, content[0]["text"])

			// Check usage if present
			if _, hasUsage := tt.openAIResponse["usage"]; hasUsage {
				usage, ok := result["usage"].(map[string]interface{})
				require.True(t, ok, "usage should be present")
				assert.NotNil(t, usage["input_tokens"])
				assert.NotNil(t, usage["output_tokens"])
			}
		})
	}
}

func TestTransformOpenAIResponseToAnthropic_EmptyChoices(t *testing.T) {
	openAIResponse := map[string]interface{}{
		"id":      "chatcmpl-empty",
		"model":   "gpt-4",
		"choices": []interface{}{},
	}

	openAIBytes, err := json.Marshal(openAIResponse)
	require.NoError(t, err)

	result, err := transformOpenAIResponseToAnthropic(openAIBytes)
	require.NoError(t, err)

	assert.Equal(t, "message", result["type"])
	assert.Equal(t, "assistant", result["role"])
}

func TestTransformOpenAIResponseToAnthropic_InvalidJSON(t *testing.T) {
	invalidJSON := []byte(`{invalid json}`)

	_, err := transformOpenAIResponseToAnthropic(invalidJSON)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse OpenAI response")
}

func TestTransformAnthropicToOpenAI_MultipleMessages(t *testing.T) {
	anthropicBody := map[string]interface{}{
		"model": "claude-3-sonnet",
		"messages": []interface{}{
			map[string]interface{}{
				"role":    "user",
				"content": "Hello",
			},
			map[string]interface{}{
				"role":    "assistant",
				"content": "Hi there!",
			},
			map[string]interface{}{
				"role":    "user",
				"content": "How are you?",
			},
		},
	}

	result := transformAnthropicToOpenAI(anthropicBody)
	messages := result["messages"].([]map[string]interface{})

	assert.Len(t, messages, 3)
	assert.Equal(t, "user", messages[0]["role"])
	assert.Equal(t, "Hello", messages[0]["content"])
	assert.Equal(t, "assistant", messages[1]["role"])
	assert.Equal(t, "Hi there!", messages[1]["content"])
	assert.Equal(t, "user", messages[2]["role"])
	assert.Equal(t, "How are you?", messages[2]["content"])
}

func TestTransformAnthropicToOpenAI_NoModel(t *testing.T) {
	anthropicBody := map[string]interface{}{
		"messages": []interface{}{
			map[string]interface{}{
				"role":    "user",
				"content": "Test",
			},
		},
	}

	result := transformAnthropicToOpenAI(anthropicBody)

	// Model should not be set if not provided
	_, hasModel := result["model"]
	assert.False(t, hasModel)

	// But messages should still be transformed
	messages := result["messages"].([]map[string]interface{})
	assert.Len(t, messages, 1)
}

func TestTransformOpenAIResponseToAnthropic_WithUsage(t *testing.T) {
	openAIResponse := map[string]interface{}{
		"id":    "chatcmpl-789",
		"model": "gpt-4",
		"choices": []interface{}{
			map[string]interface{}{
				"message": map[string]interface{}{
					"content": "Test response",
				},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]interface{}{
			"prompt_tokens":     100,
			"completion_tokens": 50,
			"total_tokens":      150,
		},
	}

	openAIBytes, _ := json.Marshal(openAIResponse)
	result, err := transformOpenAIResponseToAnthropic(openAIBytes)
	require.NoError(t, err)

	usage, ok := result["usage"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, float64(100), usage["input_tokens"])
	assert.Equal(t, float64(50), usage["output_tokens"])
}

func TestTransformAnthropicToOpenAI_ComplexContent(t *testing.T) {
	// Test with mixed content types (text and non-text)
	anthropicBody := map[string]interface{}{
		"model": "claude-3-sonnet",
		"messages": []interface{}{
			map[string]interface{}{
				"role": "user",
				"content": []interface{}{
					map[string]interface{}{
						"type": "text",
						"text": "Look at this:",
					},
					map[string]interface{}{
						"type":   "image",
						"source": map[string]interface{}{},
					},
					map[string]interface{}{
						"type": "text",
						"text": "What do you see?",
					},
				},
			},
		},
	}

	result := transformAnthropicToOpenAI(anthropicBody)
	messages := result["messages"].([]map[string]interface{})

	require.Len(t, messages, 1)
	// Should extract only text parts
	content, ok := messages[0]["content"].(string)
	require.True(t, ok)
	assert.Contains(t, content, "Look at this:")
	assert.Contains(t, content, "What do you see?")
}
