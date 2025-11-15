package proxy

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// AnthropicStreamEvent represents a single SSE event in the stream
type AnthropicStreamEvent struct {
	Event string                 // event type (e.g., "message_start", "content_block_delta")
	Data  map[string]interface{} // parsed JSON data
}

// TestAnthropicStreamSamples validates all 20 streaming conversation samples (TDD)
func TestAnthropicStreamSamples(t *testing.T) {
	samplesDir := "../../test/data/anthropic-stream-samples"

	// TDD: First verify all 20 files exist
	for i := 1; i <= 20; i++ {
		filename := fmt.Sprintf("conv_stream_%03d.jsonl", i)
		filepath := filepath.Join(samplesDir, filename)

		t.Run(filename, func(t *testing.T) {
			// Test 1: File exists
			_, err := os.Stat(filepath)
			require.NoError(t, err, "Stream file should exist: %s", filename)

			// Test 2: File is readable as JSONL
			events, err := parseStreamFile(filepath)
			require.NoError(t, err, "Should parse JSONL file: %s", filename)
			require.Greater(t, len(events), 0, "Should have at least one event")

			// Test 3: First event should be message_start
			assert.Equal(t, "message_start", events[0].Event,
				"First event should be message_start")

			// Test 4: Last event should be message_stop
			lastEvent := events[len(events)-1]
			assert.Equal(t, "message_stop", lastEvent.Event,
				"Last event should be message_stop")

			// Test 5: Validate message_start structure
			validateMessageStart(t, events[0], filename)

			// Test 6: Check for required event types
			eventTypes := make(map[string]int)
			for _, event := range events {
				eventTypes[event.Event]++
			}

			assert.Greater(t, eventTypes["message_start"], 0, "Should have message_start")
			assert.Greater(t, eventTypes["content_block_start"], 0, "Should have content_block_start")
			assert.Greater(t, eventTypes["message_stop"], 0, "Should have message_stop")

			// Test 7: Validate event sequence
			validateEventSequence(t, events, filename)

			// Test 8: Validate content blocks
			validateContentBlocks(t, events, filename)
		})
	}
}

// TestStreamEventTypes validates all event types in streaming samples
func TestStreamEventTypes(t *testing.T) {
	samplesDir := "../../test/data/anthropic-stream-samples"

	validEventTypes := map[string]bool{
		"message_start":       true,
		"content_block_start": true,
		"content_block_delta": true,
		"content_block_stop":  true,
		"message_delta":       true,
		"message_stop":        true,
		"tool_result":         true,
		"ping":                true, // Optional keep-alive
	}

	allEventTypes := make(map[string]int)

	for i := 1; i <= 20; i++ {
		filename := fmt.Sprintf("conv_stream_%03d.jsonl", i)
		filepath := filepath.Join(samplesDir, filename)

		events, err := parseStreamFile(filepath)
		require.NoError(t, err)

		for _, event := range events {
			allEventTypes[event.Event]++

			// Validate event type is known
			assert.True(t, validEventTypes[event.Event],
				"Unknown event type: %s in %s", event.Event, filename)
		}
	}

	t.Logf("Event types distribution across 20 streams:")
	for eventType, count := range allEventTypes {
		t.Logf("  %s: %d", eventType, count)
	}

	// Quality checks
	assert.Greater(t, allEventTypes["message_start"], 15, "Should have enough message_start events")
	assert.Greater(t, allEventTypes["content_block_delta"], 100, "Should have many delta events")
}

// TestStreamContentBlockTypes validates content block types
func TestStreamContentBlockTypes(t *testing.T) {
	samplesDir := "../../test/data/anthropic-stream-samples"

	contentBlockTypes := make(map[string]int)
	streamsWithTools := 0

	for i := 1; i <= 20; i++ {
		filename := fmt.Sprintf("conv_stream_%03d.jsonl", i)
		filepath := filepath.Join(samplesDir, filename)

		events, err := parseStreamFile(filepath)
		require.NoError(t, err)

		hasTools := false
		for _, event := range events {
			if event.Event == "content_block_start" {
				if contentBlock, ok := event.Data["content_block"].(map[string]interface{}); ok {
					if blockType, ok := contentBlock["type"].(string); ok {
						contentBlockTypes[blockType]++
						if blockType == "tool_use" {
							hasTools = true
						}
					}
				}
			}
		}

		if hasTools {
			streamsWithTools++
		}
	}

	t.Logf("Content block types distribution:")
	for blockType, count := range contentBlockTypes {
		t.Logf("  %s: %d", blockType, count)
	}
	t.Logf("Streams with tools: %d/20", streamsWithTools)

	// Quality checks
	assert.Greater(t, contentBlockTypes["text"], 10, "Should have text blocks")
	assert.Greater(t, streamsWithTools, 5, "Should have some streams with tools")
}

