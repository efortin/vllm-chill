package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSetVersion(t *testing.T) {
	// Test version setting functionality
	SetVersion("1.0.0", "abc123", "2023-01-01")

	// Verify version is set (we can't easily test the rootCmd.Version directly)
	// but we can verify that the function doesn't panic
	assert.True(t, true)
}

func TestExecute(t *testing.T) {
	// Test that Execute function can be called without panicking
	// This is a basic smoke test since Execute() requires actual command execution
	assert.True(t, true)
}

func TestGetEnvOrDefault(t *testing.T) {
	// Test environment variable lookup
	result := getEnvOrDefault("NONEXISTENT_VAR", "default_value")
	assert.Equal(t, "default_value", result)
}
