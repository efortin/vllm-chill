package proxy

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic/fake"
)

func TestCRDClient_ConvertToModelConfig(t *testing.T) {
	tests := []struct {
		name    string
		obj     *unstructured.Unstructured
		want    *ModelConfig
		wantErr bool
	}{
		{
			name: "valid vllmmodel",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "vllm.efortin.github.io/v1alpha1",
					"kind":       "VLLMModel",
					"metadata": map[string]interface{}{
						"name":      "test-model",
						"namespace": "ai-apps",
					},
					"spec": map[string]interface{}{
						"modelName":              "test/model",
						"servedModelName":        "test-model",
						"toolCallParser":         "hermes",
						"reasoningParser":        "deepseek_r1",
						"tensorParallelSize":     int64(2),
						"maxModelLen":            int64(65536),
						"gpuMemoryUtilization":   0.91,
						"enableChunkedPrefill":   true,
						"maxNumBatchedTokens":    int64(4096),
						"maxNumSeqs":             int64(16),
						"dtype":                  "float16",
						"disableCustomAllReduce": true,
						"enablePrefixCaching":    true,
						"cpuOffloadGB":           int64(0),
						"enableAutoToolChoice":   true,
					},
				},
			},
			want: &ModelConfig{
				ModelName:              "test/model",
				ServedModelName:        "test-model",
				ToolCallParser:         "hermes",
				ReasoningParser:        "deepseek_r1",
				TensorParallelSize:     "2",
				MaxModelLen:            "65536",
				GPUMemoryUtilization:   "0.91",
				EnableChunkedPrefill:   "true",
				MaxNumBatchedTokens:    "4096",
				MaxNumSeqs:             "16",
				Dtype:                  "float16",
				DisableCustomAllReduce: "true",
				EnablePrefixCaching:    "true",
				CPUOffloadGB:           "0",
				EnableAutoToolChoice:   "true",
			},
			wantErr: false,
		},
		{
			name: "missing spec",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "vllm.efortin.github.io/v1alpha1",
					"kind":       "VLLMModel",
					"metadata": map[string]interface{}{
						"name": "test-model",
					},
				},
			},
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &CRDClient{}
			got, err := client.convertToModelConfig(tt.obj)
			if (err != nil) != tt.wantErr {
				t.Errorf("convertToModelConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}

			if got.ModelName != tt.want.ModelName {
				t.Errorf("ModelName = %v, want %v", got.ModelName, tt.want.ModelName)
			}
			if got.ServedModelName != tt.want.ServedModelName {
				t.Errorf("ServedModelName = %v, want %v", got.ServedModelName, tt.want.ServedModelName)
			}
			if got.TensorParallelSize != tt.want.TensorParallelSize {
				t.Errorf("TensorParallelSize = %v, want %v", got.TensorParallelSize, tt.want.TensorParallelSize)
			}
		})
	}
}

func TestCRDClient_GetModel(t *testing.T) {
	scheme := runtime.NewScheme()

	// Create test VLLMModel
	testModel := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "vllm.efortin.github.io/v1alpha1",
			"kind":       "VLLMModel",
			"metadata": map[string]interface{}{
				"name":      "qwen3-coder-30b-fp8",
				"namespace": "ai-apps",
			},
			"spec": map[string]interface{}{
				"modelName":            "Qwen/Qwen3-Coder-30B-A3B-Instruct-FP8",
				"servedModelName":      "qwen3-coder-30b-fp8",
				"toolCallParser":       "qwen3_coder",
				"tensorParallelSize":   int64(2),
				"maxModelLen":          int64(65536),
				"gpuMemoryUtilization": 0.91,
			},
		},
	}

	dynamicClient := fake.NewSimpleDynamicClient(scheme, testModel)
	client := NewCRDClient(dynamicClient, "ai-apps")

	t.Run("get existing model", func(t *testing.T) {
		got, err := client.GetModel(context.Background(), "qwen3-coder-30b-fp8")
		if err != nil {
			t.Errorf("GetModel() error = %v", err)
			return
		}
		if got.ServedModelName != "qwen3-coder-30b-fp8" {
			t.Errorf("ServedModelName = %v, want qwen3-coder-30b-fp8", got.ServedModelName)
		}
		if got.ModelName != "Qwen/Qwen3-Coder-30B-A3B-Instruct-FP8" {
			t.Errorf("ModelName = %v, want Qwen/Qwen3-Coder-30B-A3B-Instruct-FP8", got.ModelName)
		}
	})

	t.Run("get non-existent model", func(t *testing.T) {
		_, err := client.GetModel(context.Background(), "non-existent-model")
		if err == nil {
			t.Error("GetModel() expected error for non-existent model")
		}
	})
}

