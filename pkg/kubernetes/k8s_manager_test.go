package kubernetes

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestK8sManager_EnsureConfigMap(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	config := &Config{
		Namespace:     "test-ns",
		Deployment:    "vllm",
		ConfigMapName: "vllm-config",
	}
	manager := NewK8sManager(clientset, config)

	modelConfig := &ModelConfig{
		ModelName:       "test/model",
		ServedModelName: "test-model",
	}

	ctx := context.Background()

	t.Run("deprecated - returns immediately", func(t *testing.T) {
		err := manager.ensureConfigMap(ctx, modelConfig)
		if err != nil {
			t.Fatalf("ensureConfigMap() error = %v", err)
		}

		// Verify ConfigMap was NOT created (deprecated function)
		_, err = clientset.CoreV1().ConfigMaps("test-ns").Get(ctx, "vllm-config", metav1.GetOptions{})
		if err == nil {
			t.Error("ConfigMap should not be created (function is deprecated)")
		}
	})
}

func TestK8sManager_EnsureService(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	config := &Config{
		Namespace: "test-ns",
	}
	manager := NewK8sManager(clientset, config)

	ctx := context.Background()

	t.Run("create new service", func(t *testing.T) {
		err := manager.ensureService(ctx)
		if err != nil {
			t.Fatalf("ensureService() error = %v", err)
		}

		// Verify Service was created
		svc, err := clientset.CoreV1().Services("test-ns").Get(ctx, "vllm-api", metav1.GetOptions{})
		if err != nil {
			t.Fatalf("Failed to get Service: %v", err)
		}

		if svc.Spec.Ports[0].Port != 80 {
			t.Errorf("Service port = %v, want 80", svc.Spec.Ports[0].Port)
		}
		if svc.Spec.Selector["app"] != "vllm" {
			t.Errorf("Service selector app = %v, want vllm", svc.Spec.Selector["app"])
		}
	})

	t.Run("service already exists", func(t *testing.T) {
		err := manager.ensureService(ctx)
		if err != nil {
			t.Fatalf("ensureService() error = %v", err)
		}

		// Should not error when service already exists
		services, err := clientset.CoreV1().Services("test-ns").List(ctx, metav1.ListOptions{})
		if err != nil {
			t.Fatalf("Failed to list services: %v", err)
		}
		if len(services.Items) != 1 {
			t.Errorf("Expected 1 service, got %d", len(services.Items))
		}
	})
}

func TestK8sManager_CreatePod(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	config := &Config{
		Namespace:     "test-ns",
		Deployment:    "vllm",
		ConfigMapName: "vllm-config",
		GPUCount:      2,
		CPUOffloadGB:  0,
	}
	manager := NewK8sManager(clientset, config)

	modelConfig := &ModelConfig{
		ModelName:              "test/model",
		ServedModelName:        "test-model",
		MaxModelLen:            "8192",
		GPUMemoryUtilization:   "0.9",
		EnableChunkedPrefill:   "false",
		MaxNumBatchedTokens:    "8192",
		MaxNumSeqs:             "256",
		Dtype:                  "auto",
		DisableCustomAllReduce: "false",
		EnablePrefixCaching:    "true",
		EnableAutoToolChoice:   "true",
		ToolCallParser:         "hermes",
	}

	ctx := context.Background()

	t.Run("create new pod", func(t *testing.T) {
		err := manager.CreatePod(ctx, modelConfig)
		if err != nil {
			t.Fatalf("CreatePod() error = %v", err)
		}

		// Verify Pod was created
		pod, err := clientset.CoreV1().Pods("test-ns").Get(ctx, "vllm", metav1.GetOptions{})
		if err != nil {
			t.Fatalf("Failed to get Pod: %v", err)
		}

		if pod.Spec.Containers[0].Name != "vllm" {
			t.Errorf("Container name = %v, want vllm", pod.Spec.Containers[0].Name)
		}

		// Verify GPU resources
		gpuLimit := pod.Spec.Containers[0].Resources.Limits["nvidia.com/gpu"]
		if gpuLimit.String() != "2" {
			t.Errorf("GPU limit = %v, want 2", gpuLimit.String())
		}
	})
}

