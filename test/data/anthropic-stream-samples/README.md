# Anthropic Streaming Conversation Samples

20 streaming conversation samples in Anthropic SSE format for testing real-time vLLM-chill transformation and integration.

## Overview

This directory contains **20 realistic streaming samples** that follow the Anthropic Messages API streaming format (Server-Sent Events). These are used for:

- ✅ Testing streaming transformation (Anthropic SSE ↔ OpenAI streaming)
- ✅ Real-time response handling
- ✅ SSE protocol validation
- ✅ Tool use in streaming context
- ✅ Chunked text delivery

## File Structure

```
anthropic-stream-samples/
├── conv_stream_001.jsonl   # Stream with tool use
├── conv_stream_002.jsonl
├── ...
├── conv_stream_020.jsonl
├── README.md               # This file
└── VALIDATION_REPORT.md    # Test results
```

## Stream Format

Each `.jsonl` file contains SSE (Server-Sent Events) in the following format:

```
# Comment line (metadata)
event: event_type
data: {"json": "data"}

event: next_event
data: {"more": "data"}
```

## Statistics

```
📊 20 streams validated:
   • 452 events total
   • 22.6 events/stream (average)
   • 287 deltas (text chunks)
   • 14.3 deltas/stream (average)
   • 13 tool uses
   • 65% streams with tools
```

## Event Types

1. **message_start** - Initialize streaming response
2. **content_block_start** - Begin a content block
3. **content_block_delta** - Stream text chunks
4. **content_block_stop** - End a content block
5. **tool_result** - Tool execution result
6. **message_delta** - Final usage/stop_reason
7. **message_stop** - End of stream

## Running Tests

### Validate All Streams
```bash
go test ./pkg/proxy -run TestAnthropicStreamSamples -v
```

### Test Transformation
```bash
go test ./pkg/proxy -run TestStreamToOpenAITransformation -v
```

### Run All Stream Tests
```bash
go test ./pkg/proxy -run "TestStream" -v
```

## Using in Tests

### Parse a Stream
```go
events, err := parseStreamFile("conv_stream_001.jsonl")
for _, event := range events {
    switch event.Event {
    case "message_start":
        // Handle start
    case "content_block_delta":
        // Stream text
    }
}
```

### Transform to OpenAI
```go
anthropicEvents, _ := parseStreamFile(filepath)
openAIEvents := transformAnthropicStreamToOpenAI(anthropicEvents)
```

## Quality Assurance

All streams validated for:
- ✅ Valid JSONL structure
- ✅ Proper SSE format
- ✅ Correct event sequence
- ✅ Balanced content blocks
- ✅ Transformation compatibility

See [VALIDATION_REPORT.md](./VALIDATION_REPORT.md) for details.

## Performance

- **Parsing**: ~120μs per stream
- **Transformation**: ~250μs per stream

## Adding New Streams

1. Create `conv_stream_NNN.jsonl`
2. Start with `message_start`
3. End with `message_stop`
4. Balance all content blocks
5. Run: `go test ./pkg/proxy -run TestAnthropicStreamSamples`

## Resources

- [Anthropic Streaming API](https://docs.anthropic.com/claude/reference/messages-streaming)
- [vLLM-chill Docs](../../docs/ANTHROPIC_FORMAT.md)
- [Test Suite](../../pkg/proxy/anthropic_stream_samples_validation_test.go)
