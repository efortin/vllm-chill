package kubernetes

import (
	"fmt"
	"strconv"
)

// ModelConfig represents a model configuration profile
type ModelConfig struct {
	// Model identification
	ModelName       string
	ServedModelName string

	// Parsing configuration
	ToolCallParser  string
	ReasoningParser string

	// GPU configuration
	GPUCount string // Number of GPUs to use (affects tensor-parallel-size and resource limits)

	// vLLM runtime parameters
	TensorParallelSize     string
	MaxModelLen            string
	GPUMemoryUtilization   string
	EnableChunkedPrefill   string
	MaxNumBatchedTokens    string
	MaxNumSeqs             string
	Dtype                  string
	DisableCustomAllReduce string
	EnablePrefixCaching    string
	CPUOffloadGB           string
	EnableAutoToolChoice   string
}

// ToConfigMapData converts ModelConfig to ConfigMap data format
func (m *ModelConfig) ToConfigMapData() map[string]string {
	return map[string]string{
		"MODEL_NAME":                m.ModelName,
		"SERVED_MODEL_NAME":         m.ServedModelName,
		"TOOL_CALL_PARSER":          m.ToolCallParser,
		"REASONING_PARSER":          m.ReasoningParser,
		"GPU_COUNT":                 m.GPUCount,
		"TENSOR_PARALLEL_SIZE":      m.TensorParallelSize,
		"MAX_MODEL_LEN":             m.MaxModelLen,
		"GPU_MEMORY_UTILIZATION":    m.GPUMemoryUtilization,
		"ENABLE_CHUNKED_PREFILL":    m.EnableChunkedPrefill,
		"MAX_NUM_BATCHED_TOKENS":    m.MaxNumBatchedTokens,
		"MAX_NUM_SEQS":              m.MaxNumSeqs,
		"DTYPE":                     m.Dtype,
		"DISABLE_CUSTOM_ALL_REDUCE": m.DisableCustomAllReduce,
		"ENABLE_PREFIX_CACHING":     m.EnablePrefixCaching,
		"CPU_OFFLOAD_GB":            m.CPUOffloadGB,
		"ENABLE_AUTO_TOOL_CHOICE":   m.EnableAutoToolChoice,
	}
}

// FromConfigMapData creates a ModelConfig from ConfigMap data
func FromConfigMapData(data map[string]string) *ModelConfig {
	// If GPU_COUNT is specified, use it for tensor-parallel-size
	// Otherwise fall back to TENSOR_PARALLEL_SIZE
	gpuCount := data["GPU_COUNT"]
	tensorParallelSize := data["TENSOR_PARALLEL_SIZE"]
	if gpuCount != "" {
		tensorParallelSize = gpuCount
	}

	return &ModelConfig{
		ModelName:              data["MODEL_NAME"],
		ServedModelName:        data["SERVED_MODEL_NAME"],
		ToolCallParser:         data["TOOL_CALL_PARSER"],
		ReasoningParser:        data["REASONING_PARSER"],
		GPUCount:               gpuCount,
		TensorParallelSize:     tensorParallelSize,
		MaxModelLen:            data["MAX_MODEL_LEN"],
		GPUMemoryUtilization:   data["GPU_MEMORY_UTILIZATION"],
		EnableChunkedPrefill:   data["ENABLE_CHUNKED_PREFILL"],
		MaxNumBatchedTokens:    data["MAX_NUM_BATCHED_TOKENS"],
		MaxNumSeqs:             data["MAX_NUM_SEQS"],
		Dtype:                  data["DTYPE"],
		DisableCustomAllReduce: data["DISABLE_CUSTOM_ALL_REDUCE"],
		EnablePrefixCaching:    data["ENABLE_PREFIX_CACHING"],
		CPUOffloadGB:           data["CPU_OFFLOAD_GB"],
		EnableAutoToolChoice:   data["ENABLE_AUTO_TOOL_CHOICE"],
	}
}

// boolToString converts a bool pointer to string
func boolToString(b *bool) string {
	if b == nil {
		return "false"
	}
	return strconv.FormatBool(*b)
}

// Validate checks if the model configuration is valid
func (m *ModelConfig) Validate() error {
	if m.ModelName == "" {
		return fmt.Errorf("modelName cannot be empty")
	}
	if m.ServedModelName == "" {
		return fmt.Errorf("servedModelName cannot be empty")
	}
	return nil
}
