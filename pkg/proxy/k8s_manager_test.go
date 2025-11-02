package proxy

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
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
		TargetHost:    "vllm-svc",
	}
	manager := NewK8sManager(clientset, config)

	modelConfig := &ModelConfig{
		ModelName:       "test/model",
		ServedModelName: "test-model",
	}

	ctx := context.Background()

	t.Run("create new configmap", func(t *testing.T) {
		err := manager.ensureConfigMap(ctx, modelConfig)
		if err != nil {
			t.Fatalf("ensureConfigMap() error = %v", err)
		}

		// Verify ConfigMap was created
		cm, err := clientset.CoreV1().ConfigMaps("test-ns").Get(ctx, "vllm-config", metav1.GetOptions{})
		if err != nil {
			t.Fatalf("Failed to get ConfigMap: %v", err)
		}

		if cm.Data["MODEL_NAME"] != "test/model" {
			t.Errorf("MODEL_NAME = %v, want test/model", cm.Data["MODEL_NAME"])
		}
		if cm.Data["SERVED_MODEL_NAME"] != "test-model" {
			t.Errorf("SERVED_MODEL_NAME = %v, want test-model", cm.Data["SERVED_MODEL_NAME"])
		}
	})

	t.Run("update existing configmap", func(t *testing.T) {
		updatedModel := &ModelConfig{
			ModelName:       "test/updated-model",
			ServedModelName: "updated-model",
		}

		err := manager.ensureConfigMap(ctx, updatedModel)
		if err != nil {
			t.Fatalf("ensureConfigMap() error = %v", err)
		}

		// Verify ConfigMap was updated
		cm, err := clientset.CoreV1().ConfigMaps("test-ns").Get(ctx, "vllm-config", metav1.GetOptions{})
		if err != nil {
			t.Fatalf("Failed to get ConfigMap: %v", err)
		}

		if cm.Data["MODEL_NAME"] != "test/updated-model" {
			t.Errorf("MODEL_NAME = %v, want test/updated-model", cm.Data["MODEL_NAME"])
		}
	})
}

