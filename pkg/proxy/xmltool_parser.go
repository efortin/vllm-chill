package proxy

import (
	"encoding/json"
	"log"
	"strings"
)

// ToolCall represents a parsed XML tool call
type ToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function ToolCallFunction `json:"function"`
}

// ToolCallFunction represents the function part of a tool call
type ToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// parseXMLToolCalls parses XML tool calls from content
// Supports formats:
// - <function=name> <parameter=key> value </tool_call>
// - <function=name>\n</function>\n</tool_call>
func parseXMLToolCalls(content string) []ToolCall {
	var toolCalls []ToolCall
	idx := 0
	callIndex := 0

	log.Printf("[XML-PARSER] Parsing XML tool calls from content (length: %d)", len(content))

	for {
		// Find start of function tag
		funcStart := strings.Index(content[idx:], "<function=")
		if funcStart == -1 {
			break
		}
		funcStart += idx

		// Find end of function tag
		funcEnd := strings.Index(content[funcStart:], ">")
		if funcEnd == -1 {
			break
		}
		funcEnd += funcStart

		// Extract function name
		toolName := strings.TrimSpace(content[funcStart+10 : funcEnd])

		// Find end of tool call
		toolCallEnd := strings.Index(content[funcEnd:], "</tool_call>")
		if toolCallEnd == -1 {
			break
		}
		toolCallEnd += funcEnd

		// Extract content between function tag and </tool_call>
		paramsContent := content[funcEnd+1 : toolCallEnd]

		// Remove optional </function> tag if present
		paramsContent = strings.Replace(paramsContent, "</function>", "", 1)

		// Parse parameters
		params := parseParameters(paramsContent)

		// Convert parameters to JSON
		argsJSON, _ := json.Marshal(params)

		toolCall := ToolCall{
			ID:   generateToolCallID(callIndex),
			Type: "function",
			Function: ToolCallFunction{
				Name:      toolName,
				Arguments: string(argsJSON),
			},
		}

		log.Printf("[XML-PARSER] Parsed tool call: name=%s, args=%s", toolName, string(argsJSON))
		toolCalls = append(toolCalls, toolCall)

		callIndex++
		// Move to next potential tool call
		idx = toolCallEnd + 12 // len("</tool_call>")
	}

	log.Printf("[XML-PARSER] Total tool calls parsed: %d", len(toolCalls))
	return toolCalls
}

// parseParameters extracts parameters from content
// Supports both formats:
// - <parameter=key> value (without closing tag)
// - <parameter=key>value</parameter> (with closing tag)
func parseParameters(content string) map[string]string {
	params := make(map[string]string)
	idx := 0

	for {
		// Find parameter tag
		paramStart := strings.Index(content[idx:], "<parameter=")
		if paramStart == -1 {
			break
		}
		paramStart += idx

		// Find end of parameter tag
		paramEnd := strings.Index(content[paramStart:], ">")
		if paramEnd == -1 {
			break
		}
		paramEnd += paramStart

		// Extract parameter name
		paramName := strings.TrimSpace(content[paramStart+11 : paramEnd])

		// Find value (until closing tag or next tag)
		valueStart := paramEnd + 1
		valueEnd := len(content)

		// Look for closing </parameter> tag first
		closingTag := "</parameter>"
		closingIdx := strings.Index(content[valueStart:], closingTag)
		if closingIdx != -1 {
			// Found closing tag, use it as the end
			valueEnd = valueStart + closingIdx
		} else {
			// No closing tag, look for next tag
			nextTag := strings.Index(content[valueStart:], "<")
			if nextTag != -1 {
				valueEnd = valueStart + nextTag
			}
		}

		// Extract and trim value
		paramValue := strings.TrimSpace(content[valueStart:valueEnd])

		if paramName != "" {
			params[paramName] = paramValue
		}

		// Move past the closing tag if present, otherwise past the opening tag
		if closingIdx != -1 {
			idx = valueEnd + len(closingTag)
		} else {
			idx = paramEnd + 1
		}
	}

	return params
}

// generateToolCallID generates a simple tool call ID
func generateToolCallID(index int) string {
	return "call_" + string(rune('a'+index%26)) + string(rune('0'+index/26))
}
