# Anthropic Conversation Samples

50 sample conversations in Anthropic API format for testing vLLM-chill transformation and integration.

## Overview

This directory contains **50 realistic conversation samples** that follow the Anthropic Messages API format. These are used for:

- ✅ Testing format transformation (Anthropic ↔ OpenAI)
- ✅ Integration testing with vLLM
- ✅ Validation of streaming responses
- ✅ Tool use examples
- ✅ Multi-turn conversation handling

## File Structure

```
anthropic-samples/
├── conversation_001.json   # Simple conversation with tools
├── conversation_002.json
├── ...
├── conversation_050.json
├── README.md              # This file
└── VALIDATION_REPORT.md   # Test results
```

## Conversation Format

Each conversation follows this schema:

```json
{
  "conversation_id": "conversation_XXX",
  "created_at": "2025-11-14T12:23:19.980183Z",
  "domain": "programming",
  "messages": [
    {
      "role": "user",
      "content": "Your question here"
    },
    {
      "role": "assistant",
      "content": [
        {
          "type": "text",
          "text": "Response text"
        },
        {
          "type": "tool_use",
          "name": "tool_name",
          "input": {...}
        }
      ]
    },
    {
      "role": "tool_result",
      "name": "tool_name",
      "content": {...}
    }
  ]
}
```

## Statistics

- **Total Conversations**: 50
- **Total Messages**: 614
- **Average Length**: 12.3 messages/conversation
- **Tool Usage**: 102 tool calls across 35 conversations
- **Domains**: programming (all 50)

## Message Roles

### `user`
User messages with questions or instructions.
```json
{
  "role": "user",
  "content": "String or array of content blocks"
}
```

### `assistant`
Assistant responses with text, thinking, and tool use.
```json
{
  "role": "assistant",
  "content": [
    {"type": "text", "text": "..."},
    {"type": "thinking", "text": "..."},
    {"type": "tool_use", "name": "...", "input": {...}}
  ]
}
```

### `tool_result`
Results from tool execution.
```json
{
  "role": "tool_result",
  "name": "tool_name",
  "content": {...}
}
```

## Content Block Types

### Text Block
```json
{
  "type": "text",
  "text": "Response content"
}
```

### Thinking Block
```json
{
  "type": "thinking",
  "text": "Internal reasoning"
}
```

### Tool Use Block
```json
{
  "type": "tool_use",
  "name": "git",
  "input": {
    "args": ["status"]
  }
}
```

## Tools Used

The conversations include various tool examples:

- **git**: Version control operations
- **api_client**: HTTP requests (GET, POST)
- **k8s**: Kubernetes operations
- **docker**: Container management

## Running Tests

### Validate All Conversations
```bash
cd /path/to/vllm-chill
go test ./pkg/proxy -run TestAnthropicConversationSamples -v
```

### Test Transformation
```bash
go test ./pkg/proxy -run TestConversationTransformation -v
```

### Analyze Complexity
```bash
go test ./pkg/proxy -run TestConversationComplexity -v
```

### Run All Validation Tests
```bash
go test ./pkg/proxy -run "TestAnthropicConversationSamples|TestConversationTransformation|TestConversationDomains|TestConversationComplexity" -v
```

## Using in Your Tests

### Load a Conversation
```go
data, err := os.ReadFile("test/data/anthropic-non-stream-samples/conversation_001.json")
if err != nil {
    t.Fatal(err)
}

var conv AnthropicConversation
err = json.Unmarshal(data, &conv)
if err != nil {
    t.Fatal(err)
}

// Use conv.Messages for testing
```

### Transform to OpenAI Format
```go
anthropicReq := map[string]interface{}{
    "model": "claude-3-5-sonnet-20241022",
    "messages": conv.Messages,
    "max_tokens": 1024,
}

openAIReq := transformAnthropicToOpenAI(anthropicReq)
```

### Send to vLLM
```go
// The transformation is automatic in vllm-chill
// Just send to /v1/messages endpoint
resp, err := http.Post(
    "https://vllm.sir-alfred.io/v1/messages",
    "application/json",
    bytes.NewReader(requestBytes),
)
```

## Example Conversations

### Simple Q&A (conversation_001)
- 14 messages
- Includes tool uses (git, api_client)
- Good for basic testing

### Complex Multi-Turn (conversation_028)
- 23 messages
- Multiple tool chains
- Good for integration testing

### Tool-Heavy (conversation_034)
- 21 messages
- Extensive tool usage
- Good for tool call testing

## Quality Assurance

All conversations are validated for:
- ✅ Valid JSON structure
- ✅ Required fields present
- ✅ Correct message format
- ✅ Valid content blocks
- ✅ Proper tool use format
- ✅ Transformation compatibility

See [VALIDATION_REPORT.md](./VALIDATION_REPORT.md) for detailed results.

## Adding New Conversations

1. Create `conversation_NNN.json` (next number)
2. Follow the schema above
3. Include all required fields
4. Run validation tests
5. Update this README if needed

```bash
# Validate your new conversation
go test ./pkg/proxy -run TestAnthropicConversationSamples/conversation_NNN
```

## Maintenance

### Updating Schema
If Anthropic API changes:
1. Update sample conversations
2. Update validation tests
3. Run full test suite
4. Update transformation functions

### Regenerating Samples
If you need fresh samples:
```bash
# Your generation script here
# Make sure to maintain the schema
```

## Performance

Benchmarks show excellent performance:
- **Parsing**: ~24μs per conversation
- **Transformation**: ~75μs per conversation

Fast enough for extensive testing without slowdown.

## Integration with vLLM-chill

These conversations are used to test:

1. **Format Detection**
   - `/v1/messages` endpoint triggers Anthropic handler

2. **Request Transformation**
   - Anthropic format → OpenAI format for vLLM

3. **Response Transformation**
   - OpenAI response → Anthropic format for client

4. **Tool Call Handling**
   - Proper XML parsing (if vLLM uses XML format)
   - Tool result integration

5. **Streaming Support**
   - SSE event transformation
   - Real-time streaming validation

## Troubleshooting

### Test Failures
If tests fail:
1. Check JSON syntax: `jq . conversation_XXX.json`
2. Verify schema: All required fields present?
3. Check message roles: Valid values?
4. Review tool format: Proper structure?

### Transformation Issues
If transformation fails:
1. Check content blocks: Valid types?
2. Verify message order: Logical flow?
3. Review tool use: Proper format?

## Resources

- [Anthropic Messages API](https://docs.anthropic.com/claude/reference/messages_post)
- [vLLM-chill Documentation](../../docs/ANTHROPIC_FORMAT.md)
- [Test Suite](../../pkg/proxy/anthropic_samples_validation_test.go)

## License

Part of vLLM-chill project. See main project LICENSE.
