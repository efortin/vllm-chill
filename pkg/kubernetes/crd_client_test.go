package kubernetes

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestCRDClient_ConvertToModelConfig_WithChatTemplateAndTokenizerMode(t *testing.T) {
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
				"chatTemplate":           "my-custom-template",
				"tokenizerMode":          "mistral",
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

	// Verify all fields including the new ones
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
	if config.ChatTemplate != "my-custom-template" {
		t.Errorf("ChatTemplate = %v, want my-custom-template", config.ChatTemplate)
	}
	if config.TokenizerMode != "mistral" {
		t.Errorf("TokenizerMode = %v, want mistral", config.TokenizerMode)
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
