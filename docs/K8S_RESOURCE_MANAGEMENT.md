# Kubernetes Resource Management

## Overview

When **model switching is enabled**, vllm-chill automatically manages the Kubernetes resources required for vLLM deployment. This includes:

- **ConfigMap**: Stores model configuration
- **Service**: Exposes the vLLM deployment
- **Deployment**: Runs the vLLM pods

## Automatic Resource Creation

When you start vllm-chill with `--enable-model-switch`, it will:

1. Check if the required resources exist
2. Create them if they don't exist
3. Use the first available VLLMModel CRD as the initial configuration

### Example

```bash
# Start vllm-chill with model switching enabled
vllm-chill serve \
  --enable-model-switch \
  --namespace ai-apps \
  --deployment vllm \
  --configmap vllm-config \
  --target-host vllm-svc
```

On startup, vllm-chill will:
- List all VLLMModel CRDs in the namespace
- Use the first model as initial configuration
- Create ConfigMap, Service, and Deployment if they don't exist

## Resource Specifications

### ConfigMap

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: vllm-config
  namespace: ai-apps
  labels:
    app: vllm
    managed-by: vllm-chill
data:
  MODEL_NAME: "..."
  SERVED_MODEL_NAME: "..."
  # ... all vLLM configuration parameters
```

### Service

```yaml
apiVersion: v1
kind: Service
metadata:
  name: vllm-svc
  namespace: ai-apps
  labels:
    app: vllm
    managed-by: vllm-chill
spec:
  selector:
    app: vllm
  ports:
  - name: http
    protocol: TCP
    port: 80
    targetPort: 8000
  type: ClusterIP
```

### Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: vllm
  namespace: ai-apps
  labels:
    app: vllm
    managed-by: vllm-chill
spec:
  replicas: 0  # Starts at 0, scaled on demand
  selector:
    matchLabels:
      app: vllm
  template:
    metadata:
      labels:
        app: vllm
    spec:
      containers:
      - name: vllm
        image: vllm/vllm-openai:latest
        # ... full vLLM configuration from ConfigMap
```

## Required RBAC Permissions

vllm-chill needs the following permissions to manage resources:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: vllm-chill
  namespace: ai-apps
rules:
- apiGroups: ["apps"]
  resources: ["deployments"]
  verbs: ["get", "list", "create", "update", "patch"]
- apiGroups: [""]
  resources: ["pods"]
  verbs: ["list", "delete", "deletecollection"]
- apiGroups: [""]
  resources: ["configmaps"]
  verbs: ["get", "create", "update", "patch"]
- apiGroups: [""]
  resources: ["services"]
  verbs: ["get", "create", "update", "patch"]
- apiGroups: ["vllm.efortin.github.io"]
  resources: ["vllmmodels"]
  verbs: ["get", "list"]
```

## Behavior

### First Startup

1. vllm-chill starts with `--enable-model-switch`
2. Lists VLLMModel CRDs in the namespace
3. If models exist:
   - Uses first model as initial configuration
   - Creates ConfigMap with model config
   - Creates Service (if not exists)
   - Creates Deployment at 0 replicas (if not exists)
4. If no models exist:
   - Logs a warning
   - Resources will be created when first model is requested

### Model Switching

When a client requests a different model:

1. vllm-chill scales deployment to 0
2. Updates ConfigMap with new model configuration
3. Scales deployment to 1
4. Waits for pod to be ready

### Existing Resources

If resources already exist (e.g., manually created):
- **ConfigMap**: Updated with new model configuration
- **Service**: Left unchanged (already exists)
- **Deployment**: Left unchanged (already exists)

This allows for gradual migration or manual customization.

## Manual vs Automatic Management

### Automatic (Recommended)

Enable model switching and let vllm-chill manage everything:

```bash
vllm-chill serve --enable-model-switch
```

**Pros:**
- Zero configuration
- Automatic resource creation
- Consistent setup

**Cons:**
- Less control over resource specifications
- Uses default vLLM image

### Manual

Create resources yourself and disable model switching:

```bash
kubectl apply -f my-custom-deployment.yaml
vllm-chill serve --enable-model-switch=false
```

**Pros:**
- Full control over resources
- Custom images, resource limits, etc.

**Cons:**
- More manual setup
- No automatic model switching

### Hybrid

Create resources manually, then enable model switching:

```bash
kubectl apply -f my-custom-deployment.yaml
vllm-chill serve --enable-model-switch
```

vllm-chill will detect existing resources and only update the ConfigMap during model switches.

## Troubleshooting

### Resources Not Created

Check logs:
```bash
kubectl logs -n ai-apps deployment/vllm-chill
```

Common issues:
- No VLLMModel CRDs defined
- Insufficient RBAC permissions
- Namespace doesn't exist

### Model Switch Fails

Check:
1. VLLMModel CRD exists and is valid
2. ConfigMap is being updated
3. Deployment is scaling properly
4. Pods are starting successfully

```bash
kubectl get vllmmodels -n ai-apps
kubectl get configmap vllm-config -n ai-apps -o yaml
kubectl get deployment vllm -n ai-apps
kubectl get pods -n ai-apps -l app=vllm
```

## Best Practices

1. **Define models first**: Create VLLMModel CRDs before starting vllm-chill
2. **Use labels**: All managed resources have `managed-by: vllm-chill` label
3. **Monitor logs**: Watch vllm-chill logs during startup and model switches
4. **Test RBAC**: Ensure service account has all required permissions
5. **Resource limits**: Consider adding resource limits to the deployment spec if managing manually
