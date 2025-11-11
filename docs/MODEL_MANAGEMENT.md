# Model Management

vLLM-Chill supports dynamic model switching without requiring redeployment. All model configurations are stored as Kubernetes CRDs and can be switched on-demand via API.

## Architecture

### Direct CRD Reading

Model configurations are read directly from VLLMModel CRDs, eliminating the need for ConfigMaps:

- **Single source of truth**: Model parameters live only in the CRD
- **No duplication**: No ConfigMap synchronization needed
- **Dynamic switching**: Switch models via API without redeploying vllm-chill
- **Immediate updates**: CRD changes are picked up on next pod creation

### How It Works

1. **Initial Model**: Set via `MODEL_ID` environment variable (e.g., `qwen3-coder-30b-fp8`)
2. **Model Switch**: Call `/proxy/models/switch` API to change active model
3. **Pod Recreation**: Current vLLM pod is stopped, next request creates pod with new model
4. **Configuration**: All vLLM parameters (maxModelLen, maxNumSeqs, etc.) come from CRD

## API Endpoints

### List Available Models

Get all models defined as VLLMModel CRDs:

```bash
curl http://vllm-chill:8080/proxy/models/available
```

Response:
```json
{
  "models": [
    {
      "name": "qwen3-coder-30b-fp8",
      "servedModelName": "qwen3-coder-30b-fp8",
      "modelName": "Qwen/Qwen3-Coder-30B-A3B-Instruct-FP8",
      "gpuCount": "2",
      "maxModelLen": "112640"
    },
    {
      "name": "deepseek-r1-fp8",
      "servedModelName": "deepseek-r1-fp8",
      "modelName": "neuralmagic/DeepSeek-R1-Distill-Qwen-32B-FP8-dynamic",
      "gpuCount": "2",
      "maxModelLen": "32768"
    }
  ],
  "count": 2
}
```

### Get Running Model

Get the currently active model and its configuration:

```bash
curl http://vllm-chill:8080/proxy/models/running
```

Response:
```json
{
  "active_model": "qwen3-coder-30b-fp8",
  "running": true,
  "config": {
    "modelName": "Qwen/Qwen3-Coder-30B-A3B-Instruct-FP8",
    "servedModelName": "qwen3-coder-30b-fp8",
    "gpuCount": "2",
    "maxModelLen": "112640",
    "toolCallParser": "qwen3_coder",
    "reasoningParser": ""
  }
}
```

### Switch Model

Switch to a different model:

```bash
curl -X POST http://vllm-chill:8080/proxy/models/switch \
  -H "Content-Type: application/json" \
  -d '{"model_id": "deepseek-r1-fp8"}'
```

Response:
```json
{
  "message": "Model switched successfully",
  "active_model": "deepseek-r1-fp8",
  "note": "vLLM pod will be recreated with the new model on next request"
}
```

**Behavior:**
- If vLLM pod is running, it will be stopped immediately
- Active model is updated to the new model ID
- Next inference request will create a new pod with the new model
- Model switch is instant (no waiting for pod creation)

## Model Configuration

### VLLMModel CRD

All model parameters are defined in the VLLMModel CRD:

```yaml
apiVersion: vllm.sir-alfred.io/v1alpha1
kind: VLLMModel
metadata:
  name: qwen3-coder-30b-fp8
spec:
  # Model Identification
  modelName: "Qwen/Qwen3-Coder-30B-A3B-Instruct-FP8"
  servedModelName: "qwen3-coder-30b-fp8"
  
  # Parsing Configuration
  toolCallParser: "qwen3_coder"
  reasoningParser: ""

  # vLLM Runtime Parameters (model-specific)
  # Note: gpuCount and cpuOffloadGB are configured at vllm-chill deployment level
  maxModelLen: 112640
  gpuMemoryUtilization: 0.91
  enableChunkedPrefill: true
  maxNumBatchedTokens: 8192
  maxNumSeqs: 16
  dtype: "float16"
  disableCustomAllReduce: true
  enablePrefixCaching: true
  enableAutoToolChoice: true
```

### Model-Specific Parameters (VLLMModel CRD)

The following parameters are stored in the VLLMModel CRD (model-specific):

- `modelName` - HuggingFace model identifier
- `servedModelName` - Model name exposed via API
- `maxModelLen` - Maximum sequence length
- `maxNumBatchedTokens` - Maximum batched tokens
- `maxNumSeqs` - Maximum number of sequences
- `toolCallParser` - Tool call parser type
- `reasoningParser` - Reasoning parser type (e.g., `deepseek_r1`)
- `gpuMemoryUtilization` - GPU memory utilization (0.0-1.0)
- `enableChunkedPrefill` - Enable chunked prefill
- `dtype` - Data type (auto, float16, bfloat16, etc.)
- `disableCustomAllReduce` - Disable custom all-reduce
- `enablePrefixCaching` - Enable prefix caching
- `enableAutoToolChoice` - Enable automatic tool choice

