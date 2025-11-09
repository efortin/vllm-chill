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
	TargetSocket  string // Unix socket path to vLLM (e.g., "/tmp/vllm.sock")
	IdleTimeout   string
	Port          string
	LogOutput     bool
	ModelID       string // Static model ID to load from CRD
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
	if c.TargetSocket == "" {
		return fmt.Errorf("target socket cannot be empty")
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

// GetTargetSocket returns the Unix socket path
func (c *Config) GetTargetSocket() string {
	return c.TargetSocket
}
