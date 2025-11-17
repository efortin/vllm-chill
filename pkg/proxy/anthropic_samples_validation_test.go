package proxy

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// AnthropicConversation represents a test conversation sample
type AnthropicConversation struct {
	ConversationID string                   `json:"conversation_id"`
	CreatedAt      string                   `json:"created_at"`
	Domain         string                   `json:"domain"`
	Messages       []map[string]interface{} `json:"messages"`
}

// TestAnthropicConversationSamples validates all 50 conversation samples
func TestAnthropicConversationSamples(t *testing.T) {
	samplesDir := "../../test/data/anthropic-non-stream-samples"

	// TDD: First verify all 50 files exist
	for i := 1; i <= 50; i++ {
		filename := fmt.Sprintf("conversation_%03d.json", i)
		filepath := filepath.Join(samplesDir, filename)

		t.Run(filename, func(t *testing.T) {
			// Test 1: File exists
			_, err := os.Stat(filepath)
			require.NoError(t, err, "Conversation file should exist: %s", filename)

			// Test 2: File is valid JSON
			data, err := os.ReadFile(filepath)
			require.NoError(t, err, "Should read file: %s", filename)

			var conv AnthropicConversation
			err = json.Unmarshal(data, &conv)
			require.NoError(t, err, "Should parse JSON: %s", filename)

			// Test 3: Required fields present
			assert.NotEmpty(t, conv.ConversationID, "Should have conversation_id")
			assert.NotEmpty(t, conv.CreatedAt, "Should have created_at timestamp")
			assert.NotEmpty(t, conv.Domain, "Should have domain")
			assert.NotEmpty(t, conv.Messages, "Should have messages")

			// Test 4: Conversation ID matches filename
			expectedID := fmt.Sprintf("conversation_%03d", i)
			assert.Equal(t, expectedID, conv.ConversationID,
				"Conversation ID should match filename")

			// Test 5: Validate message structure
			testMessageStructure(t, conv.Messages, filename)
		})
	}
}

// testMessageStructure validates the structure of messages in a conversation
func testMessageStructure(t *testing.T, messages []map[string]interface{}, filename string) {
	require.Greater(t, len(messages), 0, "Should have at least one message")

	validRoles := map[string]bool{
		"user":        true,
		"assistant":   true,
		"tool_result": true,
	}

	for idx, msg := range messages {
		msgDesc := fmt.Sprintf("%s message[%d]", filename, idx)

		// Every message must have a role
		role, hasRole := msg["role"].(string)
		require.True(t, hasRole, "%s should have role", msgDesc)
		assert.True(t, validRoles[role], "%s has invalid role: %s", msgDesc, role)

		// Validate content based on role
		switch role {
		case "user":
			testUserMessage(t, msg, msgDesc)
		case "assistant":
			testAssistantMessage(t, msg, msgDesc)
		case "tool_result":
			testToolResultMessage(t, msg, msgDesc)
		}
	}
}

// testUserMessage validates user message format
func testUserMessage(t *testing.T, msg map[string]interface{}, msgDesc string) {
	content, hasContent := msg["content"]
	require.True(t, hasContent, "%s should have content", msgDesc)

	// User content should be string or array
	switch v := content.(type) {
	case string:
		assert.NotEmpty(t, v, "%s content should not be empty", msgDesc)
	case []interface{}:
		assert.Greater(t, len(v), 0, "%s content array should not be empty", msgDesc)
	default:
		t.Errorf("%s has invalid content type: %T", msgDesc, content)
	}
}

