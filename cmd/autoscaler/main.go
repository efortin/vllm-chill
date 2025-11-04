// Package main is the entry point for the vllm-chill autoscaler.
package main

import (
	"fmt"
	"os"

	"github.com/efortin/vllm-chill/cmd/autoscaler/cmd"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	cmd.SetVersion(version, commit, date)
	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
