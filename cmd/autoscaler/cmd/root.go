package cmd

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "vllm-autoscaler",
	Short: "Kubernetes autoscaler proxy for vLLM with scale-to-zero support",
	Long: `vLLM AutoScaler is a lightweight Go proxy that automatically scales
vLLM deployments to zero when idle and wakes them up on incoming requests.

It buffers connections during scale-up and tracks activity to scale down
after a configurable idle timeout.`,
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.CompletionOptions.DisableDefaultCmd = true
}