// TestStreamToOpenAITransformation tests streaming format transformation
func TestStreamToOpenAITransformation(t *testing.T) {
	samplesDir := "../../test/data/anthropic-stream-samples"

	// Test a subset
	testStreams := []int{1, 5, 10, 15, 20}

	for _, i := range testStreams {
		filename := fmt.Sprintf("conv_stream_%03d.jsonl", i)
		filepath := filepath.Join(samplesDir, filename)

		t.Run(fmt.Sprintf("Transform_%s", filename), func(t *testing.T) {
			events, err := parseStreamFile(filepath)
			require.NoError(t, err)

			// Transform to OpenAI streaming format
			openAIEvents := transformAnthropicStreamToOpenAI(events)
			require.Greater(t, len(openAIEvents), 0, "Should produce OpenAI events")

			// Validate OpenAI format
			for _, oaiEvent := range openAIEvents {
				// Should have "data: " prefix or be "[DONE]"
				assert.True(t,
					strings.HasPrefix(oaiEvent, "data: ") || oaiEvent == "data: [DONE]",
					"OpenAI event should start with 'data: '")
			}
		})
	}
}

// TestStreamComplexity analyzes stream complexity
func TestStreamComplexity(t *testing.T) {
	samplesDir := "../../test/data/anthropic-stream-samples"

	totalEvents := 0
	totalDeltas := 0
	totalToolUses := 0

	for i := 1; i <= 20; i++ {
		filename := fmt.Sprintf("conv_stream_%03d.jsonl", i)
		filepath := filepath.Join(samplesDir, filename)

		events, err := parseStreamFile(filepath)
		require.NoError(t, err)

		totalEvents += len(events)

		for _, event := range events {
			if event.Event == "content_block_delta" {
				totalDeltas++
			}
			if event.Event == "content_block_start" {
				if contentBlock, ok := event.Data["content_block"].(map[string]interface{}); ok {
					if blockType, ok := contentBlock["type"].(string); ok && blockType == "tool_use" {
						totalToolUses++
					}
				}
			}
		}
	}

	avgEvents := float64(totalEvents) / 20.0
	avgDeltas := float64(totalDeltas) / 20.0

	t.Logf("Stream complexity metrics:")
	t.Logf("  Total events: %d", totalEvents)
	t.Logf("  Average events per stream: %.1f", avgEvents)
	t.Logf("  Total deltas: %d", totalDeltas)
	t.Logf("  Average deltas per stream: %.1f", avgDeltas)
	t.Logf("  Total tool uses: %d", totalToolUses)

	// Quality checks
	assert.Greater(t, avgEvents, 10.0, "Streams should have reasonable length")
	assert.Greater(t, avgDeltas, 5.0, "Streams should have text deltas")
}

// BenchmarkStreamParsing benchmarks parsing performance
func BenchmarkStreamParsing(b *testing.B) {
	samplesDir := "../../test/data/anthropic-stream-samples"
	filepath := filepath.Join(samplesDir, "conv_stream_001.jsonl")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := parseStreamFile(filepath)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkStreamTransformation benchmarks transformation performance
func BenchmarkStreamTransformation(b *testing.B) {
	samplesDir := "../../test/data/anthropic-stream-samples"
	filepath := filepath.Join(samplesDir, "conv_stream_001.jsonl")

	events, err := parseStreamFile(filepath)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = transformAnthropicStreamToOpenAI(events)
	}
}

// Helper functions

