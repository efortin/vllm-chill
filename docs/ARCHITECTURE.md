# vLLM AutoScaler - Architecture

## Why a Separate Proxy and Not a Sidecar?

### ❌ Sidecar Impossible for Scale-to-Zero

A **sidecar** shares the same pod as vLLM. If we scale the deployment to 0:
- The entire pod is terminated
- The sidecar autoscaler is also terminated
- No one left to detect requests and wake vLLM
- **Scale-to-zero impossible**

### ✅ Separate Proxy: The Right Solution

The proxy runs in its **own deployment**:
- Stays active even when vLLM is at 0 replicas
- Detects incoming requests
- Scales vLLM from 0 → 1 automatically
- Buffers connections during wake-up
- Scales vLLM from 1 → 0 after inactivity

## Current Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                         Internet                             │
└────────────────────────┬────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│                    Ingress (nginx)                           │
│                   vllm.example.com                           │
└────────────────────────┬────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│              vllm-chill-svc:80                          │
└────────────────────────┬────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│         vllm-chill (Deployment: 1 replica)              │
│                                                               │
│  • Always active (never scales to 0)                        │
│  • Detects if vLLM is at 0 replicas                         │
│  • Scales vLLM to 1 if necessary                            │
│  • Waits for vLLM to be Ready (max 2min)                   │
│  • Buffers connections during wake-up                       │
│  • Proxies requests to vLLM                                 │
│  • Tracks activity                                           │
│  • Scales vLLM to 0 after 5min idle                         │
│                                                               │
└────────────────────────┬────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│                   vllm-svc:80                                │
└────────────────────────┬────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│              vLLM (Deployment: 0 or 1 replica)               │
│                                                               │
│  • Scales to 0 when inactive → Frees GPUs                   │
│  • Scales to 1 on request → Loads model                     │
│  • Startup: ~60 seconds                                      │
│  • Health probes with Authorization                          │
│                                                               │
└─────────────────────────────────────────────────────────────┘
```

## Advantages of This Architecture

### ✅ Functional Scale-to-Zero
- Proxy stays active to detect requests
- vLLM can be completely stopped (0 replicas)
- GPUs 100% freed when inactive

### ✅ Minimal Overhead
- Proxy: ~50-80MB RAM, <5ms latency
- Negligible cost vs GPU savings

### ✅ Separation of Concerns
- Proxy: Routing + scaling logic
- vLLM: Inference only
- Each component can be updated independently

### ✅ Resilience
- If vLLM crashes, proxy stays active
- Can restart vLLM automatically
- Separate logs for debugging

## Alternatives Considered

### Option 1: Sidecar ❌
**Problem**: Impossible to scale to 0 (sidecar would also be terminated)

### Option 2: KEDA HTTP Add-on ❌
**Problems**:
- Hardcoded 20s timeout (vLLM = 60s)
- Complexity (Helm, CRDs, namespaces)
- No fine-grained control

### Option 3: CronJob ❌
**Problem**: No automatic wake (manual wake required)

### Option 4: Separate Proxy ✅
**Chosen solution**: Simple, reliable, functional scale-to-zero

## Resources

### AutoScaler Proxy
- **CPU**: 100m request, 200m limit
- **RAM**: 64Mi request, 128Mi limit
- **Replicas**: 1 (always active)

### vLLM
- **CPU**: No limit (GPU workload)
- **RAM**: 16Gi request, 32Gi limit
- **GPU**: 2× RTX 3090 (nvidia.com/gpu: 2)
- **Replicas**: 0 or 1 (dynamic)

## Observed Metrics

- **Wake time**: ~60 seconds
- **Scale-down delay**: 5 minutes after last request
- **Proxy overhead**: <5ms per request
- **GPU savings**: 100% when inactive
- **Proxy uptime**: 100% (never scales)

## Configuration Flow

### VLLMModel CRD → vLLM Pod (Direct)

```
┌──────────────────────────────────────────────────────────┐
│  VLLMModel CRD (cluster-scoped)                          │
│  ─────────────────────────────────                       │
│  apiVersion: vllm.sir-alfred.io/v1alpha1                 │
│  kind: VLLMModel                                          │
│  metadata:                                                │
│    name: qwen3-coder-30b-fp8                             │
│  spec:                                                    │
│    modelName: "Qwen/Qwen3-Coder-30B-A3B-Instruct-FP8"   │
│    servedModelName: "qwen3-coder-30b-fp8"               │
│    maxModelLen: 112640                                   │
│    maxNumBatchedTokens: 8192                            │
│    maxNumSeqs: 16                                        │
│    toolCallParser: "qwen3_coder"                        │
│    reasoningParser: ""                                   │
│    # Note: gpuCount, cpuOffloadGB at deployment level   │
│    ...                                                    │
└────────────────┬─────────────────────────────────────────┘
                 │
                 │ vllm-chill reads CRD directly
                 │ based on active model ID
                 │ (switchable via API)
                 ▼
┌──────────────────────────────────────────────────────────┐
│  vLLM Pod                                                 │
│  ────────                                                 │
│  vllm-chill builds pod spec with args from CRD + config: │
│  - --model "Qwen/Qwen3-Coder-30B-A3B-Instruct-FP8"      │
│  - --served-model-name "qwen3-coder-30b-fp8"            │
│  - --tensor-parallel-size 2 (from vllm-chill config)    │
│  - --max-model-len 112640 (from CRD)                     │
│  - --max-num-batched-tokens 8192 (from CRD)             │
│  - --max-num-seqs 16 (from CRD)                          │
│  - --tool-call-parser "qwen3_coder" (from CRD)          │
│  - --cpu-offload-gb 0 (from vllm-chill config)          │
│  ...                                                      │
│                                                           │
│  vLLM uses these args to configure the model at startup  │
└──────────────────────────────────────────────────────────┘
```

**Why Direct CRD Reading?**
- **Single source of truth**: Model configuration lives only in the CRD
- **No duplication**: No need to copy config into ConfigMap
- **Dynamic switching**: Can switch models via API without redeploying
- **Simpler architecture**: Fewer moving parts, easier to maintain
- **Immediate updates**: Changes to CRD are picked up on next pod creation
- **Clear separation**: Model-specific params in CRD, infrastructure params in deployment config

## API Endpoints

### Model Management

- **`GET /proxy/models/available`** - List all available models from VLLMModel CRDs
- **`GET /proxy/models/running`** - Get the currently active model and its configuration
- **`POST /proxy/models/switch`** - Switch to a different model (stops current pod, next request will start new model)

Example model switch:
```bash
curl -X POST http://vllm-chill:8080/proxy/models/switch \
  -H "Content-Type: application/json" \
  -d '{"model_id": "deepseek-r1-fp8"}'
```

### Operations

- **`POST /proxy/operations/start`** - Manually start the vLLM pod
- **`POST /proxy/operations/stop`** - Manually stop the vLLM pod

### Metrics & Monitoring

- **`/proxy/metrics`** - vLLM-Chill proxy metrics (autoscaling, requests, latency)
- **`/metrics`** - vLLM backend metrics (model inference, GPU usage) - proxied to vLLM when running
- **`/proxy/stats`** - GPU statistics
- **`/proxy/version`** - Version information

Both metrics endpoints are accessible through the same service, allowing separate monitoring of proxy and backend.

## Conclusion

The **separate proxy** architecture is the only viable solution for:
1. Functional scale-to-zero
2. Transparent automatic wake
3. Minimal overhead
4. Simplicity and maintainability

Sidecars are a good practice in many cases, but **not for scale-to-zero**.