func TestK8sManager_EnsureVLLMResources(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	config := &Config{
		Namespace:     "test-ns",
		Deployment:    "vllm",
		ConfigMapName: "vllm-config",
	}
	manager := NewK8sManager(clientset, config)

	modelConfig := &ModelConfig{
		ModelName:       "test/model",
		ServedModelName: "test-model",
	}

	ctx := context.Background()

	err := manager.EnsureVLLMResources(ctx, modelConfig)
	if err != nil {
		t.Fatalf("EnsureVLLMResources() error = %v", err)
	}

	// Note: ConfigMap is deprecated and no longer created

	_, err = clientset.CoreV1().Services("test-ns").Get(ctx, "vllm-api", metav1.GetOptions{})
	if err != nil {
		t.Errorf("Service not created: %v", err)
	}

	// Note: Pods are created on demand, not during EnsureVLLMResources
}

func TestK8sManager_BuildSystemEnvVars(t *testing.T) {
	config := &Config{
		ConfigMapName: "test-config",
	}
	manager := NewK8sManager(nil, config)

	envVars := manager.buildVLLMEnvVars()

	// Check that system env vars are present (no model-specific vars)
	requiredSystemVars := []string{
		"TORCH_CUDA_ARCH_LIST",
		"VLLM_TORCH_COMPILE_CACHE_DIR",
		"HF_HUB_ENABLE_HF_TRANSFER",
		"OMP_NUM_THREADS",
		"HF_TOKEN",
		"HUGGING_FACE_HUB_TOKEN",
	}

	envVarMap := make(map[string]bool)
	for _, ev := range envVars {
		envVarMap[ev.Name] = true
	}

	for _, required := range requiredSystemVars {
		if !envVarMap[required] {
			t.Errorf("Missing required system env var: %s", required)
		}
	}

	// Ensure model-specific vars are NOT present
	modelSpecificVars := []string{
		"MODEL_NAME",
		"SERVED_MODEL_NAME",
		"TOOL_CALL_PARSER",
		"TENSOR_PARALLEL_SIZE",
	}

	for _, modelVar := range modelSpecificVars {
		if envVarMap[modelVar] {
			t.Errorf("Model-specific env var %s should not be in system env vars", modelVar)
		}
	}
}

func TestK8sManager_BuildPodSpec(t *testing.T) {
	config := &Config{
		ConfigMapName: "test-config",
		GPUCount:      2,
		CPUOffloadGB:  0,
	}
	manager := NewK8sManager(nil, config)

	modelConfig := &ModelConfig{
		ModelName:              "test/model",
		ServedModelName:        "test-model",
		MaxModelLen:            "8192",
		GPUMemoryUtilization:   "0.9",
		EnableChunkedPrefill:   "false",
		MaxNumBatchedTokens:    "8192",
		MaxNumSeqs:             "256",
		Dtype:                  "auto",
		DisableCustomAllReduce: "false",
		EnablePrefixCaching:    "true",
		EnableAutoToolChoice:   "true",
		ToolCallParser:         "hermes",
	}

	podSpec := manager.buildPodSpec(modelConfig)

	if len(podSpec.Containers) != 1 {
		t.Fatalf("Expected 1 container, got %d", len(podSpec.Containers))
	}

	container := podSpec.Containers[0]

	if container.Name != "vllm" {
		t.Errorf("Container name = %v, want vllm", container.Name)
	}

	if container.Image != "vllm/vllm-openai:latest" {
		t.Errorf("Container image = %v, want vllm/vllm-openai:latest", container.Image)
	}

	if len(container.Ports) != 2 {
		t.Fatalf("Expected 2 ports, got %d", len(container.Ports))
	}

	// Check HTTP port (8000)
	if container.Ports[0].ContainerPort != 8000 {
		t.Errorf("Container HTTP port = %v, want 8000", container.Ports[0].ContainerPort)
	}

	// Check metrics port (8001)
	if container.Ports[1].ContainerPort != 8001 {
		t.Errorf("Container metrics port = %v, want 8001", container.Ports[1].ContainerPort)
	}

	if container.ReadinessProbe == nil {
		t.Error("ReadinessProbe is nil")
	}

	// Verify args are built from modelConfig (not env vars)
	if len(container.Args) == 0 {
		t.Error("Container Args is empty")
	}

	// Check that args contain the model name directly
	argsContainModel := false
	for i, arg := range container.Args {
		if arg == "--model" && i+1 < len(container.Args) && container.Args[i+1] == "test/model" {
			argsContainModel = true
			break
		}
	}
	if !argsContainModel {
		t.Error("Args should contain --model test/model")
	}
}

