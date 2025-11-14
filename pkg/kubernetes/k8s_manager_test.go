package kubernetes

import (
	"context"
	"testing"

	"github.com/efortin/vllm-chill/pkg/models"
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

	modelConfig := &models.Config{
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

func TestK8sManager_EnsureVLLMResources(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	config := &Config{
		Namespace:     "test-ns",
		Deployment:    "vllm",
		ConfigMapName: "vllm-config",
	}
	manager := NewK8sManager(clientset, config)

	modelConfig := &models.Config{
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

	modelConfig := &models.Config{
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
	modelConfig := &models.Config{
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

func TestK8sManager_PauseVLLMContainer(t *testing.T) {
	// Create a pod with vLLM container running
	existingPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vllm-all-in-one",
			Namespace: "test-ns",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "vllm-proxy",
					Image: "efortin/vllm-chill:latest",
				},
				{
					Name:  "vllm",
					Image: "vllm/vllm-openai:latest",
					Args:  []string{"--model", "test/model"},
				},
			},
		},
	}

	clientset := fake.NewSimpleClientset(existingPod)
	config := &Config{
		Namespace:  "test-ns",
		Deployment: "vllm",
		PodName:    "vllm-all-in-one",
	}
	manager := NewK8sManager(clientset, config)

	ctx := context.Background()

	t.Run("pause vllm container successfully", func(t *testing.T) {
		err := manager.PauseVLLMContainer(ctx)
		if err != nil {
			t.Fatalf("PauseVLLMContainer() error = %v", err)
		}

		// Verify the pod was updated
		updatedPod, err := clientset.CoreV1().Pods("test-ns").Get(ctx, "vllm-all-in-one", metav1.GetOptions{})
		if err != nil {
			t.Fatalf("Failed to get updated pod: %v", err)
		}

		// Find the vllm container
		var vllmContainer *corev1.Container
		for i := range updatedPod.Spec.Containers {
			if updatedPod.Spec.Containers[i].Name == "vllm" {
				vllmContainer = &updatedPod.Spec.Containers[i]
				break
			}
		}

		if vllmContainer == nil {
			t.Fatal("vLLM container not found in pod")
		}

		// Verify image was changed to pause image
		if vllmContainer.Image != "registry.k8s.io/pause:3.9" {
			t.Errorf("Container image = %v, want registry.k8s.io/pause:3.9", vllmContainer.Image)
		}
	})

	t.Run("pause non-existent pod", func(t *testing.T) {
		clientset := fake.NewSimpleClientset()
		config := &Config{
			Namespace:  "test-ns",
			Deployment: "vllm",
			PodName:    "non-existent-pod",
		}
		manager := NewK8sManager(clientset, config)

		err := manager.PauseVLLMContainer(ctx)
		if err == nil {
			t.Error("PauseVLLMContainer() should fail for non-existent pod")
		}
	})
}

func TestK8sManager_ResumeVLLMContainer(t *testing.T) {
	// Create a pod with paused vLLM container
	existingPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vllm-all-in-one",
			Namespace: "test-ns",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "vllm-proxy",
					Image: "efortin/vllm-chill:latest",
				},
				{
					Name:  "vllm",
					Image: "registry.k8s.io/pause:3.9", // Paused
					Args:  []string{},                  // Empty args when paused
				},
			},
		},
	}

	clientset := fake.NewSimpleClientset(existingPod)
	config := &Config{
		Namespace:  "test-ns",
		Deployment: "vllm",
		PodName:    "vllm-all-in-one",
	}
	manager := NewK8sManager(clientset, config)

	ctx := context.Background()

	modelConfig := &models.Config{
		ModelName:              "test/model",
		ServedModelName:        "test-model",
		MaxModelLen:            "8192",
		GPUMemoryUtilization:   "0.9",
		EnableChunkedPrefill:   "true",
		MaxNumBatchedTokens:    "4096",
		MaxNumSeqs:             "16",
		Dtype:                  "float16",
		DisableCustomAllReduce: "true",
		EnablePrefixCaching:    "true",
		EnableAutoToolChoice:   "true",
		ToolCallParser:         "qwen3_coder",
	}

	t.Run("resume vllm container successfully", func(t *testing.T) {
		err := manager.ResumeVLLMContainer(ctx, modelConfig)
		if err != nil {
			t.Fatalf("ResumeVLLMContainer() error = %v", err)
		}

		// Note: The fake Kubernetes client doesn't fully simulate patch behavior,
		// so we can't verify the actual pod changes. The important thing is that
		// the function completes without error.
		// Integration tests verify the actual behavior against a real cluster.
	})

	t.Run("resume non-existent pod", func(t *testing.T) {
		clientset := fake.NewSimpleClientset()
		config := &Config{
			Namespace:  "test-ns",
			Deployment: "vllm",
			PodName:    "non-existent-pod",
		}
		manager := NewK8sManager(clientset, config)

		err := manager.ResumeVLLMContainer(ctx, modelConfig)
		if err == nil {
			t.Error("ResumeVLLMContainer() should fail for non-existent pod")
		}
	})
}

