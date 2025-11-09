package kubernetes

import (
	"context"
	"fmt"
	"log"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
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
	if err := m.ensureDeployment(ctx, initialModel); err != nil {
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
// Note: In sidecar mode with Unix sockets, this service is not used for communication
// but kept for pod discovery and compatibility
func (m *K8sManager) ensureService(ctx context.Context) error {
	serviceName := "vllm"
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
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
					TargetPort: intstr.FromString("http"),
				},
				{
					Name:       "metrics",
					Protocol:   corev1.ProtocolTCP,
					Port:       8001,
					TargetPort: intstr.FromString("metrics"),
				},
			},
			Type: corev1.ServiceTypeClusterIP,
		},
	}

	_, err := m.clientset.CoreV1().Services(m.config.Namespace).Get(ctx, serviceName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			// Create new Service
			_, err = m.clientset.CoreV1().Services(m.config.Namespace).Create(ctx, service, metav1.CreateOptions{})
			if err != nil {
				return fmt.Errorf("failed to create service: %w", err)
			}
			log.Printf("Created Service %s/%s", m.config.Namespace, serviceName)
			return nil
		}
		return fmt.Errorf("failed to get service: %w", err)
	}

	// Service already exists
	log.Printf("Service %s/%s already exists", m.config.Namespace, serviceName)
	return nil
}

