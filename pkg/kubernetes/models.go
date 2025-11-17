package kubernetes

import (
	"fmt"
)

// ModelConfig represents a model configuration profile
type ModelConfig struct {
	// Model identification
	ModelName       string
	ServedModelName string

	// Parsing configuration
	ToolCallParser  string
	ReasoningParser string
	ChatTemplate    string
	TokenizerMode   string
	Quantization    string

	// vLLM runtime parameters (model-specific)
	MaxModelLen            string
	GPUMemoryUtilization   string
	EnableChunkedPrefill   string
	MaxNumBatchedTokens    string
	MaxNumSeqs             string
	Dtype                  string
	DisableCustomAllReduce string
	EnablePrefixCaching    string
	EnableAutoToolChoice   string
}

// Validate checks if the model configuration is valid
func (m *ModelConfig) Validate() error {
	if m.ModelName == "" {
		return fmt.Errorf("modelName cannot be empty")
	}
	if m.ServedModelName == "" {
		return fmt.Errorf("servedModelName cannot be empty")
	}

	// Validate mandatory vLLM runtime parameters
	if m.MaxModelLen == "" {
		return fmt.Errorf("maxModelLen is required")
	}
	if m.MaxNumBatchedTokens == "" {
		return fmt.Errorf("maxNumBatchedTokens is required")
	}
	if m.MaxNumSeqs == "" {
		return fmt.Errorf("maxNumSeqs is required")
	}
	if m.GPUMemoryUtilization == "" {
		return fmt.Errorf("gpuMemoryUtilization is required")
	}
	if m.Dtype == "" {
		return fmt.Errorf("dtype is required")
	}
	if m.EnableChunkedPrefill == "" {
		return fmt.Errorf("enableChunkedPrefill is required")
	}
	if m.DisableCustomAllReduce == "" {
		return fmt.Errorf("disableCustomAllReduce is required")
	}
	if m.EnablePrefixCaching == "" {
		return fmt.Errorf("enablePrefixCaching is required")
	}
	if m.EnableAutoToolChoice == "" {
		return fmt.Errorf("enableAutoToolChoice is required")
	}

	return nil
}
