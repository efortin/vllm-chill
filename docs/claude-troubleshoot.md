# Claude Code Troubleshooting Guide

## Quick Fixes for Common Issues

### 1. Claude Code Shows "0 tokens" or Terminates Prematurely

**Root Cause**: Missing fields in SSE `message_start` event.

**Solution**: Ensure `message_start` includes both `content: []` and `usage`:

```json
{
  "type": "message_start",
  "message": {
    "id": "msg-123",
    "content": [],  // ← REQUIRED (empty array)
    "usage": {      // ← REQUIRED
      "input_tokens": 0,
      "output_tokens": 0
    }
  }
}
```

**Why**: 
- Without `content: []`: Claude Code crashes with `undefined.push()` error
- Without `usage`: Token counter never initializes

**Send `message_delta` ONLY at stream end**:
```json
// ❌ WRONG: Sending during stream
event: content_block_delta
...
event: message_delta  // ← Causes premature termination

// ✅ CORRECT: Only at [DONE]
event: content_block_stop
event: message_delta  // ← Only here
event: message_stop
```

### 2. Tool Calls Not Working

**Root Cause**: Need to transform OpenAI `tool_calls` → Anthropic `tool_use` format.

**Configuration**:
```bash
# CRITICAL: Must be false for native tool calls
ENABLE_XML_PARSING=false
```

**vLLM generates** (OpenAI format):
```json
{
  "choices": [{
    "delta": {
      "tool_calls": [{
        "index": 0,
        "id": "call_123",
        "function": {
          "name": "get_weather",
          "arguments": "{\"location\": \"Paris\"}"
        }
      }]
    },
    "finish_reason": "tool_calls"
  }]
}
```

**Proxy transforms to** (Anthropic format):
```
event: content_block_start
data: {"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"call_123","name":"get_weather"}}

event: content_block_delta
data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"location\": \"Paris\"}"}}

event: content_block_stop
data: {"type":"content_block_stop","index":1}

event: message_delta
data: {"delta":{"stop_reason":"tool_use"},"usage":{...}}
```

**Key Points**:
- Text block is always `index: 0`
- Tool calls start at `index: 1, 2, 3...`
- `stop_reason` must be `"tool_use"` (not `"end_turn"`)

### 3. Out of Memory (OOM) Crashes

**Symptom**: Pod shows `CrashLoopBackOff`, logs show exit code 137.

**Solution**: Increase memory limits (tiktoken needs ~200-300Mi):

```yaml
resources:
  requests:
    memory: 256Mi  # Minimum
  limits:
    memory: 512Mi  # Recommended
```

### 4. XML Tool Calls (Legacy - Opus)

**When**: Model generates `<function=name>{"args"}</function>` instead of native tool_calls.

**Configuration**:
```bash
ENABLE_XML_PARSING=true  # Only for XML-generating models
```

**Note**: Cannot use both XML parsing AND native tool calls simultaneously.

## Implementation Details

### Streaming Tool Calls State Machine

```go
type toolCallState struct {
    id              string  // OpenAI call ID
    name            string  // Function name
    args            strings.Builder  // Accumulated arguments
    index           int  // OpenAI index (0, 1, 2...)
    contentBlockIdx int  // Anthropic index (1, 2, 3...)
    started         bool
}
```

**Flow**:
1. **First chunk with `tool_calls`**: Get `id` + `name` → Send `content_block_start`
2. **Each argument chunk**: Append to `args` → Send `input_json_delta`
3. **`finish_reason="tool_calls"`**: Send `content_block_stop` for each tool
4. **`[DONE]`**: Send `message_delta` with `stop_reason: "tool_use"`

### Event Sequence

**Normal text response**:
```
message_start (content:[], usage:{0,0})
content_block_start (index:0, type:text)
content_block_delta (text chunks...)
content_block_stop (index:0)
message_delta (stop_reason:end_turn, usage:{...})
message_stop
```

**Tool call response**:
```
message_start (content:[], usage:{0,0})
content_block_start (index:0, type:text) ← May be empty
content_block_stop (index:0)
content_block_start (index:1, type:tool_use, name:get_weather)
content_block_delta (index:1, input_json_delta)
content_block_stop (index:1)
message_delta (stop_reason:tool_use, usage:{...})
message_stop
```

**Multiple tool calls**:
```
message_start
content_block_start (index:0, type:text)
content_block_stop (index:0)
content_block_start (index:1, type:tool_use, name:tool_a)
content_block_delta (index:1, partial_json)
content_block_stop (index:1)
content_block_start (index:2, type:tool_use, name:tool_b)
content_block_delta (index:2, partial_json)
content_block_stop (index:2)
message_delta (stop_reason:tool_use)
message_stop
```

## Testing

Run streaming fixes tests:
```bash
go test -v ./pkg/proxy/ -run "TestMessageStart|TestToolCallsTransformation"
```

**Key test coverage**:
- `TestMessageStartStructure`: Validates `content: []` and `usage` presence
- `TestNoPeriodicMessageDelta`: Ensures no mid-stream `message_delta`
- `TestToolCallsTransformation`: Validates OpenAI → Anthropic conversion
- `TestMultipleToolCalls`: Tests concurrent tool calls
- `TestStopReasonMapping`: Validates `tool_use` vs `end_turn`

## Debugging

**Enable detailed logs**:
```bash
kubectl logs -f -l app=vllm-chill | grep -E "TOOL-CALLS|DEBUG|ERROR"
```

**Check Claude Code logs**:
```bash
ls -lt ~/.claude/debug/ | head -5
tail -100 ~/.claude/debug/latest
```

**Common errors**:
- `undefined is not an object (evaluating 'A.content.push')` → Missing `content: []`
- `JSON Parse error: Expected '}'` → Malformed SSE event
- Stream terminates early → `message_delta` sent during streaming

**Test with curl**:
```bash
curl -N -X POST https://vllm.sir-alfred.io/v1/messages \
  -H "anthropic-version: 2023-06-01" \
  -H "x-api-key: YOUR_KEY" \
  -d '{"model":"claude-haiku-4-5-20251001","messages":[{"role":"user","content":"What is the weather in Paris?"}],"tools":[{"name":"get_weather","description":"Get weather","input_schema":{"type":"object","properties":{"location":{"type":"string"}}}}],"stream":true}'
```

## Configuration Reference

| Variable | Default | Purpose |
|----------|---------|---------|
| `ENABLE_XML_PARSING` | `false` | Enable XML tool call parsing (Opus) |
| `LOG_OUTPUT` | `false` | Capture response bodies for debugging |
| Memory request | `256Mi` | Minimum for tiktoken |
| Memory limit | `512Mi` | Recommended for stable operation |

## Architecture

```
┌─────────────┐
│ Claude Code │
└──────┬──────┘
       │ Anthropic SSE
       │ (tool_use format)
┌──────▼──────────────────────────────┐
│ vllm-chill Proxy                    │
│ • Transform OpenAI → Anthropic      │
│ • Streaming tool_calls → tool_use   │
│ • Token tracking with tiktoken      │
└──────┬──────────────────────────────┘
       │ OpenAI format
       │ (tool_calls format)
┌──────▼──────┐
│    vLLM     │
└─────────────┘
```

## Timeline of Fixes

1. **Token Display Fix**: Added `content: []` and `usage` to `message_start`
2. **Premature Termination Fix**: Moved `message_delta` to stream end only
3. **Tool Calls Support**: Implemented OpenAI → Anthropic transformation
4. **OOM Fix**: Increased memory limits for tiktoken

All fixes verified with Claude Code and comprehensive test coverage.
