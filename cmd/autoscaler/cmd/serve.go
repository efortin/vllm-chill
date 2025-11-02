package cmd

import (
	"log"
	"os"

	"github.com/efortin/vllm-chill/pkg/proxy"
	"github.com/spf13/cobra"
)

var (
	namespace      string
	deployment     string
	configMapName  string
	targetHost     string
	targetPort     string
	idleTimeout    string
	managedTimeout string
	port           string
	logOutput      bool
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
			Namespace:      namespace,
			Deployment:     deployment,
			ConfigMapName:  configMapName,
			TargetHost:     targetHost,
			TargetPort:     targetPort,
			IdleTimeout:    idleTimeout,
			ManagedTimeout: managedTimeout,
			Port:           port,
			LogOutput:      logOutput,
		}

		scaler, err := proxy.NewAutoScaler(config)
		if err != nil {
			return err
		}

		log.Printf("Starting vLLM AutoScaler on :%s", port)
		log.Printf("   Target: http://%s:%s", targetHost, targetPort)
		log.Printf("   Deployment: %s/%s", namespace, deployment)
		log.Printf("   ConfigMap: %s/%s", namespace, configMapName)
		log.Printf("   Idle timeout: %s", idleTimeout)
		log.Printf("   Managed timeout: %s", managedTimeout)
		if logOutput {
			log.Printf("   Output logging: enabled")
		}

		return scaler.Start()
	},
}

func init() {
	rootCmd.AddCommand(serveCmd)

	serveCmd.Flags().StringVar(&namespace, "namespace", getEnvOrDefault("VLLM_NAMESPACE", "ai-apps"), "Kubernetes namespace")
	serveCmd.Flags().StringVar(&deployment, "deployment", getEnvOrDefault("VLLM_DEPLOYMENT", "vllm"), "Deployment name")
	serveCmd.Flags().StringVar(&configMapName, "configmap", getEnvOrDefault("VLLM_CONFIGMAP", "vllm-config"), "ConfigMap name for model configuration")
	serveCmd.Flags().StringVar(&targetHost, "target-host", getEnvOrDefault("VLLM_TARGET", "vllm-svc"), "Target service host")
	serveCmd.Flags().StringVar(&targetPort, "target-port", getEnvOrDefault("VLLM_PORT", "80"), "Target service port")
	serveCmd.Flags().StringVar(&idleTimeout, "idle-timeout", getEnvOrDefault("IDLE_TIMEOUT", "5m"), "Idle timeout before scaling to 0")
	serveCmd.Flags().StringVar(&managedTimeout, "managed-timeout", getEnvOrDefault("MANAGED_TIMEOUT", "5m"), "Timeout for managed operations")
	serveCmd.Flags().StringVar(&port, "port", getEnvOrDefault("PORT", "8080"), "HTTP server port")
	// Managed mode and metrics are now always enabled
	serveCmd.Flags().BoolVar(&logOutput, "log-output", getEnvOrDefault("LOG_OUTPUT", "false") == "true", "Log response bodies (use with caution, can be verbose)")
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
