package cmd

import (
	"log"
	"os"

	"github.com/efortin/vllm-chill/pkg/proxy"
	"github.com/spf13/cobra"
)

var (
	namespace     string
	deployment    string
	configMapName string
	targetSocket  string
	idleTimeout   string
	port          string
	logOutput     bool
	modelID       string
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the autoscaler proxy server",
	Long: `Start the HTTP proxy server that handles automatic scaling of vLLM.
	
The proxy will:
- Scale vLLM to 1 replica on incoming requests
- Buffer connections during scale-up (max 2 minutes)
- Track activity and scale to 0 after idle timeout
- Proxy all requests to the vLLM backend`,
	RunE: func(_ *cobra.Command, _ []string) error {
		config := &proxy.Config{
			Namespace:     namespace,
			Deployment:    deployment,
			ConfigMapName: configMapName,
			TargetSocket:  targetSocket,
			IdleTimeout:   idleTimeout,
			Port:          port,
			LogOutput:     logOutput,
			ModelID:       modelID,
		}

		scaler, err := proxy.NewAutoScaler(config)
		if err != nil {
			return err
		}

		// Set version information
		scaler.SetVersion(version, commit, buildDate)

		log.Printf("Starting vLLM AutoScaler on :%s", port)
		log.Printf("   Target: unix://%s", targetSocket)
		log.Printf("   Deployment: %s/%s", namespace, deployment)
		log.Printf("   ConfigMap: %s/%s", namespace, configMapName)
		log.Printf("   Model ID: %s", modelID)
		log.Printf("   Idle timeout: %s", idleTimeout)
		if logOutput {
			log.Printf("   Output logging: enabled")
		}

		return scaler.Run()
	},
}

func init() {
	rootCmd.AddCommand(serveCmd)

	serveCmd.Flags().StringVar(&namespace, "namespace", getEnvOrDefault("VLLM_NAMESPACE", "vllm"), "Kubernetes namespace")
	serveCmd.Flags().StringVar(&deployment, "deployment", getEnvOrDefault("VLLM_DEPLOYMENT", "vllm"), "Deployment name")
	serveCmd.Flags().StringVar(&configMapName, "configmap", getEnvOrDefault("VLLM_CONFIGMAP", "vllm-config"), "ConfigMap name for model configuration")
	serveCmd.Flags().StringVar(&targetSocket, "target-socket", getEnvOrDefault("VLLM_SOCKET", "/tmp/vllm.sock"), "Unix socket path to vLLM")
	serveCmd.Flags().StringVar(&idleTimeout, "idle-timeout", getEnvOrDefault("IDLE_TIMEOUT", "5m"), "Idle timeout before scaling to 0")
	serveCmd.Flags().StringVar(&port, "port", getEnvOrDefault("PORT", "8080"), "HTTP server port")
	serveCmd.Flags().StringVar(&modelID, "model-id", getEnvOrDefault("MODEL_ID", ""), "Model ID to load from VLLMModel CRD (required)")
	// vLLM is now always managed by the autoscaler
	serveCmd.Flags().BoolVar(&logOutput, "log-output", getEnvOrDefault("LOG_OUTPUT", "false") == "true", "Log response bodies (use with caution, can be verbose)")
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
