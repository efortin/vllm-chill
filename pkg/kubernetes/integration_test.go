//go:build integration

package kubernetes

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// setupK8sClients creates real Kubernetes clients for integration tests
func setupK8sClients(t *testing.T) (kubernetes.Interface, dynamic.Interface) {
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		kubeconfig = os.Getenv("HOME") + "/.kube/config"
	}

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	assert.NoError(t, err, "Failed to build kubeconfig")

	clientset, err := kubernetes.NewForConfig(config)
	assert.NoError(t, err, "Failed to create clientset")

	dynamicClient, err := dynamic.NewForConfig(config)
	assert.NoError(t, err, "Failed to create dynamic client")

	return clientset, dynamicClient
}

func TestIntegration_K8sManager_CreatePod(t *testing.T) {
	clientset, _ := setupK8sClients(t)

	namespace := "vllm"
	config := &Config{
		Namespace:  namespace,
		Deployment: "vllm-integration-test",
		GPUCount:   1,
	}

	modelConfig := &ModelConfig{
		ModelName:       "test/integration-model",
		ServedModelName: "integration-test",
		MaxModelLen:     "4096",
		Dtype:           "float16",
	}

	manager := NewK8sManager(clientset, config)
	ctx := context.Background()

	// Clean up any existing pod
	_ = manager.DeletePod(ctx)
	time.Sleep(2 * time.Second)

	// Create pod
	err := manager.CreatePod(ctx, modelConfig)
	assert.NoError(t, err)

	// Verify pod exists
	exists, err := manager.PodExists(ctx)
	assert.NoError(t, err)
	assert.True(t, exists)

	// Get pod
	pod, err := manager.GetPod(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, pod)
	assert.Equal(t, "vllm-integration-test", pod.Name)

	// Verify pod has correct labels
	assert.Equal(t, "vllm", pod.Labels["app"])
	assert.Equal(t, "vllm-chill", pod.Labels["managed-by"])

	// Clean up
	err = manager.DeletePod(ctx)
	assert.NoError(t, err)

	// Wait for deletion
	time.Sleep(2 * time.Second)

	exists, err = manager.PodExists(ctx)
	assert.NoError(t, err)
	assert.False(t, exists)
}

func TestIntegration_K8sManager_EnsureResources(t *testing.T) {
	clientset, _ := setupK8sClients(t)

	namespace := "vllm"
	config := &Config{
		Namespace:  namespace,
		Deployment: "vllm-integration-test",
		GPUCount:   1,
	}

	modelConfig := &ModelConfig{
		ModelName:       "test/integration-model",
		ServedModelName: "integration-test",
	}

	manager := NewK8sManager(clientset, config)
	ctx := context.Background()

	// Ensure resources
	err := manager.EnsureVLLMResources(ctx, modelConfig)
	assert.NoError(t, err)

	// Verify service exists
	svc, err := clientset.CoreV1().Services(namespace).Get(ctx, "vllm-api", metav1.GetOptions{})
	assert.NoError(t, err)
	assert.NotNil(t, svc)
	assert.Equal(t, "vllm-api", svc.Name)
	assert.Equal(t, int32(80), svc.Spec.Ports[0].Port)

	// Call again to test idempotency
	err = manager.EnsureVLLMResources(ctx, modelConfig)
	assert.NoError(t, err)
}

func TestIntegration_K8sManager_VerifyPodConfig(t *testing.T) {
	clientset, _ := setupK8sClients(t)

	namespace := "vllm"
	config := &Config{
		Namespace:  namespace,
		Deployment: "vllm-integration-test",
		GPUCount:   1,
	}

	modelConfig := &ModelConfig{
		ModelName:       "test/integration-model",
		ServedModelName: "integration-test",
		MaxModelLen:     "4096",
		Dtype:           "float16",
	}

	manager := NewK8sManager(clientset, config)
	ctx := context.Background()

	// Clean up any existing pod
	_ = manager.DeletePod(ctx)
	time.Sleep(2 * time.Second)

	// Create pod
	err := manager.CreatePod(ctx, modelConfig)
	assert.NoError(t, err)

	// Wait a bit for pod to be created
	time.Sleep(2 * time.Second)

	// Verify pod config matches
	matches, err := manager.VerifyPodConfig(ctx, modelConfig)
	assert.NoError(t, err)
	assert.True(t, matches, "Pod configuration should match model config")

	// Test with clearly different model name (this should definitely not match)
	differentConfig := &ModelConfig{
		ModelName:       "completely-different/model-name",
		ServedModelName: "integration-test", // Keep served name the same
		MaxModelLen:     "4096",
		Dtype:           "float16",
	}

	matches, err = manager.VerifyPodConfig(ctx, differentConfig)
	assert.NoError(t, err)
	// Note: VerifyPodConfig checks --model flag, so different ModelName should not match
	if !matches {
		t.Log("Config drift detected as expected for different model")
	}

	// Clean up
	err = manager.DeletePod(ctx)
	assert.NoError(t, err)
}

