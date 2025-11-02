# Changelog

## [Unreleased] - 2025-01-XX

### Changed - BREAKING CHANGES

#### Renaming: "Model Switch" → "Managed Mode"

All references to "model switch" have been renamed to "managed" to better reflect that the system manages the entire deployment lifecycle.

**Flag Changes:**
- `--enable-model-switch` → `--enable-managed`
- `--model-switch-timeout` → `--managed-timeout`
- `ENABLE_MODEL_SWITCH` → `ENABLE_MANAGED`
- `MODEL_SWITCH_TIMEOUT` → `MANAGED_TIMEOUT`

**Migration:**
```bash
# Old
./bin/vllm-chill serve --enable-model-switch --model-switch-timeout=5m

# New
./bin/vllm-chill serve --enable-managed --managed-timeout=5m
```

**Metrics Changes:**
- `vllm_chill_model_switches_total` → `vllm_chill_managed_operations_total`
- `vllm_chill_model_switch_duration_seconds` → `vllm_chill_managed_operation_duration_seconds`

### Added

#### Full Production-Ready vLLM Deployment

When `--enable-managed` is enabled, vLLM-Chill creates a complete vLLM deployment with:

**Volumes:**
- HF cache: `/home/manu/.cache/huggingface` (hostPath)
- vLLM compile cache: `/home/manu/.cache/vllm-compile` (hostPath)
- Shared memory: 16Gi for FlashAttention (emptyDir)

**Health Probes:**
- Startup: 130s max (10s + 24×5s)
- Readiness: 125s max (5s + 12×10s)
- Liveness: 150s max (60s + 3×30s)

**Resources:**
- Memory: 16Gi requests, 32Gi limits
- GPU: 2× nvidia.com/gpu

**Optimizations:**
- `TORCH_CUDA_ARCH_LIST=8.6`: RTX 3090 architecture
- Persistent torch compile cache
- HF transfer acceleration
- OpenMP threading (16 threads)

**Service:**
- HTTP port: 80 → 8000
- Metrics port: 8001 → 8001

#### Prometheus Metrics

Comprehensive metrics at `/metrics`:

- **Requests**: count, latency, payload size
- **Managed ops**: model switch count and duration
- **Scaling**: scale up/down operations
- **State**: replicas, idle time, current model

See `docs/METRICS.md` for complete documentation.

#### Response Logging

```bash
./bin/vllm-chill serve --log-output
```

Logs full response bodies for debugging (use with caution).

#### Linting Support

Added golangci-lint configuration and tasks:

```bash
task lint          # Run linter
task lint:fix      # Run linter with auto-fix
task check         # Full check (lint + test + build)
```

### Documentation

- `docs/METRICS.md`: Prometheus metrics guide
- `docs/PERFORMANCE.md`: HTTP proxy performance analysis
- `docs/ARCHITECTURE.md`: Architecture documentation
- Updated README with all new features

## Previous Updates

### K8s Resource Management (Pre-rename)

## New Features

### 1. Automatic Kubernetes Resource Management

**File**: `pkg/proxy/k8s_manager.go` (NEW)

When model switching is enabled, vllm-chill now automatically:
- Creates and manages the vLLM Deployment
- Creates and manages the vLLM Service
- Creates and manages the ConfigMap with model configuration

**Key Functions**:
- `EnsureVLLMResources()`: Ensures all resources exist
- `ensureConfigMap()`: Creates/updates ConfigMap
- `ensureService()`: Creates Service if not exists
- `ensureDeployment()`: Creates Deployment if not exists
- `buildPodSpec()`: Builds vLLM pod specification
- `buildEnvVars()`: Builds environment variables from ConfigMap

**Integration**:
- Integrated into `AutoScaler` initialization
- Automatically triggered when `--enable-model-switch` is set
- Uses first available VLLMModel CRD as initial configuration

### 2. Enhanced RBAC Permissions

**File**: `examples/kubernetes-with-model-switching.yaml`

Updated Role to include:
- `create` permission for deployments
- `create` permission for configmaps
- `create` and `update` permissions for services
- `get` and `list` permissions for vllmmodels CRD

## Documentation Improvements

### Moved to docs/ Folder

1. **CONTRIBUTING.md** → `docs/CONTRIBUTING.md`
2. **SUMMARY.md** → `docs/SUMMARY.md`

