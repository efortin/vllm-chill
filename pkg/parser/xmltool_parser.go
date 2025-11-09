// Package parser provides functionality for parsing XML tool calls.
package parser

import (
	"encoding/json"
	"fmt"
	"html"
	"log"
	"regexp"
	"strings"
)

// ToolCall represents a parsed tool call from XML or JSON format
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

// XMLToolParser handles parsing of XML tool calls with improved edge case handling
type XMLToolParser struct {
	debug bool
}

// NewXMLToolParser creates a new XML tool parser
func NewXMLToolParser(debug bool) *XMLToolParser {
	return &XMLToolParser{debug: debug}
}

// ParseXMLToolCalls is the main entry point for parsing XML tool calls
func (p *XMLToolParser) ParseXMLToolCalls(content string) []ToolCall {
	if p.debug {
		log.Printf("[XML-PARSER] Parsing XML tool calls from content (length: %d)", len(content))
	}

	// Preprocess content
	content = p.preprocessContent(content)

	// Check if content actually contains XML tool calls
	if !p.containsToolCallPattern(content) {
		if p.debug {
			log.Printf("[XML-PARSER] No tool call patterns detected in content")
		}
		return []ToolCall{}
	}

	var toolCalls []ToolCall

	// Try to parse standard XML format first
	toolCalls = p.parseStandardXMLFormat(content)

	// If no tool calls found, try legacy format
	if len(toolCalls) == 0 {
		toolCalls = p.parseLegacyFormat(content)
	}

	if p.debug {
		log.Printf("[XML-PARSER] Total tool calls parsed: %d", len(toolCalls))
	}
	return toolCalls
}

// containsToolCallPattern checks if content likely contains tool call XML
func (p *XMLToolParser) containsToolCallPattern(content string) bool {
	// Use regex to match tool call patterns with or without namespace
	patterns := []string{
		`<[a-zA-Z0-9_-]*:?tool_call`,
		`<[a-zA-Z0-9_-]*:?function_call`,
		`<function=`,
	}

	for _, pattern := range patterns {
		regex := regexp.MustCompile(pattern)
		if regex.MatchString(content) {
			return true
		}
	}
	return false
}

// preprocessContent cleans and normalizes the content
func (p *XMLToolParser) preprocessContent(content string) string {
	// Remove BOM
	content = strings.TrimPrefix(content, "\uFEFF")

	// Remove markdown code blocks
	codeBlockRegex := regexp.MustCompile("```[a-z]*\n")
	content = codeBlockRegex.ReplaceAllString(content, "")
	content = strings.ReplaceAll(content, "```", "")

	// Unwrap CDATA sections that wrap entire tool calls
	cdataRegex := regexp.MustCompile(`<!\[CDATA\[(.*?)\]\]>`)
	content = cdataRegex.ReplaceAllString(content, "$1")

	// Handle cases where "<" appears in non-XML contexts
	// Protect XML tags first by temporarily replacing them
	content = p.protectNonXMLLessThan(content)

	return content
}

// protectNonXMLLessThan handles "<" characters that are not part of XML tags
func (p *XMLToolParser) protectNonXMLLessThan(content string) string {
	// For now, we'll skip this protection step as it's causing issues with namespace handling
	// The HTML unescaping later in the pipeline will handle entities properly
	return content
}

// parseStandardXMLFormat parses standard XML format tool calls
func (p *XMLToolParser) parseStandardXMLFormat(content string) []ToolCall {
	var toolCalls []ToolCall
	callIndex := 0

	// Handle streaming fragments (merge parts)
	content = p.mergeStreamingFragments(content)

	// Use a single comprehensive pattern for both tool_call and function_call
	// This pattern matches tags with optional namespace prefix
	pattern := `<(?:[a-zA-Z0-9_-]+:)?(tool_call|function_call)[^>]*>`
	regex := regexp.MustCompile(pattern)
	matches := regex.FindAllStringSubmatchIndex(content, -1)

	// Track processed positions to avoid duplicates
	processedPositions := make(map[int]bool)

	for _, match := range matches {
		startIdx := match[0]

		// Skip if we've already processed this position
		if processedPositions[startIdx] {
			continue
		}
		processedPositions[startIdx] = true

		// Get the tag type (tool_call or function_call)
		tagName := content[match[2]:match[3]]

		// Find the closing tag (may not exist for truncated output)
		endIdx := p.findClosingTag(content, startIdx, tagName)

		// Extract the tool call XML
		toolCallXML := content[startIdx:endIdx]

		// Parse the tool call
		toolCall := p.parseToolCallXML(toolCallXML, tagName, callIndex)
		if toolCall != nil {
			toolCalls = append(toolCalls, *toolCall)
			callIndex++
		}
	}

	return toolCalls
}

