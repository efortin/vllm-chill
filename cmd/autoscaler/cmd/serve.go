package cmd

import (
	"log"
	"os"

	"github.com/efortin/vllm-autoscaler/pkg/proxy"
	"github.com/spf13/cobra"
)

var (
	namespace   string
	deployment  string
	targetHost  string
	targetPort  string
	idleTimeout string
	port        string
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
	RunE: func(cmd *cobra.Command, args []string) error {
		config := &proxy.Config{
			Namespace:   namespace,
			Deployment:  deployment,
			TargetHost:  targetHost,
			TargetPort:  targetPort,
			IdleTimeout: idleTimeout,
			Port:        port,
		}

		scaler, err := proxy.NewAutoScaler(config)
		if err != nil {
			return err
		}

		log.Printf("🚀 Starting vLLM AutoScaler on :%s", port)
		log.Printf("   Target: http://%s:%s", targetHost, targetPort)
		log.Printf("   Deployment: %s/%s", namespace, deployment)
		log.Printf("   Idle timeout: %s", idleTimeout)

		return scaler.Start()
	},
}

func init() {
	rootCmd.AddCommand(serveCmd)

	serveCmd.Flags().StringVar(&namespace, "namespace", getEnvOrDefault("VLLM_NAMESPACE", "ai-apps"), "Kubernetes namespace")
	serveCmd.Flags().StringVar(&deployment, "deployment", getEnvOrDefault("VLLM_DEPLOYMENT", "vllm"), "Deployment name")
	serveCmd.Flags().StringVar(&targetHost, "target-host", getEnvOrDefault("VLLM_TARGET", "vllm-svc"), "Target service host")
	serveCmd.Flags().StringVar(&targetPort, "target-port", getEnvOrDefault("VLLM_PORT", "80"), "Target service port")
	serveCmd.Flags().StringVar(&idleTimeout, "idle-timeout", getEnvOrDefault("IDLE_TIMEOUT", "5m"), "Idle timeout before scaling to 0")
	serveCmd.Flags().StringVar(&port, "port", getEnvOrDefault("PORT", "8080"), "HTTP server port")
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