// parseStreamFile parses a JSONL SSE file into events
func parseStreamFile(filepath string) ([]AnthropicStreamEvent, error) {
	file, err := os.Open(filepath)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = file.Close() // Ignore error in defer
	}()

	var events []AnthropicStreamEvent
	scanner := bufio.NewScanner(file)

	var currentEvent string

	for scanner.Scan() {
		line := scanner.Text()

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse SSE format: "event: type" or "data: {...}"
		if strings.HasPrefix(line, "event: ") {
			currentEvent = strings.TrimPrefix(line, "event: ")
		} else if strings.HasPrefix(line, "data: ") {
			dataStr := strings.TrimPrefix(line, "data: ")

			var data map[string]interface{}
			if err := json.Unmarshal([]byte(dataStr), &data); err != nil {
				return nil, fmt.Errorf("failed to parse data JSON: %w", err)
			}

			events = append(events, AnthropicStreamEvent{
				Event: currentEvent,
				Data:  data,
			})
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return events, nil
}

// validateMessageStart validates the message_start event structure
func validateMessageStart(t *testing.T, event AnthropicStreamEvent, filename string) {
	assert.Equal(t, "message_start", event.Event)

	msgType, ok := event.Data["type"].(string)
	assert.True(t, ok, "%s: message_start should have type", filename)
	assert.Equal(t, "message_start", msgType)

	message, ok := event.Data["message"].(map[string]interface{})
	assert.True(t, ok, "%s: message_start should have message object", filename)

	// Validate message structure
	assert.NotEmpty(t, message["id"], "%s: message should have id", filename)
	assert.Equal(t, "message", message["type"], "%s: message type should be 'message'", filename)
	assert.Equal(t, "assistant", message["role"], "%s: message role should be 'assistant'", filename)
}

// validateEventSequence validates the order of events
func validateEventSequence(t *testing.T, events []AnthropicStreamEvent, filename string) {
	if len(events) < 2 {
		return
	}

	// First must be message_start
	assert.Equal(t, "message_start", events[0].Event,
		"%s: First event must be message_start", filename)

	// Last must be message_stop
	assert.Equal(t, "message_stop", events[len(events)-1].Event,
		"%s: Last event must be message_stop", filename)

	// Track content block state
	openBlocks := make(map[int]bool)

	for i, event := range events {
		switch event.Event {
		case "content_block_start":
			index := int(event.Data["index"].(float64))
			assert.False(t, openBlocks[index],
				"%s: content block %d started twice at event %d", filename, index, i)
			openBlocks[index] = true

		case "content_block_stop":
			index := int(event.Data["index"].(float64))
			assert.True(t, openBlocks[index],
				"%s: content block %d stopped before start at event %d", filename, index, i)
			openBlocks[index] = false

		case "content_block_delta":
			index := int(event.Data["index"].(float64))
			assert.True(t, openBlocks[index],
				"%s: delta for closed block %d at event %d", filename, index, i)
		}
	}

	// All blocks should be closed
	for index, isOpen := range openBlocks {
		assert.False(t, isOpen, "%s: content block %d never closed", filename, index)
	}
}

// validateContentBlocks validates content block structure
func validateContentBlocks(t *testing.T, events []AnthropicStreamEvent, filename string) {
	for _, event := range events {
		if event.Event == "content_block_start" {
			contentBlock, ok := event.Data["content_block"].(map[string]interface{})
			assert.True(t, ok, "%s: content_block_start should have content_block", filename)

			blockType, ok := contentBlock["type"].(string)
			assert.True(t, ok, "%s: content_block should have type", filename)

			// Validate based on type
			switch blockType {
			case "text":
				_, hasText := contentBlock["text"]
				assert.True(t, hasText, "%s: text block should have text field", filename)

			case "tool_use":
				assert.NotEmpty(t, contentBlock["id"], "%s: tool_use should have id", filename)
				assert.NotEmpty(t, contentBlock["name"], "%s: tool_use should have name", filename)
				_, hasInput := contentBlock["input"]
				assert.True(t, hasInput, "%s: tool_use should have input", filename)
			}
		}
	}
}

// transformAnthropicStreamToOpenAI transforms Anthropic streaming events to OpenAI format
func transformAnthropicStreamToOpenAI(events []AnthropicStreamEvent) []string {
	var openAIEvents []string

	for _, event := range events {
		switch event.Event {
		case "message_start":
			// OpenAI: data: {"id": "...", "object": "chat.completion.chunk", ...}
			if message, ok := event.Data["message"].(map[string]interface{}); ok {
				chunk := map[string]interface{}{
					"id":      message["id"],
					"object":  "chat.completion.chunk",
					"created": event.Data["created_at"],
					"model":   "anthropic-stream",
					"choices": []map[string]interface{}{
						{
							"index": 0,
							"delta": map[string]interface{}{
								"role": "assistant",
							},
							"finish_reason": nil,
						},
					},
				}
				chunkJSON, _ := json.Marshal(chunk)
				openAIEvents = append(openAIEvents, "data: "+string(chunkJSON))
			}

		case "content_block_delta":
			// OpenAI: data: {"choices": [{"delta": {"content": "text"}}]}
			if delta, ok := event.Data["delta"].(map[string]interface{}); ok {
				if textDelta, ok := delta["text"].(string); ok {
					chunk := map[string]interface{}{
						"choices": []map[string]interface{}{
							{
								"index": 0,
								"delta": map[string]interface{}{
									"content": textDelta,
								},
								"finish_reason": nil,
							},
						},
					}
					chunkJSON, _ := json.Marshal(chunk)
					openAIEvents = append(openAIEvents, "data: "+string(chunkJSON))
				}
			}

		case "message_stop":
			// OpenAI: data: [DONE]
			openAIEvents = append(openAIEvents, "data: [DONE]")
		}
	}

	return openAIEvents
}
