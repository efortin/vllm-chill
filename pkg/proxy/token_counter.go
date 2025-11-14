package proxy

import (
	"strings"
)

// EstimateTokens provides a rough estimate of token count for text
// This is a simple heuristic - for accurate counts, use a proper tokenizer
func EstimateTokens(text string) int {
	if text == "" {
		return 0
	}

	// Simple heuristic: ~1 token per 4 characters (typical for English)
	// This is a rough approximation that works reasonably well for:
	// - English text
	// - Code
	// - Mixed content

	// Count characters
	charCount := len(text)

	// Estimate tokens (1 token ≈ 4 characters on average)
	tokenEstimate := charCount / 4

	// Minimum 1 token for non-empty text
	if tokenEstimate == 0 && charCount > 0 {
		tokenEstimate = 1
	}

	return tokenEstimate
}

// EstimateMessagesTokens estimates tokens for a list of messages
func EstimateMessagesTokens(messages []map[string]interface{}) int {
	totalTokens := 0

	for _, msg := range messages {
		// Add tokens for role (typically 1-2 tokens)
		totalTokens += 2

		// Add tokens for content
		if content, ok := msg["content"].(string); ok {
			totalTokens += EstimateTokens(content)
		} else if contentArray, ok := msg["content"].([]interface{}); ok {
			// Handle content array (for multimodal messages)
			for _, item := range contentArray {
				if itemMap, ok := item.(map[string]interface{}); ok {
					if text, ok := itemMap["text"].(string); ok {
						totalTokens += EstimateTokens(text)
					}
				}
			}
		}

		// Add overhead for message structure (typically 3-4 tokens)
		totalTokens += 3
	}

	// Add base overhead for conversation structure
	totalTokens += 5

	return totalTokens
}

// TokenTracker tracks tokens during streaming
type TokenTracker struct {
	inputTokens  int
	outputTokens int
	outputText   strings.Builder
}

// NewTokenTracker creates a new token tracker
func NewTokenTracker() *TokenTracker {
	return &TokenTracker{}
}

// SetInputTokens sets the estimated input tokens
func (tt *TokenTracker) SetInputTokens(tokens int) {
	tt.inputTokens = tokens
}

// AddOutputText adds text to output and updates token count
func (tt *TokenTracker) AddOutputText(text string) {
	tt.outputText.WriteString(text)
}

// GetUsage returns the current usage estimates
func (tt *TokenTracker) GetUsage() map[string]interface{} {
	outputTokens := EstimateTokens(tt.outputText.String())
	return map[string]interface{}{
		"input_tokens":  tt.inputTokens,
		"output_tokens": outputTokens,
	}
}

// GetTotalTokens returns the total token count
func (tt *TokenTracker) GetTotalTokens() int {
	outputTokens := EstimateTokens(tt.outputText.String())
	return tt.inputTokens + outputTokens
}
