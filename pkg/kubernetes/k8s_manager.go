package kubernetes

import (
	"context"
	"fmt"
	"log"

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

// EnsureVLLMResources ensures the vLLM service exists
// Note: Pod is created on demand, not at startup
// ConfigMap is no longer used - model config is read directly from CRD
func (m *K8sManager) EnsureVLLMResources(ctx context.Context, initialModel *ModelConfig) error {
	// Ensure Service exists
	if err := m.ensureService(ctx); err != nil {
		return fmt.Errorf("failed to ensure service: %w", err)
	}

	// Note: We don't create the pod here - it will be created on first request
	log.Printf("K8s resources initialized (pod will be created on demand)")
	log.Printf("Model config will be read directly from CRD: %s", initialModel.ServedModelName)

	return nil
}

// ensureConfigMap is deprecated - model config is now read directly from CRD
// Kept for backward compatibility but not used
func (m *K8sManager) ensureConfigMap(ctx context.Context, modelConfig *ModelConfig) error {
	log.Printf("ConfigMap management is deprecated - model config is read directly from CRD")
	return nil
}

// ensureService creates the vLLM service if it doesn't exist
// Note: Service name must NOT be "vllm" to avoid K8s env var conflicts (VLLM_SERVICE_HOST, etc.)
func (m *K8sManager) ensureService(ctx context.Context) error {
	serviceName := "vllm-api"
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

// CreatePod creates a new vLLM pod
func (m *K8sManager) CreatePod(ctx context.Context, modelConfig *ModelConfig) error {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      m.config.Deployment,
			Namespace: m.config.Namespace,
			Labels: map[string]string{
				"app":        "vllm",
				"managed-by": "vllm-chill",
			},
		},
		Spec: m.buildPodSpec(modelConfig),
	}

	_, err := m.clientset.CoreV1().Pods(m.config.Namespace).Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create pod: %w", err)
	}

	log.Printf("Created Pod %s/%s", m.config.Namespace, m.config.Deployment)
	return nil
}

// DeletePod deletes the vLLM pod
func (m *K8sManager) DeletePod(ctx context.Context) error {
	err := m.clientset.CoreV1().Pods(m.config.Namespace).Delete(
		ctx,
		m.config.Deployment,
		metav1.DeleteOptions{
			GracePeriodSeconds: func() *int64 { t := int64(0); return &t }(),
		},
	)
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete pod: %w", err)
	}

	log.Printf("Deleted Pod %s/%s", m.config.Namespace, m.config.Deployment)
	return nil
}

// GetPod gets the vLLM pod
func (m *K8sManager) GetPod(ctx context.Context) (*corev1.Pod, error) {
	pod, err := m.clientset.CoreV1().Pods(m.config.Namespace).Get(
		ctx,
		m.config.Deployment,
		metav1.GetOptions{},
	)
	if err != nil {
		return nil, err
	}
	return pod, nil
}

