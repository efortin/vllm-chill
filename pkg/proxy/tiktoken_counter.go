package proxy

import (
	"log"
	"strings"

	"github.com/pkoukk/tiktoken-go"
)

// TiktokenCounter provides accurate token counting using tiktoken
type TiktokenCounter struct {
	encoding *tiktoken.Tiktoken
}

// NewTiktokenCounter creates a new tiktoken-based counter
func NewTiktokenCounter(model string) (*TiktokenCounter, error) {
	// Map model to tiktoken encoding
	encodingName := getEncodingForModel(model)

	// Get encoding
	enc, err := tiktoken.GetEncoding(encodingName)
	if err != nil {
		// Fallback to cl100k_base (GPT-4/Claude encoding)
		enc, err = tiktoken.GetEncoding("cl100k_base")
		if err != nil {
			log.Printf("Failed to get tiktoken encoding: %v", err)
			return nil, err
		}
	}

	return &TiktokenCounter{
		encoding: enc,
	}, nil
}

// getEncodingForModel returns the appropriate encoding for a model
func getEncodingForModel(model string) string {
	// Claude models use cl100k_base (same as GPT-4)
	if strings.Contains(model, "claude") {
		return "cl100k_base"
	}

	// GPT-4 models
	if strings.Contains(model, "gpt-4") {
		return "cl100k_base"
	}

	// GPT-3.5 models
	if strings.Contains(model, "gpt-3.5") {
		return "cl100k_base"
	}

	// Qwen models - use cl100k_base as approximation
	if strings.Contains(model, "qwen") {
		return "cl100k_base"
	}

	// Default to cl100k_base
	return "cl100k_base"
}

// CountTokens returns the exact token count for text
func (tc *TiktokenCounter) CountTokens(text string) int {
	if tc.encoding == nil {
		// Fallback to estimation if encoding failed
		return EstimateTokens(text)
	}

	tokens := tc.encoding.Encode(text, nil, nil)
	return len(tokens)
}

// CountMessagesTokens returns the exact token count for messages
func (tc *TiktokenCounter) CountMessagesTokens(messages []map[string]interface{}) int {
	if tc.encoding == nil {
		// Fallback to estimation if encoding failed
		return EstimateMessagesTokens(messages)
	}

	totalTokens := 0

	// Format overhead per message
	// For Claude/GPT-4: each message has ~3 tokens overhead
	tokensPerMessage := 3

	for _, msg := range messages {
		// Add message overhead
		totalTokens += tokensPerMessage

		// Count role tokens
		if role, ok := msg["role"].(string); ok {
			roleTokens := tc.encoding.Encode(role, nil, nil)
			totalTokens += len(roleTokens)
		}

		// Count content tokens
		if content, ok := msg["content"].(string); ok {
			contentTokens := tc.encoding.Encode(content, nil, nil)
			totalTokens += len(contentTokens)
		} else if contentArray, ok := msg["content"].([]interface{}); ok {
			// Handle content array
			for _, item := range contentArray {
				if itemMap, ok := item.(map[string]interface{}); ok {
					if text, ok := itemMap["text"].(string); ok {
						textTokens := tc.encoding.Encode(text, nil, nil)
						totalTokens += len(textTokens)
					}
				}
			}
		}

		// Handle name field if present
		if name, ok := msg["name"].(string); ok {
			nameTokens := tc.encoding.Encode(name, nil, nil)
			totalTokens += len(nameTokens)
			totalTokens -= 1 // Name replaces role, so adjust
		}
	}

	// Add base conversation overhead
	totalTokens += 3

	return totalTokens
}

// TiktokenTracker tracks tokens during streaming with accurate counting
type TiktokenTracker struct {
	counter     *TiktokenCounter
	inputTokens int
	outputText  strings.Builder
}

// NewTiktokenTracker creates a new tiktoken-based tracker
func NewTiktokenTracker(model string) *TiktokenTracker {
	counter, err := NewTiktokenCounter(model)
	if err != nil {
		log.Printf("Failed to create tiktoken counter, using estimation: %v", err)
	}

	return &TiktokenTracker{
		counter: counter,
	}
}

// SetInputTokens sets the input token count from messages
func (tt *TiktokenTracker) SetInputTokens(messages []map[string]interface{}) {
	if tt.counter != nil {
		tt.inputTokens = tt.counter.CountMessagesTokens(messages)
	} else {
		// Fallback to estimation
		tt.inputTokens = EstimateMessagesTokens(messages)
	}
}

// SetInputTokensCount sets a pre-calculated input token count
func (tt *TiktokenTracker) SetInputTokensCount(tokens int) {
	tt.inputTokens = tokens
}

// AddOutputText adds text to output
func (tt *TiktokenTracker) AddOutputText(text string) {
	tt.outputText.WriteString(text)
}

// GetUsage returns the current usage with accurate counts
func (tt *TiktokenTracker) GetUsage() map[string]interface{} {
	var outputTokens int

	outputStr := tt.outputText.String()
	if tt.counter != nil {
		outputTokens = tt.counter.CountTokens(outputStr)
	} else {
		// Fallback to estimation
		outputTokens = EstimateTokens(outputStr)
	}

	return map[string]interface{}{
		"input_tokens":  tt.inputTokens,
		"output_tokens": outputTokens,
	}
}

// GetTotalTokens returns the total token count
func (tt *TiktokenTracker) GetTotalTokens() int {
	usage := tt.GetUsage()
	return usage["input_tokens"].(int) + usage["output_tokens"].(int)
}
