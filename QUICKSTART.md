# Quick Start Guide

## Prerequisites

- Kubernetes cluster with kubectl access
- RBAC permissions to create CRDs, Deployments, Services, ConfigMaps

## Installation

### 1. Install VLLMModel CRD

```bash
kubectl apply -f manifests/crds/vllmmodel.yaml
```

### 2. Create VLLMModel Resources

Create at least one model (cluster-scoped):

```bash
# Example: Qwen3 Coder
kubectl apply -f manifests/examples/qwen3-coder-model.yaml

# Or DeepSeek R1
kubectl apply -f manifests/examples/deepseek-r1-model.yaml
```

### 3. Deploy vllm-chill

Create namespace and deploy:

```bash
# Create namespace
kubectl create namespace vllm

# Create HF token secret (if needed for private models)
kubectl create secret generic hf-token-secret -n vllm \
  --from-literal=token=YOUR_HF_TOKEN

# Option A: Use pre-built image from GHCR (after CI/CD runs)
kubectl apply -f manifests/kubernetes-with-model-switching.yaml

# Option B: Build and deploy locally
./scripts/local-deploy.sh
```

**Note:** The Docker image must be available. Either:
- Push to `main` branch to trigger GitHub Actions build
- Or build locally with `docker build -t ghcr.io/efortin/vllm-chill:latest .`

### 4. Configure Ingress (Optional)

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: vllm-ingress
  namespace: vllm
  annotations:
    nginx.ingress.kubernetes.io/proxy-read-timeout: "1800"
    nginx.ingress.kubernetes.io/proxy-send-timeout: "1800"
spec:
  rules:
    - host: <your-domain>
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: vllm-chill-svc
                port:
                  number: 80
```

## Usage

### Test the Proxy

```bash
# Port-forward to test locally
kubectl port-forward -n vllm svc/vllm-chill-svc 8080:80

# Send a request (vLLM will auto-scale from 0 to 1)
curl http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen3-coder-30b-fp8",
    "messages": [{"role": "user", "content": "Hello"}]
  }'
```

### Monitor

```bash
# Check vllm-chill logs
kubectl logs -n vllm -f deployment/vllm-chill

# Check vLLM deployment
kubectl get deployment -n vllm vllm

# View metrics
kubectl port-forward -n vllm svc/vllm-chill-svc 8080:80
curl http://localhost:8080/metrics
```

### Model Switching

Switch models by changing the `model` field in your request:

```bash
curl http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "deepseek-r1-fp8",
    "messages": [{"role": "user", "content": "Hello"}]
  }'
```

vllm-chill will:
1. Scale down current vLLM deployment
2. Update ConfigMap with new model config
3. Scale up with new model
4. Return helpful loading message if model is switching

## Configuration

### Environment Variables

Configure vllm-chill via environment variables in the deployment:

```yaml
env:
  - name: VLLM_NAMESPACE
    value: "vllm"
  - name: VLLM_DEPLOYMENT
    value: "vllm"
  - name: VLLM_CONFIGMAP
    value: "vllm-config"
  - name: IDLE_TIMEOUT
    value: "20m"              # Scale to 0 after 20min idle
  - name: MANAGED_TIMEOUT
    value: "5m"               # Timeout for model switches
  - name: LOG_OUTPUT
    value: "false"            # Log response bodies (debug only)
```

## Troubleshooting

### vllm-chill won't start

Check prerequisites:

```bash
# Verify CRD is installed
kubectl get crd models.vllm.sir-alfred.io

# Verify RBAC permissions
kubectl auth can-i create deployments --namespace=vllm --as=system:serviceaccount:vllm:vllm-chill

# Check logs
kubectl logs -n vllm deployment/vllm-chill
```

### vLLM not scaling up

```bash
# Check if deployment exists
kubectl get deployment -n vllm vllm

# Check vllm-chill logs for errors
kubectl logs -n vllm deployment/vllm-chill

# Verify VLLMModel exists
kubectl get models
```

### Model switch fails

```bash
# Check if model exists in CRD
kubectl get models

# Verify model name matches servedModelName in CRD
kubectl get model qwen3-coder-30b-fp8 -o yaml

# Check vllm-chill logs
kubectl logs -n vllm deployment/vllm-chill
```

## Next Steps

- See [docs/METRICS.md](docs/METRICS.md) for Prometheus metrics
- See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for design details
- Check [manifests/examples/](manifests/examples/) for more VLLMModel examples
