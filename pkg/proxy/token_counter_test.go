package proxy

import (
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