func TestK8sManager_WithExistingResources(t *testing.T) {
	// Pre-create resources
	existingConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vllm-config",
			Namespace: "test-ns",
		},
		Data: map[string]string{
			"MODEL_NAME": "old/model",
		},
	}

	existingService := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vllm-api",
			Namespace: "test-ns",
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{Port: 80},
			},
		},
	}

	clientset := fake.NewSimpleClientset(existingConfigMap, existingService)
	config := &Config{
		Namespace:     "test-ns",
		Deployment:    "vllm",
		ConfigMapName: "vllm-config",
	}
	manager := NewK8sManager(clientset, config)

	modelConfig := &ModelConfig{
		ModelName:       "new/model",
		ServedModelName: "new-model",
	}

	ctx := context.Background()

	err := manager.EnsureVLLMResources(ctx, modelConfig)
	if err != nil {
		t.Fatalf("EnsureVLLMResources() error = %v", err)
	}

	// Note: ConfigMap is deprecated and no longer updated

	// Verify Service still exists
	_, err = clientset.CoreV1().Services("test-ns").Get(ctx, "vllm-api", metav1.GetOptions{})
	if err != nil {
		t.Errorf("Service should still exist: %v", err)
	}
}

func TestK8sManager_DeletePod(t *testing.T) {
	// Pre-create a pod
	existingPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vllm",
			Namespace: "test-ns",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "vllm", Image: "vllm/vllm-openai:latest"},
			},
		},
	}

	clientset := fake.NewSimpleClientset(existingPod)
	config := &Config{
		Namespace:  "test-ns",
		Deployment: "vllm",
	}
	manager := NewK8sManager(clientset, config)

	ctx := context.Background()

	t.Run("delete existing pod", func(t *testing.T) {
		err := manager.DeletePod(ctx)
		if err != nil {
			t.Fatalf("DeletePod() error = %v", err)
		}

		// Verify Pod was deleted
		_, err = clientset.CoreV1().Pods("test-ns").Get(ctx, "vllm", metav1.GetOptions{})
		if err == nil {
			t.Error("Pod should have been deleted")
		}
	})

	t.Run("delete non-existent pod", func(t *testing.T) {
		// Should not error when pod doesn't exist
		err := manager.DeletePod(ctx)
		if err != nil {
			t.Fatalf("DeletePod() should not error on non-existent pod: %v", err)
		}
	})
}

func TestK8sManager_GetPod(t *testing.T) {
	existingPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vllm",
			Namespace: "test-ns",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "vllm", Image: "vllm/vllm-openai:latest"},
			},
		},
	}

	clientset := fake.NewSimpleClientset(existingPod)
	config := &Config{
		Namespace:  "test-ns",
		Deployment: "vllm",
	}
	manager := NewK8sManager(clientset, config)

	ctx := context.Background()

	t.Run("get existing pod", func(t *testing.T) {
		pod, err := manager.GetPod(ctx)
		if err != nil {
			t.Fatalf("GetPod() error = %v", err)
		}

		if pod.Name != "vllm" {
			t.Errorf("Pod name = %v, want vllm", pod.Name)
		}
	})
}