// PodExists checks if the vLLM pod exists
func (m *K8sManager) PodExists(ctx context.Context) (bool, error) {
	_, err := m.GetPod(ctx)
	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// VerifyPodConfig checks if the running pod configuration matches the expected model config
// Returns true if config matches, false if there's a drift
func (m *K8sManager) VerifyPodConfig(ctx context.Context, modelConfig *ModelConfig) (bool, error) {
	pod, err := m.GetPod(ctx)
	if err != nil {
		if errors.IsNotFound(err) {
			return true, nil // No pod means no drift
		}
		return false, fmt.Errorf("failed to get pod: %w", err)
	}

	// Check if pod is running
	if pod.Status.Phase != corev1.PodRunning {
		return true, nil // Pod not running yet, skip verification
	}

	// Get container args from running pod
	if len(pod.Spec.Containers) == 0 {
		return false, fmt.Errorf("no containers in pod")
	}
	container := pod.Spec.Containers[0]

	// Build expected args from model config
	expectedArgs := m.buildVLLMArgs(modelConfig)

	// Compare key parameters
	actualArgsMap := argsToMap(container.Args)
	expectedArgsMap := argsToMap(expectedArgs)

	// Check critical parameters that should match
	criticalParams := []string{
		"--model",
		"--served-model-name",
		"--max-model-len",
		"--gpu-memory-utilization",
		"--max-num-batched-tokens",
		"--max-num-seqs",
		"--dtype",
		"--cpu-offload-gb",
		"--tool-call-parser",
	}

	for _, param := range criticalParams {
		actual := actualArgsMap[param]
		expected := expectedArgsMap[param]
		if actual != expected {
			log.Printf("Config drift detected: %s actual=%s expected=%s", param, actual, expected)
			return false, nil
		}
	}

	return true, nil
}

// argsToMap converts args slice to map for easier comparison
func argsToMap(args []string) map[string]string {
	m := make(map[string]string)
	for i := 0; i < len(args); i++ {
		if args[i][0] == '-' && i+1 < len(args) && args[i+1][0] != '-' {
			m[args[i]] = args[i+1]
			i++ // Skip next arg as it's the value
		} else if args[i][0] == '-' {
			m[args[i]] = "true" // Flag without value
		}
	}
	return m
}

// buildVLLMArgs builds the vLLM command-line arguments from ModelConfig
func (m *K8sManager) buildVLLMArgs(modelConfig *ModelConfig) []string {
	// Use GPU count from infrastructure config for tensor-parallel-size
	gpuCount := m.config.GPUCount
	if gpuCount == 0 {
		gpuCount = 2 // Default to 2 GPUs
	}

	args := []string{
		"--model", modelConfig.ModelName,
		"--served-model-name", modelConfig.ServedModelName,
		"--tensor-parallel-size", fmt.Sprintf("%d", gpuCount),
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

	// Use CPU offload from infrastructure config
	cpuOffloadGB := m.config.CPUOffloadGB
	args = append(args, "--cpu-offload-gb", fmt.Sprintf("%d", cpuOffloadGB))

	if modelConfig.EnableAutoToolChoice == "true" {
		args = append(args, "--enable-auto-tool-choice")
	}

	args = append(args,
		"--tool-call-parser", modelConfig.ToolCallParser,
	)

	// Add reasoning parser if specified
	if modelConfig.ReasoningParser != "" {
		args = append(args, "--reasoning-parser", modelConfig.ReasoningParser)
	}

	args = append(args,
		"--host", "0.0.0.0",
		"--port", "8000",
		"--api-key", "$(VLLM_API_KEY)",
	)

	return args
}

// buildPodSpec builds the pod specification for vLLM
func (m *K8sManager) buildPodSpec(modelConfig *ModelConfig) corev1.PodSpec {
	// Use GPU count from infrastructure config (not model config)
	gpuCount := m.config.GPUCount
	if gpuCount == 0 {
		gpuCount = 2 // Default to 2 GPUs
	}
	gpuCountStr := fmt.Sprintf("%d", gpuCount)

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
				Command:         []string{"python3", "-m", "vllm.entrypoints.openai.api_server"},
				Args:            m.buildVLLMArgs(modelConfig),
				Env:             m.buildVLLMEnvVars(),
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
						corev1.ResourceMemory: resource.MustParse("64Gi"),
						"nvidia.com/gpu":      resource.MustParse(gpuCountStr),
					},
					Requests: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("32Gi"),
						"nvidia.com/gpu":      resource.MustParse(gpuCountStr),
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
								{Name: "Authorization", Value: "Bearer $(VLLM_API_KEY)"},
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
								{Name: "Authorization", Value: "Bearer $(VLLM_API_KEY)"},
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
								{Name: "Authorization", Value: "Bearer $(VLLM_API_KEY)"},
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

// buildVLLMEnvVars builds environment variables for the vLLM container
func (m *K8sManager) buildVLLMEnvVars() []corev1.EnvVar {
	envVars := []corev1.EnvVar{
		// vLLM API key from secret
		{
			Name: "VLLM_API_KEY",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: "vllm-api-key"},
					Key:                  "api-key",
				},
			},
		},
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
