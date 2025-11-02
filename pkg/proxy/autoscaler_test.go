package proxy

import (
	"net/http"
	"net/http/httptest"
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
	}

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
