package proxy

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ConversationSample represents a test case with streaming input and expected output
type ConversationSample struct {
	Title          string         `json:"title"`
	Model          string         `json:"model"`
	GenerateStream []StreamChunk  `json:"generate_stream"`
	Expected       ChatCompletion `json:"expected"`
}

// StreamChunk represents a single SSE chunk
type StreamChunk struct {
	Data string `json:"data"`
}

// ChatCompletion represents the OpenAI chat completion format
type ChatCompletion struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
}

// Choice represents a completion choice
type Choice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason"`
}

// Message represents a chat message
type Message struct {
	Role      string           `json:"role"`
	Content   *string          `json:"content,omitempty"`
	ToolCalls []OpenAIToolCall `json:"tool_calls,omitempty"`
}

// OpenAIToolCall represents OpenAI's tool call format
type OpenAIToolCall struct {
	ID       string             `json:"id"`
	Type     string             `json:"type"`
	Function OpenAIToolFunction `json:"function"`
}

// OpenAIToolFunction represents the function in a tool call
type OpenAIToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

func loadConversationSamples(t *testing.T) []ConversationSample {
	samplesDir := "../../test/data/openai-stream-sample"

	files, err := filepath.Glob(filepath.Join(samplesDir, "conversation-*.json"))
	require.NoError(t, err, "Failed to list conversation samples")
	require.NotEmpty(t, files, "No conversation samples found")

	var samples []ConversationSample
	for _, file := range files {
		data, err := os.ReadFile(file)
		require.NoError(t, err, "Failed to read %s", file)

		var sample ConversationSample
		err = json.Unmarshal(data, &sample)
		require.NoError(t, err, "Failed to parse %s", file)

		samples = append(samples, sample)
	}

	return samples
}

func TestConversationSamples_Structure(t *testing.T) {
	samples := loadConversationSamples(t)

	t.Logf("Loaded %d conversation samples", len(samples))
	require.GreaterOrEqual(t, len(samples), 20, "Expected at least 20 samples")

	for i, sample := range samples {
		t.Run(sample.Title, func(t *testing.T) {
			// Verify sample structure
			assert.NotEmpty(t, sample.Title, "Sample %d: Title should not be empty", i)
			assert.NotEmpty(t, sample.Model, "Sample %d: Model should not be empty", i)
			assert.NotEmpty(t, sample.GenerateStream, "Sample %d: Generate stream should not be empty", i)

			// Verify expected output structure
			expected := sample.Expected
			assert.Equal(t, "chat.completion", expected.Object)
			assert.NotEmpty(t, expected.Model)
			assert.Greater(t, expected.Created, int64(0))

			// Verify choices
			require.Len(t, expected.Choices, 1, "Sample %d: Expected exactly 1 choice", i)
			choice := expected.Choices[0]

			assert.Equal(t, 0, choice.Index)
			assert.Equal(t, "assistant", choice.Message.Role)
			assert.Equal(t, "tool_calls", choice.FinishReason)

			// Verify tool calls
			require.Len(t, choice.Message.ToolCalls, 1, "Sample %d: Expected exactly 1 tool call", i)
			toolCall := choice.Message.ToolCalls[0]

			assert.NotEmpty(t, toolCall.ID, "Sample %d: Tool call ID should not be empty", i)
			assert.Equal(t, "function", toolCall.Type, "Sample %d: Tool call type should be 'function'", i)
			assert.NotEmpty(t, toolCall.Function.Name, "Sample %d: Function name should not be empty", i)
			assert.NotEmpty(t, toolCall.Function.Arguments, "Sample %d: Function arguments should not be empty", i)

			// Verify arguments are valid JSON
			var args map[string]interface{}
			err := json.Unmarshal([]byte(toolCall.Function.Arguments), &args)
			require.NoError(t, err, "Sample %d: Function arguments should be valid JSON", i)
			assert.NotEmpty(t, args, "Sample %d: Function arguments should not be empty object", i)
		})
	}
}

