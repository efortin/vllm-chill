package proxy

import (
	"fmt"
	"time"
)

// Config holds the configuration for the AutoScaler
type Config struct {
	Namespace          string
	Deployment         string
	ConfigMapName      string
	TargetHost         string
	TargetPort         string
	IdleTimeout        string
	ModelSwitchTimeout string
	Port               string
	EnableModelSwitch  bool
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	if c.Namespace == "" {
		return fmt.Errorf("namespace cannot be empty")
	}
	if c.Deployment == "" {
		return fmt.Errorf("deployment cannot be empty")
	}
	if c.EnableModelSwitch && c.ConfigMapName == "" {
		return fmt.Errorf("configmap name cannot be empty when model switching is enabled")
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
	if c.EnableModelSwitch {
		if _, err := time.ParseDuration(c.ModelSwitchTimeout); err != nil {
			return fmt.Errorf("invalid model switch timeout: %w", err)
		}
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

// GetModelSwitchTimeout parses and returns the model switch timeout duration
func (c *Config) GetModelSwitchTimeout() time.Duration {
	d, _ := time.ParseDuration(c.ModelSwitchTimeout)
	if d == 0 {
		return 5 * time.Minute // Default to 5 minutes
	}
	return d
}
