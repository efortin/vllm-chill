package proxy

import (
	"context"
	"fmt"
	"log"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
)

// K8sManager handles Kubernetes resource management for vLLM
type K8sManager struct {
	clientset kubernetes.Interface
	config    *Config
}

// NewK8sManager creates a new K8sManager
func NewK8sManager(clientset kubernetes.Interface, config *Config) *K8sManager {
	return &K8sManager{
		clientset: clientset,
		config:    config,
	}
}

// EnsureVLLMResources ensures the vLLM deployment, service, and configmap exist
func (m *K8sManager) EnsureVLLMResources(ctx context.Context, initialModel *ModelConfig) error {
	// Ensure ConfigMap exists
	if err := m.ensureConfigMap(ctx, initialModel); err != nil {
		return fmt.Errorf("failed to ensure configmap: %w", err)
	}

	// Ensure Service exists
	if err := m.ensureService(ctx); err != nil {
		return fmt.Errorf("failed to ensure service: %w", err)
	}

	// Ensure Deployment exists
	if err := m.ensureDeployment(ctx); err != nil {
		return fmt.Errorf("failed to ensure deployment: %w", err)
	}

	return nil
}

// ensureConfigMap creates or updates the ConfigMap
func (m *K8sManager) ensureConfigMap(ctx context.Context, modelConfig *ModelConfig) error {
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      m.config.ConfigMapName,
			Namespace: m.config.Namespace,
			Labels: map[string]string{
				"app":        "vllm",
				"managed-by": "vllm-chill",
			},
		},
		Data: modelConfig.ToConfigMapData(),
	}

	existing, err := m.clientset.CoreV1().ConfigMaps(m.config.Namespace).Get(ctx, m.config.ConfigMapName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			// Create new ConfigMap
			_, err = m.clientset.CoreV1().ConfigMaps(m.config.Namespace).Create(ctx, configMap, metav1.CreateOptions{})
			if err != nil {
				return fmt.Errorf("failed to create configmap: %w", err)
			}
			log.Printf("Created ConfigMap %s/%s", m.config.Namespace, m.config.ConfigMapName)
			return nil
		}
		return fmt.Errorf("failed to get configmap: %w", err)
	}

	// Update existing ConfigMap
	existing.Data = configMap.Data
	_, err = m.clientset.CoreV1().ConfigMaps(m.config.Namespace).Update(ctx, existing, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update configmap: %w", err)
	}
	log.Printf("Updated ConfigMap %s/%s", m.config.Namespace, m.config.ConfigMapName)
	return nil
}

// ensureService creates the vLLM service if it doesn't exist
func (m *K8sManager) ensureService(ctx context.Context) error {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      m.config.TargetHost,
			Namespace: m.config.Namespace,
			Labels: map[string]string{
				"app":        "vllm",
				"managed-by": "vllm-chill",
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app": "vllm",
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Protocol:   corev1.ProtocolTCP,
					Port:       80,
					TargetPort: intstr.FromInt(8000),
				},
			},
			Type: corev1.ServiceTypeClusterIP,
		},
	}

	_, err := m.clientset.CoreV1().Services(m.config.Namespace).Get(ctx, m.config.TargetHost, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			// Create new Service
			_, err = m.clientset.CoreV1().Services(m.config.Namespace).Create(ctx, service, metav1.CreateOptions{})
			if err != nil {
				return fmt.Errorf("failed to create service: %w", err)
			}
			log.Printf("Created Service %s/%s", m.config.Namespace, m.config.TargetHost)
			return nil
		}
		return fmt.Errorf("failed to get service: %w", err)
	}

	// Service already exists
	log.Printf("Service %s/%s already exists", m.config.Namespace, m.config.TargetHost)
	return nil
}

// ensureDeployment creates the vLLM deployment if it doesn't exist
func (m *K8sManager) ensureDeployment(ctx context.Context) error {
	replicas := int32(0) // Start at 0, proxy will scale up on demand

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      m.config.Deployment,
			Namespace: m.config.Namespace,
			Labels: map[string]string{
				"app":        "vllm",
				"managed-by": "vllm-chill",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "vllm",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "vllm",
					},
				},
				Spec: m.buildPodSpec(),
			},
		},
	}

	_, err := m.clientset.AppsV1().Deployments(m.config.Namespace).Get(ctx, m.config.Deployment, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			// Create new Deployment
			_, err = m.clientset.AppsV1().Deployments(m.config.Namespace).Create(ctx, deployment, metav1.CreateOptions{})
			if err != nil {
				return fmt.Errorf("failed to create deployment: %w", err)
			}
			log.Printf("Created Deployment %s/%s", m.config.Namespace, m.config.Deployment)
			return nil
		}
		return fmt.Errorf("failed to get deployment: %w", err)
	}

	// Deployment already exists
	log.Printf("Deployment %s/%s already exists", m.config.Namespace, m.config.Deployment)
	return nil
}

