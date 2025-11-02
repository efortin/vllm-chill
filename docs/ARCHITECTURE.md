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

## Conclusion

The **separate proxy** architecture is the only viable solution for:
1. Functional scale-to-zero
2. Transparent automatic wake
3. Minimal overhead
4. Simplicity and maintainability

Sidecars are a good practice in many cases, but **not for scale-to-zero**.