func TestK8sManager_PodExists(t *testing.T) {
	existingPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vllm",
			Namespace: "test-ns",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "vllm", Image: "vllm/vllm-openai:latest"},
			},
		},
	}

	clientset := fake.NewSimpleClientset(existingPod)
	config := &Config{
		Namespace:  "test-ns",
		Deployment: "vllm",
	}
	manager := NewK8sManager(clientset, config)

	ctx := context.Background()

	t.Run("pod exists", func(t *testing.T) {
		exists, err := manager.PodExists(ctx)
		if err != nil {
			t.Fatalf("PodExists() error = %v", err)
		}

		if !exists {
			t.Error("Pod should exist")
		}
	})

	t.Run("pod does not exist", func(t *testing.T) {
		emptyClientset := fake.NewSimpleClientset()
		emptyManager := NewK8sManager(emptyClientset, config)

		exists, err := emptyManager.PodExists(ctx)
		if err != nil {
			t.Fatalf("PodExists() error = %v", err)
		}

		if exists {
			t.Error("Pod should not exist")
		}
	})
}

func TestK8sManager_VerifyPodConfig(t *testing.T) {
	modelConfig := &ModelConfig{
		ModelName:              "test/model",
		ServedModelName:        "test-model",
		MaxModelLen:            "8192",
		GPUMemoryUtilization:   "0.9",
		EnableChunkedPrefill:   "false",
		MaxNumBatchedTokens:    "8192",
		MaxNumSeqs:             "256",
		Dtype:                  "auto",
		DisableCustomAllReduce: "false",
		EnablePrefixCaching:    "true",
		EnableAutoToolChoice:   "true",
		ToolCallParser:         "hermes",
	}

	config := &Config{
		Namespace:     "test-ns",
		Deployment:    "vllm",
		GPUCount:      2,
		CPUOffloadGB:  0,
		ConfigMapName: "vllm-config",
	}
	manager := NewK8sManager(nil, config)

	// Build the actual args that would be generated
	expectedArgs := manager.buildVLLMArgs(modelConfig)

	// Create a pod with matching args - must be Running to trigger verification
	existingPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vllm",
			Namespace: "test-ns",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "vllm",
					Image: "vllm/vllm-openai:latest",
					Args:  expectedArgs,
				},
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
	}

	clientset := fake.NewSimpleClientset(existingPod)
	manager = NewK8sManager(clientset, config)

	ctx := context.Background()

	t.Run("matching config", func(t *testing.T) {
		matches, err := manager.VerifyPodConfig(ctx, modelConfig)
		if err != nil {
			t.Fatalf("VerifyPodConfig() error = %v", err)
		}

		if !matches {
			t.Error("Config should match")
		}
	})

	t.Run("non-matching config", func(t *testing.T) {
		// Manually create different args (simple hardcoded args to avoid buildVLLMArgs issues)
		differentArgs := []string{
			"--model", "different/model",
			"--served-model-name", "different-model",
			"--max-model-len", "4096",
		}

		// Create a pod with the different config
		differentPod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "vllm",
				Namespace: "test-ns",
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  "vllm",
						Image: "vllm/vllm-openai:latest",
						Args:  differentArgs,
					},
				},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
			},
		}

		differentClientset := fake.NewSimpleClientset(differentPod)
		differentManager := NewK8sManager(differentClientset, config)

		// Try to verify with the original model config (should not match)
		matches, err := differentManager.VerifyPodConfig(ctx, modelConfig)
		if err != nil {
			t.Fatalf("VerifyPodConfig() error = %v", err)
		}

		if matches {
			t.Error("Config should not match")
		}
	})
}

func TestArgsToMap(t *testing.T) {
	args := []string{
		"--model", "test/model",
		"--served-model-name", "test-model",
		"--max-model-len", "8192",
		"--gpu-memory-utilization", "0.9",
	}

	argsMap := argsToMap(args)

	expectedMap := map[string]string{
		"--model":                  "test/model",
		"--served-model-name":      "test-model",
		"--max-model-len":          "8192",
		"--gpu-memory-utilization": "0.9",
	}

	for key, expectedValue := range expectedMap {
		if value, ok := argsMap[key]; !ok {
			t.Errorf("Missing key %s", key)
		} else if value != expectedValue {
			t.Errorf("Key %s: got %v, want %v", key, value, expectedValue)
		}
	}
}
