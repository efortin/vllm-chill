package kubernetes

import (
	"testing"

	"github.com/efortin/vllm-chill/pkg/apis/vllm/v1alpha1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestModelNotFoundError(t *testing.T) {
	err := &ModelNotFoundError{ModelID: "test-model"}
	expectedMsg := "model 'test-model' not found"
	if err.Error() != expectedMsg {
		t.Errorf("Error() = %v, want %v", err.Error(), expectedMsg)
	}
}

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
					"apiVersion": "vllm.sir-alfred.io/v1alpha1",
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
				MaxModelLen:            "65536",
				GPUMemoryUtilization:   "0.91",
				EnableChunkedPrefill:   "true",
				MaxNumBatchedTokens:    "4096",
				MaxNumSeqs:             "16",
				Dtype:                  "float16",
				DisableCustomAllReduce: "true",
				EnablePrefixCaching:    "true",
				EnableAutoToolChoice:   "true",
			},
			wantErr: false,
		},
		{
			name: "missing spec",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "vllm.sir-alfred.io/v1alpha1",
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
		})
	}
}

// Note: GetModel and ListModels tests are skipped because the fake dynamic client
// has issues with custom GVR. These methods are tested in integration tests.

func TestConvertUnstructuredToVLLMModel(t *testing.T) {
	tests := []struct {
		name    string
		obj     *unstructured.Unstructured
		wantErr bool
	}{
		{
			name: "valid model",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "vllm.sir-alfred.io/v1alpha1",
					"kind":       "VLLMModel",
					"metadata": map[string]interface{}{
						"name":      "test-model",
						"namespace": "default",
					},
					"spec": map[string]interface{}{
						"modelName":       "test/model",
						"servedModelName": "test-model",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "missing spec",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "vllm.sir-alfred.io/v1alpha1",
					"kind":       "VLLMModel",
					"metadata": map[string]interface{}{
						"name": "test-model",
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := &v1alpha1.VLLMModel{}
			err := convertUnstructuredToVLLMModel(tt.obj, model)
			if (err != nil) != tt.wantErr {
				t.Errorf("convertUnstructuredToVLLMModel() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if model.Name != "test-model" {
					t.Errorf("Name = %v, want test-model", model.Name)
				}
				if model.Spec.ModelName != "test/model" {
					t.Errorf("ModelName = %v, want test/model", model.Spec.ModelName)
				}
			}
		})
	}
}

func TestCRDClient_ConvertToModelConfig_AllFields(t *testing.T) {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "vllm.sir-alfred.io/v1alpha1",
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
	if config.EnableAutoToolChoice != "false" {
		t.Errorf("EnableAutoToolChoice = %v, want false", config.EnableAutoToolChoice)
	}
	// Note: cpuOffloadGB is now infrastructure-level, not in ModelConfig
}