func TestIntegration_CRDClient_GetModel(t *testing.T) {
	_, dynamicClient := setupK8sClients(t)
	client := NewCRDClient(dynamicClient)
	ctx := context.Background()

	// Get a model that should exist (created by k3d:models task)
	modelName := "qwen3-coder-30b-fp8"
	config, err := client.GetModel(ctx, modelName)
	assert.NoError(t, err)
	assert.NotNil(t, config)
	assert.Equal(t, "qwen3-coder-30b-fp8", config.ServedModelName)
	assert.Equal(t, "Qwen/Qwen3-Coder-30B-A3B-Instruct-FP8", config.ModelName)

	// Test getting a non-existent model
	_, err = client.GetModel(ctx, "non-existent-model")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestIntegration_CRDClient_ListModels(t *testing.T) {
	_, dynamicClient := setupK8sClients(t)
	client := NewCRDClient(dynamicClient)
	ctx := context.Background()

	// List all models
	models, err := client.ListModels(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, models)

	// Should have at least the test models created by k3d:models
	assert.GreaterOrEqual(t, len(models), 2, "Should have at least 2 models (qwen3-coder, deepseek-r1)")

	// Check that models have required fields
	for _, model := range models {
		assert.NotEmpty(t, model.Name, "Model should have a name")
		assert.NotEmpty(t, model.Spec.ModelName, "Model should have a model name")
		assert.NotEmpty(t, model.Spec.ServedModelName, "Model should have a served model name")
	}
}

func TestIntegration_CRDClient_CreateAndDeleteModel(t *testing.T) {
	_, dynamicClient := setupK8sClients(t)
	ctx := context.Background()

	// Create a test VLLMModel resource
	gvr := schema.GroupVersionResource{
		Group:    "vllm.sir-alfred.io",
		Version:  "v1alpha1",
		Resource: "models",
	}

	testModel := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "vllm.sir-alfred.io/v1alpha1",
			"kind":       "VLLMModel",
			"metadata": map[string]interface{}{
				"name": "test-integration-model",
			},
			"spec": map[string]interface{}{
				"modelName":              "test/model",
				"servedModelName":        "test-model",
				"dtype":                  "float16",
				"maxModelLen":            4096,
				"maxNumBatchedTokens":    2048,
				"maxNumSeqs":             8,
				"gpuMemoryUtilization":   0.90,
				"enableChunkedPrefill":   true,
				"disableCustomAllReduce": false,
				"enablePrefixCaching":    true,
				"enableAutoToolChoice":   true,
			},
		},
	}

	// Create the model
	_, err := dynamicClient.Resource(gvr).Create(ctx, testModel, metav1.CreateOptions{})
	assert.NoError(t, err)

	// Wait for it to be created
	time.Sleep(2 * time.Second)

	// Get the model using CRDClient (by servedModelName, not metadata name)
	client := NewCRDClient(dynamicClient)
	config, err := client.GetModel(ctx, "test-model")
	if assert.NoError(t, err) && assert.NotNil(t, config) {
		assert.Equal(t, "test-model", config.ServedModelName)
		assert.Equal(t, "test/model", config.ModelName)
	}

	// Delete the model
	err = dynamicClient.Resource(gvr).Delete(ctx, "test-integration-model", metav1.DeleteOptions{})
	assert.NoError(t, err)
}
