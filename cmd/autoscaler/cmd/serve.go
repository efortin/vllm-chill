package cmd

import (
	"context"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/efortin/vllm-chill/pkg/proxy"
	"github.com/efortin/vllm-chill/pkg/rbac"
	"github.com/spf13/cobra"
)

var (
	namespace        string
	deployment       string
	configMapName    string
	idleTimeout      string
	port             string
	logOutput        bool
	modelID          string
	gpuCount         int
	cpuOffloadGB     int
	publicEndpoint   string
	enableXMLParsing bool
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
		// Verify RBAC permissions at startup
		log.Println("Verifying RBAC permissions...")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := rbac.VerifyPermissions(ctx, namespace); err != nil {
			log.Printf("RBAC permission check failed: %v", err)
			return err
		}
		log.Println("RBAC permissions verified successfully")

		config := &proxy.Config{
			Namespace:        namespace,
			Deployment:       deployment,
			ConfigMapName:    configMapName,
			IdleTimeout:      idleTimeout,
			Port:             port,
			LogOutput:        logOutput,
			ModelID:          modelID,
			GPUCount:         gpuCount,
			CPUOffloadGB:     cpuOffloadGB,
			PublicEndpoint:   publicEndpoint,
			EnableXMLParsing: enableXMLParsing,
		}

		scaler, err := proxy.NewAutoScaler(config)
		if err != nil {
			return err
		}

		// Set version information
		scaler.SetVersion(version, commit, buildDate)

		log.Printf("Starting vLLM AutoScaler on :%s", port)
		targetHost := getEnvOrDefault("VLLM_TARGET", "vllm-api")
		targetPort := getEnvOrDefault("VLLM_PORT", "80")
		log.Printf("   Target: http://%s:%s", targetHost, targetPort)
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
	serveCmd.Flags().StringVar(&idleTimeout, "idle-timeout", getEnvOrDefault("IDLE_TIMEOUT", "5m"), "Idle timeout before scaling to 0")
	serveCmd.Flags().StringVar(&port, "port", getEnvOrDefault("PORT", "8080"), "HTTP server port")
	serveCmd.Flags().StringVar(&modelID, "model-id", getEnvOrDefault("MODEL_ID", ""), "Model ID to load from VLLMModel CRD (required)")
	serveCmd.Flags().IntVar(&gpuCount, "gpu-count", getEnvOrDefaultInt("GPU_COUNT", 2), "Number of GPUs to allocate (infrastructure-level)")
	serveCmd.Flags().IntVar(&cpuOffloadGB, "cpu-offload-gb", getEnvOrDefaultInt("CPU_OFFLOAD_GB", 0), "CPU offload in GB (infrastructure-level)")
	serveCmd.Flags().StringVar(&publicEndpoint, "public-endpoint", getEnvOrDefault("PUBLIC_ENDPOINT", ""), "Public-facing endpoint URL (e.g., https://vllm.sir-alfred.io)")
	serveCmd.Flags().BoolVar(&enableXMLParsing, "enable-xml-parsing", getEnvOrDefault("ENABLE_XML_PARSING", "false") == "true", "Enable XML tool call parsing (default: false)")
	// vLLM is now always managed by the autoscaler
	serveCmd.Flags().BoolVar(&logOutput, "log-output", getEnvOrDefault("LOG_OUTPUT", "false") == "true", "Log response bodies (use with caution, can be verbose)")
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvOrDefaultInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}
