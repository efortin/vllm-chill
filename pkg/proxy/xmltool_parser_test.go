package proxy

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

	for _, tc := range testCases {
		t.Run(tc.ID+"_"+tc.Description, func(t *testing.T) {
			// Parse the XML
			toolCalls := parseXMLToolCalls(tc.ModelOutputXML)

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

// TestXMLToolParser_BasicWellFormed tests case 01: basic well-formed tool call
func TestXMLToolParser_BasicWellFormed(t *testing.T) {
	xml := `<tool_call>
  <tool_name>web_search</tool_name>
  <tool_arguments>{"query":"keda http addon timeout","top_k":5}</tool_arguments>
</tool_call>`

	toolCalls := parseXMLToolCalls(xml)

	require.Len(t, toolCalls, 1)
	assert.Equal(t, "web_search", toolCalls[0].Function.Name)

	var args map[string]interface{}
	err := json.Unmarshal([]byte(toolCalls[0].Function.Arguments), &args)
	require.NoError(t, err)
	assert.Equal(t, "keda http addon timeout", args["query"])
	assert.Equal(t, float64(5), args["top_k"])
}

// TestXMLToolParser_CDATA tests case 02: CDATA wrapped arguments
func TestXMLToolParser_CDATA(t *testing.T) {
	xml := `<tool_call>
  <tool_name>get_metrics</tool_name>
  <tool_arguments><![CDATA[{"path":"/metrics","filter":"vllm:num_requests_running|vllm:engine_state&debug"}]]></tool_arguments>
</tool_call>`

	toolCalls := parseXMLToolCalls(xml)

	require.Len(t, toolCalls, 1)
	assert.Equal(t, "get_metrics", toolCalls[0].Function.Name)

	var args map[string]interface{}
	err := json.Unmarshal([]byte(toolCalls[0].Function.Arguments), &args)
	require.NoError(t, err)
	assert.Equal(t, "/metrics", args["path"])
	assert.Equal(t, "vllm:num_requests_running|vllm:engine_state&debug", args["filter"])
}

// TestXMLToolParser_MultipleToolCalls tests case 03: multiple tool calls
func TestXMLToolParser_MultipleToolCalls(t *testing.T) {
	xml := `<tool_call>
  <tool_name>k8s.scale</tool_name>
  <tool_arguments>{"namespace":"ai-apps","deployment":"vllm","replicas":0}</tool_arguments>
</tool_call>
<tool_call>
  <tool_name>notify</tool_name>
  <tool_arguments>{"channel":"ops","message":"vLLM scaled to 0 after 5m idle"}</tool_arguments>
</tool_call>`

	toolCalls := parseXMLToolCalls(xml)

	require.Len(t, toolCalls, 2)
	assert.Equal(t, "k8s.scale", toolCalls[0].Function.Name)
	assert.Equal(t, "notify", toolCalls[1].Function.Name)
}

// TestXMLToolParser_FunctionCallVariant tests case 04: <function_call> instead of <tool_call>
func TestXMLToolParser_FunctionCallVariant(t *testing.T) {
	xml := `<function_call>
  <name>http.get</name>
  <arguments>{"url":"http://localhost:8000/is_sleeping","timeout":2}</arguments>
</function_call>`

	toolCalls := parseXMLToolCalls(xml)

	require.Len(t, toolCalls, 1)
	assert.Equal(t, "http.get", toolCalls[0].Function.Name)
}

// TestXMLToolParser_TruncatedOutput tests case 05: missing closing tag
func TestXMLToolParser_TruncatedOutput(t *testing.T) {
	xml := `<tool_call>
  <tool_name>k8s.annotate</tool_name>
  <tool_arguments>{"namespace":"ai-apps","deployment":"vllm","annotation":"last-activity","value":"1730515200"}
<!-- missing </tool_call> -->`

	toolCalls := parseXMLToolCalls(xml)

	require.Len(t, toolCalls, 1)
	assert.Equal(t, "k8s.annotate", toolCalls[0].Function.Name)
}

// TestXMLToolParser_EscapedEntities tests case 06: XML entity escaping
func TestXMLToolParser_EscapedEntities(t *testing.T) {
	xml := `<tool_call>
  <tool_name>log.search</tool_name>
  <tool_arguments>{"pattern":"WARNING.*api_server.py:966","since":"-15m","where":"pod:vllm &amp; ns:ai-apps"}</tool_arguments>
</tool_call>`

	toolCalls := parseXMLToolCalls(xml)

	require.Len(t, toolCalls, 1)

	var args map[string]interface{}
	err := json.Unmarshal([]byte(toolCalls[0].Function.Arguments), &args)
	require.NoError(t, err)
	assert.Equal(t, "pod:vllm & ns:ai-apps", args["where"])
}

// TestXMLToolParser_NestedXMLArguments tests case 07: nested XML instead of JSON
func TestXMLToolParser_NestedXMLArguments(t *testing.T) {
	xml := `<tool_call>
  <tool_name>k8s.scale</tool_name>
  <tool_arguments>
    <args>
      <namespace>ai-apps</namespace>
      <deployment>vllm</deployment>
      <replicas type="int">1</replicas>
    </args>
  </tool_arguments>
</tool_call>`

	toolCalls := parseXMLToolCalls(xml)

	require.Len(t, toolCalls, 1)
	assert.Equal(t, "k8s.scale", toolCalls[0].Function.Name)

	var args map[string]interface{}
	err := json.Unmarshal([]byte(toolCalls[0].Function.Arguments), &args)
	require.NoError(t, err)
	assert.Equal(t, "ai-apps", args["namespace"])
	assert.Equal(t, "vllm", args["deployment"])
	assert.Equal(t, float64(1), args["replicas"])
}

// TestXMLToolParser_WhitespaceInTags tests case 08: whitespace around tag content
func TestXMLToolParser_WhitespaceInTags(t *testing.T) {
	xml := `<tool_call>
  <tool_name>
    http.post
  </tool_name>
  <tool_arguments>{"url":"https://vllm.sir-alfred.io/sleep","body":{"level":2}}</tool_arguments>
</tool_call>`

	toolCalls := parseXMLToolCalls(xml)

	require.Len(t, toolCalls, 1)
	assert.Equal(t, "http.post", toolCalls[0].Function.Name)
}

// TestXMLToolParser_SurroundingNoise tests case 09: text before/after tool call
func TestXMLToolParser_SurroundingNoise(t *testing.T) {
	xml := `<think>I'll check activity then scale down…</think>
Okay, scaling to zero now:
<tool_call>
  <tool_name>k8s.scale</tool_name>
  <tool_arguments>{"namespace":"ai-apps","deployment":"vllm","replicas":0}</tool_arguments>
</tool_call>
Done.`

	toolCalls := parseXMLToolCalls(xml)

	require.Len(t, toolCalls, 1)
	assert.Equal(t, "k8s.scale", toolCalls[0].Function.Name)
}

// TestXMLToolParser_StreamingFragments tests case 10: streaming with part attribute
func TestXMLToolParser_StreamingFragments(t *testing.T) {
	xml := `<tool_call part="1/2">
  <tool_name>log.search</tool_name>
  <tool_arguments>{"pattern":"vllm:num_requests_running","since":"-5m",</tool_arguments>
</tool_call>
<tool_call part="2/2">
  <tool_arguments>"where":"pod:vllm"}</tool_arguments>
</tool_call>`

	toolCalls := parseXMLToolCalls(xml)

	require.Len(t, toolCalls, 1)
	assert.Equal(t, "log.search", toolCalls[0].Function.Name)

	var args map[string]interface{}
	err := json.Unmarshal([]byte(toolCalls[0].Function.Arguments), &args)
	require.NoError(t, err)
	assert.Equal(t, "vllm:num_requests_running", args["pattern"])
	assert.Equal(t, "pod:vllm", args["where"])
}

// TestXMLToolParser_WrongTagName tests case 11: <arguments> instead of <tool_arguments>
func TestXMLToolParser_WrongTagName(t *testing.T) {
	xml := `<tool_call>
  <tool_name>notify</tool_name>
  <arguments>{"channel":"ops","message":"wake in progress"}</arguments>
</tool_call>`

	toolCalls := parseXMLToolCalls(xml)

	require.Len(t, toolCalls, 1)
	assert.Equal(t, "notify", toolCalls[0].Function.Name)
}

// TestXMLToolParser_NamespacePrefix tests case 12: XML namespaces
func TestXMLToolParser_NamespacePrefix(t *testing.T) {
	xml := `<qwen:tool_call xmlns:qwen="http://qwen.ai/schema">
  <qwen:tool_name>k8s.annotate</qwen:tool_name>
  <qwen:tool_arguments>{"namespace":"ai-apps","deployment":"vllm","annotation":"last-activity","value":"1730515200"}</qwen:tool_arguments>
</qwen:tool_call>`

	toolCalls := parseXMLToolCalls(xml)

	require.Len(t, toolCalls, 1)
	assert.Equal(t, "k8s.annotate", toolCalls[0].Function.Name)
}

// TestXMLToolParser_ComplexToolName tests case 13: tool name with special chars
func TestXMLToolParser_ComplexToolName(t *testing.T) {
	xml := `<tool_call>
  <tool_name>k8s.rbac/patch-role</tool_name>
  <tool_arguments>{"namespace":"ai-apps","name":"vllm-scaler","rules":[{"apiGroups":["apps"],"resources":["deployments/scale"],"verbs":["patch","update"]}]}</tool_arguments>
</tool_call>`

	toolCalls := parseXMLToolCalls(xml)

	require.Len(t, toolCalls, 1)
	assert.Equal(t, "k8s.rbac/patch-role", toolCalls[0].Function.Name)
}

// TestXMLToolParser_MarkdownCodeBlock tests case 14: XML in markdown code block
func TestXMLToolParser_MarkdownCodeBlock(t *testing.T) {
	xml := `Here is the call:
` + "```xml" + `
<tool_call>
  <tool_name>http.get</tool_name>
  <tool_arguments>{"url":"http://vllm-svc/metrics","timeout":2}</tool_arguments>
</tool_call>
` + "```"

	toolCalls := parseXMLToolCalls(xml)

	require.Len(t, toolCalls, 1)
	assert.Equal(t, "http.get", toolCalls[0].Function.Name)
}

// TestXMLToolParser_CDATAWrapper tests case 15: CDATA wrapping entire tool_call
func TestXMLToolParser_CDATAWrapper(t *testing.T) {
	xml := `<![CDATA[
<tool_call>
  <tool_name>k8s.scale</tool_name>
  <tool_arguments>{"namespace":"ai-apps","deployment":"vllm","replicas":1}</tool_arguments>
</tool_call>
]]>`

	toolCalls := parseXMLToolCalls(xml)

	require.Len(t, toolCalls, 1)
	assert.Equal(t, "k8s.scale", toolCalls[0].Function.Name)
}

// TestXMLToolParser_EmptyArguments tests case 16: empty tool_arguments
func TestXMLToolParser_EmptyArguments(t *testing.T) {
	xml := `<tool_call>
  <tool_name>server_info</tool_name>
  <tool_arguments></tool_arguments>
</tool_call>`

	toolCalls := parseXMLToolCalls(xml)

	require.Len(t, toolCalls, 1)
	assert.Equal(t, "server_info", toolCalls[0].Function.Name)

	var args map[string]interface{}
	err := json.Unmarshal([]byte(toolCalls[0].Function.Arguments), &args)
	require.NoError(t, err)
	assert.Empty(t, args)
}

// TestXMLToolParser_BOMAndUnicodeWhitespace tests case 17: BOM and unicode whitespace
func TestXMLToolParser_BOMAndUnicodeWhitespace(t *testing.T) {
	xml := "\uFEFF \t \n<tool_call>\n  <tool_name>k8s.get</tool_name>\n  <tool_arguments>{\"kind\":\"Deployment\",\"namespace\":\"ai-apps\",\"name\":\"vllm\"}</tool_arguments>\n</tool_call>"

	toolCalls := parseXMLToolCalls(xml)

	require.Len(t, toolCalls, 1)
	assert.Equal(t, "k8s.get", toolCalls[0].Function.Name)
}

// TestXMLToolParser_EscapedQuotes tests case 18: escaped quotes in JSON
func TestXMLToolParser_EscapedQuotes(t *testing.T) {
	xml := `<tool_call>
  <tool_name>notify</tool_name>
  <tool_arguments>{"channel":"ops","message":"Scaled to 0 after \"idle\" window"}</tool_arguments>
</tool_call>`

	toolCalls := parseXMLToolCalls(xml)

	require.Len(t, toolCalls, 1)

	var args map[string]interface{}
	err := json.Unmarshal([]byte(toolCalls[0].Function.Arguments), &args)
	require.NoError(t, err)
	assert.Equal(t, `Scaled to 0 after "idle" window`, args["message"])
}

// TestXMLToolParser_ComplexJSON tests case 19: complex nested JSON
func TestXMLToolParser_ComplexJSON(t *testing.T) {
	xml := `<tool_call>
  <tool_name>http.post</tool_name>
  <tool_arguments>{"url":"https://vllm.sir-alfred.io/sleep?level=2","headers":{"Authorization":"Bearer token-abc123"},"body":{}}</tool_arguments>
</tool_call>`

	toolCalls := parseXMLToolCalls(xml)

	require.Len(t, toolCalls, 1)

	var args map[string]interface{}
	err := json.Unmarshal([]byte(toolCalls[0].Function.Arguments), &args)
	require.NoError(t, err)
	assert.Equal(t, "https://vllm.sir-alfred.io/sleep?level=2", args["url"])
	headers := args["headers"].(map[string]interface{})
	assert.Equal(t, "Bearer token-abc123", headers["Authorization"])
}

// TestXMLToolParser_WrapperElement tests case 20: spurious wrapper element
func TestXMLToolParser_WrapperElement(t *testing.T) {
	xml := `<response>
  <meta>ignored</meta>
  <tool_call>
    <tool_name>k8s.annotate</tool_name>
    <tool_arguments>{"namespace":"ai-apps","deployment":"vllm","annotation":"last-activity","value":"now"}</tool_arguments>
  </tool_call>
</response>`

	toolCalls := parseXMLToolCalls(xml)

	require.Len(t, toolCalls, 1)
	assert.Equal(t, "k8s.annotate", toolCalls[0].Function.Name)
}

// TestXMLToolParser_PlainLessThanCharacter tests that plain < character doesn't trigger XML parsing
func TestXMLToolParser_PlainLessThanCharacter(t *testing.T) {
	// Simulates a response like "ls\n<" where < is just a shell prompt character, not XML
	text := `ls

<`

	toolCalls := parseXMLToolCalls(text)

	// Should not parse any tool calls from plain text with just a < character
	require.Len(t, toolCalls, 0)
}

// TestXMLToolParser_IncompleteFunctionTag tests that incomplete <function pattern doesn't trigger
func TestXMLToolParser_IncompleteFunctionTag(t *testing.T) {
	// Simulates text that contains "<function" but isn't actually a tool call
	text := `The <function should not trigger parsing`

	toolCalls := parseXMLToolCalls(text)

	// Should not parse any tool calls from incomplete patterns
	require.Len(t, toolCalls, 0)
}

// TestXMLToolParser_SimilarButNotToolCall tests patterns that look like tool calls but aren't
func TestXMLToolParser_SimilarButNotToolCall(t *testing.T) {
	testCases := []struct {
		name string
		text string
	}{
		{
			name: "tool_callable interface",
			text: `The <tool_callable interface provides methods`,
		},
		{
			name: "tool_call_name variable",
			text: `Using <tool_call_name> as a variable name`,
		},
		{
			name: "function_calling library",
			text: `Import the <function_calling library`,
		},
		{
			name: "less than comparison",
			text: `If x < tool_call then return`,
		},
		{
			name: "multiple less than",
			text: `< < <tool_call`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			toolCalls := parseXMLToolCalls(tc.text)
			// These should not parse as tool calls
			assert.Len(t, toolCalls, 0, "Text '%s' should not be detected as a tool call", tc.text)
		})
	}
}