func TestK8sManager_EnsureService(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	config := &Config{
		Namespace:  "test-ns",
		TargetHost: "vllm-svc",
	}
	manager := NewK8sManager(clientset, config)

	ctx := context.Background()

	t.Run("create new service", func(t *testing.T) {
		err := manager.ensureService(ctx)
		if err != nil {
			t.Fatalf("ensureService() error = %v", err)
		}

		// Verify Service was created
		svc, err := clientset.CoreV1().Services("test-ns").Get(ctx, "vllm-svc", metav1.GetOptions{})
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

func TestK8sManager_EnsureDeployment(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	config := &Config{
		Namespace:     "test-ns",
		Deployment:    "vllm",
		ConfigMapName: "vllm-config",
	}
	manager := NewK8sManager(clientset, config)

	ctx := context.Background()

	t.Run("create new deployment", func(t *testing.T) {
		err := manager.ensureDeployment(ctx)
		if err != nil {
			t.Fatalf("ensureDeployment() error = %v", err)
		}

		// Verify Deployment was created
		dep, err := clientset.AppsV1().Deployments("test-ns").Get(ctx, "vllm", metav1.GetOptions{})
		if err != nil {
			t.Fatalf("Failed to get Deployment: %v", err)
		}

		if *dep.Spec.Replicas != 0 {
			t.Errorf("Deployment replicas = %v, want 0", *dep.Spec.Replicas)
		}
		if dep.Spec.Template.Spec.Containers[0].Name != "vllm" {
			t.Errorf("Container name = %v, want vllm", dep.Spec.Template.Spec.Containers[0].Name)
		}
	})

	t.Run("deployment already exists", func(t *testing.T) {
		err := manager.ensureDeployment(ctx)
		if err != nil {
			t.Fatalf("ensureDeployment() error = %v", err)
		}

		// Should not error when deployment already exists
		deployments, err := clientset.AppsV1().Deployments("test-ns").List(ctx, metav1.ListOptions{})
		if err != nil {
			t.Fatalf("Failed to list deployments: %v", err)
		}
		if len(deployments.Items) != 1 {
			t.Errorf("Expected 1 deployment, got %d", len(deployments.Items))
		}
	})
}

func TestK8sManager_EnsureVLLMResources(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	config := &Config{
		Namespace:     "test-ns",
		Deployment:    "vllm",
		ConfigMapName: "vllm-config",
		TargetHost:    "vllm-svc",
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

	// Verify all resources were created
	_, err = clientset.CoreV1().ConfigMaps("test-ns").Get(ctx, "vllm-config", metav1.GetOptions{})
	if err != nil {
		t.Errorf("ConfigMap not created: %v", err)
	}

	_, err = clientset.CoreV1().Services("test-ns").Get(ctx, "vllm-svc", metav1.GetOptions{})
	if err != nil {
		t.Errorf("Service not created: %v", err)
	}

	_, err = clientset.AppsV1().Deployments("test-ns").Get(ctx, "vllm", metav1.GetOptions{})
	if err != nil {
		t.Errorf("Deployment not created: %v", err)
	}
}

func TestK8sManager_BuildEnvVars(t *testing.T) {
	config := &Config{
		ConfigMapName: "test-config",
	}
	manager := NewK8sManager(nil, config)

	envVars := manager.buildEnvVars()

	// Check that all required env vars are present
	requiredVars := []string{
		"MODEL_NAME",
		"SERVED_MODEL_NAME",
		"TOOL_CALL_PARSER",
		"REASONING_PARSER",
		"TENSOR_PARALLEL_SIZE",
		"MAX_MODEL_LEN",
		"GPU_MEMORY_UTILIZATION",
		"ENABLE_CHUNKED_PREFILL",
		"MAX_NUM_BATCHED_TOKENS",
		"MAX_NUM_SEQS",
		"DTYPE",
		"DISABLE_CUSTOM_ALL_REDUCE",
		"ENABLE_PREFIX_CACHING",
		"CPU_OFFLOAD_GB",
		"ENABLE_AUTO_TOOL_CHOICE",
	}

	envVarMap := make(map[string]bool)
	for _, ev := range envVars {
		envVarMap[ev.Name] = true
		// Verify all env vars reference the correct ConfigMap
		if ev.ValueFrom == nil || ev.ValueFrom.ConfigMapKeyRef == nil {
			t.Errorf("Env var %s missing ConfigMapKeyRef", ev.Name)
			continue
		}
		if ev.ValueFrom.ConfigMapKeyRef.Name != "test-config" {
			t.Errorf("Env var %s references wrong ConfigMap: %s", ev.Name, ev.ValueFrom.ConfigMapKeyRef.Name)
		}
	}

	for _, required := range requiredVars {
		if !envVarMap[required] {
			t.Errorf("Missing required env var: %s", required)
		}
	}
}

func TestK8sManager_BuildPodSpec(t *testing.T) {
	config := &Config{
		ConfigMapName: "test-config",
	}
	manager := NewK8sManager(nil, config)

	podSpec := manager.buildPodSpec()

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

	if len(container.Ports) != 1 {
		t.Fatalf("Expected 1 port, got %d", len(container.Ports))
	}

	if container.Ports[0].ContainerPort != 8000 {
		t.Errorf("Container port = %v, want 8000", container.Ports[0].ContainerPort)
	}

	if container.ReadinessProbe == nil {
		t.Error("ReadinessProbe is nil")
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
			Name:      "vllm-svc",
			Namespace: "test-ns",
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{Port: 80},
			},
		},
	}

	replicas := int32(1)
	existingDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vllm",
			Namespace: "test-ns",
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
		},
	}

	clientset := fake.NewSimpleClientset(existingConfigMap, existingService, existingDeployment)
	config := &Config{
		Namespace:     "test-ns",
		Deployment:    "vllm",
		ConfigMapName: "vllm-config",
		TargetHost:    "vllm-svc",
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

	// Verify ConfigMap was updated
	cm, err := clientset.CoreV1().ConfigMaps("test-ns").Get(ctx, "vllm-config", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Failed to get ConfigMap: %v", err)
	}
	if cm.Data["MODEL_NAME"] != "new/model" {
		t.Errorf("ConfigMap not updated, MODEL_NAME = %v, want new/model", cm.Data["MODEL_NAME"])
	}

	// Verify Service still exists
	_, err = clientset.CoreV1().Services("test-ns").Get(ctx, "vllm-svc", metav1.GetOptions{})
	if err != nil {
		t.Errorf("Service should still exist: %v", err)
	}

	// Verify Deployment still exists
	_, err = clientset.AppsV1().Deployments("test-ns").Get(ctx, "vllm", metav1.GetOptions{})
	if err != nil {
		t.Errorf("Deployment should still exist: %v", err)
	}
}
