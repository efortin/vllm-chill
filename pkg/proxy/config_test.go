package proxy

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name        string
		config      Config
		expectError bool
	}{
		{
			name: "valid config",
			config: Config{
				Namespace:   "test-ns",
				Deployment:  "test-deployment",
				IdleTimeout: "5m",
				Port:        "8080",
				ModelID:     "test-model",
			},
			expectError: false,
		},
		{
			name: "empty namespace",
			config: Config{
				Namespace:   "",
				Deployment:  "test-deployment",
				IdleTimeout: "5m",
				Port:        "8080",
				ModelID:     "test-model",
			},
			expectError: true,
		},
		{
			name: "empty deployment",
			config: Config{
				Namespace:   "test-ns",
				Deployment:  "",
				IdleTimeout: "5m",
				Port:        "8080",
				ModelID:     "test-model",
			},
			expectError: true,
		},
		{
			name: "invalid idle timeout",
			config: Config{
				Namespace:   "test-ns",
				Deployment:  "test-deployment",
				IdleTimeout: "invalid",
				Port:        "8080",
				ModelID:     "test-model",
			},
			expectError: true,
		},
		{
			name: "empty model ID",
			config: Config{
				Namespace:   "test-ns",
				Deployment:  "test-deployment",
				IdleTimeout: "5m",
				Port:        "8080",
				ModelID:     "",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestConfigGetIdleTimeout(t *testing.T) {
	tests := []struct {
		name     string
		timeout  string
		expected time.Duration
	}{
		{
			name:     "5 minutes",
			timeout:  "5m",
			expected: 5 * time.Minute,
		},
		{
			name:     "10 seconds",
			timeout:  "10s",
			expected: 10 * time.Second,
		},
		{
			name:     "1 hour",
			timeout:  "1h",
			expected: 1 * time.Hour,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := Config{IdleTimeout: tt.timeout}
			result := config.GetIdleTimeout()
			assert.Equal(t, tt.expected, result)
		})
	}
}
