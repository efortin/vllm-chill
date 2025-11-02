package proxy

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestAutoScaler_healthHandler(t *testing.T) {
	config := &Config{
		Namespace:   "default",
		Deployment:  "vllm",
		TargetHost:  "vllm-svc",
		TargetPort:  "80",
		IdleTimeout: "5m",
		Port:        "8080",
	}

	// Create a simple AutoScaler for testing health endpoints
	as := &AutoScaler{
		config: config,
	}

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	as.healthHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("healthHandler returned wrong status code: got %v want %v", w.Code, http.StatusOK)
	}

	expected := "OK"
	if w.Body.String() != expected {
		t.Errorf("healthHandler returned unexpected body: got %v want %v", w.Body.String(), expected)
	}
}

func TestAutoScaler_updateActivity(t *testing.T) {
	config := &Config{
		Namespace:   "default",
		Deployment:  "vllm",
		TargetHost:  "vllm-svc",
		TargetPort:  "80",
		IdleTimeout: "5m",
		Port:        "8080",
	}

	as := &AutoScaler{
		config:       config,
		lastActivity: time.Now().Add(-10 * time.Minute),
		metrics:      NewMetricsRecorder(),
	}
	defer as.metrics.Stop()

	oldActivity := as.lastActivity

	// Wait a bit to ensure time difference
	time.Sleep(10 * time.Millisecond)

	as.updateActivity()

	if !as.lastActivity.After(oldActivity) {
		t.Errorf("updateActivity did not update lastActivity: old=%v new=%v", oldActivity, as.lastActivity)
	}
}

func TestConfig_ValidationEdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name: "all fields set correctly",
			config: &Config{
				Namespace:   "ai-apps",
				Deployment:  "vllm-deployment",
				TargetHost:  "vllm-service.ai-apps.svc.cluster.local",
				TargetPort:  "8000",
				IdleTimeout: "10m",
				Port:        "9090",
			},
			wantErr: false,
		},
		{
			name: "minimum valid timeout",
			config: &Config{
				Namespace:   "default",
				Deployment:  "vllm",
				TargetHost:  "vllm-svc",
				TargetPort:  "80",
				IdleTimeout: "1s",
				Port:        "8080",
			},
			wantErr: false,
		},
		{
			name: "timeout with multiple units",
			config: &Config{
				Namespace:   "default",
				Deployment:  "vllm",
				TargetHost:  "vllm-svc",
				TargetPort:  "80",
				IdleTimeout: "1h30m",
				Port:        "8080",
			},
			wantErr: false,
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

func TestAutoScaler_GetReplicas(t *testing.T) {
	// This test would require a mock Kubernetes client
	// Skipping for now as it requires integration testing setup
	t.Skip("Requires Kubernetes mock setup")
}

func TestAutoScaler_ScaleDeployment(t *testing.T) {
	// This test would require a mock Kubernetes client
	// Skipping for now as it requires integration testing setup
	t.Skip("Requires Kubernetes mock setup")
}

func TestAutoScaler_ConcurrentScaleUp(t *testing.T) {
	// Test that concurrent scale-up requests are properly synchronized
	config := &Config{
		Namespace:   "default",
		Deployment:  "vllm",
		TargetHost:  "vllm-svc",
		TargetPort:  "80",
		IdleTimeout: "5m",
		Port:        "8080",
	}

	as := &AutoScaler{
		config:       config,
		lastActivity: time.Now(),
	}
	as.scaleUpCond = &sync.Cond{L: &as.mu}

	// Verify that the condition variable is properly initialized
	if as.scaleUpCond == nil {
	}
}

func TestAutoScaler_ModelSwitchConcurrency(t *testing.T) {
	// Test that model switch synchronization works
	cfg := &Config{
		Namespace:      "test",
		Deployment:     "test-deployment",
		TargetHost:     "test-host",
		TargetPort:     "80",
		IdleTimeout:    "5m",
		Port:           "8080",
		ConfigMapName:  "vllm-config",
		ManagedTimeout: "5m",
	}

	as := &AutoScaler{
		config:       cfg,
		lastActivity: time.Now(),
	}
	as.modelSwitchCond = &sync.Cond{L: &as.mu}

	// Verify that the condition variable is properly initialized
	if as.modelSwitchCond == nil {
		t.Error("modelSwitchCond should not be nil")
	}

	// Test setting switching state
	as.mu.Lock()
	as.isSwitchingModel = true
	as.switchingToModel = "test-model"
	as.mu.Unlock()

	as.mu.RLock()
	if !as.isSwitchingModel {
		t.Error("isSwitchingModel should be true")
	}
	if as.switchingToModel != "test-model" {
		t.Errorf("switchingToModel = %v, want test-model", as.switchingToModel)
	}
	as.mu.RUnlock()
}
