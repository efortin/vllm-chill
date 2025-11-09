package kubernetes

import (
	"context"
	"fmt"
	"strconv"

	"github.com/efortin/vllm-chill/pkg/apis/vllm/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

var vllmModelGVR = schema.GroupVersionResource{
	Group:    "vllm.sir-alfred.io",
	Version:  "v1alpha1",
	Resource: "models",
}

// CRDClient handles VLLMModel CRD operations
type CRDClient struct {
	dynamicClient dynamic.Interface
}

// NewCRDClient creates a new CRD client
func NewCRDClient(dynamicClient dynamic.Interface) *CRDClient {
	return &CRDClient{
		dynamicClient: dynamicClient,
	}
}

// GetModel retrieves a VLLMModel by its served model name
func (c *CRDClient) GetModel(ctx context.Context, servedModelName string) (*ModelConfig, error) {
	// List all VLLMModels (cluster-scoped)
	list, err := c.dynamicClient.Resource(vllmModelGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list VLLMModels: %w", err)
	}

	// Find the model with matching servedModelName
	for _, item := range list.Items {
		spec, found, err := unstructured.NestedMap(item.Object, "spec")
		if err != nil || !found {
			continue
		}

		served, found, err := unstructured.NestedString(spec, "servedModelName")
		if err != nil || !found {
			continue
		}

		if served == servedModelName {
			return c.convertToModelConfig(&item)
		}
	}

	return nil, fmt.Errorf("VLLMModel with servedModelName '%s' not found", servedModelName)
}

// convertToModelConfig converts an unstructured VLLMModel to ModelConfig
func (c *CRDClient) convertToModelConfig(u *unstructured.Unstructured) (*ModelConfig, error) {
	spec, found, err := unstructured.NestedMap(u.Object, "spec")
	if err != nil || !found {
		return nil, fmt.Errorf("spec not found in VLLMModel")
	}

	config := &ModelConfig{}

	// Model identification
	if modelName, found, _ := unstructured.NestedString(spec, "modelName"); found {
		config.ModelName = modelName
	}
	if servedModelName, found, _ := unstructured.NestedString(spec, "servedModelName"); found {
		config.ServedModelName = servedModelName
	}

	// Parsing configuration
	if toolCallParser, found, _ := unstructured.NestedString(spec, "toolCallParser"); found {
		config.ToolCallParser = toolCallParser
	}
	if reasoningParser, found, _ := unstructured.NestedString(spec, "reasoningParser"); found {
		config.ReasoningParser = reasoningParser
	}

	// vLLM runtime parameters
	if tensorParallelSize, found, _ := unstructured.NestedInt64(spec, "tensorParallelSize"); found {
		config.TensorParallelSize = strconv.FormatInt(tensorParallelSize, 10)
	}
	if maxModelLen, found, _ := unstructured.NestedInt64(spec, "maxModelLen"); found {
		config.MaxModelLen = strconv.FormatInt(maxModelLen, 10)
	}
	if gpuMemoryUtilization, found, _ := unstructured.NestedFloat64(spec, "gpuMemoryUtilization"); found {
		config.GPUMemoryUtilization = strconv.FormatFloat(gpuMemoryUtilization, 'f', 2, 64)
	}
	if enableChunkedPrefill, found, _ := unstructured.NestedBool(spec, "enableChunkedPrefill"); found {
		config.EnableChunkedPrefill = strconv.FormatBool(enableChunkedPrefill)
	}
	if maxNumBatchedTokens, found, _ := unstructured.NestedInt64(spec, "maxNumBatchedTokens"); found {
		config.MaxNumBatchedTokens = strconv.FormatInt(maxNumBatchedTokens, 10)
	}
	if maxNumSeqs, found, _ := unstructured.NestedInt64(spec, "maxNumSeqs"); found {
		config.MaxNumSeqs = strconv.FormatInt(maxNumSeqs, 10)
	}
	if dtype, found, _ := unstructured.NestedString(spec, "dtype"); found {
		config.Dtype = dtype
	}
	if disableCustomAllReduce, found, _ := unstructured.NestedBool(spec, "disableCustomAllReduce"); found {
		config.DisableCustomAllReduce = strconv.FormatBool(disableCustomAllReduce)
	}
	if enablePrefixCaching, found, _ := unstructured.NestedBool(spec, "enablePrefixCaching"); found {
		config.EnablePrefixCaching = strconv.FormatBool(enablePrefixCaching)
	}
	if cpuOffloadGB, found, _ := unstructured.NestedInt64(spec, "cpuOffloadGB"); found {
		config.CPUOffloadGB = strconv.FormatInt(cpuOffloadGB, 10)
	}
	if enableAutoToolChoice, found, _ := unstructured.NestedBool(spec, "enableAutoToolChoice"); found {
		config.EnableAutoToolChoice = strconv.FormatBool(enableAutoToolChoice)
	}

	return config, nil
}

// ListModels returns all VLLMModels (cluster-scoped)
func (c *CRDClient) ListModels(ctx context.Context) ([]*v1alpha1.VLLMModel, error) {
	list, err := c.dynamicClient.Resource(vllmModelGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list VLLMModels: %w", err)
	}

	models := make([]*v1alpha1.VLLMModel, 0, len(list.Items))
	for _, item := range list.Items {
		model := &v1alpha1.VLLMModel{}
		if err := convertUnstructuredToVLLMModel(&item, model); err != nil {
			continue
		}
		models = append(models, model)
	}

	return models, nil
}

// convertUnstructuredToVLLMModel converts unstructured to typed VLLMModel
func convertUnstructuredToVLLMModel(u *unstructured.Unstructured, model *v1alpha1.VLLMModel) error {
	gvk := u.GetObjectKind().GroupVersionKind()
	model.TypeMeta = metav1.TypeMeta{
		APIVersion: gvk.GroupVersion().String(),
		Kind:       gvk.Kind,
	}
	model.ObjectMeta = metav1.ObjectMeta{
		Name:      u.GetName(),
		Namespace: u.GetNamespace(),
	}

	spec, found, err := unstructured.NestedMap(u.Object, "spec")
	if err != nil || !found {
		return fmt.Errorf("spec not found")
	}

	// Parse spec fields
	if modelName, found, _ := unstructured.NestedString(spec, "modelName"); found {
		model.Spec.ModelName = modelName
	}
	if servedModelName, found, _ := unstructured.NestedString(spec, "servedModelName"); found {
		model.Spec.ServedModelName = servedModelName
	}

	return nil
}
