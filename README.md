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
- **Dynamic Model Switching**: Switch between models via API without redeploying
- **Direct CRD Reading**: Model config read directly from CRD (no ConfigMap duplication)
- **Automatic Resource Management**: Creates and manages vLLM Pod and Service
- **Prometheus Metrics**: Always enabled at `/proxy/metrics` endpoint
- **Lightweight**: ~2MB Docker image, <50MB RAM
- **Architecture**: linux/amd64 with optional GPU stats support (NVML)

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

## Monitoring

vLLM-Chill exposes comprehensive Prometheus metrics at `/proxy/metrics`:

**Operational Metrics:**
- `vllm_chill_requests_total` - Total requests by method/path/status
- `vllm_chill_request_duration_seconds` - Request latency histograms
- `vllm_chill_request_payload_bytes` / `vllm_chill_response_payload_bytes` - Payload sizes
- `vllm_chill_managed_operations_total` - Model switch operations (success/failure)
- `vllm_chill_managed_operation_duration_seconds` - Model switch duration

**Scaling Metrics:**
- `vllm_chill_scale_operations_total` - Scale up/down operations
- `vllm_chill_scale_operation_duration_seconds` - Scaling duration
- `vllm_chill_current_replicas` - Current vLLM replica count (0 or 1)
- `vllm_chill_idle_time_seconds` - Time since last activity

**vLLM Lifecycle:**
- `vllm_chill_vllm_state` - Current state (0=stopped, 1=starting, 2=running, 3=stopping)
- `vllm_chill_vllm_startup_duration_seconds` - Cold start time
- `vllm_chill_vllm_shutdown_duration_seconds` - Shutdown time
- `vllm_chill_current_model` - Currently loaded model (1 if loaded, 0 otherwise)

**Performance:**
- `vllm_chill_proxy_latency_seconds` - Overhead added by proxy
- `vllm_chill_xml_parsing_total` - XML tool call parsing (for tool-enabled models)
- `vllm_chill_xml_tool_calls_detected_total` - Total tool calls detected

See [docs/METRICS.md](docs/METRICS.md) for detailed metric descriptions and Grafana dashboard examples.

## Documentation

- [QUICKSTART.md](QUICKSTART.md) - Installation and basic usage
- [docs/MODEL_MANAGEMENT.md](docs/MODEL_MANAGEMENT.md) - Dynamic model switching guide
- [docs/METRICS.md](docs/METRICS.md) - Prometheus metrics reference
- [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) - Design details

## Development

```bash
# Run tests
task test

# Run linter
task lint

# Build
task build

# Run locally
go run ./cmd/autoscaler serve
```

## License

MIT
