package proxy

import (
	"fmt"
	"time"
)

// Config holds the configuration for the AutoScaler
type Config struct {
	Namespace      string
	Deployment     string
	ConfigMapName  string
	TargetHost     string
	TargetPort     string
	IdleTimeout    string
	ManagedTimeout string
	Port           string
	LogOutput      bool
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
	if c.TargetHost == "" {
		return fmt.Errorf("target host cannot be empty")
	}
	if c.TargetPort == "" {
		return fmt.Errorf("target port cannot be empty")
	}
	if _, err := time.ParseDuration(c.IdleTimeout); err != nil {
		return fmt.Errorf("invalid idle timeout: %w", err)
	}
	if _, err := time.ParseDuration(c.ManagedTimeout); err != nil {
		return fmt.Errorf("invalid managed timeout: %w", err)
	}
	return nil
}

// GetIdleTimeout parses and returns the idle timeout duration
func (c *Config) GetIdleTimeout() time.Duration {
	d, _ := time.ParseDuration(c.IdleTimeout)
	return d
}

// GetTargetURL returns the full target URL
func (c *Config) GetTargetURL() string {
	return fmt.Sprintf("http://%s:%s", c.TargetHost, c.TargetPort)
}

// GetManagedTimeout parses and returns the managed mode timeout duration
func (c *Config) GetManagedTimeout() time.Duration {
	d, _ := time.ParseDuration(c.ManagedTimeout)
	if d == 0 {
		return 5 * time.Minute // Default to 5 minutes
	}
	return d
}
