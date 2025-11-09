package proxy_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/efortin/vllm-chill/pkg/proxy"
)

var _ = Describe("Config", func() {
	Describe("Validate", func() {
		DescribeTable("validation tests",
			func(config *proxy.Config, expectError bool) {
				err := config.Validate()
				if expectError {
					Expect(err).To(HaveOccurred())
				} else {
					Expect(err).NotTo(HaveOccurred())
				}
			},
			Entry("all fields set correctly",
				&proxy.Config{
					Namespace:     "ai-apps",
					Deployment:    "vllm-deployment",
					ConfigMapName: "vllm-config",
					TargetHost:    "vllm-service.ai-apps.svc.cluster.local",
					TargetPort:    "8000",
					IdleTimeout:   "10m",
					Port:          "9090",
					ModelID:       "test-model",
				},
				false,
			),
			Entry("empty namespace",
				&proxy.Config{
					Namespace:     "",
					Deployment:    "vllm-deployment",
					ConfigMapName: "vllm-config",
					TargetHost:    "vllm-service.ai-apps.svc.cluster.local",
					TargetPort:    "8000",
					IdleTimeout:   "10m",
					Port:          "9090",
					ModelID:       "test-model",
				},
				true,
			),
			Entry("empty deployment",
				&proxy.Config{
					Namespace:     "ai-apps",
					Deployment:    "",
					ConfigMapName: "vllm-config",
					TargetHost:    "vllm-service.ai-apps.svc.cluster.local",
					TargetPort:    "8000",
					IdleTimeout:   "10m",
					Port:          "9090",
					ModelID:       "test-model",
				},
				true,
			),
			Entry("empty configmap name",
				&proxy.Config{
					Namespace:     "ai-apps",
					Deployment:    "vllm-deployment",
					ConfigMapName: "",
					TargetHost:    "vllm-service.ai-apps.svc.cluster.local",
					TargetPort:    "8000",
					IdleTimeout:   "10m",
					Port:          "9090",
					ModelID:       "test-model",
				},
				true,
			),
			Entry("empty target host",
				&proxy.Config{
					Namespace:     "ai-apps",
					Deployment:    "vllm-deployment",
					ConfigMapName: "vllm-config",
					TargetHost:    "",
					TargetPort:    "8000",
					IdleTimeout:   "10m",
					Port:          "9090",
					ModelID:       "test-model",
				},
				true,
			),
			Entry("empty target port",
				&proxy.Config{
					Namespace:     "ai-apps",
					Deployment:    "vllm-deployment",
					ConfigMapName: "vllm-config",
					TargetHost:    "vllm-service.ai-apps.svc.cluster.local",
					TargetPort:    "",
					IdleTimeout:   "10m",
					Port:          "9090",
					ModelID:       "test-model",
				},
				true,
			),
			Entry("invalid idle timeout",
				&proxy.Config{
					Namespace:     "ai-apps",
					Deployment:    "vllm-deployment",
					ConfigMapName: "vllm-config",
					TargetHost:    "vllm-service.ai-apps.svc.cluster.local",
					TargetPort:    "8000",
					IdleTimeout:   "invalid",
					Port:          "9090",
					ModelID:       "test-model",
				},
				true,
			),
			Entry("empty model ID",
				&proxy.Config{
					Namespace:     "ai-apps",
					Deployment:    "vllm-deployment",
					ConfigMapName: "vllm-config",
					TargetHost:    "vllm-service.ai-apps.svc.cluster.local",
					TargetPort:    "8000",
					IdleTimeout:   "10m",
					Port:          "9090",
					ModelID:       "",
				},
				true,
			),
		)
	})

	Describe("GetIdleTimeout", func() {
		DescribeTable("timeout parsing tests",
			func(idleTimeout string, expected time.Duration) {
				config := &proxy.Config{IdleTimeout: idleTimeout}
				result := config.GetIdleTimeout()
				Expect(result).To(Equal(expected))
			},
			Entry("valid timeout", "5m", 5*time.Minute),
			Entry("zero timeout", "0s", time.Duration(0)),
			Entry("custom timeout", "1h30m", time.Hour+30*time.Minute),
		)
	})

	Describe("GetTargetURL", func() {
		DescribeTable("URL generation tests",
			func(host, port, expected string) {
				config := &proxy.Config{
					TargetHost: host,
					TargetPort: port,
				}
				result := config.GetTargetURL()
				Expect(result).To(Equal(expected))
			},
			Entry("standard case", "localhost", "8080", "http://localhost:8080"),
			Entry("different host and port", "api.example.com", "443", "http://api.example.com:443"),
		)
	})
})