// findClosingTag finds the closing tag for an XML element
func (p *XMLToolParser) findClosingTag(content string, startIdx int, tagName string) int {
	// Look for closing tag patterns (with or without namespace)
	closingPatterns := []string{
		fmt.Sprintf(`</%s>`, tagName),
		fmt.Sprintf(`</[a-zA-Z0-9_-]*:%s>`, tagName),
		fmt.Sprintf(`<[a-zA-Z0-9_-]*:/%s>`, tagName),
	}

	closingPattern := "(" + strings.Join(closingPatterns, "|") + ")"
	closingRegex := regexp.MustCompile(closingPattern)
	closingMatches := closingRegex.FindStringIndex(content[startIdx:])

	if closingMatches != nil {
		return startIdx + closingMatches[1]
	}

	// No closing tag found, try to find next tool call or end of content
	nextToolCall := regexp.MustCompile(`<[^:>]*:?(tool_call|function_call)[^>]*>`)
	nextMatches := nextToolCall.FindStringIndex(content[startIdx+1:])

	if nextMatches != nil {
		return startIdx + 1 + nextMatches[0]
	}

	return len(content)
}

// mergeStreamingFragments merges tool calls with part attributes
func (p *XMLToolParser) mergeStreamingFragments(content string) string {
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
	firstStart := matches[0][0]
	lastEnd := matches[len(matches)-1][1]

	result := content[:firstStart] + mergedContent.String() + content[lastEnd:]
	return result
}

// parseToolCallXML parses a single tool call XML fragment
func (p *XMLToolParser) parseToolCallXML(xmlContent string, tagType string, callIndex int) *ToolCall {
	// Strip namespace prefixes for easier parsing
	xmlContent = p.stripNamespaces(xmlContent)

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
			toolName = p.extractTagContent(xmlContent, "tool_name")
		} else {
			toolName = p.extractTagContent(xmlContent, "name")
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
		argsContent := p.extractTagContent(xmlContent, "tool_arguments")
		if argsContent == "" {
			argsContent = p.extractTagContent(xmlContent, "arguments")
		}
		if argsContent == "" {
			argsContent = p.extractTagContent(xmlContent, "args")
		}
		argsJSON = p.parseArguments(argsContent)
	} else {
		argsContent := p.extractTagContent(xmlContent, "arguments")
		if argsContent == "" {
			argsContent = p.extractTagContent(xmlContent, "args")
		}
		argsJSON = p.parseArguments(argsContent)
	}

	return &ToolCall{
		ID:   p.generateToolCallID(callIndex),
		Type: "function",
		Function: ToolCallFunction{
			Name:      toolName,
			Arguments: argsJSON,
		},
	}
}

// stripNamespaces removes XML namespace prefixes
func (p *XMLToolParser) stripNamespaces(content string) string {
	// Remove namespace declarations
	nsRegex := regexp.MustCompile(`\s+xmlns[^=]*="[^"]*"`)
	content = nsRegex.ReplaceAllString(content, "")

	// Remove namespace prefixes from opening and closing tags
	// Match patterns like <qwen:tool_call> or </qwen:tool_call>
	prefixRegex := regexp.MustCompile(`<(/?)([a-zA-Z0-9_-]+):`)
	content = prefixRegex.ReplaceAllString(content, "<$1")

	return content
}

