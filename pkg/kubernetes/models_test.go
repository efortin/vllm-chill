package kubernetes

import (
	"testing"
)

func TestModelConfig_Validate(t *testing.T) {
	validConfig := &ModelConfig{
		ModelName:              "test/model",
		ServedModelName:        "test-model",
		MaxModelLen:            "65536",
		MaxNumBatchedTokens:    "4096",
		MaxNumSeqs:             "16",
		GPUMemoryUtilization:   "0.91",
		Dtype:                  "float16",
		EnableChunkedPrefill:   "true",
		DisableCustomAllReduce: "true",
		EnablePrefixCaching:    "true",
		EnableAutoToolChoice:   "true",
	}

	tests := []struct {
		name    string
		config  *ModelConfig
		wantErr bool
	}{
		{
			name:    "valid config",
			config:  validConfig,
			wantErr: false,
		},
		{
			name: "missing model name",
			config: &ModelConfig{
				ServedModelName:        "test-model",
				MaxModelLen:            "65536",
				MaxNumBatchedTokens:    "4096",
				MaxNumSeqs:             "16",
				GPUMemoryUtilization:   "0.91",
				Dtype:                  "float16",
				EnableChunkedPrefill:   "true",
				DisableCustomAllReduce: "true",
				EnablePrefixCaching:    "true",
				EnableAutoToolChoice:   "true",
			},
			wantErr: true,
		},
		{
			name: "missing served model name",
			config: &ModelConfig{
				ModelName:              "test/model",
				MaxModelLen:            "65536",
				MaxNumBatchedTokens:    "4096",
				MaxNumSeqs:             "16",
				GPUMemoryUtilization:   "0.91",
				Dtype:                  "float16",
				EnableChunkedPrefill:   "true",
				DisableCustomAllReduce: "true",
				EnablePrefixCaching:    "true",
				EnableAutoToolChoice:   "true",
			},
			wantErr: true,
		},
		{
			name: "missing maxModelLen",
			config: &ModelConfig{
				ModelName:              "test/model",
				ServedModelName:        "test-model",
				MaxNumBatchedTokens:    "4096",
				MaxNumSeqs:             "16",
				GPUMemoryUtilization:   "0.91",
				Dtype:                  "float16",
				EnableChunkedPrefill:   "true",
				DisableCustomAllReduce: "true",
				EnablePrefixCaching:    "true",
				EnableAutoToolChoice:   "true",
			},
			wantErr: true,
		},
		{
			name: "missing maxNumBatchedTokens",
			config: &ModelConfig{
				ModelName:              "test/model",
				ServedModelName:        "test-model",
				MaxModelLen:            "65536",
				MaxNumSeqs:             "16",
				GPUMemoryUtilization:   "0.91",
				Dtype:                  "float16",
				EnableChunkedPrefill:   "true",
				DisableCustomAllReduce: "true",
				EnablePrefixCaching:    "true",
				EnableAutoToolChoice:   "true",
			},
			wantErr: true,
		},
		{
			name: "missing dtype",
			config: &ModelConfig{
				ModelName:              "test/model",
				ServedModelName:        "test-model",
				MaxModelLen:            "65536",
				MaxNumBatchedTokens:    "4096",
				MaxNumSeqs:             "16",
				GPUMemoryUtilization:   "0.91",
				EnableChunkedPrefill:   "true",
				DisableCustomAllReduce: "true",
				EnablePrefixCaching:    "true",
				EnableAutoToolChoice:   "true",
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
