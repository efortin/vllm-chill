package proxy

import (
	"github.com/efortin/vllm-chill/pkg/parser"
)

// ToolCall is re-exported from parser package for backward compatibility
type ToolCall = parser.ToolCall

// ToolCallFunction is re-exported from parser package for backward compatibility
type ToolCallFunction = parser.ToolCallFunction

// parseXMLToolCalls parses XML tool calls from content
// This is a wrapper function that delegates to the parser package
// Supports multiple formats:
// - <tool_call><tool_name>...</tool_name><tool_arguments>...</tool_arguments></tool_call>
// - <function_call><name>...</name><arguments>...</arguments></function_call>
// - <function=name> <parameter=key> value </tool_call> (legacy format)
func parseXMLToolCalls(content string) []ToolCall {
	return parser.ParseXMLToolCalls(content)
}
