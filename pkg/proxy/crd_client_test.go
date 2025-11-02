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