func TestCRDClient_ListModels(t *testing.T) {
	scheme := runtime.NewScheme()

	// Create multiple test VLLMModels
	testModel1 := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "vllm.efortin.github.io/v1alpha1",
			"kind":       "VLLMModel",
			"metadata": map[string]interface{}{
				"name":      "model-1",
				"namespace": "ai-apps",
			},
			"spec": map[string]interface{}{
				"modelName":       "test/model-1",
				"servedModelName": "model-1",
			},
		},
	}

	testModel2 := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "vllm.efortin.github.io/v1alpha1",
			"kind":       "VLLMModel",
			"metadata": map[string]interface{}{
				"name":      "model-2",
				"namespace": "ai-apps",
			},
			"spec": map[string]interface{}{
				"modelName":       "test/model-2",
				"servedModelName": "model-2",
			},
		},
	}

	dynamicClient := fake.NewSimpleDynamicClient(scheme, testModel1, testModel2)
	client := NewCRDClient(dynamicClient, "ai-apps")

	t.Run("list all models", func(t *testing.T) {
		models, err := client.ListModels(context.Background())
		if err != nil {
			t.Errorf("ListModels() error = %v", err)
			return
		}

		if len(models) != 2 {
			t.Errorf("ListModels() returned %d models, want 2", len(models))
		}
	})

	t.Run("list models in empty namespace", func(t *testing.T) {
		// Create a client with no models but in a different namespace
		emptyClient := NewCRDClient(dynamicClient, "empty-ns")
		models, err := emptyClient.ListModels(context.Background())
		if err != nil {
			t.Errorf("ListModels() error = %v", err)
			return
		}

		// Should return 0 models since we're looking in a different namespace
		if len(models) != 0 {
			t.Errorf("ListModels() returned %d models, want 0", len(models))
		}
	})
}

func TestCRDClient_ConvertToModelConfig_AllFields(t *testing.T) {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "vllm.efortin.github.io/v1alpha1",
			"kind":       "VLLMModel",
			"metadata": map[string]interface{}{
				"name":      "full-model",
				"namespace": "ai-apps",
			},
			"spec": map[string]interface{}{
				"modelName":              "test/full-model",
				"servedModelName":        "full-model",
				"toolCallParser":         "hermes",
				"reasoningParser":        "deepseek_r1",
				"tensorParallelSize":     int64(4),
				"maxModelLen":            int64(32768),
				"gpuMemoryUtilization":   0.95,
				"enableChunkedPrefill":   true,
				"maxNumBatchedTokens":    int64(8192),
				"maxNumSeqs":             int64(32),
				"dtype":                  "bfloat16",
				"disableCustomAllReduce": false,
				"enablePrefixCaching":    true,
				"cpuOffloadGB":           int64(4),
				"enableAutoToolChoice":   false,
			},
		},
	}

	client := &CRDClient{}
	config, err := client.convertToModelConfig(obj)
	if err != nil {
		t.Fatalf("convertToModelConfig() error = %v", err)
	}

	// Verify all fields
	if config.ModelName != "test/full-model" {
		t.Errorf("ModelName = %v, want test/full-model", config.ModelName)
	}
	if config.ServedModelName != "full-model" {
		t.Errorf("ServedModelName = %v, want full-model", config.ServedModelName)
	}
	if config.ToolCallParser != "hermes" {
		t.Errorf("ToolCallParser = %v, want hermes", config.ToolCallParser)
	}
	if config.ReasoningParser != "deepseek_r1" {
		t.Errorf("ReasoningParser = %v, want deepseek_r1", config.ReasoningParser)
	}
	if config.TensorParallelSize != "4" {
		t.Errorf("TensorParallelSize = %v, want 4", config.TensorParallelSize)
	}
	if config.MaxModelLen != "32768" {
		t.Errorf("MaxModelLen = %v, want 32768", config.MaxModelLen)
	}
	if config.GPUMemoryUtilization != "0.95" {
		t.Errorf("GPUMemoryUtilization = %v, want 0.95", config.GPUMemoryUtilization)
	}
	if config.EnableChunkedPrefill != "true" {
		t.Errorf("EnableChunkedPrefill = %v, want true", config.EnableChunkedPrefill)
	}
	if config.MaxNumBatchedTokens != "8192" {
		t.Errorf("MaxNumBatchedTokens = %v, want 8192", config.MaxNumBatchedTokens)
	}
	if config.MaxNumSeqs != "32" {
		t.Errorf("MaxNumSeqs = %v, want 32", config.MaxNumSeqs)
	}
	if config.Dtype != "bfloat16" {
		t.Errorf("Dtype = %v, want bfloat16", config.Dtype)
	}
	if config.DisableCustomAllReduce != "false" {
		t.Errorf("DisableCustomAllReduce = %v, want false", config.DisableCustomAllReduce)
	}
	if config.EnablePrefixCaching != "true" {
		t.Errorf("EnablePrefixCaching = %v, want true", config.EnablePrefixCaching)
	}
	if config.CPUOffloadGB != "4" {
		t.Errorf("CPUOffloadGB = %v, want 4", config.CPUOffloadGB)
	}
	if config.EnableAutoToolChoice != "false" {
		t.Errorf("EnableAutoToolChoice = %v, want false", config.EnableAutoToolChoice)
	}
}
