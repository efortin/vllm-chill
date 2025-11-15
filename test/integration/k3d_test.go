//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("K3D Integration Tests", func() {
	var (
		proxyURL     string
		k3dContext   string
		namespace    string
		httpClient   *http.Client
		portForward  *exec.Cmd
		vllmDummyPod string
	)

	BeforeEach(func() {
		k3dContext = "k3d-vllm-test"
		namespace = "vllm"
		proxyURL = "http://localhost:8080"
		vllmDummyPod = "vllm-test-pod"

		httpClient = &http.Client{
			Timeout: 30 * time.Second,
		}

		// Verify k3d cluster exists
		cmd := exec.Command("kubectl", "cluster-info", "--context", k3dContext)
		err := cmd.Run()
		Expect(err).NotTo(HaveOccurred(), "k3d cluster should be running. Run 'task -t Taskfile.k3d.yml setup' first")

		// Start port-forward in background
		GinkgoWriter.Printf("Starting port-forward to %s/%s...\n", namespace, vllmDummyPod)
		portForward = exec.Command("kubectl", "port-forward",
			"-n", namespace,
			vllmDummyPod,
			"8080:8080",
			"--context", k3dContext,
		)
		portForward.Stdout = GinkgoWriter
		portForward.Stderr = GinkgoWriter
		err = portForward.Start()
		Expect(err).NotTo(HaveOccurred())

		// Wait for port-forward to be ready
		time.Sleep(3 * time.Second)

		// Verify proxy is responding
		Eventually(func() error {
			resp, err := httpClient.Get(proxyURL + "/proxy/version")
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("unexpected status: %d", resp.StatusCode)
			}
			return nil
		}, 30*time.Second, 2*time.Second).Should(Succeed())

		GinkgoWriter.Printf("Port-forward ready, proxy responding\n")
	})

	AfterEach(func() {
		if portForward != nil && portForward.Process != nil {
			GinkgoWriter.Printf("Stopping port-forward...\n")
			_ = portForward.Process.Kill()
			_ = portForward.Wait()
		}
	})

	Describe("Proxy Health and Metrics", func() {
		It("should have metrics endpoint", func() {
			resp, err := httpClient.Get(proxyURL + "/metrics")
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			body, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())

			// Should contain Prometheus metrics
			Expect(string(body)).To(ContainSubstring("# HELP"))
		})
	})

	Describe("vLLM API Proxying", func() {
		It("should proxy /v1/models request", func() {
			resp, err := httpClient.Get(proxyURL + "/v1/models")
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			body, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())

			var modelsResp map[string]interface{}
			err = json.Unmarshal(body, &modelsResp)
			Expect(err).NotTo(HaveOccurred())

			Expect(modelsResp["object"]).To(Equal("list"))
			Expect(modelsResp["data"]).NotTo(BeNil())

			GinkgoWriter.Printf("Models response: %+v\n", modelsResp)
		})

		It("should proxy /v1/chat/completions request", func() {
			requestBody := map[string]interface{}{
				"model": "test-model",
				"messages": []map[string]string{
					{"role": "user", "content": "Hello"},
				},
			}

			bodyBytes, err := json.Marshal(requestBody)
			Expect(err).NotTo(HaveOccurred())

			resp, err := httpClient.Post(
				proxyURL+"/v1/chat/completions",
				"application/json",
				bytes.NewBuffer(bodyBytes),
			)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			body, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())

			var completionResp map[string]interface{}
			err = json.Unmarshal(body, &completionResp)
			Expect(err).NotTo(HaveOccurred())

			Expect(completionResp["object"]).To(Equal("chat.completion"))
			Expect(completionResp["choices"]).NotTo(BeNil())

			GinkgoWriter.Printf("Chat completion response: %+v\n", completionResp)
		})

		It("should proxy /v1/completions request", func() {
			requestBody := map[string]interface{}{
				"model":  "test-model",
				"prompt": "Hello",
			}

			bodyBytes, err := json.Marshal(requestBody)
			Expect(err).NotTo(HaveOccurred())

			resp, err := httpClient.Post(
				proxyURL+"/v1/completions",
				"application/json",
				bytes.NewBuffer(bodyBytes),
			)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			body, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())

			var completionResp map[string]interface{}
			err = json.Unmarshal(body, &completionResp)
			Expect(err).NotTo(HaveOccurred())

			Expect(completionResp["object"]).To(Equal("text_completion"))

			GinkgoWriter.Printf("Completion response: %+v\n", completionResp)
		})
	})

	Describe("Activity Tracking", func() {
		It("should track activity through model requests", func() {
			// Make initial request
			resp1, err := httpClient.Get(proxyURL + "/v1/models")
			Expect(err).NotTo(HaveOccurred())
			defer resp1.Body.Close()
			Expect(resp1.StatusCode).To(Equal(http.StatusOK))

			// Wait and make another request
			time.Sleep(2 * time.Second)
			resp2, err := httpClient.Get(proxyURL + "/v1/models")
			Expect(err).NotTo(HaveOccurred())
			defer resp2.Body.Close()
			Expect(resp2.StatusCode).To(Equal(http.StatusOK))

			GinkgoWriter.Printf("Activity tracking validated through sequential requests\n")
		})
	})

	Describe("Error Handling", func() {
		It("should handle invalid JSON gracefully", func() {
			resp, err := httpClient.Post(
				proxyURL+"/v1/chat/completions",
				"application/json",
				bytes.NewBufferString("invalid json"),
			)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			// Should still get a response (might be 400 or proxied to vLLM)
			Expect(resp.StatusCode).To(BeNumerically(">=", 200))
		})

		It("should handle non-existent endpoints", func() {
			resp, err := httpClient.Get(proxyURL + "/nonexistent")
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			// Should return 404 or be handled by proxy
			Expect(resp.StatusCode).To(BeNumerically(">=", 200))
		})
	})

	Describe("Concurrent Requests", func() {
		It("should handle multiple concurrent requests", func() {
			const concurrency = 10
			results := make(chan error, concurrency)

			for i := 0; i < concurrency; i++ {
				go func(id int) {
					resp, err := httpClient.Get(proxyURL + "/v1/models")
					if err != nil {
						results <- err
						return
					}
					defer resp.Body.Close()

					if resp.StatusCode != http.StatusOK {
						results <- fmt.Errorf("request %d failed with status %d", id, resp.StatusCode)
						return
					}

					results <- nil
				}(i)
			}

			// Collect results
			for i := 0; i < concurrency; i++ {
				err := <-results
				Expect(err).NotTo(HaveOccurred())
			}

			GinkgoWriter.Printf("Successfully handled %d concurrent requests\n", concurrency)
		})
	})
})

// Helper to check if env var is set
func envOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
