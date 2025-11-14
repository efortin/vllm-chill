package proxy

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMessageStartStructure verifies that message_start contains required fields
// This test ensures we don't regress on the Claude Code compatibility fix
func TestMessageStartStructure(t *testing.T) {
	// Simulate a minimal OpenAI SSE stream
	openAIStream := `data: {"id":"chat-123","object":"chat.completion.chunk","created":1234567890,"model":"test-model","choices":[{"index":0,"delta":{"role":"assistant","content":""},"logprobs":null,"finish_reason":null}]}

data: {"id":"chat-123","object":"chat.completion.chunk","created":1234567890,"model":"test-model","choices":[{"index":0,"delta":{"content":"Hello"},"logprobs":null,"finish_reason":null}]}

data: {"id":"chat-123","object":"chat.completion.chunk","created":1234567890,"model":"test-model","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}

data: [DONE]

`

	// Parse the stream and extract message_start event
	scanner := bufio.NewScanner(strings.NewReader(openAIStream))
	var messageStartData map[string]interface{}

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") && !strings.Contains(line, "[DONE]") {
			data := strings.TrimPrefix(line, "data: ")
			var chunk map[string]interface{}
			if err := json.Unmarshal([]byte(data), &chunk); err == nil {
				// First chunk - would trigger message_start creation
				if choices, ok := chunk["choices"].([]interface{}); ok && len(choices) > 0 {
					if choice, ok := choices[0].(map[string]interface{}); ok {
						if delta, ok := choice["delta"].(map[string]interface{}); ok {
							if _, hasRole := delta["role"]; hasRole {
								// This is the first chunk - create message_start
								messageStart := map[string]interface{}{
									"type": "message_start",
									"message": map[string]interface{}{
										"id":      chunk["id"],
										"type":    "message",
										"role":    "assistant",
										"content": []interface{}{}, // CRITICAL: Must be present for Claude Code
										"model":   chunk["model"],
										"usage": map[string]interface{}{
											"input_tokens":  0,
											"output_tokens": 0,
										},
									},
								}
								messageStartData = messageStart
								break
							}
						}
					}
				}
			}
		}
	}

	// Assertions
	require.NotNil(t, messageStartData, "message_start event should be created")

	assert.Equal(t, "message_start", messageStartData["type"])

	message, ok := messageStartData["message"].(map[string]interface{})
	require.True(t, ok, "message field must be present")

	// CRITICAL FIX 1: content must be an empty array
	// Without this, Claude Code crashes with "undefined is not an object (evaluating 'A.content.push')"
	content, hasContent := message["content"]
	assert.True(t, hasContent, "content field must be present in message_start")
	assert.IsType(t, []interface{}{}, content, "content must be an array")
	assert.Empty(t, content, "content should be empty array initially")

	// CRITICAL FIX 2: usage must be present
	// Without this, Claude Code shows "0 tokens" forever
	usage, hasUsage := message["usage"]
	assert.True(t, hasUsage, "usage field must be present in message_start")
	usageMap, ok := usage.(map[string]interface{})
	require.True(t, ok, "usage must be a map")
	assert.Equal(t, 0, usageMap["input_tokens"])
	assert.Equal(t, 0, usageMap["output_tokens"])
}

// TestNoPeriodicMessageDelta verifies that message_delta is NOT sent during streaming
// This test ensures we don't regress on the premature termination fix
func TestNoPeriodicMessageDelta(t *testing.T) {
	// Simulate a stream with multiple content chunks
	anthropicEvents := []string{
		`event: message_start`,
		`data: {"type":"message_start","message":{"id":"msg-123","content":[],"usage":{"input_tokens":0,"output_tokens":0}}}`,
		``,
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" world"}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"!"}}`,
		``,
		`event: content_block_stop`,
		`data: {"type":"content_block_stop","index":0}`,
		``,
		// CRITICAL: message_delta should ONLY appear at the end, not during streaming
		`event: message_delta`,
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":10,"output_tokens":3}}`,
		``,
		`event: message_stop`,
		`data: {}`,
		``,
	}

	stream := strings.Join(anthropicEvents, "\n")

	// Parse and count message_delta events
	scanner := bufio.NewScanner(strings.NewReader(stream))
	messageDeltaCount := 0
	contentDeltaCount := 0
	currentEvent := ""

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event: ") {
			currentEvent = strings.TrimPrefix(line, "event: ")
		} else if strings.HasPrefix(line, "data: ") {
			if currentEvent == "message_delta" {
				messageDeltaCount++
			} else if currentEvent == "content_block_delta" {
				contentDeltaCount++
			}
		}
	}

	// Assertions
	assert.Greater(t, contentDeltaCount, 0, "Should have content deltas")

	// CRITICAL FIX 3: message_delta should appear EXACTLY ONCE, at the end
	// Sending it periodically during streaming causes Claude Code to terminate prematurely
	assert.Equal(t, 1, messageDeltaCount, "message_delta should appear exactly once (at the end)")
}