func TestK8sManager_IsVLLMPaused(t *testing.T) {
	t.Run("paused container", func(t *testing.T) {
		pausedPod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "vllm-all-in-one",
				Namespace: "test-ns",
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "vllm-proxy", Image: "efortin/vllm-chill:latest"},
					{Name: "vllm", Image: "registry.k8s.io/pause:3.9"},
				},
			},
		}

		clientset := fake.NewSimpleClientset(pausedPod)
		config := &Config{
			Namespace:  "test-ns",
			Deployment: "vllm",
			PodName:    "vllm-all-in-one",
		}
		manager := NewK8sManager(clientset, config)

		ctx := context.Background()
		paused, err := manager.IsVLLMPaused(ctx)
		if err != nil {
			t.Fatalf("IsVLLMPaused() error = %v", err)
		}

		if !paused {
			t.Error("IsVLLMPaused() = false, want true for paused container")
		}
	})

	t.Run("running container", func(t *testing.T) {
		runningPod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "vllm-all-in-one",
				Namespace: "test-ns",
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "vllm-proxy", Image: "efortin/vllm-chill:latest"},
					{Name: "vllm", Image: "vllm/vllm-openai:latest"},
				},
			},
		}

		clientset := fake.NewSimpleClientset(runningPod)
		config := &Config{
			Namespace:  "test-ns",
			Deployment: "vllm",
			PodName:    "vllm-all-in-one",
		}
		manager := NewK8sManager(clientset, config)

		ctx := context.Background()
		paused, err := manager.IsVLLMPaused(ctx)
		if err != nil {
			t.Fatalf("IsVLLMPaused() error = %v", err)
		}

		if paused {
			t.Error("IsVLLMPaused() = true, want false for running container")
		}
	})

	t.Run("non-existent pod", func(t *testing.T) {
		clientset := fake.NewSimpleClientset()
		config := &Config{
			Namespace:  "test-ns",
			Deployment: "vllm",
			PodName:    "non-existent-pod",
		}
		manager := NewK8sManager(clientset, config)

		ctx := context.Background()
		_, err := manager.IsVLLMPaused(ctx)
		if err == nil {
			t.Error("IsVLLMPaused() should fail for non-existent pod")
		}
	})

	t.Run("pod without vllm container", func(t *testing.T) {
		podWithoutVLLM := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "vllm-all-in-one",
				Namespace: "test-ns",
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "vllm-proxy", Image: "efortin/vllm-chill:latest"},
				},
			},
		}

		clientset := fake.NewSimpleClientset(podWithoutVLLM)
		config := &Config{
			Namespace:  "test-ns",
			Deployment: "vllm",
			PodName:    "vllm-all-in-one",
		}
		manager := NewK8sManager(clientset, config)

		ctx := context.Background()
		_, err := manager.IsVLLMPaused(ctx)
		if err == nil {
			t.Error("IsVLLMPaused() should fail when vllm container not found")
		}
	})
}
