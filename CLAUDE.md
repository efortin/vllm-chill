# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

This repository implements vLLM-Chill, a Kubernetes autoscaler proxy for vLLM that enables scale-to-zero functionality for large language models. It acts as a proxy that automatically scales vLLM deployments to zero when idle and wakes them up on incoming requests.

## Key Components

### Architecture
The system uses a separate proxy deployment that stays active to detect requests and scale vLLM pods, enabling true scale-to-zero. The proxy handles:
- Automatic scaling of vLLM from 0 → 1 on request
- Buffering connections during wake-up (max 2 minutes)
- Tracking activity and scaling down after idle timeout
- Proxying requests to vLLM backend
- Dynamic model switching without redeployment

### Core Files
- `cmd/autoscaler/main.go` - Entry point
- `pkg/proxy/autoscaler.go` - Main proxy and scaling logic
- `pkg/kubernetes/` - Kubernetes integration for managing vLLM pods
- `pkg/apis/vllm/v1alpha1/` - VLLMModel CRD definitions

## Development Setup

### Prerequisites
- Kubernetes cluster with kubectl access
- RBAC permissions to create CRDs, Deployments, Services, ConfigMaps
- Go 1.24+

### Building
```bash
# Build the binary
go build -o vllm-chill ./cmd/autoscaler

# Build Docker image
docker build -t vllm-chill:latest .
```

### Running Tests
```bash
# Run unit tests
go test ./...

# Run integration tests
go test -v ./pkg/kubernetes/integration_test.go

# Run a single test
go test -v ./pkg/proxy -run TestAutoScaler
```

### Linting
```bash
# Run golangci-lint (if installed)
golangci-lint run

# Format code
go fmt ./...
```

## Model Management

### VLLMModel CRD
All model configurations are stored as Kubernetes Custom Resource Definitions (CRDs). The CRD includes:
- Model identification (`modelName`, `servedModelName`)
- Parsing configuration (`toolCallParser`, `reasoningParser`)
- vLLM runtime parameters (all required fields)

### Dynamic Switching
Models can be switched dynamically via API without redeployment:
- GET `/proxy/models/available` - List all available models
- GET `/proxy/models/running` - Get currently active model
- POST `/proxy/models/switch` - Switch to a different model

## API Endpoints

### Proxy Endpoints
- `GET /health` - Health check
- `GET /readyz` - Readiness probe
- `GET /proxy/metrics` - Combined vLLM + proxy metrics
- `GET /proxy/version` - Version information
- `POST /proxy/models/switch` - Switch active model
- `GET /proxy/models/available` - List available models
- `GET /proxy/models/running` - Get running model info

### Model Endpoints
- `GET /v1/models` - List available models (OpenAI format)
- `POST /v1/chat/completions` - Chat completions (OpenAI format)
- `POST /v1/messages` - Chat completions (Anthropic format)

## Configuration

### Environment Variables
- `VLLM_NAMESPACE` - Kubernetes namespace (default: "vllm")
- `VLLM_DEPLOYMENT` - Deployment name (default: "vllm")
- `VLLM_CONFIGMAP` - ConfigMap name (default: "vllm-config")
- `MODEL_ID` - Model ID to load from VLLMModel CRD (required)
- `IDLE_TIMEOUT` - Idle timeout before scaling to 0 (default: "5m")
- `PORT` - HTTP server port (default: "8080")
- `GPU_COUNT` - Number of GPUs to allocate
- `CPU_OFFLOAD_GB` - CPU offload in GB
- `PUBLIC_ENDPOINT` - Public-facing endpoint URL

## Key Design Patterns

### Scale-to-Zero Architecture
The system uses a separate proxy deployment that stays active to detect requests and scale vLLM pods, enabling true scale-to-zero. This is critical because:
- A sidecar approach would terminate along with vLLM when scaled to 0
- The proxy must remain active to intercept requests and trigger scaling
- Connection buffering prevents timeouts during vLLM startup (max 2 minutes)

### Anthropic API Compatibility
The proxy transforms Anthropic Messages API format to OpenAI format for vLLM compatibility:
- Converts `/v1/messages` to `/v1/chat/completions`
- Transforms tool calls between formats
- Handles streaming responses properly
- Maintains token counting for billing purposes

### Kubernetes Integration
The proxy integrates deeply with Kubernetes:
- Reads VLLMModel CRDs directly for configuration
- Creates/deletes vLLM pods using Kubernetes API
- Verifies pod configurations to detect drift
- Watches CRDs for configuration changes

### Model Switching
Dynamic model switching works by:
- Reading model configuration from CRDs at startup
- Allowing runtime model switching via API
- Stopping current pod and starting new one with different model
- Maintaining seamless operation during transitions