// extractTagContent extracts content from a tag, handling CDATA and edge cases
func (p *XMLToolParser) extractTagContent(xmlContent, tagName string) string {
	// Try with CDATA (with or without namespace)
	cdataPatterns := []string{
		fmt.Sprintf(`<%s[^>]*><!\[CDATA\[(.*?)\]\]></%s>`, tagName, tagName),
		fmt.Sprintf(`<[a-zA-Z0-9_-]*:%s[^>]*><!\[CDATA\[(.*?)\]\]></[a-zA-Z0-9_-]*:%s>`, tagName, tagName),
	}
	for _, pattern := range cdataPatterns {
		cdataRegex := regexp.MustCompile(pattern)
		if match := cdataRegex.FindStringSubmatch(xmlContent); match != nil {
			if len(match) > 1 && match[1] != "" {
				return match[1]
			}
			if len(match) > 2 && match[2] != "" {
				return match[2]
			}
		}
	}

	// Try normal extraction with dotall flag (to match newlines)
	// Handle both with and without namespace
	patterns := []string{
		fmt.Sprintf(`<%s[^>]*>([\s\S]*?)</%s>`, tagName, tagName),
		fmt.Sprintf(`<[a-zA-Z0-9_-]*:%s[^>]*>([\s\S]*?)</[a-zA-Z0-9_-]*:%s>`, tagName, tagName),
	}

	for _, pattern := range patterns {
		regex := regexp.MustCompile(pattern)
		if match := regex.FindStringSubmatch(xmlContent); match != nil {
			content := strings.TrimSpace(match[1])
			// Unescape any remaining entities
			content = html.UnescapeString(content)
			if content != "" {
				return content
			}
		}
	}

	// Try without closing tag (for truncated output)
	truncatedPatterns := []string{
		fmt.Sprintf(`<%s[^>]*>([\s\S]*)$`, tagName),
		fmt.Sprintf(`<[a-zA-Z0-9_-]*:%s[^>]*>([\s\S]*)$`, tagName),
	}

	for _, pattern := range truncatedPatterns {
		regex := regexp.MustCompile(pattern)
		if match := regex.FindStringSubmatch(xmlContent); match != nil {
			content := match[1]
			// Remove any trailing comment or incomplete tags
			if idx := strings.Index(content, "<!--"); idx != -1 {
				content = content[:idx]
			}
			// Remove any incomplete opening tags
			if idx := strings.LastIndex(content, "<"); idx != -1 {
				// Check if this is an incomplete tag
				if !strings.Contains(content[idx:], ">") {
					content = content[:idx]
				}
			}
			content = strings.TrimSpace(content)
			content = html.UnescapeString(content)
			if content != "" {
				return content
			}
		}
	}

	return ""
}

// parseArguments converts argument content to JSON string
func (p *XMLToolParser) parseArguments(argsContent string) string {
	argsContent = strings.TrimSpace(argsContent)

	// Handle empty arguments
	if argsContent == "" {
		return "{}"
	}

	// Check if it's already JSON
	if strings.HasPrefix(argsContent, "{") || strings.HasPrefix(argsContent, "[") {
		// Validate and clean the JSON
		var test interface{}
		if err := json.Unmarshal([]byte(argsContent), &test); err == nil {
			return argsContent
		}

		// Try to fix common JSON issues
		fixedJSON := p.fixCommonJSONIssues(argsContent)
		if err := json.Unmarshal([]byte(fixedJSON), &test); err == nil {
			return fixedJSON
		}
	}

	// Try to parse as nested XML
	if strings.HasPrefix(argsContent, "<") {
		return p.parseNestedXMLArguments(argsContent)
	}

	// Default to empty object
	return "{}"
}

// fixCommonJSONIssues attempts to fix common JSON formatting issues
func (p *XMLToolParser) fixCommonJSONIssues(jsonStr string) string {
	// Handle trailing commas
	trailingCommaRegex := regexp.MustCompile(`,(\s*[}\]])`)
	jsonStr = trailingCommaRegex.ReplaceAllString(jsonStr, "$1")

	// Handle single quotes (convert to double quotes)
	// This is a simplified approach - a full implementation would need proper parsing
	if !strings.Contains(jsonStr, `"`) && strings.Contains(jsonStr, `'`) {
		jsonStr = strings.ReplaceAll(jsonStr, `'`, `"`)
	}

	return jsonStr
}

