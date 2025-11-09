package parser

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCase represents a test case from the JSON file
type TestCase struct {
	ID             string         `json:"id"`
	Description    string         `json:"description"`
	ModelOutputXML string         `json:"model_output_xml"`
	ExpectedTools  []ExpectedTool `json:"expected_tools"`
}

// ExpectedTool represents the expected tool call structure
type ExpectedTool struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

func loadTestCases(t *testing.T) []TestCase {
	data, err := os.ReadFile("../../test/data/tool-calls.json")
	require.NoError(t, err, "Failed to read test data file")

	var testCases []TestCase
	err = json.Unmarshal(data, &testCases)
	require.NoError(t, err, "Failed to parse test data JSON")

	return testCases
}

func TestXMLToolParser_AllTestCases(t *testing.T) {
	testCases := loadTestCases(t)
	parser := NewXMLToolParser(false) // Disable debug logging for tests

	for _, tc := range testCases {
		t.Run(tc.ID+"_"+tc.Description, func(t *testing.T) {
			// Parse the XML
			toolCalls := parser.ParseXMLToolCalls(tc.ModelOutputXML)

			// Verify we got the expected number of tool calls
			require.Len(t, toolCalls, len(tc.ExpectedTools),
				"Expected %d tool calls but got %d", len(tc.ExpectedTools), len(toolCalls))

			// Verify each tool call
			for i, expected := range tc.ExpectedTools {
				actual := toolCalls[i]

				// Verify tool name
				assert.Equal(t, expected.Name, actual.Function.Name,
					"Tool call %d: name mismatch", i)

				// Parse actual arguments JSON
				var actualArgs map[string]interface{}
				err := json.Unmarshal([]byte(actual.Function.Arguments), &actualArgs)
				require.NoError(t, err, "Tool call %d: failed to parse arguments JSON", i)

				// Compare arguments
				assert.Equal(t, expected.Arguments, actualArgs,
					"Tool call %d: arguments mismatch", i)

				// Verify type is set correctly
				assert.Equal(t, "function", actual.Type,
					"Tool call %d: type should be 'function'", i)

				// Verify ID is set
				assert.NotEmpty(t, actual.ID,
					"Tool call %d: ID should not be empty", i)
			}
		})
	}
}

