package proxy_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/efortin/vllm-chill/pkg/proxy"
	"github.com/efortin/vllm-chill/pkg/stats"
	"github.com/gin-gonic/gin"
)

var _ = Describe("MetricsHandler", func() {
	var (
		mockVLLMServer *httptest.Server
		autoscaler     *proxy.AutoScaler
		router         *gin.Engine
	)

	BeforeEach(func() {
		gin.SetMode(gin.TestMode)

		// Create mock vLLM server that returns sample metrics
		mockVLLMServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/metrics" {
				w.Header().Set("Content-Type", "text/plain")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`# vLLM sample metrics
vllm_num_requests_running 5
vllm_engine_state 1
vllm_cache_usage_percent 42.5
`))
			} else {
				w.WriteHeader(http.StatusNotFound)
			}
		}))

		// Parse mock server URL
		targetURL, err := url.Parse(mockVLLMServer.URL)
		Expect(err).NotTo(HaveOccurred())

		// Create autoscaler with mock target
		autoscaler = &proxy.AutoScaler{}
		autoscaler.SetTargetURL(targetURL)
		autoscaler.SetMetrics(stats.NewMetricsRecorder())

		// Setup router
		router = gin.New()
		router.GET("/proxy/metrics", autoscaler.MetricsHandler)
	})

	AfterEach(func() {
		if mockVLLMServer != nil {
			mockVLLMServer.Close()
		}
		if autoscaler != nil && autoscaler.GetMetrics() != nil {
			autoscaler.GetMetrics().Stop()
		}
	})

	Context("when vLLM server is available", func() {
		It("should combine vLLM and proxy metrics", func() {
			req := httptest.NewRequest(http.MethodGet, "/proxy/metrics", nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusOK))
			Expect(w.Header().Get("Content-Type")).To(Equal("text/plain; version=0.0.4"))

			body := w.Body.String()
			// Should contain vLLM metrics section
			Expect(body).To(ContainSubstring("# vLLM Metrics"))
			Expect(body).To(ContainSubstring("vllm_num_requests_running 5"))
			Expect(body).To(ContainSubstring("vllm_engine_state 1"))
			Expect(body).To(ContainSubstring("vllm_cache_usage_percent 42.5"))

			// Should contain proxy metrics section
			Expect(body).To(ContainSubstring("# Proxy Metrics"))
			// Prometheus metrics should be present
			Expect(body).To(ContainSubstring("go_"))
		})
	})

	Context("when vLLM server is unavailable", func() {
		BeforeEach(func() {
			// Close the mock server to simulate unavailability
			mockVLLMServer.Close()
		})

		It("should still return proxy metrics with error message for vLLM", func() {
			req := httptest.NewRequest(http.MethodGet, "/proxy/metrics", nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusOK))

			body := w.Body.String()
			// Should contain error message for vLLM metrics
			Expect(body).To(ContainSubstring("# vLLM metrics unavailable"))

			// Should still contain proxy metrics
			Expect(body).To(ContainSubstring("# Proxy Metrics"))
			Expect(body).To(ContainSubstring("go_"))
		})
	})

	Context("when vLLM server returns non-200 status", func() {
		BeforeEach(func() {
			mockVLLMServer.Close()

			// Create server that returns 503
			mockVLLMServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusServiceUnavailable)
			}))

			targetURL, err := url.Parse(mockVLLMServer.URL)
			Expect(err).NotTo(HaveOccurred())
			autoscaler.SetTargetURL(targetURL)
		})

		It("should include status code in error message", func() {
			req := httptest.NewRequest(http.MethodGet, "/proxy/metrics", nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusOK))

			body := w.Body.String()
			Expect(body).To(ContainSubstring("# vLLM metrics returned status 503"))
			Expect(body).To(ContainSubstring("# Proxy Metrics"))
		})
	})
})