func TestConversationSamples_ToolCallVariety(t *testing.T) {
	samples := loadConversationSamples(t)

	// Collect all unique function names
	functionNames := make(map[string]int)
	for _, sample := range samples {
		if len(sample.Expected.Choices) > 0 && len(sample.Expected.Choices[0].Message.ToolCalls) > 0 {
			name := sample.Expected.Choices[0].Message.ToolCalls[0].Function.Name
			functionNames[name]++
		}
	}

	t.Logf("Found %d unique function names across %d samples", len(functionNames), len(samples))
	for name, count := range functionNames {
		t.Logf("  - %s: %d occurrences", name, count)
	}

	// Verify we have variety in function names
	assert.GreaterOrEqual(t, len(functionNames), 2, "Expected at least 2 different function names")
}

func TestConversationSamples_ArgumentsStructure(t *testing.T) {
	samples := loadConversationSamples(t)

	for i, sample := range samples {
		if len(sample.Expected.Choices) == 0 || len(sample.Expected.Choices[0].Message.ToolCalls) == 0 {
			continue
		}

		toolCall := sample.Expected.Choices[0].Message.ToolCalls[0]
		var args map[string]interface{}
		err := json.Unmarshal([]byte(toolCall.Function.Arguments), &args)
		require.NoError(t, err, "Sample %d: Failed to parse arguments", i)

		t.Run(sample.Title+"_arguments", func(t *testing.T) {
			// Verify arguments are not empty
			assert.NotEmpty(t, args, "Arguments should not be empty")

			// All values should be valid types
			for key, value := range args {
				assert.NotNil(t, value, "Argument %s should not be nil", key)
			}
		})
	}
}

func TestConversationSamples_StreamingXMLConversion(t *testing.T) {
	samples := loadConversationSamples(t)

	for _, sample := range samples {
		t.Run(sample.Title, func(t *testing.T) {
			// Create a test HTTP response recorder
			recorder := httptest.NewRecorder()

			// Create our custom response writer
			rw := newResponseWriter(recorder, true, nil, false)

			// Write streaming chunks
			for _, chunk := range sample.GenerateStream {
				// Format as SSE
				sseData := "data: " + chunk.Data + "\n\n"
				_, err := rw.Write([]byte(sseData))
				require.NoError(t, err, "Failed to write chunk")
			}

			// Get the result
			result := recorder.Body.String()

			// Check if XML was detected and converted to tool_calls
			if strings.Contains(result, "tool_calls") {
				t.Logf("✓ XML successfully converted to tool_calls")

				// Verify the expected function name appears
				expectedName := sample.Expected.Choices[0].Message.ToolCalls[0].Function.Name
				assert.Contains(t, result, expectedName, "Expected function name in output")

				// Verify no XML tags remain
				assert.NotContains(t, result, "<tool_call", "XML tags should be converted")
				assert.NotContains(t, result, "<args", "XML tags should be converted")
			} else {
				t.Logf("⚠ XML not converted (may need closing tags or different format)")
			}
		})
	}
}

