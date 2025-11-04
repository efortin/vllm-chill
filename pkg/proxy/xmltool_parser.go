package proxy

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"log"
	"regexp"
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
// Supports multiple formats:
// - <tool_call><tool_name>...</tool_name><tool_arguments>...</tool_arguments></tool_call>
// - <function_call><name>...</name><arguments>...</arguments></function_call>
// - <function=name> <parameter=key> value </tool_call> (legacy format)
func parseXMLToolCalls(content string) []ToolCall {
	var toolCalls []ToolCall

	log.Printf("[XML-PARSER] Parsing XML tool calls from content (length: %d)", len(content))

	// Preprocess content
	content = preprocessContent(content)

	// Try to parse standard XML format first
	toolCalls = parseStandardXMLFormat(content)

	// If no tool calls found, try legacy format
	if len(toolCalls) == 0 {
		toolCalls = parseLegacyFormat(content)
	}

	log.Printf("[XML-PARSER] Total tool calls parsed: %d", len(toolCalls))
	return toolCalls
}

// preprocessContent cleans and normalizes the content
func preprocessContent(content string) string {
	// Remove BOM
	content = strings.TrimPrefix(content, "\uFEFF")

	// Remove markdown code blocks
	codeBlockRegex := regexp.MustCompile("```[a-z]*\n")
	content = codeBlockRegex.ReplaceAllString(content, "")
	content = strings.ReplaceAll(content, "```", "")

	// Unwrap CDATA sections that wrap entire tool calls
	cdataRegex := regexp.MustCompile(`<!\[CDATA\[(.*?)\]\]>`)
	content = cdataRegex.ReplaceAllString(content, "$1")

	return content
}

// parseStandardXMLFormat parses standard XML format tool calls
func parseStandardXMLFormat(content string) []ToolCall {
	var toolCalls []ToolCall
	callIndex := 0

	// Handle streaming fragments (merge parts)
	content = mergeStreamingFragments(content)

	// Find all tool_call or function_call tags
	patterns := []string{
		`<[^:>]*:?tool_call[^>]*>`,
		`<[^:>]*:?function_call[^>]*>`,
	}

	for _, pattern := range patterns {
		regex := regexp.MustCompile(pattern)
		matches := regex.FindAllStringIndex(content, -1)

		for _, match := range matches {
			startIdx := match[0]

			// Determine the tag name (with or without namespace)
			tagStart := content[startIdx:match[1]]
			var tagName string
			if strings.Contains(tagStart, "tool_call") {
				tagName = "tool_call"
			} else {
				tagName = "function_call"
			}

			// Find the closing tag (may not exist for truncated output)
			closingPattern := fmt.Sprintf(`</%s>|<[^:>]*:?/%s>`, tagName, tagName)
			closingRegex := regexp.MustCompile(closingPattern)
			closingMatches := closingRegex.FindStringIndex(content[startIdx:])

			var endIdx int
			if closingMatches != nil {
				endIdx = startIdx + closingMatches[1]
			} else {
				// No closing tag found, try to extract until end or next tool call
				endIdx = len(content)
			}

			// Extract the tool call XML
			toolCallXML := content[startIdx:endIdx]

			// Parse the tool call
			toolCall := parseToolCallXML(toolCallXML, tagName, callIndex)
			if toolCall != nil {
				toolCalls = append(toolCalls, *toolCall)
				callIndex++
			}
		}
	}

	return toolCalls
}

