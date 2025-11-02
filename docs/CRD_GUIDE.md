# CRD-Based Dynamic Model Switching

## Quick Start

```bash
# 1. Install CRD
kubectl apply -f manifests/crds/vllmmodel.yaml

# 2. Create models
kubectl apply -f manifests/examples/

# 3. Deploy
kubectl apply -f examples/kubernetes-with-model-switching.yaml

# 4. Use
curl -X POST http://proxy/v1/chat/completions \
  -d '{"model": "qwen3-coder-30b-fp8", "messages": [...]}'
```

## Define a Model

```yaml
apiVersion: vllm.efortin.github.io/v1alpha1
kind: VLLMModel
metadata:
  name: my-model
  namespace: ai-apps
spec:
  modelName: "org/model-name"
  servedModelName: "my-model"
  toolCallParser: "hermes"              # enum: "", hermes, mistral, llama3_json, internlm2, qwen3_coder, granite
  reasoningParser: ""                   # enum: "", deepseek_r1
  tensorParallelSize: 2
  maxModelLen: 65536
  gpuMemoryUtilization: 0.91
  enableChunkedPrefill: true
  maxNumBatchedTokens: 4096
  maxNumSeqs: 16
  dtype: "float16"                      # enum: auto, half, float16, bfloat16, float, float32
  disableCustomAllReduce: true
  enablePrefixCaching: true
  cpuOffloadGB: 0
  enableAutoToolChoice: true
```

## User Experience

### Model Already Loaded
```bash
$ curl http://proxy/v1/chat/completions -d '{"model": "qwen3-coder-30b-fp8", ...}'
# → Immediate response
```

### Model Loading (2-5 min)
```bash
$ curl http://proxy/v1/chat/completions -d '{"model": "deepseek-r1-fp8", ...}'
# → HTTP 503
{
  "error": {
    "message": "Model 'deepseek-r1-fp8' is currently loading. Please wait and retry in a few moments.",
    "type": "model_loading",
    "code": "model_switching_in_progress"
  }
}
# Retry-After: 30
```

## Configuration

```bash
./bin/vllm-chill serve \
  --enable-model-switch \
  --configmap vllm-config \
  --namespace ai-apps \
  --deployment vllm \
  --model-switch-timeout 5m
```

## Management

```bash
# List models
kubectl get vllmmodels -n ai-apps

# Add model
kubectl apply -f my-model.yaml

# Update model
kubectl edit vllmmodel my-model -n ai-apps

# Delete model
kubectl delete vllmmodel my-model -n ai-apps
```

## Validation

The CRD enforces:
- **dtype**: auto, half, float16, bfloat16, float, float32
- **toolCallParser**: "", hermes, mistral, llama3_json, internlm2, qwen3_coder, granite
- **reasoningParser**: "", deepseek_r1
- **tensorParallelSize**: >= 1
- **maxModelLen**: >= 512
- **gpuMemoryUtilization**: 0.1 - 1.0

Invalid values are rejected by Kubernetes.
