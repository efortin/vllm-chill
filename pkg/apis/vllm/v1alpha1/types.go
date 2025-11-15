package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// VLLMModel is a specification for a vLLM model configuration
type VLLMModel struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VLLMModelSpec   `json:"spec"`
	Status VLLMModelStatus `json:"status,omitempty"`
}

// VLLMModelSpec defines the desired state of VLLMModel
type VLLMModelSpec struct {
	// Model Identification
	ModelName       string `json:"modelName"`
	ServedModelName string `json:"servedModelName"`

	// Parsing Configuration
	ToolCallParser  string `json:"toolCallParser,omitempty"`
	ReasoningParser string `json:"reasoningParser,omitempty"`
	ChatTemplate    string `json:"chatTemplate,omitempty"`
	TokenizerMode   string `json:"tokenizerMode,omitempty"`

	// vLLM Runtime Parameters (model-specific)
	// Note: gpuCount and cpuOffloadGB are infrastructure-level, configured in vllm-chill
	MaxModelLen            int     `json:"maxModelLen,omitempty"`
	GPUMemoryUtilization   float64 `json:"gpuMemoryUtilization,omitempty"`
	EnableChunkedPrefill   *bool   `json:"enableChunkedPrefill,omitempty"`
	MaxNumBatchedTokens    int     `json:"maxNumBatchedTokens,omitempty"`
	MaxNumSeqs             int     `json:"maxNumSeqs,omitempty"`
	Dtype                  string  `json:"dtype,omitempty"`
	DisableCustomAllReduce *bool   `json:"disableCustomAllReduce,omitempty"`
	EnablePrefixCaching    *bool   `json:"enablePrefixCaching,omitempty"`
	EnableAutoToolChoice   *bool   `json:"enableAutoToolChoice,omitempty"`
}

// VLLMModelStatus defines the observed state of VLLMModel
type VLLMModelStatus struct {
	Phase       string      `json:"phase,omitempty"`
	LastUpdated metav1.Time `json:"lastUpdated,omitempty"`
	Message     string      `json:"message,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// VLLMModelList is a list of VLLMModel resources
type VLLMModelList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []VLLMModel `json:"items"`
}
