package proxy

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTokenUsageHandling tests token usage in transformation
func TestTokenUsageHandling(t *testing.T) {
	t.Run("OpenAI response with usage should transform to Anthropic", func(t *testing.T) {
		// OpenAI response with usage
		openAIResp := map[string]interface{}{
			"id":      "chatcmpl-123",
			"object":  "chat.completion",
			"created": 1234567890,
			"model":   "gpt-4",
			"choices": []interface{}{
				map[string]interface{}{
					"index": 0,
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": "Hello! How can I help you today?",
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]interface{}{
				"prompt_tokens":     10,
				"completion_tokens": 8,
				"total_tokens":      18,
			},
		}

		// Marshal to JSON
		openAIBytes, err := json.Marshal(openAIResp)
		require.NoError(t, err)

		// Transform
		anthropicResp, err := transformOpenAIResponseToAnthropic(openAIBytes)
		require.NoError(t, err)

		// Check usage is present
		usage, ok := anthropicResp["usage"].(map[string]interface{})
		assert.True(t, ok, "usage should be present in Anthropic response")
		assert.Equal(t, 10, int(usage["input_tokens"].(float64)))
		assert.Equal(t, 8, int(usage["output_tokens"].(float64)))

		// usage should NOT have total_tokens (Anthropic doesn't use this field)
		_, hasTotalTokens := usage["total_tokens"]
		assert.False(t, hasTotalTokens, "total_tokens should not be in Anthropic response")
	})

	t.Run("OpenAI response without usage should handle gracefully", func(t *testing.T) {
		// OpenAI response WITHOUT usage (some providers might not include it)
		openAIResp := map[string]interface{}{
			"id":      "chatcmpl-456",
			"object":  "chat.completion",
			"created": 1234567890,
			"model":   "gpt-4",
			"choices": []interface{}{
				map[string]interface{}{
					"index": 0,
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": "Hello!",
					},
					"finish_reason": "stop",
				},
			},
			// NO usage field
		}

		// Marshal to JSON
		openAIBytes, err := json.Marshal(openAIResp)
		require.NoError(t, err)

		// Transform
		anthropicResp, err := transformOpenAIResponseToAnthropic(openAIBytes)
		require.NoError(t, err)

		// Check usage is not present (it's optional)
		usage, hasUsage := anthropicResp["usage"]
		if hasUsage {
			assert.Nil(t, usage, "usage should be nil if not provided")
		}
	})

	t.Run("OpenAI response with integer tokens should handle correctly", func(t *testing.T) {
		// Some OpenAI providers return integers instead of floats
		openAIResp := map[string]interface{}{
			"id":      "chatcmpl-789",
			"object":  "chat.completion",
			"created": 1234567890,
			"model":   "gpt-4",
			"choices": []interface{}{
				map[string]interface{}{
					"index": 0,
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": "Test",
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]interface{}{
				"prompt_tokens":     int(25), // Integer, not float
				"completion_tokens": int(15), // Integer, not float
				"total_tokens":      int(40), // Integer, not float
			},
		}

		// Marshal to JSON
		openAIBytes, err := json.Marshal(openAIResp)
		require.NoError(t, err)

		// Transform
		anthropicResp, err := transformOpenAIResponseToAnthropic(openAIBytes)
		require.NoError(t, err)

		// Check usage is present
		usage, ok := anthropicResp["usage"].(map[string]interface{})
		assert.True(t, ok, "usage should be present in Anthropic response")

		// Check values (should work with int or float)
		inputTokens := usage["input_tokens"]
		outputTokens := usage["output_tokens"]

		// Handle both int and float64 (JSON unmarshal default)
		var inputVal, outputVal int
		switch v := inputTokens.(type) {
		case float64:
			inputVal = int(v)
		case int:
			inputVal = v
		}
		switch v := outputTokens.(type) {
		case float64:
			outputVal = int(v)
		case int:
			outputVal = v
		}

		assert.Equal(t, 25, inputVal)
		assert.Equal(t, 15, outputVal)
	})
}

// TestUsageInRealScenarios tests usage with real-world scenarios
func TestUsageInRealScenarios(t *testing.T) {
	t.Run("Claude Code stops early - investigate", func(t *testing.T) {
		// Simulate a scenario where usage might cause Claude to stop
		openAIResp := map[string]interface{}{
			"id":      "chatcmpl-real",
			"object":  "chat.completion",
			"created": 1234567890,
			"model":   "qwen3-coder:30b",
			"choices": []interface{}{
				map[string]interface{}{
					"index": 0,
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": "I'll analyze this codebase and create a CLAUDE.md file with the necessary information for Claude Code instances to operate effectively.\n\nFirst, let me explore the repository structure to understand what we're working with.",
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]interface{}{
				"prompt_tokens":     100,
				"completion_tokens": 42,
				"total_tokens":      142,
			},
		}

		// Marshal to JSON
		openAIBytes, err := json.Marshal(openAIResp)
		require.NoError(t, err)

		// Transform
		anthropicResp, err := transformOpenAIResponseToAnthropic(openAIBytes)
		require.NoError(t, err)

		// Verify the response structure
		assert.Equal(t, "message", anthropicResp["type"])
		assert.Equal(t, "assistant", anthropicResp["role"])
		assert.Equal(t, "end_turn", anthropicResp["stop_reason"])

		// Check content
		content := anthropicResp["content"].([]map[string]interface{})
		assert.Len(t, content, 1)
		assert.Equal(t, "text", content[0]["type"])
		assert.Contains(t, content[0]["text"], "I'll analyze this codebase")

		// Check usage
		usage := anthropicResp["usage"].(map[string]interface{})
		assert.Equal(t, 100, int(usage["input_tokens"].(float64)))
		assert.Equal(t, 42, int(usage["output_tokens"].(float64)))

		// Print the full response for debugging
		respJSON, _ := json.MarshalIndent(anthropicResp, "", "  ")
		t.Logf("Full Anthropic response:\n%s", string(respJSON))
	})

	t.Run("vLLM response without usage field", func(t *testing.T) {
		// vLLM might not always include usage
		openAIResp := map[string]interface{}{
			"id":      "vllm-123",
			"object":  "chat.completion",
			"created": 1234567890,
			"model":   "qwen3-coder:30b",
			"choices": []interface{}{
				map[string]interface{}{
					"index": 0,
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": "Testing response without usage",
					},
					"finish_reason": "stop",
				},
			},
			// Intentionally no usage field
		}

		// Marshal to JSON
		openAIBytes, err := json.Marshal(openAIResp)
		require.NoError(t, err)

		// Transform
		anthropicResp, err := transformOpenAIResponseToAnthropic(openAIBytes)
		require.NoError(t, err)

		// Should still work without usage
		assert.Equal(t, "message", anthropicResp["type"])
		assert.Equal(t, "assistant", anthropicResp["role"])

		// Usage might not be present
		_, hasUsage := anthropicResp["usage"]
		t.Logf("Has usage field: %v", hasUsage)

		// Response should still be valid for Claude Code
		respJSON, _ := json.MarshalIndent(anthropicResp, "", "  ")
		t.Logf("Response without usage:\n%s", string(respJSON))
	})
}