// testAssistantMessage validates assistant message format
func testAssistantMessage(t *testing.T, msg map[string]interface{}, msgDesc string) {
	content, hasContent := msg["content"]
	require.True(t, hasContent, "%s should have content", msgDesc)

	// Assistant content should be array
	contentArray, isArray := content.([]interface{})
	require.True(t, isArray, "%s content should be array", msgDesc)
	require.Greater(t, len(contentArray), 0, "%s content array should not be empty", msgDesc)

	validContentTypes := map[string]bool{
		"text":     true,
		"thinking": true,
		"tool_use": true,
	}

	// Validate each content block
	for blockIdx, block := range contentArray {
		blockMap, isMap := block.(map[string]interface{})
		require.True(t, isMap, "%s content[%d] should be object", msgDesc, blockIdx)

		blockType, hasType := blockMap["type"].(string)
		require.True(t, hasType, "%s content[%d] should have type", msgDesc, blockIdx)
		assert.True(t, validContentTypes[blockType],
			"%s content[%d] has invalid type: %s", msgDesc, blockIdx, blockType)

		// Validate based on block type
		switch blockType {
		case "text", "thinking":
			text, hasText := blockMap["text"].(string)
			assert.True(t, hasText, "%s content[%d] should have text", msgDesc, blockIdx)
			assert.NotEmpty(t, text, "%s content[%d] text should not be empty", msgDesc, blockIdx)

		case "tool_use":
			name, hasName := blockMap["name"].(string)
			assert.True(t, hasName, "%s content[%d] should have name", msgDesc, blockIdx)
			assert.NotEmpty(t, name, "%s content[%d] name should not be empty", msgDesc, blockIdx)

			_, hasInput := blockMap["input"]
			assert.True(t, hasInput, "%s content[%d] should have input", msgDesc, blockIdx)
		}
	}
}

// testToolResultMessage validates tool_result message format
func testToolResultMessage(t *testing.T, msg map[string]interface{}, msgDesc string) {
	name, hasName := msg["name"].(string)
	assert.True(t, hasName, "%s should have name", msgDesc)
	assert.NotEmpty(t, name, "%s name should not be empty", msgDesc)

	_, hasContent := msg["content"]
	assert.True(t, hasContent, "%s should have content", msgDesc)
}

// TestConversationTransformation tests that conversations can be transformed to OpenAI format
func TestConversationTransformation(t *testing.T) {
	samplesDir := "../../test/data/anthropic-non-stream-samples"

	// Test a subset of conversations for transformation
	testConversations := []int{1, 10, 25, 50}

	for _, i := range testConversations {
		filename := fmt.Sprintf("conversation_%03d.json", i)
		filepath := filepath.Join(samplesDir, filename)

		t.Run(fmt.Sprintf("Transform_%s", filename), func(t *testing.T) {
			data, err := os.ReadFile(filepath)
			require.NoError(t, err)

			var conv AnthropicConversation
			err = json.Unmarshal(data, &conv)
			require.NoError(t, err)

			// Transform each user message to OpenAI format
			for idx, msg := range conv.Messages {
				if msg["role"] == "user" {
					// Create Anthropic request format
					anthropicReq := map[string]interface{}{
						"model": "claude-3-5-sonnet-20241022",
						"messages": []interface{}{
							msg,
						},
						"max_tokens": 1024,
					}

					// Transform to OpenAI
					openAIReq := transformAnthropicToOpenAI(anthropicReq)

					// Validate transformation
					assert.NotNil(t, openAIReq, "Should transform to OpenAI format")
					assert.Equal(t, "claude-3-5-sonnet-20241022", openAIReq["model"])

					messages, ok := openAIReq["messages"].([]map[string]interface{})
					require.True(t, ok, "Should have messages array")
					assert.Greater(t, len(messages), 0,
						"Message %d from %s should transform", idx, filename)

					// Verify all messages have role and content
					for _, m := range messages {
						assert.NotEmpty(t, m["role"], "Transformed message should have role")
						assert.NotNil(t, m["content"], "Transformed message should have content")
					}
				}
			}
		})
	}
}

