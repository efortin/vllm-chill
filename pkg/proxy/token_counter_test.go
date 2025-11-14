package proxy

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestEstimateTokens tests the token estimation function
func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected int
	}{
		{
			name:     "empty string",
			text:     "",
			expected: 0,
		},
		{
			name:     "short text",
			text:     "Hello",
			expected: 1, // 5 chars / 4 ≈ 1
		},
		{
			name:     "typical sentence",
			text:     "Hello! How can I help you today?",
			expected: 8, // 33 chars / 4 ≈ 8
		},
		{
			name:     "code snippet",
			text:     "func main() { fmt.Println(\"Hello, World!\") }",
			expected: 11, // 45 chars / 4 ≈ 11
		},
		{
			name:     "long paragraph",
			text:     "I'll analyze this codebase and create a CLAUDE.md file with the necessary information for Claude Code instances to operate effectively. First, let me explore the repository structure to understand what we're working with.",
			expected: 55, // 220 chars / 4 = 55
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := EstimateTokens(tt.text)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestEstimateMessagesTokens tests token estimation for messages
func TestEstimateMessagesTokens(t *testing.T) {
	messages := []map[string]interface{}{
		{
			"role":    "system",
			"content": "You are a helpful assistant.",
		},
		{
			"role":    "user",
			"content": "Hello! How can I help you today?",
		},
		{
			"role":    "assistant",
			"content": "I can help you with various tasks including answering questions, writing code, and more.",
		},
	}

	tokens := EstimateMessagesTokens(messages)

	// Should have:
	// - System message: 2 (role) + 7 (content ≈ 29/4) + 3 (overhead) = 12
	// - User message: 2 (role) + 8 (content ≈ 33/4) + 3 (overhead) = 13
	// - Assistant message: 2 (role) + 21 (content ≈ 86/4) + 3 (overhead) = 26
	// - Base overhead: 5
	// Total: 12 + 13 + 26 + 5 = 56

	assert.Greater(t, tokens, 50)
	assert.Less(t, tokens, 60)
}

// TestTokenTracker tests the token tracker
func TestTokenTracker(t *testing.T) {
	tracker := NewTokenTracker()

	// Set input tokens
	tracker.SetInputTokens(100)

	// Add output text incrementally (simulating streaming)
	tracker.AddOutputText("Hello! ")
	tracker.AddOutputText("How can ")
	tracker.AddOutputText("I help ")
	tracker.AddOutputText("you today?")

	// Get usage
	usage := tracker.GetUsage()

	assert.Equal(t, 100, usage["input_tokens"])

	// Output should be "Hello! How can I help you today?" (33 chars / 4 ≈ 8 tokens)
	outputTokens := usage["output_tokens"].(int)
	assert.Equal(t, 8, outputTokens)

	// Total tokens
	total := tracker.GetTotalTokens()
	assert.Equal(t, 108, total)
}

// TestTokenTrackerWithComplexContent tests with more complex content
func TestTokenTrackerWithComplexContent(t *testing.T) {
	tracker := NewTokenTracker()

	// Simulate a real scenario
	messages := []map[string]interface{}{
		{
			"role": "user",
			"content": []interface{}{
				map[string]interface{}{
					"type": "text",
					"text": "Please analyze this code",
				},
			},
		},
	}

	inputTokens := EstimateMessagesTokens(messages)
	tracker.SetInputTokens(inputTokens)

	// Simulate streaming response
	response := "I'll analyze this codebase and create a CLAUDE.md file with the necessary information for Claude Code instances to operate effectively.\n\nFirst, let me explore the repository structure to understand what we're working with."

	// Add in chunks (simulating streaming)
	chunkSize := 20
	for i := 0; i < len(response); i += chunkSize {
		end := i + chunkSize
		if end > len(response) {
			end = len(response)
		}
		tracker.AddOutputText(response[i:end])
	}

	usage := tracker.GetUsage()

	// Check that we have both input and output tokens
	assert.Greater(t, usage["input_tokens"].(int), 0)
	assert.Greater(t, usage["output_tokens"].(int), 40) // Should be around 53

	t.Logf("Estimated usage: input=%d, output=%d", usage["input_tokens"], usage["output_tokens"])
}

// TestTokenTrackerZeroValues tests edge case with zero tokens
func TestTokenTrackerZeroValues(t *testing.T) {
	tracker := NewTokenTracker()

	// Don't set input tokens - should default to 0
	usage := tracker.GetUsage()

	assert.Equal(t, 0, usage["input_tokens"])
	assert.Equal(t, 0, usage["output_tokens"])
	assert.Equal(t, 0, tracker.GetTotalTokens())
}

// TestTokenTrackerEmptyOutput tests with no output
func TestTokenTrackerEmptyOutput(t *testing.T) {
	tracker := NewTokenTracker()
	tracker.SetInputTokens(50)

	// Don't add any output text
	usage := tracker.GetUsage()

	assert.Equal(t, 50, usage["input_tokens"])
	assert.Equal(t, 0, usage["output_tokens"])
	assert.Equal(t, 50, tracker.GetTotalTokens())
}

// TestEstimateMessagesTokensWithEmptyArray tests with empty messages array
func TestEstimateMessagesTokensWithEmptyArray(t *testing.T) {
	messages := []map[string]interface{}{}
	tokens := EstimateMessagesTokens(messages)

	// Should still return base overhead
	assert.Greater(t, tokens, 0)
	assert.Less(t, tokens, 10)
}

// TestEstimateMessagesTokensWithToolResults tests messages with tool results
func TestEstimateMessagesTokensWithToolResults(t *testing.T) {
	messages := []map[string]interface{}{
		{
			"role": "user",
			"content": []interface{}{
				map[string]interface{}{
					"type": "text",
					"text": "What is the weather in Paris?",
				},
			},
		},
		{
			"role": "assistant",
			"content": []interface{}{
				map[string]interface{}{
					"type": "tool_use",
					"id":   "toolu_123",
					"name": "get_weather",
					"input": map[string]interface{}{
						"location": "Paris",
					},
				},
			},
		},
		{
			"role": "user",
			"content": []interface{}{
				map[string]interface{}{
					"type":        "tool_result",
					"tool_use_id": "toolu_123",
					"content":     "The weather in Paris is sunny and 22°C",
				},
			},
		},
	}

	tokens := EstimateMessagesTokens(messages)

	// Should account for all message content including tool results
	// Tool calls add overhead, expect at least 20 tokens
	assert.Greater(t, tokens, 20)
	t.Logf("Estimated tokens with tool results: %d", tokens)
}

// TestEstimateMessagesTokensWithMixedContentTypes tests with various content types
func TestEstimateMessagesTokensWithMixedContentTypes(t *testing.T) {
	messages := []map[string]interface{}{
		{
			"role": "user",
			"content": []interface{}{
				map[string]interface{}{
					"type": "text",
					"text": "Analyze this image",
				},
				map[string]interface{}{
					"type": "image",
					"source": map[string]interface{}{
						"type": "base64",
						"data": "...",
					},
				},
			},
		},
	}

	tokens := EstimateMessagesTokens(messages)

	// Should count text content and add overhead for image
	assert.Greater(t, tokens, 10)
	t.Logf("Estimated tokens with mixed content: %d", tokens)
}

// TestTokenTrackerLargeOutput tests with large output
func TestTokenTrackerLargeOutput(t *testing.T) {
	tracker := NewTokenTracker()
	tracker.SetInputTokens(100)

	// Add a large amount of text (simulating a long response)
	largeText := `This is a comprehensive analysis of the codebase.
It includes detailed explanations of the architecture, implementation details,
and recommendations for future improvements. The system uses a microservices
architecture with multiple components communicating through well-defined APIs.
Each service is responsible for a specific domain and follows clean architecture
principles. The codebase demonstrates good separation of concerns and maintainability.`

	// Add in realistic streaming chunks
	words := []string{}
	for _, word := range strings.Split(largeText, " ") {
		words = append(words, word+" ")
		if len(words) >= 5 {
			tracker.AddOutputText(strings.Join(words, ""))
			words = []string{}
		}
	}
	if len(words) > 0 {
		tracker.AddOutputText(strings.Join(words, ""))
	}

	usage := tracker.GetUsage()

	assert.Equal(t, 100, usage["input_tokens"])
	assert.Greater(t, usage["output_tokens"].(int), 100) // Should be significant
	assert.Greater(t, tracker.GetTotalTokens(), 200)

	t.Logf("Large output estimation: input=%d, output=%d, total=%d",
		usage["input_tokens"], usage["output_tokens"], tracker.GetTotalTokens())
}

// TestEstimateTokensSpecialCharacters tests with special characters
func TestEstimateTokensSpecialCharacters(t *testing.T) {
	tests := []struct {
		name      string
		text      string
		minTokens int
	}{
		{
			name:      "unicode characters",
			text:      "Hello 世界! こんにちは",
			minTokens: 3,
		},
		{
			name:      "emojis",
			text:      "Great work! 🎉 👍 🚀",
			minTokens: 3,
		},
		{
			name:      "code with special chars",
			text:      "const regex = /^[a-zA-Z0-9_]+$/;",
			minTokens: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := EstimateTokens(tt.text)
			assert.GreaterOrEqual(t, result, tt.minTokens)
			t.Logf("%s: %d tokens for %d chars", tt.name, result, len(tt.text))
		})
	}
}