// ensureDeployment creates the vLLM deployment if it doesn't exist
func (m *K8sManager) ensureDeployment(ctx context.Context, modelConfig *ModelConfig) error {
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
			Strategy: appsv1.DeploymentStrategy{
				Type: appsv1.RecreateDeploymentStrategyType,
			},
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
				Spec: m.buildPodSpec(modelConfig),
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

// buildVLLMArgs builds the vLLM command-line arguments from ModelConfig
func (m *K8sManager) buildVLLMArgs(modelConfig *ModelConfig) []string {
	args := []string{
		"python3", "-m", "vllm.entrypoints.openai.api_server",
		"--model", modelConfig.ModelName,
		"--served-model-name", modelConfig.ServedModelName,
		"--tensor-parallel-size", modelConfig.TensorParallelSize,
		"--max-model-len", modelConfig.MaxModelLen,
		"--gpu-memory-utilization", modelConfig.GPUMemoryUtilization,
	}

	// Add optional boolean flags
	if modelConfig.EnableChunkedPrefill == "true" {
		args = append(args, "--enable-chunked-prefill")
	}

	args = append(args,
		"--max-num-batched-tokens", modelConfig.MaxNumBatchedTokens,
		"--max-num-seqs", modelConfig.MaxNumSeqs,
		"--dtype", modelConfig.Dtype,
	)

	if modelConfig.DisableCustomAllReduce == "true" {
		args = append(args, "--disable-custom-all-reduce")
	}

	if modelConfig.EnablePrefixCaching == "true" {
		args = append(args, "--enable-prefix-caching")
	}

	args = append(args, "--cpu-offload-gb", modelConfig.CPUOffloadGB)

	if modelConfig.EnableAutoToolChoice == "true" {
		args = append(args, "--enable-auto-tool-choice")
	}

	args = append(args,
		"--tool-call-parser", modelConfig.ToolCallParser,
		"--host", "0.0.0.0",
		"--port", "8000",
		"--api-key", "token-abc123",
	)

	return args
}

// buildPodSpec builds the pod specification for vLLM
func (m *K8sManager) buildPodSpec(modelConfig *ModelConfig) corev1.PodSpec {
	return corev1.PodSpec{
		TerminationGracePeriodSeconds: func() *int64 { t := int64(0); return &t }(),
		Volumes: []corev1.Volume{
			{
				Name: "hf-cache",
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: "/home/manu/.cache/huggingface",
						Type: func() *corev1.HostPathType { t := corev1.HostPathDirectoryOrCreate; return &t }(),
					},
				},
			},
			{
				Name: "vllm-compile-cache",
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: "/home/manu/.cache/vllm-compile",
						Type: func() *corev1.HostPathType { t := corev1.HostPathDirectoryOrCreate; return &t }(),
					},
				},
			},
			{
				Name: "shm",
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{
						Medium:    corev1.StorageMediumMemory,
						SizeLimit: func() *resource.Quantity { q := resource.MustParse("16Gi"); return &q }(),
					},
				},
			},
		},
		Containers: []corev1.Container{
			{
				Name:            "vllm",
				Image:           "vllm/vllm-openai:latest",
				ImagePullPolicy: corev1.PullIfNotPresent,
				Args:            m.buildVLLMArgs(modelConfig),
				Env:             m.buildSystemEnvVars(),
				Ports: []corev1.ContainerPort{
					{
						ContainerPort: 8000,
						Name:          "http",
					},
					{
						ContainerPort: 8001,
						Name:          "metrics",
					},
				},
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceMemory:                 resource.MustParse("32Gi"),
						corev1.ResourceName("nvidia.com/gpu"): resource.MustParse("2"),
					},
					Requests: corev1.ResourceList{
						corev1.ResourceMemory:                 resource.MustParse("16Gi"),
						corev1.ResourceName("nvidia.com/gpu"): resource.MustParse("2"),
					},
				},
				VolumeMounts: []corev1.VolumeMount{
					{
						Name:      "hf-cache",
						MountPath: "/root/.cache/huggingface",
					},
					{
						Name:      "vllm-compile-cache",
						MountPath: "/root/.cache/vllm",
					},
					{
						Name:      "shm",
						MountPath: "/dev/shm",
					},
				},
				StartupProbe: &corev1.Probe{
					ProbeHandler: corev1.ProbeHandler{
						HTTPGet: &corev1.HTTPGetAction{
							Path: "/health",
							Port: intstr.FromString("http"),
							HTTPHeaders: []corev1.HTTPHeader{
								{Name: "Authorization", Value: "Bearer token-abc123"},
							},
						},
					},
					InitialDelaySeconds: 10,
					PeriodSeconds:       5,
					TimeoutSeconds:      5,
					FailureThreshold:    24,
				},
				ReadinessProbe: &corev1.Probe{
					ProbeHandler: corev1.ProbeHandler{
						HTTPGet: &corev1.HTTPGetAction{
							Path: "/health",
							Port: intstr.FromString("http"),
							HTTPHeaders: []corev1.HTTPHeader{
								{Name: "Authorization", Value: "Bearer token-abc123"},
							},
						},
					},
					InitialDelaySeconds: 5,
					PeriodSeconds:       10,
					TimeoutSeconds:      5,
					FailureThreshold:    12,
				},
				LivenessProbe: &corev1.Probe{
					ProbeHandler: corev1.ProbeHandler{
						HTTPGet: &corev1.HTTPGetAction{
							Path: "/health",
							Port: intstr.FromString("http"),
							HTTPHeaders: []corev1.HTTPHeader{
								{Name: "Authorization", Value: "Bearer token-abc123"},
							},
						},
					},
					InitialDelaySeconds: 60,
					PeriodSeconds:       30,
					TimeoutSeconds:      5,
					FailureThreshold:    3,
				},
			},
		},
	}
}

// buildSystemEnvVars builds system-level environment variables (non-model-specific)
func (m *K8sManager) buildSystemEnvVars() []corev1.EnvVar {
	envVars := []corev1.EnvVar{
		// System optimization environment variables
		{
			Name:  "TORCH_CUDA_ARCH_LIST",
			Value: "8.6",
		},
		{
			Name:  "VLLM_TORCH_COMPILE_CACHE_DIR",
			Value: "/root/.cache/vllm/torch_compile_cache",
		},
		{
			Name:  "HF_HUB_ENABLE_HF_TRANSFER",
			Value: "1",
		},
		{
			Name:  "OMP_NUM_THREADS",
			Value: "16",
		},
		// HF Token from secret (optional - will fail silently if secret doesn't exist)
		{
			Name: "HF_TOKEN",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: "hf-token-secret"},
					Key:                  "token",
					Optional:             func() *bool { b := true; return &b }(),
				},
			},
		},
		{
			Name: "HUGGING_FACE_HUB_TOKEN",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: "hf-token-secret"},
					Key:                  "token",
					Optional:             func() *bool { b := true; return &b }(),
				},
			},
		},
	}

	return envVars
}
