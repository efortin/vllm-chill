package proxy

import (
	"testing"
	"time"
)

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name: "all fields set correctly",
			config: &Config{
				Namespace:      "ai-apps",
				Deployment:     "vllm-deployment",
				ConfigMapName:  "vllm-config",
				TargetHost:     "vllm-service.ai-apps.svc.cluster.local",
				TargetPort:     "8000",
				IdleTimeout:    "10m",
				ManagedTimeout: "5m",
				Port:           "9090",
				ModelID:        "test-model",
			},
			wantErr: false,
		},
		{
			name: "empty namespace",
			config: &Config{
				Namespace:      "",
				Deployment:     "vllm-deployment",
				ConfigMapName:  "vllm-config",
				TargetHost:     "vllm-service.ai-apps.svc.cluster.local",
				TargetPort:     "8000",
				IdleTimeout:    "10m",
				ManagedTimeout: "5m",
				Port:           "9090",
				ModelID:        "test-model",
			},
			wantErr: true,
		},
		{
			name: "empty deployment",
			config: &Config{
				Namespace:      "ai-apps",
				Deployment:     "",
				ConfigMapName:  "vllm-config",
				TargetHost:     "vllm-service.ai-apps.svc.cluster.local",
				TargetPort:     "8000",
				IdleTimeout:    "10m",
				ManagedTimeout: "5m",
				Port:           "9090",
				ModelID:        "test-model",
			},
			wantErr: true,
		},
		{
			name: "empty configmap name",
			config: &Config{
				Namespace:      "ai-apps",
				Deployment:     "vllm-deployment",
				ConfigMapName:  "",
				TargetHost:     "vllm-service.ai-apps.svc.cluster.local",
				TargetPort:     "8000",
				IdleTimeout:    "10m",
				ManagedTimeout: "5m",
				Port:           "9090",
				ModelID:        "test-model",
			},
			wantErr: true,
		},
		{
			name: "empty target host",
			config: &Config{
				Namespace:      "ai-apps",
				Deployment:     "vllm-deployment",
				ConfigMapName:  "vllm-config",
				TargetHost:     "",
				TargetPort:     "8000",
				IdleTimeout:    "10m",
				ManagedTimeout: "5m",
				Port:           "9090",
				ModelID:        "test-model",
			},
			wantErr: true,
		},
		{
			name: "empty target port",
			config: &Config{
				Namespace:      "ai-apps",
				Deployment:     "vllm-deployment",
				ConfigMapName:  "vllm-config",
				TargetHost:     "vllm-service.ai-apps.svc.cluster.local",
				TargetPort:     "",
				IdleTimeout:    "10m",
				ManagedTimeout: "5m",
				Port:           "9090",
				ModelID:        "test-model",
			},
			wantErr: true,
		},
		{
			name: "invalid idle timeout",
			config: &Config{
				Namespace:      "ai-apps",
				Deployment:     "vllm-deployment",
				ConfigMapName:  "vllm-config",
				TargetHost:     "vllm-service.ai-apps.svc.cluster.local",
				TargetPort:     "8000",
				IdleTimeout:    "invalid",
				ManagedTimeout: "5m",
				Port:           "9090",
				ModelID:        "test-model",
			},
			wantErr: true,
		},
		{
			name: "invalid managed timeout",
			config: &Config{
				Namespace:      "ai-apps",
				Deployment:     "vllm-deployment",
				ConfigMapName:  "vllm-config",
				TargetHost:     "vllm-service.ai-apps.svc.cluster.local",
				TargetPort:     "8000",
				IdleTimeout:    "10m",
				ManagedTimeout: "invalid",
				Port:           "9090",
				ModelID:        "test-model",
			},
			wantErr: true,
		},
		{
			name: "empty model ID",
			config: &Config{
				Namespace:      "ai-apps",
				Deployment:     "vllm-deployment",
				ConfigMapName:  "vllm-config",
				TargetHost:     "vllm-service.ai-apps.svc.cluster.local",
				TargetPort:     "8000",
				IdleTimeout:    "10m",
				ManagedTimeout: "5m",
				Port:           "9090",
				ModelID:        "",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Config.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestConfig_GetIdleTimeout(t *testing.T) {
	tests := []struct {
		name     string
		config   *Config
		expected time.Duration
	}{
		{
			name: "valid timeout",
			config: &Config{
				IdleTimeout: "5m",
			},
			expected: 5 * time.Minute,
		},
		{
			name: "zero timeout",
			config: &Config{
				IdleTimeout: "0s",
			},
			expected: 0,
		},
		{
			name: "custom timeout",
			config: &Config{
				IdleTimeout: "1h30m",
			},
			expected: time.Hour + 30*time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.GetIdleTimeout()
			if result != tt.expected {
				t.Errorf("Config.GetIdleTimeout() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestConfig_GetTargetURL(t *testing.T) {
	tests := []struct {
		name     string
		config   *Config
		expected string
	}{
		{
			name: "standard case",
			config: &Config{
				TargetHost: "localhost",
				TargetPort: "8080",
			},
			expected: "http://localhost:8080",
		},
		{
			name: "different host and port",
			config: &Config{
				TargetHost: "api.example.com",
				TargetPort: "443",
			},
			expected: "http://api.example.com:443",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.GetTargetURL()
			if result != tt.expected {
				t.Errorf("Config.GetTargetURL() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestConfig_GetManagedTimeout(t *testing.T) {
	tests := []struct {
		name     string
		config   *Config
		expected time.Duration
	}{
		{
			name: "valid timeout",
			config: &Config{
				ManagedTimeout: "5m",
			},
			expected: 5 * time.Minute,
		},
		{
			name: "zero timeout",
			config: &Config{
				ManagedTimeout: "0s",
			},
			expected: 5 * time.Minute, // Should default to 5 minutes
		},
		{
			name: "invalid timeout",
			config: &Config{
				ManagedTimeout: "invalid",
			},
			expected: 5 * time.Minute, // Should default to 5 minutes
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.GetManagedTimeout()
			if result != tt.expected {
				t.Errorf("Config.GetManagedTimeout() = %v, want %v", result, tt.expected)
			}
		})
	}
}
