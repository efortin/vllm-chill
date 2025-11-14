# Fake vLLM Server

A lightweight HTTP server that simulates the vLLM API for integration testing without requiring GPUs or large model downloads.

## Features

- **Minimal resource usage**: ~1MB Docker image, <10MB RAM
- **Fast startup**: <1 second
- **vLLM-compatible API**: Implements key endpoints
- **Configurable**: Model name and port via environment variables

## Supported Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/health` | GET | Health check endpoint |
| `/v1/models` | GET | List available models |
| `/v1/chat/completions` | POST | Chat completions API |
| `/v1/completions` | POST | Text completions API |

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `MODEL_NAME` | `test/model` | The underlying model name |
| `SERVED_MODEL_NAME` | `test-model` | The served model name |
| `PORT` | `8000` | Server port |

## Usage

### Docker

```bash
# Build the image
docker build -t vllm-dummy:local .

# Run the container
docker run -p 8000:8000 \
  -e MODEL_NAME="my/model" \
  -e SERVED_MODEL_NAME="my-model" \
  vllm-dummy:local
```

### Kubernetes (k3d)

```bash
# Build and import to k3d
docker build -t vllm-dummy:local .
k3d image import vllm-dummy:local -c your-cluster

# Deploy
kubectl apply -f ../../manifests/ci/vllm-dummy.yaml
```

### Task Automation

```bash
# Setup k3d cluster with fake vLLM
task k3d:setup

# Test fake vLLM endpoints
task k3d:test:vllm-dummy

# Test pause/resume functionality
task k3d:test:pause-resume

# View logs
task k3d:logs:vllm
task k3d:logs:proxy
```

## Testing

```bash
# Health check
curl http://localhost:8000/health

# List models
curl http://localhost:8000/v1/models

# Chat completion
curl http://localhost:8000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "test-model",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'
```

## Example Response

### `/v1/models`
```json
{
  "object": "list",
  "data": [
    {
      "id": "test-model",
      "object": "model",
      "created": 1700000000,
      "owned_by": "vllm",
      "root": "test/model",
      "parent": null
    }
  ]
}
```

### `/v1/chat/completions`
```json
{
  "id": "chatcmpl-123",
  "object": "chat.completion",
  "created": 1700000000,
  "model": "test-model",
  "choices": [{
    "index": 0,
    "message": {
      "role": "assistant",
      "content": "This is a fake response from the mock vLLM server."
    },
    "finish_reason": "stop"
  }],
  "usage": {
    "prompt_tokens": 10,
    "completion_tokens": 15,
    "total_tokens": 25
  }
}
```

## Integration with vllm-chill

The fake vLLM server is designed to work seamlessly with vllm-chill's pause/resume functionality:

1. **Sidecar deployment**: Runs alongside vllm-chill proxy in the same pod
2. **Localhost communication**: Uses 127.0.0.1 for minimal latency
3. **Pause/Resume testing**: Container can be swapped with pause:3.9 image
4. **Health probes**: Responds to readiness/liveness checks

## Architecture

```
┌─────────────────────────────────────┐
│        vllm-test-pod                │
├──────────────────┬──────────────────┤
│  vllm-proxy      │  vllm-dummy       │
│  :8080           │  :8000           │
│  (vllm-chill)    │  (mock server)   │
│                  │                  │
│  ┌─────────────┐ │  ┌─────────────┐│
│  │ Pause/Resume│ │  │ vLLM API    ││
│  │ Logic       │→│→→│ Endpoints   ││
│  └─────────────┘ │  └─────────────┘│
└──────────────────┴──────────────────┘
         ↓
    127.0.0.1:8000
```

## Benefits for Testing

1. **Speed**: No GPU initialization or model loading
2. **Consistency**: Predictable responses for testing
3. **Resource efficiency**: Runs on any machine
4. **CI/CD friendly**: Fast integration tests
5. **Pause/Resume validation**: Test container switching without GPUs

## Limitations

- Does not perform actual LLM inference
- Responses are static mock data
- Does not validate input prompts
- No streaming support (yet)
- No model-specific behavior

For full vLLM testing with real inference, use the actual vLLM container with appropriate GPU resources.