// mergeStreamingFragments merges tool calls with part attributes
func mergeStreamingFragments(content string) string {
	// Find all tool_call tags with part attribute
	partRegex := regexp.MustCompile(`<tool_call[^>]*part="[^"]*"[^>]*>([\s\S]*?)</tool_call>`)
	matches := partRegex.FindAllStringSubmatchIndex(content, -1)

	if len(matches) <= 1 {
		return content
	}

	// Extract fragments
	var toolName string
	var arguments strings.Builder

	for _, matchIdx := range matches {
		fragment := content[matchIdx[2]:matchIdx[3]]

		// Extract tool_name if present
		nameRegex := regexp.MustCompile(`<tool_name>([\s\S]*?)</tool_name>`)
		if nameMatch := nameRegex.FindStringSubmatch(fragment); nameMatch != nil {
			toolName = strings.TrimSpace(nameMatch[1])
		}

		// Extract tool_arguments content
		argsRegex := regexp.MustCompile(`<tool_arguments>([\s\S]*?)</tool_arguments>`)
		if argsMatch := argsRegex.FindStringSubmatch(fragment); argsMatch != nil {
			arguments.WriteString(strings.TrimSpace(argsMatch[1]))
		} else {
			// Handle case where only arguments are in the fragment (no closing tag)
			argsRegex2 := regexp.MustCompile(`<tool_arguments>([\s\S]*?)$`)
			if argsMatch2 := argsRegex2.FindStringSubmatch(fragment); argsMatch2 != nil {
				arguments.WriteString(strings.TrimSpace(argsMatch2[1]))
			}
		}
	}

	// Build merged tool call
	var mergedContent strings.Builder
	mergedContent.WriteString("<tool_call>")
	if toolName != "" {
		mergedContent.WriteString("<tool_name>")
		mergedContent.WriteString(toolName)
		mergedContent.WriteString("</tool_name>")
	}
	mergedContent.WriteString("<tool_arguments>")
	mergedContent.WriteString(arguments.String())
	mergedContent.WriteString("</tool_arguments>")
	mergedContent.WriteString("</tool_call>")

	// Replace all fragments with single merged version
	// Find the start of first match and end of last match
	firstStart := matches[0][0]
	lastEnd := matches[len(matches)-1][1]

	result := content[:firstStart] + mergedContent.String() + content[lastEnd:]
	return result
}

// parseToolCallXML parses a single tool call XML fragment
func parseToolCallXML(xmlContent string, tagType string, callIndex int) *ToolCall {
	// Strip namespace prefixes for easier parsing
	xmlContent = stripNamespaces(xmlContent)

	// Unescape HTML entities
	xmlContent = html.UnescapeString(xmlContent)

	// Extract tool name - try attribute first, then tag content
	var toolName string

	// Try to extract name from attribute: <tool_call name="...">
	nameAttrRegex := regexp.MustCompile(`<(?:tool_call|function_call)[^>]*\s+name\s*=\s*["']([^"']+)["']`)
	if match := nameAttrRegex.FindStringSubmatch(xmlContent); match != nil {
		toolName = match[1]
	} else {
		// Fall back to tag content
		if tagType == "tool_call" {
			toolName = extractTagContent(xmlContent, "tool_name")
		} else {
			toolName = extractTagContent(xmlContent, "name")
		}
	}

	if toolName == "" {
		return nil
	}

	toolName = strings.TrimSpace(toolName)

	// Extract arguments
	var argsJSON string
	if tagType == "tool_call" {
		// Try multiple tag names for arguments
		argsContent := extractTagContent(xmlContent, "tool_arguments")
		if argsContent == "" {
			argsContent = extractTagContent(xmlContent, "arguments")
		}
		if argsContent == "" {
			argsContent = extractTagContent(xmlContent, "args")
		}
		argsJSON = parseArguments(argsContent)
	} else {
		argsContent := extractTagContent(xmlContent, "arguments")
		if argsContent == "" {
			argsContent = extractTagContent(xmlContent, "args")
		}
		argsJSON = parseArguments(argsContent)
	}

	return &ToolCall{
		ID:   generateToolCallID(callIndex),
		Type: "function",
		Function: ToolCallFunction{
			Name:      toolName,
			Arguments: argsJSON,
		},
	}
}

// stripNamespaces removes XML namespace prefixes
func stripNamespaces(content string) string {
	// Remove namespace declarations
	nsRegex := regexp.MustCompile(`\s+xmlns[^=]*="[^"]*"`)
	content = nsRegex.ReplaceAllString(content, "")

	// Remove namespace prefixes from tags
	prefixRegex := regexp.MustCompile(`</?[^:>]*:`)
	content = prefixRegex.ReplaceAllStringFunc(content, func(match string) string {
		if strings.HasPrefix(match, "</") {
			return "</"
		}
		return "<"
	})

	return content
}

