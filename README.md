# vLLM AutoScaler

[![Docker Hub](https://img.shields.io/docker/pulls/yourusername/vllm-chill)](https://hub.docker.com/r/yourusername/vllm-chill)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

A lightweight Go proxy that automatically scales vLLM deployments to zero when idle and wakes them up on incoming requests.

## Motivation

**The Problem**: vLLM keeps GPUs fully allocated even when idle, wasting:
- GPU memory (16-22GB per GPU)
- Power (~300W per GPU)
- Compute resources that could be used by other workloads

For home labs and small clusters with limited GPUs, this is costly when vLLM sits idle for hours.

**Why Existing Solutions Don't Work**:
- **vLLM sleep mode**: Incompatible with optimizations (`--enable-prefix-caching`, `--enable-chunked-prefill`), crashes on wake with CUDA errors
- **KEDA HTTP Add-on**: Hardcoded 20s timeout (vLLM takes ~60s to start), complex setup
- **CronJob scale-down**: No automatic wake, race conditions

**This Solution**: A simple Go proxy that enables true scale-to-zero (100% GPU freed) while keeping all vLLM optimizations. ~60s cold start is acceptable for home labs and low-traffic scenarios.

**Real Results**: In production, vLLM is idle 70% of the time → 2× RTX 3090 GPUs completely freed when not in use.

## Features

- ✅ **Scale to Zero**: Automatically scales to 0 replicas after 5 minutes of inactivity
- ✅ **Automatic Wake**: Scales to 1 replica on first request
- ✅ **Connection Buffering**: Keeps connections open during scale-up (up to 2 minutes)
- ✅ **Ultra Lightweight**: ~2MB Docker image (FROM scratch), <50MB RAM usage
- ✅ **Simple**: Single Go binary, no external dependencies (just k8s client-go)
- ✅ **CLI with Cobra**: Clean command-line interface
- ✅ **Multi-arch**: Supports linux/amd64 and linux/arm64

## Why a separate proxy and not a sidecar?

### ❌ Sidecar impossible for scale-to-zero
A **sidecar** shares the same pod as vLLM. If we scale to 0:
- The entire pod is terminated
- The sidecar autoscaler is also terminated
- No one left to detect requests and wake vLLM
- **Scale-to-zero impossible**

### ✅ Separate proxy: the right solution
The proxy runs in its **own deployment**:
- Stays active even when vLLM is at 0 replicas
- Detects incoming requests
- Scales vLLM from 0 → 1 automatically
- Buffers connections during wake-up
- Scales vLLM from 1 → 0 after inactivity

See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for detailed explanation.

## Why not vLLM sleep mode or KEDA?

**vLLM sleep mode**:
- ❌ Incompatible with `--enable-prefix-caching`, `--enable-chunked-prefill`, `--enable-auto-tool-choice`
- ❌ Crashes on wake with CUDA errors
- ❌ Not true scale-to-zero (pod still runs, consuming CPU/memory)

**KEDA HTTP Add-on**:
- ❌ Hardcoded 20s timeout (vLLM takes ~60s to start)
- ❌ Complex: Helm charts, CRDs, multiple namespaces
- ❌ No fine-grained control over wake timeouts

## Architecture

```
Internet → Ingress → vllm-chill-svc → vllm-chill (proxy)
                                              ↓
                                           vllm-svc → vllm (pod)
```

The proxy:
1. Receives all HTTP requests
2. Checks if vLLM is scaled to 0
3. If yes: scales to 1 and waits for pod to be Ready (max 2min)
4. Proxies the request to vLLM
5. Tracks activity
6. Scales to 0 after 5 minutes of inactivity

## Installation

### Prerequisites

- Go 1.23+
- Docker
- Kubernetes cluster with kubectl access
- [Task](https://taskfile.dev/) (optional, but recommended)

### Quick Start with Docker Hub

Pull and run the latest image:
```bash
docker pull yourusername/vllm-chill:latest

docker run -v ~/.kube:/root/.kube \
  yourusername/vllm-chill:latest \
  serve --namespace ai-apps --idle-timeout 5m
```

### Development

Using Task:
```bash
# Development (test + build)
task dev

# Run tests with coverage
task test

# Build and push Docker image to Docker Hub
IMAGE=yourusername/vllm-chill TAG=v1.0.0 task docker

# Run locally
task run
```

Or using Go directly:
```bash
# Install and run
go install github.com/yourusername/vllm-chill/cmd/autoscaler@latest
autoscaler serve

# Or run from source
go run ./cmd/autoscaler serve --namespace ai-apps --idle-timeout 10m
```

## Configuration

### Command-line Flags

```bash
vllm-chill serve [flags]

Flags:
  --namespace string      Kubernetes namespace (default "ai-apps")
  --deployment string     Deployment name (default "vllm")
  --target-host string    Target service host (default "vllm-svc")
  --target-port string    Target service port (default "80")
  --idle-timeout string   Idle timeout before scaling to 0 (default "5m")
  --port string           HTTP server port (default "8080")
```

### Environment Variables

All flags can be set via environment variables:

- `VLLM_NAMESPACE`
- `VLLM_DEPLOYMENT`
- `VLLM_TARGET`
- `VLLM_PORT`
- `IDLE_TIMEOUT`
- `PORT`

## Monitoring

```bash
# View logs
kubectl -n ai-apps logs -f deployment/vllm-chill

# Check health
curl http://localhost:8080/health

# Get status
kubectl -n ai-apps get pods -l app=vllm-chill
```

## Project Structure

```
.
├── cmd/
│   └── autoscaler/         # Main entry point
│       ├── cmd/            # Cobra commands
│       └── main.go
├── pkg/
│   └── proxy/              # Core proxy logic
│       ├── autoscaler.go
│       ├── autoscaler_test.go
│       ├── config.go
│       └── config_test.go
├── docs/
│   ├── ARCHITECTURE.md     # Architecture decisions
│   └── DOCKER_BUILD.md     # Multi-arch build strategy
├── .github/
│   └── workflows/          # GitHub Actions CI/CD
├── Dockerfile              # Multi-stage build (manual)
├── Dockerfile.goreleaser   # Minimal build (releases)
├── .goreleaser.yml         # GoReleaser configuration
├── Taskfile.yml            # Task automation (4 tasks)
└── README.md
```

## Multi-Architecture Builds

Go compiles natively for all architectures.

- **Dockerfile**: Standard multi-stage build for manual testing
- **Dockerfile.goreleaser**: Uses pre-compiled binaries (10x faster)
- GoReleaser cross-compiles for amd64/arm64 without emulation

See [docs/DOCKER_BUILD.md](docs/DOCKER_BUILD.md) for technical details.

**Performance**:
- Manual build: ~2-3 minutes
- GoReleaser (with emulation): ~5-8 minutes per arch
- GoReleaser (native): ~30 seconds per arch 🚀

## CI/CD

This project uses GitHub Actions for:
- **Tests**: Runs on every push/PR
- **Docker**: Builds and pushes to Docker Hub on main branch
- **Releases**: Creates GitHub releases and Docker images on tags (via GoReleaser)

To create a release:
```bash
git tag -a v1.0.0 -m "Release v1.0.0"
git push origin v1.0.0
```

## Contributing

We welcome contributions! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for details.

## License

MIT
