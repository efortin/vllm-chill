// Package cmd provides the command-line interface for vllm-chill.
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	version   string
	commit    string
	buildDate string
)

var rootCmd = &cobra.Command{
	Use:   "vllm-chill",
	Short: "Kubernetes autoscaler proxy for vLLM with scale-to-zero support",
	Long: `vLLM AutoScaler is a lightweight Go proxy that automatically scales
vLLM deployments to zero when idle and wakes them up on incoming requests.

It buffers connections during scale-up and tracks activity to scale down
after a configurable idle timeout.`,
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}

// SetVersion sets the version information for the CLI.
func SetVersion(ver, cmt, date string) {
	rootCmd.Version = fmt.Sprintf("%s (commit: %s, built: %s)", ver, cmt, date)
	// Also set for the serve command to pass to the proxy
	version = ver
	commit = cmt
	buildDate = date
}

func init() {
	rootCmd.CompletionOptions.DisableDefaultCmd = true
}