// extractTagContent extracts content from a tag, handling CDATA
func extractTagContent(xmlContent, tagName string) string {
	// Try with CDATA
	cdataPattern := fmt.Sprintf(`<%s[^>]*><!\[CDATA\[(.*?)\]\]></%s>`, tagName, tagName)
	cdataRegex := regexp.MustCompile(cdataPattern)
	if match := cdataRegex.FindStringSubmatch(xmlContent); match != nil {
		return match[1]
	}

	// Try normal extraction with dotall flag (to match newlines)
	pattern := fmt.Sprintf(`<%s[^>]*>([\s\S]*?)</%s>`, tagName, tagName)
	regex := regexp.MustCompile(pattern)
	if match := regex.FindStringSubmatch(xmlContent); match != nil {
		return strings.TrimSpace(match[1])
	}

	// Try without closing tag (for truncated output)
	pattern2 := fmt.Sprintf(`<%s[^>]*>([\s\S]*)$`, tagName)
	regex2 := regexp.MustCompile(pattern2)
	if match := regex2.FindStringSubmatch(xmlContent); match != nil {
		content := match[1]
		// Remove any trailing comment or incomplete tags
		if idx := strings.Index(content, "<!--"); idx != -1 {
			content = content[:idx]
		}
		return strings.TrimSpace(content)
	}

	return ""
}

// parseArguments converts argument content to JSON string
func parseArguments(argsContent string) string {
	argsContent = strings.TrimSpace(argsContent)

	// Handle empty arguments
	if argsContent == "" {
		return "{}"
	}

	// Check if it's already JSON
	if strings.HasPrefix(argsContent, "{") || strings.HasPrefix(argsContent, "[") {
		// Validate and return as-is
		var test interface{}
		if err := json.Unmarshal([]byte(argsContent), &test); err == nil {
			return argsContent
		}
	}

	// Try to parse as nested XML
	if strings.HasPrefix(argsContent, "<") {
		return parseNestedXMLArguments(argsContent)
	}

	// Default to empty object
	return "{}"
}

// parseNestedXMLArguments converts nested XML to JSON
func parseNestedXMLArguments(xmlContent string) string {
	// Simple XML to map conversion
	result := make(map[string]interface{})

	// Find all simple tags - match opening tag, content, closing tag
	// Pattern: <tagname [type="..."]>content</tagname>
	tagRegex := regexp.MustCompile(`<([a-zA-Z_][a-zA-Z0-9_-]*)(?:\s+type="([^"]+)")?>([^<]*)</[a-zA-Z_][a-zA-Z0-9_-]*>`)
	matches := tagRegex.FindAllStringSubmatch(xmlContent, -1)

	for _, match := range matches {
		tagName := match[1]
		// Skip wrapper tags like 'args'
		if tagName == "args" {
			continue
		}
		typeAttr := match[2]
		value := strings.TrimSpace(match[3])

		// Convert based on type attribute
		if typeAttr == "int" {
			var intVal int
			if _, err := fmt.Sscanf(value, "%d", &intVal); err == nil {
				result[tagName] = intVal
			} else {
				// If parsing fails, store as string
				result[tagName] = value
			}
		} else {
			result[tagName] = value
		}
	}

	if len(result) > 0 {
		jsonBytes, _ := json.Marshal(result)
		return string(jsonBytes)
	}

	return "{}"
}

// parseLegacyFormat parses the legacy <function=name> format
func parseLegacyFormat(content string) []ToolCall {
	var toolCalls []ToolCall
	idx := 0
	callIndex := 0

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

	return toolCalls
}

// parseParameters extracts parameters from content (legacy format)
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

// Unused but kept for potential XML parsing needs
var _ = xml.Unmarshal