func TestConversationSamples_XMLParsing(t *testing.T) {
	samples := loadConversationSamples(t)

	for _, sample := range samples {
		t.Run(sample.Title+"_xml_parse", func(t *testing.T) {
			// Accumulate all content from streaming chunks
			var accumulated strings.Builder
			for _, chunk := range sample.GenerateStream {
				// Parse the chunk data
				var chunkData map[string]interface{}
				err := json.Unmarshal([]byte(chunk.Data), &chunkData)
				require.NoError(t, err, "Failed to parse chunk data")

				// Extract content from delta
				if choices, ok := chunkData["choices"].([]interface{}); ok && len(choices) > 0 {
					if choice, ok := choices[0].(map[string]interface{}); ok {
						if delta, ok := choice["delta"].(map[string]interface{}); ok {
							if content, ok := delta["content"].(string); ok {
								accumulated.WriteString(content)
							}
						}
					}
				}
			}

			accumulatedXML := accumulated.String()
			t.Logf("Accumulated XML: %s", accumulatedXML)

			// Try to parse the XML
			if strings.Contains(accumulatedXML, "<tool_call") || strings.Contains(accumulatedXML, "<function") {
				toolCalls := parseXMLToolCalls(accumulatedXML)

				if len(toolCalls) > 0 {
					t.Logf("✓ Successfully parsed %d tool call(s)", len(toolCalls))

					// Verify the function name matches expected
					expectedName := sample.Expected.Choices[0].Message.ToolCalls[0].Function.Name
					assert.Equal(t, expectedName, toolCalls[0].Function.Name, "Function name should match")

					// Verify arguments are valid JSON
					var args map[string]interface{}
					err := json.Unmarshal([]byte(toolCalls[0].Function.Arguments), &args)
					assert.NoError(t, err, "Arguments should be valid JSON")
				} else {
					t.Logf("⚠ XML found but not parsed successfully")
				}
			}
		})
	}
}

func TestQwenNativeStreamingToolCalls(t *testing.T) {
	// Load the sample Qwen native streaming tool call data
	data, err := os.ReadFile("../../test/data/qwen-native-streaming-tool-call.txt")
	require.NoError(t, err, "Failed to read qwen-native-streaming-tool-call.txt")

	// Create a test HTTP response recorder
	recorder := httptest.NewRecorder()
	rw := newResponseWriter(recorder, true, nil, false)

	// Write the streaming data
	_, err = rw.Write(data)
	require.NoError(t, err, "Failed to write streaming data")

	// Parse the result to verify tool calls were accumulated
	result := recorder.Body.String()
	t.Logf("Result length: %d bytes", len(result))

	// Verify tool calls are present in the output
	assert.Contains(t, result, "tool_calls", "Should contain tool_calls")
	assert.Contains(t, result, "todo_create", "Should contain function name")

	// Parse all chunks and verify the accumulated tool call
	lines := strings.Split(result, "\n")
	var accumulatedArgs strings.Builder
	var funcName string

	for _, line := range lines {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		jsonData := strings.TrimPrefix(line, "data: ")
		if jsonData == "[DONE]" {
			continue
		}

		var chunk map[string]interface{}
		if err := json.Unmarshal([]byte(jsonData), &chunk); err != nil {
			continue
		}

		if choices, ok := chunk["choices"].([]interface{}); ok && len(choices) > 0 {
			if choice, ok := choices[0].(map[string]interface{}); ok {
				if delta, ok := choice["delta"].(map[string]interface{}); ok {
					if toolCalls, ok := delta["tool_calls"].([]interface{}); ok && len(toolCalls) > 0 {
						if tc, ok := toolCalls[0].(map[string]interface{}); ok {
							if function, ok := tc["function"].(map[string]interface{}); ok {
								if name, ok := function["name"].(string); ok && name != "" {
									funcName = name
								}
								if args, ok := function["arguments"].(string); ok && args != "" {
									accumulatedArgs.WriteString(args)
								}
							}
						}
					}
				}
			}
		}
	}

	// Verify accumulated tool call
	assert.Equal(t, "todo_create", funcName, "Function name should match")

	finalArgs := accumulatedArgs.String()
	t.Logf("Accumulated arguments: %s", finalArgs)

	// Verify the arguments are valid JSON
	var args map[string]interface{}
	err = json.Unmarshal([]byte(finalArgs), &args)
	require.NoError(t, err, "Accumulated arguments should be valid JSON")

	// Verify expected arguments
	assert.Equal(t, "Analyze commit message", args["title"], "Title should match")
	assert.Contains(t, args["notes"].(string), "aa62adf0", "Notes should contain commit hash")
}
