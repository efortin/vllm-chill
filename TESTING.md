# Testing Guide

This document describes how to run tests for vllm-chill, including unit tests and integration tests with k3d.

## Prerequisites

- Go 1.23 or later
- [Task](https://taskfile.dev/) (optional, but recommended)
- [k3d](https://k3d.io/) for integration tests
- kubectl configured

## Quick Start

### Using Task (Recommended)

```bash
# Run unit tests only
task test

# Run integration tests (automatically sets up k3d)
task test:integration

# Run all tests (unit + integration)
task test:all

# Generate coverage reports
task test:coverage:all
```

### Using Go directly

```bash
# Unit tests
go test ./... -v

# Integration tests (requires k3d cluster setup first)
go test -v -tags=integration ./pkg/kubernetes -timeout 5m
```

## Test Types

### Unit Tests

Unit tests test individual functions and components in isolation using mocks and fakes.

**Location**: `*_test.go` files without build tags

**Run with**:
```bash
task test
# or
go test ./... -v
```

**Current Coverage**: ~48.5%

**Key packages tested**:
- `pkg/models` - 100% coverage
- `pkg/operation` - 92.6% coverage
- `pkg/parser` - 82.0% coverage
- `pkg/kubernetes` - 68.3% coverage (unit tests)
- `pkg/proxy` - 35.1% coverage
- `pkg/stats` - 32.8% coverage
- `pkg/scaling` - 19.6% coverage

### Integration Tests

Integration tests run against a real Kubernetes cluster (k3d) and test the complete integration of components.

**Location**: `pkg/kubernetes/integration_test.go`

**Build Tag**: `//go:build integration`

**Run with**:
```bash
task test:integration
# or manually:
task k3d:setup
go test -v -tags=integration ./pkg/kubernetes -timeout 5m
```

**What they test**:
- Creating and deleting pods in Kubernetes
- Service creation and management
- CRD operations (VLLMModel CRUD)
- Pod configuration verification
- Waiting for pod readiness

## Setting up k3d Cluster

The integration tests require a k3d cluster with the VLLMModel CRD installed.

### Automatic Setup (Recommended)

```bash
task k3d:setup
```

This will:
1. Install k3d if not present
2. Create a k3d cluster named `vllm-test`
3. Create the `vllm` namespace
4. Install the VLLMModel CRD
5. Create test model resources

### Manual Setup

```bash
# Create cluster
k3d cluster create vllm-test --agents 1 --wait --timeout 2m

# Create namespace
kubectl create namespace vllm

# Install CRD
kubectl apply -f manifests/crds/vllmmodel.yaml

# Wait for CRD to be ready
kubectl wait --for condition=established --timeout=60s crd/models.vllm.sir-alfred.io

# Create test models
kubectl apply -f manifests/examples/qwen3-coder-model.yaml
kubectl apply -f manifests/examples/deepseek-r1-model.yaml
```

### Teardown

```bash
task k3d:teardown
# or
k3d cluster delete vllm-test
```

## Coverage Reports

### Unit Test Coverage

```bash
task test
# or
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out -o coverage.html
```

Open `coverage.html` in your browser to see the coverage report.

### Integration Test Coverage

```bash
task test:integration:coverage
```

This generates `integration-coverage.html` showing which integration tests cover which code paths.

### Combined Coverage

```bash
task test:coverage:all
```

This runs both unit and integration tests and merges the coverage reports into `combined-coverage.html`.

## CI/CD Integration

The GitHub Actions workflow (`.github/workflows/k3d.yml`) automatically:
1. Sets up a k3d cluster using `task k3d:setup`
2. Installs the CRD
3. Runs integration tests
4. Deploys RBAC from `manifests/ci/rbac.yaml`
5. Validates CRD schemas
6. Tests RBAC permissions

**Trigger**: Push to `main` or pull request

**Task Installation**: The CI uses `sudo snap install task --classic` to install Task

## Writing Tests

### Unit Test Example

```go
func TestMyFunction(t *testing.T) {
    // Arrange
    input := "test"

    // Act
    result := MyFunction(input)

    // Assert
    assert.Equal(t, "expected", result)
}
```

### Integration Test Example

```go
//go:build integration
// +build integration

package kubernetes

func TestIntegration_MyFeature(t *testing.T) {
    clientset, dynamicClient := setupK8sClients(t)

    // Test against real Kubernetes
    // ...
}
```

**Important**: Integration tests must have the `//go:build integration` build tag at the top of the file.

## Troubleshooting

### k3d cluster won't start

```bash
# Delete and recreate
k3d cluster delete vllm-test
task k3d:setup
```

### CRD not established

```bash
# Check CRD status
kubectl get crd models.vllm.sir-alfred.io

# Reapply CRD
kubectl apply -f manifests/crds/vllmmodel.yaml
```

### Integration tests timeout

Increase the timeout:
```bash
go test -v -tags=integration ./pkg/kubernetes -timeout 10m
```

### Cannot connect to k3d cluster

```bash
# Ensure k3d cluster is running
k3d cluster list

# Get kubeconfig
k3d kubeconfig get vllm-test

# Test connection
kubectl cluster-info
```

## Test Organization

```
vllm-chill/
├── pkg/
│   ├── kubernetes/
│   │   ├── *_test.go          # Unit tests
│   │   └── integration_test.go # Integration tests (build tag)
│   ├── models/
│   │   └── *_test.go          # Unit tests
│   ├── proxy/
│   │   └── *_test.go          # Unit tests
│   └── ...
├── Taskfile.yml               # Task definitions
└── TESTING.md                 # This file
```

## Available Task Commands

```bash
task --list
```

Output:
```
task: Available tasks for this project:
* build:                     Build binary
* check:                     Run full checks (lint, test, build)
* dev:                       Run tests and build locally
* docker:                    Build and push multi-arch Docker image
* docker:dev:                Build and push Docker image with :dev tag
* k3d:setup:                 Create k3d cluster with CRDs and test models
* k3d:teardown:              Delete k3d cluster
* lint:                      Run golangci-lint
* lint:fix:                  Run golangci-lint with auto-fix
* run:                       Run locally (requires kubeconfig)
* test:                      Run all tests with coverage
* test:all:                  Run all tests (unit + integration)
* test:coverage:all:         Run all tests with combined coverage
* test:integration:          Run integration tests (requires k3d cluster)
* test:integration:coverage: Run integration tests with coverage report
```