// TestXMLToolParser_EdgeCases tests additional edge cases with "<" characters
func TestXMLToolParser_EdgeCases(t *testing.T) {
	parser := NewXMLToolParser(false)

	t.Run("less_than_in_arguments", func(t *testing.T) {
		xml := `<tool_call>
  <tool_name>check_condition</tool_name>
  <tool_arguments>{"expression":"if a < b then true","threshold":5}</tool_arguments>
</tool_call>`

		toolCalls := parser.ParseXMLToolCalls(xml)
		require.Len(t, toolCalls, 1)
		assert.Equal(t, "check_condition", toolCalls[0].Function.Name)

		var args map[string]interface{}
		err := json.Unmarshal([]byte(toolCalls[0].Function.Arguments), &args)
		require.NoError(t, err)
		assert.Equal(t, "if a < b then true", args["expression"])
	})

	t.Run("mixed_content_with_comparison", func(t *testing.T) {
		content := `Let me check if x < 10:
<tool_call>
  <tool_name>evaluate</tool_name>
  <tool_arguments>{"condition":"x < 10","x":5}</tool_arguments>
</tool_call>
The result shows x < 10 is true.`

		toolCalls := parser.ParseXMLToolCalls(content)
		require.Len(t, toolCalls, 1)
		assert.Equal(t, "evaluate", toolCalls[0].Function.Name)

		var args map[string]interface{}
		err := json.Unmarshal([]byte(toolCalls[0].Function.Arguments), &args)
		require.NoError(t, err)
		assert.Equal(t, "x < 10", args["condition"])
	})

	t.Run("incomplete_tag_with_less_than", func(t *testing.T) {
		xml := `<tool_call>
  <tool_name>math_op</tool_name>
  <tool_arguments>{"op":"&lt;","left":3,"right":5}</tool_arguments>
</tool_call>`

		toolCalls := parser.ParseXMLToolCalls(xml)
		require.Len(t, toolCalls, 1)
		assert.Equal(t, "math_op", toolCalls[0].Function.Name)

		var args map[string]interface{}
		err := json.Unmarshal([]byte(toolCalls[0].Function.Arguments), &args)
		require.NoError(t, err)
		assert.Equal(t, "<", args["op"])
	})

	t.Run("html_entities_mixed", func(t *testing.T) {
		xml := `<tool_call>
  <tool_name>compare</tool_name>
  <tool_arguments>{"test":"a &lt; b &amp;&amp; c > d","value":10}</tool_arguments>
</tool_call>`

		toolCalls := parser.ParseXMLToolCalls(xml)
		require.Len(t, toolCalls, 1)
		assert.Equal(t, "compare", toolCalls[0].Function.Name)

		var args map[string]interface{}
		err := json.Unmarshal([]byte(toolCalls[0].Function.Arguments), &args)
		require.NoError(t, err)
		assert.Equal(t, "a < b && c > d", args["test"])
	})

	t.Run("malformed_json_with_trailing_comma", func(t *testing.T) {
		xml := `<tool_call>
  <tool_name>process</tool_name>
  <tool_arguments>{"key1":"value1","key2":"value2",}</tool_arguments>
</tool_call>`

		toolCalls := parser.ParseXMLToolCalls(xml)
		require.Len(t, toolCalls, 1)
		assert.Equal(t, "process", toolCalls[0].Function.Name)

		// Should fix the trailing comma
		var args map[string]interface{}
		err := json.Unmarshal([]byte(toolCalls[0].Function.Arguments), &args)
		require.NoError(t, err)
		assert.Equal(t, "value1", args["key1"])
		assert.Equal(t, "value2", args["key2"])
	})

	t.Run("nested_xml_with_types", func(t *testing.T) {
		xml := `<tool_call>
  <tool_name>config</tool_name>
  <tool_arguments>
    <args>
      <enabled type="bool">true</enabled>
      <count type="int">42</count>
      <ratio type="float">3.14</ratio>
      <name>test</name>
    </args>
  </tool_arguments>
</tool_call>`

		toolCalls := parser.ParseXMLToolCalls(xml)
		require.Len(t, toolCalls, 1)
		assert.Equal(t, "config", toolCalls[0].Function.Name)

		var args map[string]interface{}
		err := json.Unmarshal([]byte(toolCalls[0].Function.Arguments), &args)
		require.NoError(t, err)
		assert.Equal(t, true, args["enabled"])
		assert.Equal(t, float64(42), args["count"])
		assert.Equal(t, float64(3.14), args["ratio"])
		assert.Equal(t, "test", args["name"])
	})

	t.Run("multiple_less_than_symbols", func(t *testing.T) {
		content := `Check: a < b < c < d
<tool_call>
  <tool_name>chain_compare</tool_name>
  <tool_arguments>{"expression":"a < b < c < d","values":[1,2,3,4]}</tool_arguments>
</tool_call>
Result: a < b < c < d is true`

		toolCalls := parser.ParseXMLToolCalls(content)
		require.Len(t, toolCalls, 1)
		assert.Equal(t, "chain_compare", toolCalls[0].Function.Name)

		var args map[string]interface{}
		err := json.Unmarshal([]byte(toolCalls[0].Function.Arguments), &args)
		require.NoError(t, err)
		assert.Equal(t, "a < b < c < d", args["expression"])
	})

	t.Run("incomplete_parameter_closing_tag", func(t *testing.T) {
		// Test case for incomplete </parameter tag (missing >)
		content := `<function=search>
<parameter=query>test search</parameter
<parameter=count>5</parameter>
</function>`

		toolCalls := parser.ParseXMLToolCalls(content)
		require.Len(t, toolCalls, 1)
		assert.Equal(t, "search", toolCalls[0].Function.Name)

		args := toolCalls[0].Function.Arguments
		require.NotNil(t, args)

		// Parse the arguments JSON
		var parsedArgs map[string]interface{}
		err := json.Unmarshal([]byte(args), &parsedArgs)
		require.NoError(t, err)

		// Verify both parameters are correctly extracted
		assert.Equal(t, "test search", parsedArgs["query"])
		assert.Equal(t, "5", parsedArgs["count"])
	})

	t.Run("no_tool_calls_with_less_than", func(t *testing.T) {
		content := `This is just text with a < b comparison and no tool calls`

		toolCalls := parser.ParseXMLToolCalls(content)
		assert.Empty(t, toolCalls)
	})
}