// parseNestedXMLArguments converts nested XML to JSON
func (p *XMLToolParser) parseNestedXMLArguments(xmlContent string) string {
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
		switch typeAttr {
		case "int":
			var intVal int
			if _, err := fmt.Sscanf(value, "%d", &intVal); err == nil {
				result[tagName] = intVal
			} else {
				result[tagName] = value
			}
		case "bool":
			result[tagName] = value == "true"
		case "float":
			var floatVal float64
			if _, err := fmt.Sscanf(value, "%f", &floatVal); err == nil {
				result[tagName] = floatVal
			} else {
				result[tagName] = value
			}
		default:
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
func (p *XMLToolParser) parseLegacyFormat(content string) []ToolCall {
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
			// Try without closing tag for truncated output
			toolCallEnd = len(content[funcEnd:])
		}
		toolCallEnd += funcEnd

		// Extract content between function tag and </tool_call>
		paramsContent := content[funcEnd+1 : toolCallEnd]

		// Remove optional </function> tag if present
		paramsContent = strings.Replace(paramsContent, "</function>", "", 1)

		// Parse parameters
		params := p.parseParameters(paramsContent)

		// Convert parameters to JSON
		argsJSON, _ := json.Marshal(params)

		toolCall := ToolCall{
			ID:   p.generateToolCallID(callIndex),
			Type: "function",
			Function: ToolCallFunction{
				Name:      toolName,
				Arguments: string(argsJSON),
			},
		}

		if p.debug {
			log.Printf("[XML-PARSER] Parsed tool call: name=%s, args=%s", toolName, string(argsJSON))
		}
		toolCalls = append(toolCalls, toolCall)

		callIndex++
		// Move to next potential tool call
		if strings.Contains(content[funcEnd:], "</tool_call>") {
			idx = toolCallEnd + 12 // len("</tool_call>")
		} else {
			idx = toolCallEnd
		}
	}

	return toolCalls
}

// parseParameters extracts parameters from content (legacy format)
func (p *XMLToolParser) parseParameters(content string) map[string]string {
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

		if p.debug {
			log.Printf("[XML-PARSER] Processing parameter: %s", paramName)
			endPreview := paramEnd + 50
			if endPreview > len(content) {
				endPreview = len(content)
			}
			log.Printf("[XML-PARSER] Content after param tag: %q", content[paramEnd+1:endPreview])
		}

		// Find value (until closing tag or next tag)
		valueStart := paramEnd + 1
		var valueEnd int

		// Look for closing tag - check for incomplete tag first since it's a substring
		// of the complete tag
		closingTag := "</parameter"
		closingIdx := strings.Index(content[valueStart:], closingTag)

		if closingIdx != -1 {
			// Check if it's a complete tag (has >) or incomplete
			nextCharIdx := valueStart + closingIdx + len(closingTag)
			if nextCharIdx < len(content) && content[nextCharIdx] == '>' {
				// It's a complete tag
				closingTag = "</parameter>"
				if p.debug {
					log.Printf("[XML-PARSER] Found complete closing tag at position %d", closingIdx)
				}
			} else if p.debug {
				// It's an incomplete tag
				log.Printf("[XML-PARSER] Found incomplete closing tag '%s' at position %d", closingTag, closingIdx)
			}
			valueEnd = valueStart + closingIdx
		} else {
			// No closing tag, look for next tag
			nextTag := strings.Index(content[valueStart:], "<")
			if nextTag != -1 {
				valueEnd = valueStart + nextTag
			} else {
				valueEnd = len(content)
			}
			if p.debug {
				log.Printf("[XML-PARSER] No closing tag found, using next tag at position %d", nextTag)
			}
		}

		// Extract and trim value
		paramValue := strings.TrimSpace(content[valueStart:valueEnd])

		if paramName != "" {
			params[paramName] = paramValue
		}

		// Move past the closing tag if present, otherwise past the value end
		if closingIdx != -1 {
			idx = valueEnd + len(closingTag)
		} else {
			// No closing tag found, move to valueEnd
			idx = valueEnd
			if idx == len(content) {
				// We've reached the end
				break
			}
		}
	}

	return params
}

// generateToolCallID generates a simple tool call ID
func (p *XMLToolParser) generateToolCallID(index int) string {
	letter := 'a' + rune(index%26)
	num := index / 26
	if num == 0 {
		return fmt.Sprintf("call_%c", letter)
	}
	return fmt.Sprintf("call_%c%d", letter, num)
}

// ParseXMLToolCalls parses XML tool calls from content.
//
// This is a convenience function for backward compatibility.
func ParseXMLToolCalls(content string) []ToolCall {
	parser := NewXMLToolParser(true)
	return parser.ParseXMLToolCalls(content)
}
