# vLLM-Chill

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

Lightweight Go proxy that automatically scales vLLM deployments to zero when idle. Configure models via Kubernetes CRDs and select which model to run via environment variable.

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

- **Scale to Zero**: Automatically scales to 0 replicas after configurable idle timeout
- **Automatic Wake**: Scales to 1 replica on first request, buffers connections during startup
- **CRD-Based Model Configuration**: Define models as cluster-scoped Kubernetes resources
- **Static Model Selection**: Configure which model to run via `MODEL_ID` environment variable
- **Automatic Resource Management**: Creates and manages vLLM Deployment, Service, and ConfigMap
- **Prometheus Metrics**: Always enabled at `/metrics` endpoint
- **Lightweight**: ~2MB Docker image, <50MB RAM
- **Multi-arch**: linux/amd64 and linux/arm64

## Why This Exists

vLLM sleep mode is incompatible with key optimizations and crashes on wake. KEDA HTTP Add-on has hardcoded timeouts too short for vLLM startup. This proxy runs as a separate deployment (not a sidecar) so it stays alive when vLLM scales to zero, enabling true scale-to-zero with automatic wake.

## Architecture

```
Internet → Ingress → vllm-chill-svc → vllm-chill (proxy)
                                              ↓
                                           vllm-svc → vllm (pod)
```

1. Receives all HTTP requests
2. Scales vLLM from 0→1 if needed, buffers connections
3. Proxies requests to vLLM
4. Scales to 0 after idle timeout

## Quick Start

See [QUICKSTART.md](QUICKSTART.md) for installation instructions.

## Documentation

- [QUICKSTART.md](QUICKSTART.md) - Installation and basic usage
- [docs/CRD_GUIDE.md](docs/CRD_GUIDE.md) - VLLMModel CRD reference
- [docs/METRICS.md](docs/METRICS.md) - Prometheus metrics
- [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) - Design details

## Development

```bash
# Run tests
task test

# Build
task build

# Run locally
go run ./cmd/autoscaler serve
```

## License

MIT
