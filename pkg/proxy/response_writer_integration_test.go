package proxy

import (
	"bytes"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResponseWriter_StreamingXMLConversion(t *testing.T) {
	// Exact format from vllm-chill logs (WITH closing tags)
	streamingChunks := []string{
		`data: {"id":"chatcmpl-test","object":"chat.completion.chunk","created":1762238668,"model":"qwen3-coder-30b-fp8","choices":[{"index":0,"delta":{"role":"assistant","content":""},"logprobs":null,"finish_reason":null}],"prompt_token_ids":null}` + "\n\n",
		`data: {"id":"chatcmpl-test","object":"chat.completion.chunk","created":1762238668,"model":"qwen3-coder-30b-fp8","choices":[{"index":0,"delta":{"content":"<"},"logprobs":null,"finish_reason":null,"token_ids":null}]}` + "\n\n",
		`data: {"id":"chatcmpl-test","object":"chat.completion.chunk","created":1762238668,"model":"qwen3-coder-30b-fp8","choices":[{"index":0,"delta":{"content":"function"},"logprobs":null,"finish_reason":null,"token_ids":null}]}` + "\n\n",
		`data: {"id":"chatcmpl-test","object":"chat.completion.chunk","created":1762238668,"model":"qwen3-coder-30b-fp8","choices":[{"index":0,"delta":{"content":"="},"logprobs":null,"finish_reason":null,"token_ids":null}]}` + "\n\n",
		`data: {"id":"chatcmpl-test","object":"chat.completion.chunk","created":1762238668,"model":"qwen3-coder-30b-fp8","choices":[{"index":0,"delta":{"content":"ls"},"logprobs":null,"finish_reason":null,"token_ids":null}]}` + "\n\n",
		`data: {"id":"chatcmpl-test","object":"chat.completion.chunk","created":1762238668,"model":"qwen3-coder-30b-fp8","choices":[{"index":0,"delta":{"content":">\n"},"logprobs":null,"finish_reason":null,"token_ids":null}]}` + "\n\n",
		`data: {"id":"chatcmpl-test","object":"chat.completion.chunk","created":1762238668,"model":"qwen3-coder-30b-fp8","choices":[{"index":0,"delta":{"content":"<"},"logprobs":null,"finish_reason":null,"token_ids":null}]}` + "\n\n",
		`data: {"id":"chatcmpl-test","object":"chat.completion.chunk","created":1762238668,"model":"qwen3-coder-30b-fp8","choices":[{"index":0,"delta":{"content":"parameter"},"logprobs":null,"finish_reason":null,"token_ids":null}]}` + "\n\n",
		`data: {"id":"chatcmpl-test","object":"chat.completion.chunk","created":1762238668,"model":"qwen3-coder-30b-fp8","choices":[{"index":0,"delta":{"content":"=path"},"logprobs":null,"finish_reason":null,"token_ids":null}]}` + "\n\n",
		`data: {"id":"chatcmpl-test","object":"chat.completion.chunk","created":1762238668,"model":"qwen3-coder-30b-fp8","choices":[{"index":0,"delta":{"content":">\n"},"logprobs":null,"finish_reason":null,"token_ids":null}]}` + "\n\n",
		`data: {"id":"chatcmpl-test","object":"chat.completion.chunk","created":1762238668,"model":"qwen3-coder-30b-fp8","choices":[{"index":0,"delta":{"content":".\n"},"logprobs":null,"finish_reason":null,"token_ids":null}]}` + "\n\n",
		`data: {"id":"chatcmpl-test","object":"chat.completion.chunk","created":1762238668,"model":"qwen3-coder-30b-fp8","choices":[{"index":0,"delta":{"content":"</"},"logprobs":null,"finish_reason":null,"token_ids":null}]}` + "\n\n",
		`data: {"id":"chatcmpl-test","object":"chat.completion.chunk","created":1762238668,"model":"qwen3-coder-30b-fp8","choices":[{"index":0,"delta":{"content":"parameter"},"logprobs":null,"finish_reason":null,"token_ids":null}]}` + "\n\n",
		`data: {"id":"chatcmpl-test","object":"chat.completion.chunk","created":1762238668,"model":"qwen3-coder-30b-fp8","choices":[{"index":0,"delta":{"content":">\n"},"logprobs":null,"finish_reason":null,"token_ids":null}]}` + "\n\n",
		`data: {"id":"chatcmpl-test","object":"chat.completion.chunk","created":1762238668,"model":"qwen3-coder-30b-fp8","choices":[{"index":0,"delta":{"content":"</"},"logprobs":null,"finish_reason":null,"token_ids":null}]}` + "\n\n",
		`data: {"id":"chatcmpl-test","object":"chat.completion.chunk","created":1762238668,"model":"qwen3-coder-30b-fp8","choices":[{"index":0,"delta":{"content":"function"},"logprobs":null,"finish_reason":null,"token_ids":null}]}` + "\n\n",
		`data: {"id":"chatcmpl-test","object":"chat.completion.chunk","created":1762238668,"model":"qwen3-coder-30b-fp8","choices":[{"index":0,"delta":{"content":">\n"},"logprobs":null,"finish_reason":null,"token_ids":null}]}` + "\n\n",
		`data: {"id":"chatcmpl-test","object":"chat.completion.chunk","created":1762238668,"model":"qwen3-coder-30b-fp8","choices":[{"index":0,"delta":{"content":"</tool_call>"},"logprobs":null,"finish_reason":null,"token_ids":null}]}` + "\n\n",
		`data: {"id":"chatcmpl-test","object":"chat.completion.chunk","created":1762238668,"model":"qwen3-coder-30b-fp8","choices":[{"index":0,"delta":{"content":""},"logprobs":null,"finish_reason":"stop","stop_reason":null,"token_ids":null}]}` + "\n\n",
		`data: [DONE]` + "\n\n",
	}

	t.Run("chunk by chunk streaming", func(t *testing.T) {
		// Create a test HTTP response recorder
		recorder := httptest.NewRecorder()

		// Create our custom response writer
		rw := newResponseWriter(recorder, true, nil)

		// Write chunks one by one (simulating streaming)
		for i, chunk := range streamingChunks {
			t.Logf("Writing chunk %d: %s", i, chunk[:min(50, len(chunk))])
			n, err := rw.Write([]byte(chunk))
			if err != nil {
				t.Fatalf("Failed to write chunk %d: %v", i, err)
			}
			t.Logf("Wrote %d bytes for chunk %d", n, i)
		}

		// Get the result
		result := recorder.Body.String()
		t.Logf("Result length: %d bytes", len(result))
		if len(result) > 200 {
			t.Logf("Result preview: %s", result[:200])
		} else {
			t.Logf("Result: %s", result)
		}

		// Check if XML was detected and converted
		if !strings.Contains(result, "tool_calls") {
			t.Error("Expected 'tool_calls' in result, but not found - XML not converted!")
		}

		if strings.Contains(result, "<function=") {
			t.Error("Found '<function=' in result - XML was not converted!")
		}

		// Verify tool call structure
		if !strings.Contains(result, `"name":"ls"`) {
			t.Error("Expected function name 'ls' in tool call")
		}

		// Arguments are JSON-encoded string, so path will be escaped
		if !strings.Contains(result, `\"path\":\".\"`) {
			t.Error("Expected parameter 'path':'.' in tool call arguments")
		}
	})

	t.Run("all at once (like reverse proxy buffer)", func(t *testing.T) {
		// Create a test HTTP response recorder
		recorder := httptest.NewRecorder()

		// Create our custom response writer
		rw := newResponseWriter(recorder, true, nil)

		// Write all chunks at once (simulating buffered write)
		allChunks := bytes.Join([][]byte{[]byte(strings.Join(streamingChunks, ""))}, []byte{})
		t.Logf("Writing all %d bytes at once", len(allChunks))

		n, err := rw.Write(allChunks)
		if err != nil {
			t.Fatalf("Failed to write: %v", err)
		}
		t.Logf("Wrote %d bytes", n)

		// Get the result
		result := recorder.Body.String()
		t.Logf("Result length: %d bytes", len(result))

		// Check if XML was detected and converted
		if !strings.Contains(result, "tool_calls") {
			t.Error("Expected 'tool_calls' in result, but not found - XML not converted!")
		}

		if strings.Contains(result, "<function=") {
			t.Error("Found '<function=' in result - XML was not converted!")
		}
	})
}

func TestResponseWriter_WithoutClosingTags(t *testing.T) {
	// Format WITHOUT closing tags (as mentioned by user)
	streamingChunks := []string{
		`data: {"id":"chatcmpl-test2","object":"chat.completion.chunk","created":1762238668,"model":"qwen3-coder-30b-fp8","choices":[{"index":0,"delta":{"role":"assistant","content":""},"logprobs":null,"finish_reason":null}],"prompt_token_ids":null}` + "\n\n",
		`data: {"id":"chatcmpl-test2","object":"chat.completion.chunk","created":1762238668,"model":"qwen3-coder-30b-fp8","choices":[{"index":0,"delta":{"content":"<"},"logprobs":null,"finish_reason":null,"token_ids":null}]}` + "\n\n",
		`data: {"id":"chatcmpl-test2","object":"chat.completion.chunk","created":1762238668,"model":"qwen3-coder-30b-fp8","choices":[{"index":0,"delta":{"content":"function"},"logprobs":null,"finish_reason":null,"token_ids":null}]}` + "\n\n",
		`data: {"id":"chatcmpl-test2","object":"chat.completion.chunk","created":1762238668,"model":"qwen3-coder-30b-fp8","choices":[{"index":0,"delta":{"content":"="},"logprobs":null,"finish_reason":null,"token_ids":null}]}` + "\n\n",
		`data: {"id":"chatcmpl-test2","object":"chat.completion.chunk","created":1762238668,"model":"qwen3-coder-30b-fp8","choices":[{"index":0,"delta":{"content":"ls"},"logprobs":null,"finish_reason":null,"token_ids":null}]}` + "\n\n",
		`data: {"id":"chatcmpl-test2","object":"chat.completion.chunk","created":1762238668,"model":"qwen3-coder-30b-fp8","choices":[{"index":0,"delta":{"content":">"},"logprobs":null,"finish_reason":null,"token_ids":null}]}` + "\n\n",
		`data: {"id":"chatcmpl-test2","object":"chat.completion.chunk","created":1762238668,"model":"qwen3-coder-30b-fp8","choices":[{"index":0,"delta":{"content":" "},"logprobs":null,"finish_reason":null,"token_ids":null}]}` + "\n\n",
		`data: {"id":"chatcmpl-test2","object":"chat.completion.chunk","created":1762238668,"model":"qwen3-coder-30b-fp8","choices":[{"index":0,"delta":{"content":"<"},"logprobs":null,"finish_reason":null,"token_ids":null}]}` + "\n\n",
		`data: {"id":"chatcmpl-test2","object":"chat.completion.chunk","created":1762238668,"model":"qwen3-coder-30b-fp8","choices":[{"index":0,"delta":{"content":"parameter"},"logprobs":null,"finish_reason":null,"token_ids":null}]}` + "\n\n",
		`data: {"id":"chatcmpl-test2","object":"chat.completion.chunk","created":1762238668,"model":"qwen3-coder-30b-fp8","choices":[{"index":0,"delta":{"content":"="},"logprobs":null,"finish_reason":null,"token_ids":null}]}` + "\n\n",
		`data: {"id":"chatcmpl-test2","object":"chat.completion.chunk","created":1762238668,"model":"qwen3-coder-30b-fp8","choices":[{"index":0,"delta":{"content":"path"},"logprobs":null,"finish_reason":null,"token_ids":null}]}` + "\n\n",
		`data: {"id":"chatcmpl-test2","object":"chat.completion.chunk","created":1762238668,"model":"qwen3-coder-30b-fp8","choices":[{"index":0,"delta":{"content":">"},"logprobs":null,"finish_reason":null,"token_ids":null}]}` + "\n\n",
		`data: {"id":"chatcmpl-test2","object":"chat.completion.chunk","created":1762238668,"model":"qwen3-coder-30b-fp8","choices":[{"index":0,"delta":{"content":" "},"logprobs":null,"finish_reason":null,"token_ids":null}]}` + "\n\n",
		`data: {"id":"chatcmpl-test2","object":"chat.completion.chunk","created":1762238668,"model":"qwen3-coder-30b-fp8","choices":[{"index":0,"delta":{"content":"internal"},"logprobs":null,"finish_reason":null,"token_ids":null}]}` + "\n\n",
		`data: {"id":"chatcmpl-test2","object":"chat.completion.chunk","created":1762238668,"model":"qwen3-coder-30b-fp8","choices":[{"index":0,"delta":{"content":"/"},"logprobs":null,"finish_reason":null,"token_ids":null}]}` + "\n\n",
		`data: {"id":"chatcmpl-test2","object":"chat.completion.chunk","created":1762238668,"model":"qwen3-coder-30b-fp8","choices":[{"index":0,"delta":{"content":"agent"},"logprobs":null,"finish_reason":null,"token_ids":null}]}` + "\n\n",
		`data: {"id":"chatcmpl-test2","object":"chat.completion.chunk","created":1762238668,"model":"qwen3-coder-30b-fp8","choices":[{"index":0,"delta":{"content":" "},"logprobs":null,"finish_reason":null,"token_ids":null}]}` + "\n\n",
		`data: {"id":"chatcmpl-test2","object":"chat.completion.chunk","created":1762238668,"model":"qwen3-coder-30b-fp8","choices":[{"index":0,"delta":{"content":"</"},"logprobs":null,"finish_reason":null,"token_ids":null}]}` + "\n\n",
		`data: {"id":"chatcmpl-test2","object":"chat.completion.chunk","created":1762238668,"model":"qwen3-coder-30b-fp8","choices":[{"index":0,"delta":{"content":"tool_call"},"logprobs":null,"finish_reason":null,"token_ids":null}]}` + "\n\n",
		`data: {"id":"chatcmpl-test2","object":"chat.completion.chunk","created":1762238668,"model":"qwen3-coder-30b-fp8","choices":[{"index":0,"delta":{"content":">"},"logprobs":null,"finish_reason":null,"token_ids":null}]}` + "\n\n",
		`data: {"id":"chatcmpl-test2","object":"chat.completion.chunk","created":1762238668,"model":"qwen3-coder-30b-fp8","choices":[{"index":0,"delta":{"content":""},"logprobs":null,"finish_reason":"stop","stop_reason":null,"token_ids":null}]}` + "\n\n",
		`data: [DONE]` + "\n\n",
	}

	t.Run("without closing tags - streaming", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		rw := newResponseWriter(recorder, true, nil)

		for i, chunk := range streamingChunks {
			_, err := rw.Write([]byte(chunk))
			if err != nil {
				t.Fatalf("Failed to write chunk %d: %v", i, err)
			}
		}

		result := recorder.Body.String()
		t.Logf("Result length: %d bytes", len(result))

		if !strings.Contains(result, "tool_calls") {
			t.Error("Expected 'tool_calls' in result - XML not converted!")
			t.Logf("Result preview: %s", result[:min(500, len(result))])
		}

		if strings.Contains(result, "<function=") {
			t.Error("Found '<function=' in result - XML was not converted!")
		}

		if !strings.Contains(result, `"name":"ls"`) {
			t.Error("Expected function name 'ls' in tool call")
		}

		// Arguments are JSON-encoded string
		if !strings.Contains(result, `\"path\":\"internal/agent\"`) {
			t.Error("Expected parameter 'path':'internal/agent' in tool call arguments")
		}
	})
}

