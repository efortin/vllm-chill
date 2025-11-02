package proxy

import (
	"testing"
)

func TestModelConfig_ToConfigMapData(t *testing.T) {
	config := &ModelConfig{
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
	}

	data := config.ToConfigMapData()

	tests := []struct {
		key      string
		expected string
	}{
		{"MODEL_NAME", "test/model"},
		{"SERVED_MODEL_NAME", "test-model"},
		{"TOOL_CALL_PARSER", "hermes"},
		{"REASONING_PARSER", "deepseek_r1"},
		{"TENSOR_PARALLEL_SIZE", "2"},
		{"MAX_MODEL_LEN", "65536"},
		{"GPU_MEMORY_UTILIZATION", "0.91"},
		{"ENABLE_CHUNKED_PREFILL", "true"},
		{"MAX_NUM_BATCHED_TOKENS", "4096"},
		{"MAX_NUM_SEQS", "16"},
		{"DTYPE", "float16"},
		{"DISABLE_CUSTOM_ALL_REDUCE", "true"},
		{"ENABLE_PREFIX_CACHING", "true"},
		{"CPU_OFFLOAD_GB", "0"},
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
		"TENSOR_PARALLEL_SIZE":      "2",
		"MAX_MODEL_LEN":             "65536",
		"GPU_MEMORY_UTILIZATION":    "0.91",
		"ENABLE_CHUNKED_PREFILL":    "true",
		"MAX_NUM_BATCHED_TOKENS":    "4096",
		"MAX_NUM_SEQS":              "16",
		"DTYPE":                     "float16",
		"DISABLE_CUSTOM_ALL_REDUCE": "true",
		"ENABLE_PREFIX_CACHING":     "true",
		"CPU_OFFLOAD_GB":            "0",
		"ENABLE_AUTO_TOOL_CHOICE":   "true",
	}

	config := FromConfigMapData(data)

	if config.ModelName != "test/model" {
		t.Errorf("ModelName = %v, want test/model", config.ModelName)
	}
	if config.ServedModelName != "test-model" {
		t.Errorf("ServedModelName = %v, want test-model", config.ServedModelName)
	}
	if config.ToolCallParser != "hermes" {
		t.Errorf("ToolCallParser = %v, want hermes", config.ToolCallParser)
	}
	if config.TensorParallelSize != "2" {
		t.Errorf("TensorParallelSize = %v, want 2", config.TensorParallelSize)
	}
}

func TestModelConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  *ModelConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: &ModelConfig{
				ModelName:       "test/model",
				ServedModelName: "test-model",
			},
			wantErr: false,
		},
		{
			name: "missing model name",
			config: &ModelConfig{
				ServedModelName: "test-model",
			},
			wantErr: true,
		},
		{
			name: "missing served model name",
			config: &ModelConfig{
				ModelName: "test/model",
			},
			wantErr: true,
		},
		{
			name:    "empty config",
			config:  &ModelConfig{},
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

func TestBoolToString(t *testing.T) {
	tests := []struct {
		name     string
		input    *bool
		expected string
	}{
		{
			name:     "nil pointer",
			input:    nil,
			expected: "false",
		},
		{
			name:     "true pointer",
			input:    boolPtr(true),
			expected: "true",
		},
		{
			name:     "false pointer",
			input:    boolPtr(false),
			expected: "false",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := boolToString(tt.input)
			if got != tt.expected {
				t.Errorf("boolToString() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func boolPtr(b bool) *bool {
	return &b
}
