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
- ✅ **CRD-Based Model Switching**: Define models as Kubernetes resources, switch dynamically (see [docs/CRD_GUIDE.md](docs/CRD_GUIDE.md))
- ✅ **Automatic K8s Resource Management**: Creates and manages Deployment, Service, and ConfigMap when model switching is enabled (see [docs/K8S_RESOURCE_MANAGEMENT.md](docs/K8S_RESOURCE_MANAGEMENT.md))
- ✅ **User-Friendly Loading**: Returns helpful messages during model switches
- ✅ **Prometheus Metrics**: Comprehensive metrics for monitoring (see [docs/METRICS.md](docs/METRICS.md))
- ✅ **Output Logging**: Optional logging of generated output for debugging
- ✅ **Connection Buffering**: Keeps connections open during scale-up (up to 2 minutes)
- ✅ **Ultra Lightweight**: ~2MB Docker image (FROM scratch), <50MB RAM usage
- ✅ **High Performance**: Negligible proxy overhead (<0.01% of inference time) (see [docs/PERFORMANCE.md](docs/PERFORMANCE.md))
- ✅ **Simple**: Single Go binary, no external dependencies (just k8s client-go)
- ✅ **Validated**: CRD enforces valid vLLM parameters (dtype, parsers, etc.)
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
  --namespace string              Kubernetes namespace (default "ai-apps")
  --deployment string             Deployment name (default "vllm")
  --configmap string              ConfigMap name for model configuration (default "vllm-config")
  --target-host string            Target service host (default "vllm-svc")
  --target-port string            Target service port (default "80")
  --idle-timeout string           Idle timeout before scaling to 0 (default "5m")
  --model-switch-timeout string   Timeout for model switching (default "5m")
  --port string                   HTTP server port (default "8080")
  --enable-model-switch           Enable dynamic model switching (default false)
  --enable-metrics                Enable Prometheus metrics endpoint (default true)
  --log-output                    Log response bodies (use with caution)
```

For details on dynamic model switching, see [docs/CRD_GUIDE.md](docs/CRD_GUIDE.md).

### Environment Variables

All flags can be set via environment variables:

- `VLLM_NAMESPACE`
- `VLLM_DEPLOYMENT`
- `VLLM_CONFIGMAP`
- `VLLM_TARGET`
- `VLLM_PORT`
- `IDLE_TIMEOUT`
- `MODEL_SWITCH_TIMEOUT`
- `PORT`
- `ENABLE_MODEL_SWITCH`
- `ENABLE_METRICS`
- `LOG_OUTPUT`

## Monitoring

### Health Checks

```bash
# View logs
kubectl -n ai-apps logs -f deployment/vllm-chill

# Check health
curl http://localhost:8080/health

# Get status
kubectl -n ai-apps get pods -l app=vllm-chill
```

### Prometheus Metrics

vLLM-Chill exposes comprehensive metrics at `/metrics`:

```bash
# View metrics
curl http://localhost:8080/metrics
```

**Available metrics:**
- Request count, latency, payload size
- Model switch operations and duration
- Scale operations and duration
- Current state (replicas, idle time, loaded model)

See [docs/METRICS.md](docs/METRICS.md) for complete documentation.

**Quick Grafana queries:**

```promql
# Request rate
rate(vllm_chill_requests_total[5m])

# Request latency p95
histogram_quantile(0.95, rate(vllm_chill_request_duration_seconds_bucket[5m]))

# Model switch success rate
sum(rate(vllm_chill_model_switches_total{status="success"}[5m])) / sum(rate(vllm_chill_model_switches_total[5m]))
```

## Project Structure

```
.
├── cmd/
│   └── autoscaler/                # Main entry point
│       ├── cmd/                   # Cobra commands
│       └── main.go
├── pkg/
│   └── proxy/                     # Core proxy logic
│       ├── autoscaler.go          # Main autoscaler logic
│       ├── config.go              # Configuration
│       ├── crd_client.go          # CRD client for model definitions
│       ├── k8s_manager.go         # Kubernetes resource management
│       ├── models.go              # Model configuration structures
│       ├── model_switcher.go     # Model switching logic
│       ├── metrics.go             # Prometheus metrics
│       ├── response_writer.go     # HTTP response capture
│       └── *_test.go              # Tests
├── docs/
│   ├── ARCHITECTURE.md            # Architecture decisions
│   ├── CRD_GUIDE.md               # Model CRD documentation
│   ├── K8S_RESOURCE_MANAGEMENT.md # Resource management
│   ├── METRICS.md                 # Prometheus metrics guide
│   ├── PERFORMANCE.md             # Performance analysis
│   └── DOCKER_BUILD.md            # Multi-arch build strategy
├── examples/
│   └── kubernetes-with-model-switching.yaml
├── .github/
│   └── workflows/                 # GitHub Actions CI/CD
├── Dockerfile                     # Multi-stage build (manual)
├── Dockerfile.goreleaser          # Minimal build (releases)
├── .goreleaser.yml                # GoReleaser configuration
├── Taskfile.yml                   # Task automation
└── README.md
```

The codebase is organized into logical modules:
- **cmd/autoscaler**: CLI entry point
- **pkg/proxy**: Core functionality (autoscaling, model switching, metrics)
- **docs**: Comprehensive documentation
- **examples**: Sample Kubernetes manifests

## Multi-Architecture Builds

Go compiles natively for all architectures.

- **Dockerfile**: Standard multi-stage build for manual testing
- **Dockerfile.goreleaser**: Uses pre-compiled binaries (10x faster)
- GoReleaser cross-compiles for amd64/arm64 without emulation

See [docs/DOCKER_BUILD.md](docs/DOCKER_BUILD.md) for technical details.

**Performance**:
- Manual build: ~2-3 minutes
- GoReleaser (with emulation): ~5-8 minutes per arch
- GoReleaser (native): ~30 seconds per arch

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

We welcome contributions! Please see [docs/CONTRIBUTING.md](docs/CONTRIBUTING.md) for details.

## License

MIT