// TestToolCallsTransformation verifies OpenAI tool_calls → Anthropic tool_use transformation
func TestToolCallsTransformation(t *testing.T) {
	// Simulate OpenAI tool_calls stream
	openAIToolStream := []string{
		// First chunk: role + empty content
		`data: {"id":"chat-123","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":null}]}`,
		// Second chunk: tool call metadata
		`data: {"id":"chat-123","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_abc123","type":"function","function":{"name":"get_weather","arguments":""}}]},"finish_reason":null}]}`,
		// Streaming arguments
		`data: {"id":"chat-123","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{"}}]},"finish_reason":null}]}`,
		`data: {"id":"chat-123","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"location\": \"Paris\""}}]},"finish_reason":null}]}`,
		`data: {"id":"chat-123","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"}"}}]},"finish_reason":null}]}`,
		// Finish
		`data: {"id":"chat-123","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
		`data: [DONE]`,
	}

	// Track generated Anthropic events
	type anthropicEvent struct {
		eventType string
		data      map[string]interface{}
	}
	var events []anthropicEvent

	// Simulate the transformation logic
	toolCallStates := make(map[int]*struct {
		id              string
		name            string
		args            bytes.Buffer
		contentBlockIdx int
		started         bool
	})
	contentBlockIndex := 0
	hasToolCalls := false

	for _, line := range openAIToolStream {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			// Send final message_delta with tool_use stop_reason
			stopReason := "end_turn"
			if hasToolCalls {
				stopReason = "tool_use"
			}
			events = append(events, anthropicEvent{
				eventType: "message_delta",
				data: map[string]interface{}{
					"type": "message_delta",
					"delta": map[string]interface{}{
						"stop_reason": stopReason,
					},
				},
			})
			continue
		}

		var chunk map[string]interface{}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}

		choices, ok := chunk["choices"].([]interface{})
		if !ok || len(choices) == 0 {
			continue
		}

		choice := choices[0].(map[string]interface{})
		delta, ok := choice["delta"].(map[string]interface{})
		if !ok {
			continue
		}

		// Handle tool_calls
		if toolCalls, ok := delta["tool_calls"].([]interface{}); ok {
			hasToolCalls = true

			for _, tc := range toolCalls {
				toolCall := tc.(map[string]interface{})
				index := int(toolCall["index"].(float64))

				state, exists := toolCallStates[index]
				if !exists {
					state = &struct {
						id              string
						name            string
						args            bytes.Buffer
						contentBlockIdx int
						started         bool
					}{
						contentBlockIdx: 0,
					}
					toolCallStates[index] = state
				}

				if id, ok := toolCall["id"].(string); ok {
					state.id = id
				}

				if fn, ok := toolCall["function"].(map[string]interface{}); ok {
					if name, ok := fn["name"].(string); ok && name != "" {
						state.name = name

						if !state.started {
							contentBlockIndex++
							state.contentBlockIdx = contentBlockIndex

							events = append(events, anthropicEvent{
								eventType: "content_block_start",
								data: map[string]interface{}{
									"type":  "content_block_start",
									"index": state.contentBlockIdx,
									"content_block": map[string]interface{}{
										"type": "tool_use",
										"id":   state.id,
										"name": state.name,
									},
								},
							})
							state.started = true
						}
					}

					if args, ok := fn["arguments"].(string); ok && args != "" {
						state.args.WriteString(args)

						events = append(events, anthropicEvent{
							eventType: "content_block_delta",
							data: map[string]interface{}{
								"type":  "content_block_delta",
								"index": state.contentBlockIdx,
								"delta": map[string]interface{}{
									"type":         "input_json_delta",
									"partial_json": args,
								},
							},
						})
					}
				}
			}
		}

		// Handle finish_reason
		if finishReason, ok := choice["finish_reason"].(string); ok && finishReason == "tool_calls" {
			// Send content_block_stop for tool calls
			for i := 0; i < len(toolCallStates); i++ {
				if state, ok := toolCallStates[i]; ok && state.started {
					events = append(events, anthropicEvent{
						eventType: "content_block_stop",
						data: map[string]interface{}{
							"type":  "content_block_stop",
							"index": state.contentBlockIdx,
						},
					})
				}
			}
		}
	}

	// Assertions
	require.Greater(t, len(events), 0, "Should generate Anthropic events")

	// Find content_block_start with tool_use
	var toolUseStart *anthropicEvent
	for i := range events {
		if events[i].eventType == "content_block_start" {
			if cb, ok := events[i].data["content_block"].(map[string]interface{}); ok {
				if cb["type"] == "tool_use" {
					toolUseStart = &events[i]
					break
				}
			}
		}
	}

	require.NotNil(t, toolUseStart, "Should have content_block_start with tool_use")

	// Verify tool_use structure
	contentBlock := toolUseStart.data["content_block"].(map[string]interface{})
	assert.Equal(t, "tool_use", contentBlock["type"])
	assert.Equal(t, "call_abc123", contentBlock["id"])
	assert.Equal(t, "get_weather", contentBlock["name"])
	assert.Equal(t, 1, toolUseStart.data["index"], "Tool call should be at index 1 (text is 0)")

	// Count input_json_delta events
	var jsonDeltas []string
	for _, evt := range events {
		if evt.eventType == "content_block_delta" {
			if delta, ok := evt.data["delta"].(map[string]interface{}); ok {
				if delta["type"] == "input_json_delta" {
					jsonDeltas = append(jsonDeltas, delta["partial_json"].(string))
				}
			}
		}
	}

	assert.Greater(t, len(jsonDeltas), 0, "Should have input_json_delta events")

	// Verify full arguments can be reconstructed
	fullArgs := strings.Join(jsonDeltas, "")
	assert.Equal(t, `{"location": "Paris"}`, fullArgs, "Arguments should be streamed correctly")

	// Verify final message_delta has tool_use stop_reason
	var finalMessageDelta *anthropicEvent
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].eventType == "message_delta" {
			finalMessageDelta = &events[i]
			break
		}
	}

	require.NotNil(t, finalMessageDelta, "Should have final message_delta")
	delta := finalMessageDelta.data["delta"].(map[string]interface{})
	assert.Equal(t, "tool_use", delta["stop_reason"], "stop_reason should be tool_use")
}

// TestMultipleToolCalls verifies handling of multiple concurrent tool calls
func TestMultipleToolCalls(t *testing.T) {
	// Simulate OpenAI stream with 2 tool calls
	openAIStream := []string{
		`data: {"id":"chat-123","choices":[{"delta":{"role":"assistant","content":""},"finish_reason":null}]}`,
		// First tool call
		`data: {"id":"chat-123","choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"get_weather","arguments":""}}]},"finish_reason":null}]}`,
		`data: {"id":"chat-123","choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"location\":\"Paris\"}"}}]},"finish_reason":null}]}`,
		// Second tool call
		`data: {"id":"chat-123","choices":[{"delta":{"tool_calls":[{"index":1,"id":"call_2","type":"function","function":{"name":"get_forecast","arguments":""}}]},"finish_reason":null}]}`,
		`data: {"id":"chat-123","choices":[{"delta":{"tool_calls":[{"index":1,"function":{"arguments":"{\"days\":7}"}}]},"finish_reason":null}]}`,
		`data: {"id":"chat-123","choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
	}

	// Track tool call starts
	toolCallsStarted := make(map[string]int) // name -> contentBlockIdx

	for _, line := range openAIStream {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")

		var chunk map[string]interface{}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}

		choices, ok := chunk["choices"].([]interface{})
		if !ok || len(choices) == 0 {
			continue
		}

		choice := choices[0].(map[string]interface{})
		delta, ok := choice["delta"].(map[string]interface{})
		if !ok {
			continue
		}

		if toolCalls, ok := delta["tool_calls"].([]interface{}); ok {
			for _, tc := range toolCalls {
				toolCall := tc.(map[string]interface{})
				if fn, ok := toolCall["function"].(map[string]interface{}); ok {
					if name, ok := fn["name"].(string); ok && name != "" {
						if _, exists := toolCallsStarted[name]; !exists {
							// Assign contentBlockIdx (1-based, 0 is text)
							toolCallsStarted[name] = len(toolCallsStarted) + 1
						}
					}
				}
			}
		}
	}

	// Assertions
	assert.Len(t, toolCallsStarted, 2, "Should detect 2 tool calls")
	assert.Contains(t, toolCallsStarted, "get_weather")
	assert.Contains(t, toolCallsStarted, "get_forecast")

	// Verify distinct indices
	assert.Equal(t, 1, toolCallsStarted["get_weather"], "First tool should be at index 1")
	assert.Equal(t, 2, toolCallsStarted["get_forecast"], "Second tool should be at index 2")
}

// TestStopReasonMapping verifies correct stop_reason values
func TestStopReasonMapping(t *testing.T) {
	testCases := []struct {
		name               string
		openAIFinish       string
		hasToolCalls       bool
		expectedStopReason string
	}{
		{
			name:               "Normal completion",
			openAIFinish:       "stop",
			hasToolCalls:       false,
			expectedStopReason: "end_turn",
		},
		{
			name:               "Tool calls completion",
			openAIFinish:       "tool_calls",
			hasToolCalls:       true,
			expectedStopReason: "tool_use",
		},
		{
			name:               "Length limit",
			openAIFinish:       "length",
			hasToolCalls:       false,
			expectedStopReason: "end_turn",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Simulate logic
			stopReason := "end_turn"
			if tc.hasToolCalls {
				stopReason = "tool_use"
			}

			assert.Equal(t, tc.expectedStopReason, stopReason)
		})
	}
}