func TestResponseWriter_ReasoningContentPassthrough(t *testing.T) {
	// Simulate DeepSeek R1 streaming with reasoning_content
	streamingChunks := []string{
		`data: {"id":"chatcmpl-test3","object":"chat.completion.chunk","created":1762238668,"model":"deepseek-r1-fp8","choices":[{"index":0,"delta":{"role":"assistant","content":""},"logprobs":null,"finish_reason":null}]}` + "\n\n",
		`data: {"id":"chatcmpl-test3","object":"chat.completion.chunk","created":1762238668,"model":"deepseek-r1-fp8","choices":[{"index":0,"delta":{"reasoning_content":"I need to"},"logprobs":null,"finish_reason":null,"token_ids":null}]}` + "\n\n",
		`data: {"id":"chatcmpl-test3","object":"chat.completion.chunk","created":1762238668,"model":"deepseek-r1-fp8","choices":[{"index":0,"delta":{"reasoning_content":" add the"},"logprobs":null,"finish_reason":null,"token_ids":null}]}` + "\n\n",
		`data: {"id":"chatcmpl-test3","object":"chat.completion.chunk","created":1762238668,"model":"deepseek-r1-fp8","choices":[{"index":0,"delta":{"reasoning_content":" numbers"},"logprobs":null,"finish_reason":null,"token_ids":null}]}` + "\n\n",
		`data: {"id":"chatcmpl-test3","object":"chat.completion.chunk","created":1762238668,"model":"deepseek-r1-fp8","choices":[{"index":0,"delta":{"reasoning_content":" 2 and 2."},"logprobs":null,"finish_reason":null,"token_ids":null}]}` + "\n\n",
		`data: {"id":"chatcmpl-test3","object":"chat.completion.chunk","created":1762238668,"model":"deepseek-r1-fp8","choices":[{"index":0,"delta":{"content":"The"},"logprobs":null,"finish_reason":null,"token_ids":null}]}` + "\n\n",
		`data: {"id":"chatcmpl-test3","object":"chat.completion.chunk","created":1762238668,"model":"deepseek-r1-fp8","choices":[{"index":0,"delta":{"content":" answer"},"logprobs":null,"finish_reason":null,"token_ids":null}]}` + "\n\n",
		`data: {"id":"chatcmpl-test3","object":"chat.completion.chunk","created":1762238668,"model":"deepseek-r1-fp8","choices":[{"index":0,"delta":{"content":" is 4."},"logprobs":null,"finish_reason":null,"token_ids":null}]}` + "\n\n",
		`data: {"id":"chatcmpl-test3","object":"chat.completion.chunk","created":1762238668,"model":"deepseek-r1-fp8","choices":[{"index":0,"delta":{"content":""},"logprobs":null,"finish_reason":"stop","stop_reason":null,"token_ids":null}]}` + "\n\n",
		`data: [DONE]` + "\n\n",
	}

	t.Run("reasoning_content is passed through unmodified", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		rw := newResponseWriter(recorder, true, nil)

		// Write chunks one by one (simulating streaming)
		for i, chunk := range streamingChunks {
			_, err := rw.Write([]byte(chunk))
			if err != nil {
				t.Fatalf("Failed to write chunk %d: %v", i, err)
			}
		}

		result := recorder.Body.String()
		t.Logf("Result length: %d bytes", len(result))

		// Verify reasoning_content is present in output
		if !strings.Contains(result, "reasoning_content") {
			t.Error("Expected 'reasoning_content' in result - it was stripped!")
			t.Logf("Result: %s", result)
		}

		// Verify reasoning content values are preserved
		if !strings.Contains(result, "I need to") {
			t.Error("Expected reasoning text 'I need to' in result")
		}

		if !strings.Contains(result, " add the") {
			t.Error("Expected reasoning text ' add the' in result")
		}

		// Verify regular content is also present (check for individual chunks, not concatenated)
		if !strings.Contains(result, `"content":"The"`) || !strings.Contains(result, `"content":" answer"`) {
			t.Error("Expected regular content chunks in result")
			t.Logf("Result preview: %s", result[:min(500, len(result))])
		}

		// Verify [DONE] marker is present
		if !strings.Contains(result, "[DONE]") {
			t.Error("Expected [DONE] marker in result")
		}

		// Ensure XML parsing was NOT triggered by reasoning_content
		if strings.Contains(result, "tool_calls") {
			t.Error("Found 'tool_calls' in result - reasoning_content should not trigger XML parsing!")
		}
	})

	t.Run("reasoning_content does not trigger XML mode", func(t *testing.T) {
		// Mix reasoning_content with text that looks like XML in regular content
		mixedChunks := []string{
			`data: {"id":"chatcmpl-test4","object":"chat.completion.chunk","created":1762238668,"model":"deepseek-r1-fp8","choices":[{"index":0,"delta":{"role":"assistant","content":""},"logprobs":null,"finish_reason":null}]}` + "\n\n",
			`data: {"id":"chatcmpl-test4","object":"chat.completion.chunk","created":1762238668,"model":"deepseek-r1-fp8","choices":[{"index":0,"delta":{"reasoning_content":"Let me think..."},"logprobs":null,"finish_reason":null,"token_ids":null}]}` + "\n\n",
			`data: {"id":"chatcmpl-test4","object":"chat.completion.chunk","created":1762238668,"model":"deepseek-r1-fp8","choices":[{"index":0,"delta":{"content":"Here's some text"},"logprobs":null,"finish_reason":null,"token_ids":null}]}` + "\n\n",
			`data: {"id":"chatcmpl-test4","object":"chat.completion.chunk","created":1762238668,"model":"deepseek-r1-fp8","choices":[{"index":0,"delta":{"content":""},"logprobs":null,"finish_reason":"stop","stop_reason":null,"token_ids":null}]}` + "\n\n",
			`data: [DONE]` + "\n\n",
		}

		recorder := httptest.NewRecorder()
		rw := newResponseWriter(recorder, true, nil)

		for _, chunk := range mixedChunks {
			_, err := rw.Write([]byte(chunk))
			if err != nil {
				t.Fatalf("Failed to write chunk: %v", err)
			}
		}

		result := recorder.Body.String()

		// Verify both reasoning_content and content are present
		if !strings.Contains(result, "reasoning_content") {
			t.Error("Expected 'reasoning_content' in result")
		}

		if !strings.Contains(result, "Let me think...") {
			t.Error("Expected reasoning text 'Let me think...' in result")
		}

		if !strings.Contains(result, "Here's some text") {
			t.Error("Expected regular content 'Here's some text' in result")
		}

		// Ensure all chunks were passed through (not buffered)
		chunkCount := strings.Count(result, "data: {")
		if chunkCount < len(mixedChunks)-1 { // -1 for [DONE]
			t.Errorf("Expected at least %d data chunks, got %d - chunks may have been buffered", len(mixedChunks)-1, chunkCount)
		}
	})
}
