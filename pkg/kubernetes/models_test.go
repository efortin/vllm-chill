package kubernetes

import (
	"testing"

	"github.com/efortin/vllm-chill/pkg/models"
)

func TestModelConfig_ToConfigMapData(t *testing.T) {
	config := &models.Config{
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
	}

	//nolint:staticcheck // Testing deprecated function for backward compatibility
	data := config.ToConfigMapData()

	tests := []struct {
		key      string
		expected string
	}{
		{"MODEL_NAME", "test/model"},
		{"SERVED_MODEL_NAME", "test-model"},
		{"TOOL_CALL_PARSER", "hermes"},
		{"REASONING_PARSER", "deepseek_r1"},
		{"MAX_MODEL_LEN", "65536"},
		{"GPU_MEMORY_UTILIZATION", "0.91"},
		{"ENABLE_CHUNKED_PREFILL", "true"},
		{"MAX_NUM_BATCHED_TOKENS", "4096"},
		{"MAX_NUM_SEQS", "16"},
		{"DTYPE", "float16"},
		{"DISABLE_CUSTOM_ALL_REDUCE", "true"},
		{"ENABLE_PREFIX_CACHING", "true"},
		{"ENABLE_AUTO_TOOL_CHOICE", "true"},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			if got := data[tt.key]; got != tt.expected {
				t.Errorf("ToConfigMapData()[%s] = %v, want %v", tt.key, got, tt.expected)
			}
		})
	}
}

func TestFromConfigMapData(t *testing.T) {
	data := map[string]string{
		"MODEL_NAME":                "test/model",
		"SERVED_MODEL_NAME":         "test-model",
		"TOOL_CALL_PARSER":          "hermes",
		"REASONING_PARSER":          "deepseek_r1",
		"MAX_MODEL_LEN":             "65536",
		"GPU_MEMORY_UTILIZATION":    "0.91",
		"ENABLE_CHUNKED_PREFILL":    "true",
		"MAX_NUM_BATCHED_TOKENS":    "4096",
		"MAX_NUM_SEQS":              "16",
		"DTYPE":                     "float16",
		"DISABLE_CUSTOM_ALL_REDUCE": "true",
		"ENABLE_PREFIX_CACHING":     "true",
		"ENABLE_AUTO_TOOL_CHOICE":   "true",
	}

	//nolint:staticcheck // Testing deprecated function for backward compatibility
	config := models.FromConfigMapData(data)

	if config.ModelName != "test/model" {
		t.Errorf("ModelName = %v, want test/model", config.ModelName)
	}
	if config.ServedModelName != "test-model" {
		t.Errorf("ServedModelName = %v, want test-model", config.ServedModelName)
	}
	if config.ToolCallParser != "hermes" {
		t.Errorf("ToolCallParser = %v, want hermes", config.ToolCallParser)
	}
	// Note: TensorParallelSize and CPUOffloadGB are now infrastructure-level
}

func TestModelConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  *models.Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: &models.Config{
				ModelName:       "test/model",
				ServedModelName: "test-model",
			},
			wantErr: false,
		},
		{
			name: "missing model name",
			config: &models.Config{
				ServedModelName: "test-model",
			},
			wantErr: true,
		},
		{
			name: "missing served model name",
			config: &models.Config{
				ModelName: "test/model",
			},
			wantErr: true,
		},
		{
			name:    "empty config",
			config:  &models.Config{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestBoolToString removed - this function is internal to the models package