// buildPodSpec builds the pod specification for vLLM
func (m *K8sManager) buildPodSpec() corev1.PodSpec {
	return corev1.PodSpec{
		Containers: []corev1.Container{
			{
				Name:    "vllm",
				Image:   "vllm/vllm-openai:latest",
				Command: []string{"/bin/bash", "-c"},
				Args: []string{
					`EXTRA_ARGS=""

# Add optional reasoning parser if set
if [ -n "$REASONING_PARSER" ]; then
  EXTRA_ARGS="$EXTRA_ARGS --reasoning-parser $REASONING_PARSER"
fi

# Build command with all configurable parameters
python3 -m vllm.entrypoints.openai.api_server \
  --model "$MODEL_NAME" \
  --served-model-name "$SERVED_MODEL_NAME" \
  --tensor-parallel-size "$TENSOR_PARALLEL_SIZE" \
  --max-model-len "$MAX_MODEL_LEN" \
  --gpu-memory-utilization "$GPU_MEMORY_UTILIZATION" \
  $([ "$ENABLE_CHUNKED_PREFILL" = "true" ] && echo "--enable-chunked-prefill") \
  --max-num-batched-tokens "$MAX_NUM_BATCHED_TOKENS" \
  --max-num-seqs "$MAX_NUM_SEQS" \
  --dtype "$DTYPE" \
  $([ "$DISABLE_CUSTOM_ALL_REDUCE" = "true" ] && echo "--disable-custom-all-reduce") \
  $([ "$ENABLE_PREFIX_CACHING" = "true" ] && echo "--enable-prefix-caching") \
  --cpu-offload-gb "$CPU_OFFLOAD_GB" \
  $([ "$ENABLE_AUTO_TOOL_CHOICE" = "true" ] && echo "--enable-auto-tool-choice") \
  --tool-call-parser "$TOOL_CALL_PARSER" \
  $EXTRA_ARGS \
  --host 0.0.0.0 \
  --port 8000`,
				},
				Env: m.buildEnvVars(),
				Ports: []corev1.ContainerPort{
					{
						ContainerPort: 8000,
						Name:          "http",
					},
				},
				ReadinessProbe: &corev1.Probe{
					ProbeHandler: corev1.ProbeHandler{
						HTTPGet: &corev1.HTTPGetAction{
							Path: "/health",
							Port: intstr.FromInt(8000),
						},
					},
					InitialDelaySeconds: 60,
					PeriodSeconds:       5,
					TimeoutSeconds:      3,
				},
			},
		},
	}
}

// buildEnvVars builds environment variables from ConfigMap
func (m *K8sManager) buildEnvVars() []corev1.EnvVar {
	configMapName := m.config.ConfigMapName
	envVars := []corev1.EnvVar{
		{
			Name: "MODEL_NAME",
			ValueFrom: &corev1.EnvVarSource{
				ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: configMapName},
					Key:                  "MODEL_NAME",
				},
			},
		},
		{
			Name: "SERVED_MODEL_NAME",
			ValueFrom: &corev1.EnvVarSource{
				ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: configMapName},
					Key:                  "SERVED_MODEL_NAME",
				},
			},
		},
		{
			Name: "TOOL_CALL_PARSER",
			ValueFrom: &corev1.EnvVarSource{
				ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: configMapName},
					Key:                  "TOOL_CALL_PARSER",
				},
			},
		},
		{
			Name: "REASONING_PARSER",
			ValueFrom: &corev1.EnvVarSource{
				ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: configMapName},
					Key:                  "REASONING_PARSER",
				},
			},
		},
		{
			Name: "TENSOR_PARALLEL_SIZE",
			ValueFrom: &corev1.EnvVarSource{
				ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: configMapName},
					Key:                  "TENSOR_PARALLEL_SIZE",
				},
			},
		},
		{
			Name: "MAX_MODEL_LEN",
			ValueFrom: &corev1.EnvVarSource{
				ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: configMapName},
					Key:                  "MAX_MODEL_LEN",
				},
			},
		},
		{
			Name: "GPU_MEMORY_UTILIZATION",
			ValueFrom: &corev1.EnvVarSource{
				ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: configMapName},
					Key:                  "GPU_MEMORY_UTILIZATION",
				},
			},
		},
		{
			Name: "ENABLE_CHUNKED_PREFILL",
			ValueFrom: &corev1.EnvVarSource{
				ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: configMapName},
					Key:                  "ENABLE_CHUNKED_PREFILL",
				},
			},
		},
		{
			Name: "MAX_NUM_BATCHED_TOKENS",
			ValueFrom: &corev1.EnvVarSource{
				ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: configMapName},
					Key:                  "MAX_NUM_BATCHED_TOKENS",
				},
			},
		},
		{
			Name: "MAX_NUM_SEQS",
			ValueFrom: &corev1.EnvVarSource{
				ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: configMapName},
					Key:                  "MAX_NUM_SEQS",
				},
			},
		},
		{
			Name: "DTYPE",
			ValueFrom: &corev1.EnvVarSource{
				ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: configMapName},
					Key:                  "DTYPE",
				},
			},
		},
		{
			Name: "DISABLE_CUSTOM_ALL_REDUCE",
			ValueFrom: &corev1.EnvVarSource{
				ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: configMapName},
					Key:                  "DISABLE_CUSTOM_ALL_REDUCE",
				},
			},
		},
		{
			Name: "ENABLE_PREFIX_CACHING",
			ValueFrom: &corev1.EnvVarSource{
				ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: configMapName},
					Key:                  "ENABLE_PREFIX_CACHING",
				},
			},
		},
		{
			Name: "CPU_OFFLOAD_GB",
			ValueFrom: &corev1.EnvVarSource{
				ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: configMapName},
					Key:                  "CPU_OFFLOAD_GB",
				},
			},
		},
		{
			Name: "ENABLE_AUTO_TOOL_CHOICE",
			ValueFrom: &corev1.EnvVarSource{
				ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: configMapName},
					Key:                  "ENABLE_AUTO_TOOL_CHOICE",
				},
			},
		},
	}

	return envVars
}