// TestConversationDomains validates domain categorization
func TestConversationDomains(t *testing.T) {
	samplesDir := "../../test/data/anthropic-non-stream-samples"

	validDomains := map[string]bool{
		"programming":     true,
		"debugging":       true,
		"architecture":    true,
		"devops":          true,
		"data_science":    true,
		"web_development": true,
		"testing":         true,
		"security":        true,
		"performance":     true,
		"general":         true,
	}

	domainCounts := make(map[string]int)

	for i := 1; i <= 50; i++ {
		filename := fmt.Sprintf("conversation_%03d.json", i)
		filepath := filepath.Join(samplesDir, filename)

		data, err := os.ReadFile(filepath)
		require.NoError(t, err)

		var conv AnthropicConversation
		err = json.Unmarshal(data, &conv)
		require.NoError(t, err)

		// Validate domain
		assert.True(t, validDomains[conv.Domain],
			"%s has invalid domain: %s", filename, conv.Domain)

		domainCounts[conv.Domain]++
	}

	// Print domain distribution
	t.Logf("Domain distribution across 50 conversations:")
	for domain, count := range domainCounts {
		t.Logf("  %s: %d", domain, count)
	}

	// Ensure we have at least one domain
	assert.GreaterOrEqual(t, len(domainCounts), 1,
		"Should have at least 1 domain")

	// Count total conversations with valid domains
	totalWithValidDomains := 0
	for _, count := range domainCounts {
		totalWithValidDomains += count
	}

	assert.Equal(t, 50, totalWithValidDomains,
		"All 50 conversations should have valid domains")
}

// TestConversationComplexity analyzes conversation complexity
func TestConversationComplexity(t *testing.T) {
	samplesDir := "../../test/data/anthropic-non-stream-samples"

	var totalMessages int
	var totalToolUses int
	var conversationsWithTools int

	for i := 1; i <= 50; i++ {
		filename := fmt.Sprintf("conversation_%03d.json", i)
		filepath := filepath.Join(samplesDir, filename)

		data, err := os.ReadFile(filepath)
		require.NoError(t, err)

		var conv AnthropicConversation
		err = json.Unmarshal(data, &conv)
		require.NoError(t, err)

		totalMessages += len(conv.Messages)

		hasTools := false
		for _, msg := range conv.Messages {
			if msg["role"] == "assistant" {
				if content, ok := msg["content"].([]interface{}); ok {
					for _, block := range content {
						if blockMap, ok := block.(map[string]interface{}); ok {
							if blockMap["type"] == "tool_use" {
								totalToolUses++
								hasTools = true
							}
						}
					}
				}
			}
		}

		if hasTools {
			conversationsWithTools++
		}
	}

	avgMessages := float64(totalMessages) / 50.0

	t.Logf("Conversation complexity metrics:")
	t.Logf("  Total messages: %d", totalMessages)
	t.Logf("  Average messages per conversation: %.1f", avgMessages)
	t.Logf("  Total tool uses: %d", totalToolUses)
	t.Logf("  Conversations with tools: %d", conversationsWithTools)

	// Quality checks
	assert.Greater(t, avgMessages, 2.0, "Conversations should have reasonable length")
	assert.Greater(t, conversationsWithTools, 10, "Should have enough tool usage examples")
}

// BenchmarkConversationParsing benchmarks parsing performance
func BenchmarkConversationParsing(b *testing.B) {
	samplesDir := "../../test/data/anthropic-non-stream-samples"
	filepath := filepath.Join(samplesDir, "conversation_001.json")

	data, err := os.ReadFile(filepath)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var conv AnthropicConversation
		_ = json.Unmarshal(data, &conv)
	}
}

// BenchmarkConversationTransformation benchmarks transformation performance
func BenchmarkConversationTransformation(b *testing.B) {
	samplesDir := "../../test/data/anthropic-non-stream-samples"
	filepath := filepath.Join(samplesDir, "conversation_001.json")

	data, err := os.ReadFile(filepath)
	if err != nil {
		b.Fatal(err)
	}

	var conv AnthropicConversation
	if err := json.Unmarshal(data, &conv); err != nil {
		b.Fatal(err)
	}

	anthropicReq := map[string]interface{}{
		"model":      "claude-3-5-sonnet-20241022",
		"messages":   conv.Messages,
		"max_tokens": 1024,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = transformAnthropicToOpenAI(anthropicReq)
	}
}