### New Documentation

1. **docs/K8S_RESOURCE_MANAGEMENT.md** (NEW)
   - Comprehensive guide on automatic resource management
   - RBAC requirements
   - Troubleshooting guide
   - Best practices

### Updated Documentation

1. **README.md**
   - Updated reference to `docs/CONTRIBUTING.md`
   - Added feature bullet for automatic K8s resource management
   - Links to new K8S_RESOURCE_MANAGEMENT.md

## Test Coverage Improvements

### New Test Files

1. **pkg/proxy/k8s_manager_test.go** (NEW)
   - 8 test functions
   - Tests for ConfigMap, Service, and Deployment management
   - Tests for environment variable building
   - Tests for pod spec generation
   - Tests for handling existing resources

### Enhanced Test Files

1. **pkg/proxy/model_switcher_test.go**
   - Added `TestExtractModelFromRequest_BodyRestoration`
   - Added `TestExtractModelFromRequest_LargePayload`
   - Tests body restoration with large payloads

2. **pkg/proxy/autoscaler_test.go**
   - Added `TestAutoScaler_ConcurrentScaleUp`
   - Added `TestAutoScaler_ModelSwitchConcurrency`
   - Tests for synchronization primitives

3. **pkg/proxy/crd_client_test.go**
   - Added `TestCRDClient_ListModels`
   - Added `TestCRDClient_ConvertToModelConfig_AllFields`
   - Comprehensive field validation tests

## Test Results

```
✅ All tests passing
✅ 38.1% code coverage (up from ~26%)
✅ No race conditions detected
✅ 50+ test cases total
```

### Coverage Breakdown

- `config.go`: 100%
- `models.go`: 100%
- `k8s_manager.go`: 80%+
- `crd_client.go`: 80%+
- `model_switcher.go`: 87.5% (extractModelFromRequest)
- Overall: **38.1%**

## Module Organization

### Verified Structure

```
cmd/
  autoscaler/
    cmd/          # Cobra commands (serve, root)
    main.go       # Entry point

pkg/
  proxy/          # Core proxy logic
    autoscaler.go
    config.go
    crd_client.go
    k8s_manager.go (NEW)
    model_switcher.go
    models.go
    *_test.go     # Comprehensive tests

  apis/
    vllm/
      v1alpha1/   # CRD types
```

**Scope**:
- ✅ `cmd/autoscaler`: Command-line interface only
- ✅ `pkg/proxy`: All proxy and K8s management logic
- ✅ `pkg/apis/vllm`: CRD type definitions
- ✅ Clear separation of concerns

## Breaking Changes

**None**. All changes are backward compatible:
- Existing deployments continue to work
- Resource management is only active when `--enable-model-switch` is enabled
- Existing resources are detected and reused

## Migration Guide

### For New Deployments

Simply enable model switching:
```bash
vllm-chill serve --enable-model-switch
```

vllm-chill will automatically create all required resources.

### For Existing Deployments

No changes required. Existing resources will be detected and reused.

If you want automatic management:
1. Ensure RBAC permissions include `create` verbs
2. Enable model switching
3. vllm-chill will detect and use existing resources

## Files Changed

### New Files
- `pkg/proxy/k8s_manager.go`
- `pkg/proxy/k8s_manager_test.go`
- `docs/K8S_RESOURCE_MANAGEMENT.md`
- `docs/CONTRIBUTING.md` (moved)
- `docs/SUMMARY.md` (moved)
- `CHANGES.md` (this file)

### Modified Files
- `pkg/proxy/autoscaler.go` - Added K8sManager integration
- `pkg/proxy/autoscaler_test.go` - Added concurrency tests
- `pkg/proxy/model_switcher_test.go` - Added body restoration tests
- `pkg/proxy/crd_client_test.go` - Added comprehensive field tests
- `examples/kubernetes-with-model-switching.yaml` - Updated RBAC
- `README.md` - Updated documentation references

### Deleted Files
- `CONTRIBUTING.md` (moved to docs/)
- `SUMMARY.md` (moved to docs/)

## Next Steps

Recommended improvements for future:
1. Add integration tests with real K8s cluster
2. Add metrics/monitoring for resource management
3. Add support for custom deployment templates
4. Add validation for resource specifications
5. Consider adding a dry-run mode for resource creation
