package proxy

import (
	"fmt"
	"time"
)

// Config holds the configuration for the AutoScaler
type Config struct {
	Namespace     string
	Deployment    string
	ConfigMapName string
	IdleTimeout   string
	Port          string
	LogOutput     bool
	ModelID       string // Static model ID to load from CRD
	GPUCount      int    // Number of GPUs to allocate (infrastructure-level)
	CPUOffloadGB  int    // CPU offload in GB (infrastructure-level)
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	if c.Namespace == "" {
		return fmt.Errorf("namespace cannot be empty")
	}
	if c.Deployment == "" {
		return fmt.Errorf("deployment cannot be empty")
	}
	if c.ConfigMapName == "" {
		return fmt.Errorf("configmap name cannot be empty")
	}
	if _, err := time.ParseDuration(c.IdleTimeout); err != nil {
		return fmt.Errorf("invalid idle timeout: %w", err)
	}
	if c.ModelID == "" {
		return fmt.Errorf("model ID cannot be empty")
	}
	return nil
}

// GetIdleTimeout parses and returns the idle timeout duration
func (c *Config) GetIdleTimeout() time.Duration {
	d, _ := time.ParseDuration(c.IdleTimeout)
	return d
}
