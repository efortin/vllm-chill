package proxy_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/efortin/vllm-chill/pkg/proxy"
)

var _ = Describe("AutoScaler", func() {
	Describe("Config Validation Edge Cases", func() {
		DescribeTable("edge case validation",
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
			Entry("minimum valid timeout",
				&proxy.Config{
					Namespace:     "default",
					Deployment:    "vllm",
					ConfigMapName: "vllm-config",
					TargetHost:    "vllm-svc",
					TargetPort:    "80",
					IdleTimeout:   "1s",
					Port:          "8080",
					ModelID:       "test-model",
				},
				false,
			),
			Entry("timeout with multiple units",
				&proxy.Config{
					Namespace:     "default",
					Deployment:    "vllm",
					ConfigMapName: "vllm-config",
					TargetHost:    "vllm-svc",
					TargetPort:    "80",
					IdleTimeout:   "1h30m",
					Port:          "8080",
					ModelID:       "test-model",
				},
				false,
			),
		)
	})

	Describe("Integration tests", func() {
		Context("when creating a new AutoScaler", func() {
			It("should require Kubernetes cluster access", func() {
				Skip("Requires Kubernetes cluster - integration test")

				config := &proxy.Config{
					Namespace:     "default",
					Deployment:    "vllm",
					ConfigMapName: "vllm-config",
					TargetHost:    "vllm-svc",
					TargetPort:    "80",
					IdleTimeout:   "5m",
					Port:          "8080",
					ModelID:       "test-model",
				}

				_, err := proxy.NewAutoScaler(config)
				Expect(err).To(HaveOccurred()) // Will fail without K8s cluster
			})
		})
	})
})
