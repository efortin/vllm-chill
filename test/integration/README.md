# Integration Tests

This directory contains comprehensive E2E integration tests for vllm-chill that run against a real k3d Kubernetes cluster.

## Prerequisites

1. **k3d** installed (for local Kubernetes cluster)
2. **kubectl** configured
3. **Docker** running

## Quick Start

### 1. Setup k3d Cluster

```bash
# Create k3d cluster with all resources
task -t Taskfile.k3d.yml setup
```

This will:
- Create a k3d cluster named `vllm-test`
- Install VLLMModel CRDs
- Deploy test models
- Deploy vllm-dummy (fake vLLM server)
- Deploy vllm-chill proxy in sidecar mode

### 2. Run E2E Integration Tests

```bash
# Run E2E tests
task -t Taskfile.k3d.yml test:e2e

# Run with coverage
task -t Taskfile.k3d.yml test:e2e:coverage
```

### 3. Run All Tests with Combined Coverage

```bash
# Run unit tests + kubernetes integration tests + E2E tests
task -t Taskfile.k3d.yml test:coverage
```

This generates a combined coverage report including:
- Unit tests (all packages)
- Kubernetes integration tests
- E2E integration tests

## Test Coverage

The integration tests cover:

### Proxy Functionality
- ✅ Metrics endpoint (`/metrics`)
- ✅ Activity tracking

### vLLM API Proxying
- ✅ `/v1/models` - List available models
- ✅ `/v1/chat/completions` - Chat completion requests
- ✅ `/v1/completions` - Text completion requests

### Error Handling
- ✅ Invalid JSON handling
- ✅ Non-existent endpoints
- ✅ Concurrent request handling

### Real-World Scenarios
- ✅ Multiple concurrent requests
- ✅ Activity time updates
- ✅ Request proxying to vLLM backend

## Architecture

The tests use:
- **k3d cluster**: Lightweight Kubernetes for testing
- **vllm-dummy**: Fake vLLM server that simulates API responses
- **vllm-chill proxy**: Real proxy running in sidecar mode
- **Port-forward**: Tests connect to proxy via kubectl port-forward

## Test Structure

```
test/integration/
├── README.md           # This file
├── suite_test.go       # Ginkgo test suite setup
└── k3d_test.go         # E2E integration tests
```

## Running Individual Test Suites

```bash
# Only Kubernetes integration tests
task -t Taskfile.k3d.yml test:go

# Only E2E integration tests
task -t Taskfile.k3d.yml test:e2e

# All k3d validation tests (CRD, RBAC, etc.)
task -t Taskfile.k3d.yml test
```

## Debugging

### View Proxy Logs
```bash
task -t Taskfile.k3d.yml logs:proxy
```

### View vLLM Dummy Logs
```bash
task -t Taskfile.k3d.yml logs:vllm
```

### Check Pod Status
```bash
kubectl get pods -n vllm --context k3d-vllm-test
```

### Manual Port-Forward (for debugging)
```bash
kubectl port-forward -n vllm vllm-test-pod 8080:8080 --context k3d-vllm-test
```

Then test endpoints manually:
```bash
curl http://localhost:8080/proxy/version
curl http://localhost:8080/v1/models
```

## Cleanup

```bash
# Delete k3d cluster
task -t Taskfile.k3d.yml teardown
```

## Coverage Goals

With integration tests, we achieve comprehensive coverage of:
- Proxy request handling (~60%+ of pkg/proxy)
- HTTP server functionality
- Activity tracking
- Metrics recording
- Real Kubernetes operations

Combined with unit tests, total project coverage reaches **70%+** (compared to 48% with unit tests alone).

## CI/CD Integration

These tests can be run in CI/CD pipelines:

```yaml
# Example GitHub Actions
- name: Setup k3d
  run: task -t Taskfile.k3d.yml setup

- name: Run Integration Tests
  run: task -t Taskfile.k3d.yml test:coverage

- name: Upload Coverage
  uses: codecov/codecov-action@v3
  with:
    files: ./combined-coverage.out
```

## Notes

- Tests use `//+build integration` tag to separate from unit tests
- Port-forward is automatically managed by BeforeEach/AfterEach
- Tests wait for proxy to be ready before executing
- Concurrent test execution is supported
- Each test is independent and can run in any order