### Infrastructure Parameters (vllm-chill Config)

The following parameters are configured at the vllm-chill deployment level (infrastructure-level):

- `gpuCount` - Number of GPUs to allocate (via `--gpu-count` flag or `GPU_COUNT` env var)
- `cpuOffloadGB` - CPU offload in GB (via `--cpu-offload-gb` flag or `CPU_OFFLOAD_GB` env var)

These are infrastructure concerns, not model-specific, so they're configured once for the deployment.

## Use Cases

### 1. Development/Testing

Switch between models for testing without redeploying:

```bash
# Test with Qwen3 Coder
curl -X POST http://vllm-chill:8080/proxy/models/switch \
  -d '{"model_id": "qwen3-coder-30b-fp8"}'

# Test with DeepSeek R1
curl -X POST http://vllm-chill:8080/proxy/models/switch \
  -d '{"model_id": "deepseek-r1-fp8"}'
```

### 2. Multi-Tenant Scenarios

Different users can use different models:

```bash
# User A prefers coding model
curl -X POST http://vllm-chill:8080/proxy/models/switch \
  -d '{"model_id": "qwen3-coder-30b-fp8"}'

# User B prefers reasoning model
curl -X POST http://vllm-chill:8080/proxy/models/switch \
  -d '{"model_id": "deepseek-r1-fp8"}'
```

### 3. Cost Optimization

Switch to smaller models during off-peak hours:

```bash
# Peak hours: Use large model
curl -X POST http://vllm-chill:8080/proxy/models/switch \
  -d '{"model_id": "qwen3-coder-30b-fp8"}'

# Off-peak: Use smaller model
curl -X POST http://vllm-chill:8080/proxy/models/switch \
  -d '{"model_id": "qwen3-coder-7b-fp8"}'
```

### 4. A/B Testing

Compare model performance:

```bash
# Version A
curl -X POST http://vllm-chill:8080/proxy/models/switch \
  -d '{"model_id": "model-v1"}'

# Version B
curl -X POST http://vllm-chill:8080/proxy/models/switch \
  -d '{"model_id": "model-v2"}'
```

## Best Practices

### 1. Pre-cache Models

Ensure models are downloaded before switching:

```bash
# Download models to shared cache volume
kubectl exec -it vllm-pod -- huggingface-cli download Qwen/Qwen3-Coder-30B-A3B-Instruct-FP8
```

### 2. Monitor Switch Operations

Check logs after switching:

```bash
kubectl logs -f deployment/vllm-chill -n vllm
```

### 3. Verify Model Availability

Always check available models before switching:

```bash
curl http://vllm-chill:8080/proxy/models/available | jq '.models[].name'
```

### 4. Handle Switch Latency

Model switches require pod recreation (~60s startup time):

- Current pod is stopped immediately
- Next request triggers new pod creation
- First request after switch will wait for pod startup (max 2 minutes)

### 5. Use Consistent Naming

Use the CRD metadata name as the model ID:

```yaml
metadata:
  name: qwen3-coder-30b-fp8  # Use this as model_id
spec:
  servedModelName: "qwen3-coder-30b-fp8"  # Should match
```

## Troubleshooting

### Model Not Found

```json
{
  "error": "Model 'unknown-model' not found"
}
```

**Solution**: Check available models:
```bash
curl http://vllm-chill:8080/proxy/models/available
```

### Pod Creation Failed

Check vllm-chill logs:
```bash
kubectl logs -f deployment/vllm-chill -n vllm
```

Common issues:
- Model not in cache (long download time)
- Insufficient GPU memory
- Invalid model parameters in CRD

### Switch Takes Too Long

Model switches stop the current pod immediately, but the new pod takes ~60s to start:

- Check pod status: `kubectl get pods -n vllm`
- Check pod logs: `kubectl logs -f vllm -n vllm`
- Verify model is cached: Check HuggingFace cache volume

## Migration from ConfigMap

If you're migrating from ConfigMap-based configuration:

1. **Remove ConfigMap references**: ConfigMaps are no longer used
2. **Update CRDs**: Ensure all parameters are in VLLMModel CRD
3. **Set MODEL_ID**: Use CRD metadata name as MODEL_ID
4. **Test switching**: Verify model switching works with new architecture

## Future Enhancements

Potential improvements:

- **Hot model switching**: Switch without stopping current pod
- **Model preloading**: Pre-load models in background
- **Scheduled switching**: Automatic model switching based on schedule
- **Load balancing**: Multiple models running simultaneously
- **Model versioning**: Track and rollback model versions
