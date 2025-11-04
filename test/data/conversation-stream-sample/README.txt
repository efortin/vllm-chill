# vLLM-Chill E2E XML→JSON Tool Call Test Set

This archive contains **30 streaming conversation examples** where the assistant emits a *bad* XML tool call across OpenAI **SSE-like chunks** (`generate_stream`), and an `expected` object representing the normalized **OpenAI JSON `tool_calls`** your parser should return.

## File format

Each `conversation-XX.json` has:
- `title`: short scenario description
- `model`: model name used (qwen3-coder-30b-fp8)
- `generate_stream`: array of objects with a `data` field that contains an OpenAI-compatible `chat.completion.chunk` JSON string.
  - The chunks gradually stream text and the <tool_call ...> XML (with intentional inconsistencies).
  - The last chunk has `"finish_reason": "stop"`.
- `expected`: normalized OpenAI tool-call JSON your E2E should compare against.

## Notes

- XML uses various malformed patterns: stray spaces, attributes formatting, CDATA, comments, namespaces, empty args, etc.
- Target functions include: k8s.scale, vllm.sleep, vllm.wake, metrics.query, http.get, docker.start/stop, git.clone, k8s.rollout_restart.
- Arguments are already the canonical JSON expected after your parser runs.

Good luck & happy testing!
Generated: 2025-11-04T09:11:56.053041Z
