package proxy_test

import (
	"encoding/json"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/efortin/vllm-chill/pkg/parser"
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

var _ = Describe("XMLToolParser", func() {
	var testCases []TestCase

	BeforeEach(func() {
		data, err := os.ReadFile("../../test/data/tool-calls.json")
		Expect(err).NotTo(HaveOccurred(), "Failed to read test data file")

		err = json.Unmarshal(data, &testCases)
		Expect(err).NotTo(HaveOccurred(), "Failed to parse test data JSON")
	})

	Describe("All test cases from JSON", func() {
		It("should process all test cases correctly", func() {
			for _, tc := range testCases {
				By(tc.ID + "_" + tc.Description)

				// Parse the XML
				toolCalls := parser.ParseXMLToolCalls(tc.ModelOutputXML)

				// Verify we got the expected number of tool calls
				Expect(toolCalls).To(HaveLen(len(tc.ExpectedTools)),
					"Expected %d tool calls but got %d", len(tc.ExpectedTools), len(toolCalls))

				// Verify each tool call
				for i, expected := range tc.ExpectedTools {
					actual := toolCalls[i]

					// Verify tool name
					Expect(actual.Function.Name).To(Equal(expected.Name),
						"Tool call %d: name mismatch", i)

					// Parse actual arguments JSON
					var actualArgs map[string]interface{}
					err := json.Unmarshal([]byte(actual.Function.Arguments), &actualArgs)
					Expect(err).NotTo(HaveOccurred(), "Tool call %d: failed to parse arguments JSON", i)

					// Compare arguments
					Expect(actualArgs).To(Equal(expected.Arguments),
						"Tool call %d: arguments mismatch", i)

					// Verify type is set correctly
					Expect(actual.Type).To(Equal("function"),
						"Tool call %d: type should be 'function'", i)

					// Verify ID is set
					Expect(actual.ID).NotTo(BeEmpty(),
						"Tool call %d: ID should not be empty", i)
				}
			}
		})
	})

	Describe("Individual test cases", func() {
		Context("basic well-formed tool call", func() {
			It("should parse correctly", func() {
				xml := `<tool_call>
  <tool_name>web_search</tool_name>
  <tool_arguments>{"query":"keda http addon timeout","top_k":5}</tool_arguments>
</tool_call>`

				toolCalls := parser.ParseXMLToolCalls(xml)

				Expect(toolCalls).To(HaveLen(1))
				Expect(toolCalls[0].Function.Name).To(Equal("web_search"))

				var args map[string]interface{}
				err := json.Unmarshal([]byte(toolCalls[0].Function.Arguments), &args)
				Expect(err).NotTo(HaveOccurred())
				Expect(args["query"]).To(Equal("keda http addon timeout"))
				Expect(args["top_k"]).To(Equal(float64(5)))
			})
		})

		Context("CDATA wrapped arguments", func() {
			It("should parse correctly", func() {
				xml := `<tool_call>
  <tool_name>get_metrics</tool_name>
  <tool_arguments><![CDATA[{"path":"/metrics","filter":"vllm:num_requests_running|vllm:engine_state&debug"}]]></tool_arguments>
</tool_call>`

				toolCalls := parser.ParseXMLToolCalls(xml)

				Expect(toolCalls).To(HaveLen(1))
				Expect(toolCalls[0].Function.Name).To(Equal("get_metrics"))

				var args map[string]interface{}
				err := json.Unmarshal([]byte(toolCalls[0].Function.Arguments), &args)
				Expect(err).NotTo(HaveOccurred())
				Expect(args["path"]).To(Equal("/metrics"))
				Expect(args["filter"]).To(Equal("vllm:num_requests_running|vllm:engine_state&debug"))
			})
		})

		Context("multiple tool calls", func() {
			It("should parse all calls correctly", func() {
				xml := `<tool_call>
  <tool_name>k8s.scale</tool_name>
  <tool_arguments>{"namespace":"ai-apps","deployment":"vllm","replicas":0}</tool_arguments>
</tool_call>
<tool_call>
  <tool_name>notify</tool_name>
  <tool_arguments>{"channel":"ops","message":"vLLM scaled to 0 after 5m idle"}</tool_arguments>
</tool_call>`

				toolCalls := parser.ParseXMLToolCalls(xml)

				Expect(toolCalls).To(HaveLen(2))
				Expect(toolCalls[0].Function.Name).To(Equal("k8s.scale"))
				Expect(toolCalls[1].Function.Name).To(Equal("notify"))
			})
		})

		Context("function_call variant", func() {
			It("should parse <function_call> tags", func() {
				xml := `<function_call>
  <name>http.get</name>
  <arguments>{"url":"http://localhost:8000/is_sleeping","timeout":2}</arguments>
</function_call>`

				toolCalls := parser.ParseXMLToolCalls(xml)

				Expect(toolCalls).To(HaveLen(1))
				Expect(toolCalls[0].Function.Name).To(Equal("http.get"))
			})
		})

		Context("truncated output", func() {
			It("should handle missing closing tags", func() {
				xml := `<tool_call>
  <tool_name>k8s.annotate</tool_name>
  <tool_arguments>{"namespace":"ai-apps","deployment":"vllm","annotation":"last-activity","value":"1730515200"}
<!-- missing </tool_call> -->`

				toolCalls := parser.ParseXMLToolCalls(xml)

				Expect(toolCalls).To(HaveLen(1))
				Expect(toolCalls[0].Function.Name).To(Equal("k8s.annotate"))
			})
		})

		Context("escaped XML entities", func() {
			It("should handle &amp; and other entities", func() {
				xml := `<tool_call>
  <tool_name>log.search</tool_name>
  <tool_arguments>{"pattern":"WARNING.*api_server.py:966","since":"-15m","where":"pod:vllm &amp; ns:ai-apps"}</tool_arguments>
</tool_call>`

				toolCalls := parser.ParseXMLToolCalls(xml)

				Expect(toolCalls).To(HaveLen(1))

				var args map[string]interface{}
				err := json.Unmarshal([]byte(toolCalls[0].Function.Arguments), &args)
				Expect(err).NotTo(HaveOccurred())
				Expect(args["where"]).To(Equal("pod:vllm & ns:ai-apps"))
			})
		})

		Context("nested XML arguments", func() {
			It("should convert nested XML to JSON", func() {
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

				toolCalls := parser.ParseXMLToolCalls(xml)

				Expect(toolCalls).To(HaveLen(1))
				Expect(toolCalls[0].Function.Name).To(Equal("k8s.scale"))

				var args map[string]interface{}
				err := json.Unmarshal([]byte(toolCalls[0].Function.Arguments), &args)
				Expect(err).NotTo(HaveOccurred())
				Expect(args["namespace"]).To(Equal("ai-apps"))
				Expect(args["deployment"]).To(Equal("vllm"))
				Expect(args["replicas"]).To(Equal(float64(1)))
			})
		})

		Context("whitespace in tags", func() {
			It("should trim whitespace from tag content", func() {
				xml := `<tool_call>
  <tool_name>
    http.post
  </tool_name>
  <tool_arguments>{"url":"https://vllm.sir-alfred.io/sleep","body":{"level":2}}</tool_arguments>
</tool_call>`

				toolCalls := parser.ParseXMLToolCalls(xml)

				Expect(toolCalls).To(HaveLen(1))
				Expect(toolCalls[0].Function.Name).To(Equal("http.post"))
			})
		})

		Context("surrounding noise", func() {
			It("should extract tool calls from mixed content", func() {
				xml := `<think>I'll check activity then scale down…</think>
Okay, scaling to zero now:
<tool_call>
  <tool_name>k8s.scale</tool_name>
  <tool_arguments>{"namespace":"ai-apps","deployment":"vllm","replicas":0}</tool_arguments>
</tool_call>
Done.`

				toolCalls := parser.ParseXMLToolCalls(xml)

				Expect(toolCalls).To(HaveLen(1))
				Expect(toolCalls[0].Function.Name).To(Equal("k8s.scale"))
			})
		})

		Context("streaming fragments", func() {
			It("should merge fragments with part attribute", func() {
				xml := `<tool_call part="1/2">
  <tool_name>log.search</tool_name>
  <tool_arguments>{"pattern":"vllm:num_requests_running","since":"-5m",</tool_arguments>
</tool_call>
<tool_call part="2/2">
  <tool_arguments>"where":"pod:vllm"}</tool_arguments>
</tool_call>`

				toolCalls := parser.ParseXMLToolCalls(xml)

				Expect(toolCalls).To(HaveLen(1))
				Expect(toolCalls[0].Function.Name).To(Equal("log.search"))

				var args map[string]interface{}
				err := json.Unmarshal([]byte(toolCalls[0].Function.Arguments), &args)
				Expect(err).NotTo(HaveOccurred())
				Expect(args["pattern"]).To(Equal("vllm:num_requests_running"))
				Expect(args["where"]).To(Equal("pod:vllm"))
			})
		})

		Context("wrong tag names", func() {
			It("should handle <arguments> instead of <tool_arguments>", func() {
				xml := `<tool_call>
  <tool_name>notify</tool_name>
  <arguments>{"channel":"ops","message":"wake in progress"}</arguments>
</tool_call>`

				toolCalls := parser.ParseXMLToolCalls(xml)

				Expect(toolCalls).To(HaveLen(1))
				Expect(toolCalls[0].Function.Name).To(Equal("notify"))
			})
		})

		Context("XML namespaces", func() {
			It("should handle namespace prefixes", func() {
				xml := `<qwen:tool_call xmlns:qwen="http://qwen.ai/schema">
  <qwen:tool_name>k8s.annotate</qwen:tool_name>
  <qwen:tool_arguments>{"namespace":"ai-apps","deployment":"vllm","annotation":"last-activity","value":"1730515200"}</qwen:tool_arguments>
</qwen:tool_call>`

				toolCalls := parser.ParseXMLToolCalls(xml)

				Expect(toolCalls).To(HaveLen(1))
				Expect(toolCalls[0].Function.Name).To(Equal("k8s.annotate"))
			})
		})

		Context("complex tool names", func() {
			It("should handle special characters in tool names", func() {
				xml := `<tool_call>
  <tool_name>k8s.rbac/patch-role</tool_name>
  <tool_arguments>{"namespace":"ai-apps","name":"vllm-scaler","rules":[{"apiGroups":["apps"],"resources":["deployments/scale"],"verbs":["patch","update"]}]}</tool_arguments>
</tool_call>`

				toolCalls := parser.ParseXMLToolCalls(xml)

				Expect(toolCalls).To(HaveLen(1))
				Expect(toolCalls[0].Function.Name).To(Equal("k8s.rbac/patch-role"))
			})
		})

		Context("markdown code blocks", func() {
			It("should extract XML from markdown code blocks", func() {
				xml := `Here is the call:
` + "```xml" + `
<tool_call>
  <tool_name>http.get</tool_name>
  <tool_arguments>{"url":"http://vllm-svc/metrics","timeout":2}</tool_arguments>
</tool_call>
` + "```"

				toolCalls := parser.ParseXMLToolCalls(xml)

				Expect(toolCalls).To(HaveLen(1))
				Expect(toolCalls[0].Function.Name).To(Equal("http.get"))
			})
		})

		Context("CDATA wrapper", func() {
			It("should handle CDATA wrapping entire tool_call", func() {
				xml := `<![CDATA[
<tool_call>
  <tool_name>k8s.scale</tool_name>
  <tool_arguments>{"namespace":"ai-apps","deployment":"vllm","replicas":1}</tool_arguments>
</tool_call>
]]>`

				toolCalls := parser.ParseXMLToolCalls(xml)

				Expect(toolCalls).To(HaveLen(1))
				Expect(toolCalls[0].Function.Name).To(Equal("k8s.scale"))
			})
		})

		Context("empty arguments", func() {
			It("should handle empty tool_arguments", func() {
				xml := `<tool_call>
  <tool_name>server_info</tool_name>
  <tool_arguments></tool_arguments>
</tool_call>`

				toolCalls := parser.ParseXMLToolCalls(xml)

				Expect(toolCalls).To(HaveLen(1))
				Expect(toolCalls[0].Function.Name).To(Equal("server_info"))

				var args map[string]interface{}
				err := json.Unmarshal([]byte(toolCalls[0].Function.Arguments), &args)
				Expect(err).NotTo(HaveOccurred())
				Expect(args).To(BeEmpty())
			})
		})

		Context("BOM and unicode whitespace", func() {
			It("should handle BOM and unicode whitespace", func() {
				xml := "\uFEFF \t \n<tool_call>\n  <tool_name>k8s.get</tool_name>\n  <tool_arguments>{\"kind\":\"Deployment\",\"namespace\":\"ai-apps\",\"name\":\"vllm\"}</tool_arguments>\n</tool_call>"

				toolCalls := parser.ParseXMLToolCalls(xml)

				Expect(toolCalls).To(HaveLen(1))
				Expect(toolCalls[0].Function.Name).To(Equal("k8s.get"))
			})
		})

		Context("escaped quotes", func() {
			It("should handle escaped quotes in JSON", func() {
				xml := `<tool_call>
  <tool_name>notify</tool_name>
  <tool_arguments>{"channel":"ops","message":"Scaled to 0 after \"idle\" window"}</tool_arguments>
</tool_call>`

				toolCalls := parser.ParseXMLToolCalls(xml)

				Expect(toolCalls).To(HaveLen(1))

				var args map[string]interface{}
				err := json.Unmarshal([]byte(toolCalls[0].Function.Arguments), &args)
				Expect(err).NotTo(HaveOccurred())
				Expect(args["message"]).To(Equal(`Scaled to 0 after "idle" window`))
			})
		})

		Context("complex JSON", func() {
			It("should handle complex nested JSON", func() {
				xml := `<tool_call>
  <tool_name>http.post</tool_name>
  <tool_arguments>{"url":"https://vllm.sir-alfred.io/sleep?level=2","headers":{"Authorization":"Bearer token-abc123"},"body":{}}</tool_arguments>
</tool_call>`

				toolCalls := parser.ParseXMLToolCalls(xml)

				Expect(toolCalls).To(HaveLen(1))

				var args map[string]interface{}
				err := json.Unmarshal([]byte(toolCalls[0].Function.Arguments), &args)
				Expect(err).NotTo(HaveOccurred())
				Expect(args["url"]).To(Equal("https://vllm.sir-alfred.io/sleep?level=2"))
				headers := args["headers"].(map[string]interface{})
				Expect(headers["Authorization"]).To(Equal("Bearer token-abc123"))
			})
		})

		Context("wrapper element", func() {
			It("should extract tool calls from wrapper elements", func() {
				xml := `<response>
  <meta>ignored</meta>
  <tool_call>
    <tool_name>k8s.annotate</tool_name>
    <tool_arguments>{"namespace":"ai-apps","deployment":"vllm","annotation":"last-activity","value":"now"}</tool_arguments>
  </tool_call>
</response>`

				toolCalls := parser.ParseXMLToolCalls(xml)

				Expect(toolCalls).To(HaveLen(1))
				Expect(toolCalls[0].Function.Name).To(Equal("k8s.annotate"))
			})
		})
	})
})
