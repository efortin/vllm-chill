package proxy

import (
	"testing"
)

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: &Config{
				Namespace:      "default",
				Deployment:     "vllm",
				ConfigMapName:  "vllm-config",
				TargetHost:     "vllm-svc",
				TargetPort:     "80",
				IdleTimeout:    "5m",
				ManagedTimeout: "5m",
				Port:           "8080",
			},
			wantErr: false,
		},
		{
			name: "empty namespace",
			config: &Config{
				Namespace:      "",
				Deployment:     "vllm",
				ConfigMapName:  "vllm-config",
				TargetHost:     "vllm-svc",
				TargetPort:     "80",
				IdleTimeout:    "5m",
				ManagedTimeout: "5m",
				Port:           "8080",
			},
			wantErr: true,
		},
		{
			name: "empty deployment",
			config: &Config{
				Namespace:      "default",
				Deployment:     "",
				ConfigMapName:  "vllm-config",
				TargetHost:     "vllm-svc",
				TargetPort:     "80",
				IdleTimeout:    "5m",
				ManagedTimeout: "5m",
				Port:           "8080",
			},
			wantErr: true,
		},
		{
			name: "empty target host",
			config: &Config{
				Namespace:      "default",
				Deployment:     "vllm",
				ConfigMapName:  "vllm-config",
				TargetHost:     "",
				TargetPort:     "80",
				IdleTimeout:    "5m",
				ManagedTimeout: "5m",
				Port:           "8080",
			},
			wantErr: true,
		},
		{
			name: "empty target port",
			config: &Config{
				Namespace:      "default",
				Deployment:     "vllm",
				ConfigMapName:  "vllm-config",
				TargetHost:     "vllm-svc",
				TargetPort:     "",
				IdleTimeout:    "5m",
				ManagedTimeout: "5m",
				Port:           "8080",
			},
			wantErr: true,
		},
		{
			name: "invalid idle timeout",
			config: &Config{
				Namespace:      "default",
				Deployment:     "vllm",
				ConfigMapName:  "vllm-config",
				TargetHost:     "vllm-svc",
				TargetPort:     "80",
				IdleTimeout:    "invalid",
				ManagedTimeout: "5m",
				Port:           "8080",
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
		wantSecs int64
	}{
		{
			name: "5 minutes",
			config: &Config{
				IdleTimeout: "5m",
			},
			wantSecs: 300,
		},
		{
			name: "1 hour",
			config: &Config{
				IdleTimeout: "1h",
			},
			wantSecs: 3600,
		},
		{
			name: "30 seconds",
			config: &Config{
				IdleTimeout: "30s",
			},
			wantSecs: 30,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.config.GetIdleTimeout()
			if got.Seconds() != float64(tt.wantSecs) {
				t.Errorf("Config.GetIdleTimeout() = %v seconds, want %v seconds", got.Seconds(), tt.wantSecs)
			}
		})
	}
}

func TestConfig_GetTargetURL(t *testing.T) {
	tests := []struct {
		name   string
		config *Config
		want   string
	}{
		{
			name: "default config",
			config: &Config{
				TargetHost: "vllm-svc",
				TargetPort: "80",
			},
			want: "http://vllm-svc:80",
		},
		{
			name: "custom port",
			config: &Config{
				TargetHost: "localhost",
				TargetPort: "8000",
			},
			want: "http://localhost:8000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.config.GetTargetURL(); got != tt.want {
				t.Errorf("Config.GetTargetURL() = %v, want %v", got, tt.want)
			}
		})
	}
}
