package proxy

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"time"

	"github.com/efortin/vllm-chill/pkg/stats"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("WaitForVLLMHealth", func() {
	var (
		as     *AutoScaler
		ctx    context.Context
		server *httptest.Server
	)

	AfterEach(func() {
		if server != nil {
			server.Close()
		}
		if as != nil && as.metrics != nil {
			as.metrics.Stop()
		}
	})

	Describe("immediate success", func() {
		It("should return immediately when vLLM is already healthy", func() {
			// Create a test server that responds OK immediately
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/health" {
					w.WriteHeader(http.StatusOK)
					if _, err := fmt.Fprintln(w, "OK"); err != nil {
						GinkgoWriter.Printf("Failed to write response: %v\n", err)
					}
				}
			}))

			targetURL, err := url.Parse(server.URL)
			Expect(err).NotTo(HaveOccurred())

			as = &AutoScaler{
				targetURL: targetURL,
				metrics:   stats.NewMetricsRecorder(),
			}

			ctx = context.Background()
			start := time.Now()

			err = as.waitForVLLMHealth(ctx, 10*time.Second)
			elapsed := time.Since(start)

			Expect(err).NotTo(HaveOccurred())
			Expect(elapsed).To(BeNumerically("<", 5*time.Second), "should complete within 5 seconds")
		})
	})

	Describe("retry until healthy", func() {
		It("should retry until vLLM becomes healthy", func() {
			var isReady atomic.Bool
			isReady.Store(false)
			requestCount := atomic.Int32{}

			// Create a test server that simulates startup delay
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requestCount.Add(1)
				if r.URL.Path == "/health" {
					if !isReady.Load() {
						// Still loading, return 503
						w.WriteHeader(http.StatusServiceUnavailable)
						if _, err := fmt.Fprintln(w, "Loading"); err != nil {
							GinkgoWriter.Printf("Failed to write response: %v\n", err)
						}
						return
					}
					// Ready now
					w.WriteHeader(http.StatusOK)
					if _, err := fmt.Fprintln(w, "OK"); err != nil {
						GinkgoWriter.Printf("Failed to write response: %v\n", err)
					}
				}
			}))

			// Simulate vLLM becoming ready after 3 seconds
			go func() {
				time.Sleep(3 * time.Second)
				isReady.Store(true)
			}()

			targetURL, err := url.Parse(server.URL)
			Expect(err).NotTo(HaveOccurred())

			as = &AutoScaler{
				targetURL: targetURL,
				metrics:   stats.NewMetricsRecorder(),
			}

			ctx = context.Background()
			start := time.Now()

			err = as.waitForVLLMHealth(ctx, 10*time.Second)
			elapsed := time.Since(start)

			Expect(err).NotTo(HaveOccurred())
			Expect(elapsed).To(BeNumerically(">=", 2*time.Second), "should take at least 2 seconds")
			Expect(elapsed).To(BeNumerically("<", 5*time.Second), "should complete within 5 seconds")

			// Should have made multiple requests (retry logic)
			count := requestCount.Load()
			Expect(count).To(BeNumerically(">=", 2), "should retry at least once")
		})
	})

	Describe("timeout", func() {
		It("should timeout when vLLM never becomes healthy", func() {
			// Create a test server that never becomes healthy
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/health" {
					w.WriteHeader(http.StatusServiceUnavailable)
					if _, err := fmt.Fprintln(w, "Loading"); err != nil {
						GinkgoWriter.Printf("Failed to write response: %v\n", err)
					}
				}
			}))

			targetURL, err := url.Parse(server.URL)
			Expect(err).NotTo(HaveOccurred())

			as = &AutoScaler{
				targetURL: targetURL,
				metrics:   stats.NewMetricsRecorder(),
			}

			ctx = context.Background()
			start := time.Now()

			err = as.waitForVLLMHealth(ctx, 3*time.Second)
			elapsed := time.Since(start)

			Expect(err).To(HaveOccurred(), "should timeout")
			Expect(elapsed).To(BeNumerically(">=", 2*time.Second), "should wait at least 2 seconds")
			Expect(elapsed).To(BeNumerically("<", 4*time.Second), "should timeout around 3 seconds")
		})
	})

	Describe("context cancellation", func() {
		It("should respect context cancellation", func() {
			// Create a test server that never becomes healthy
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/health" {
					w.WriteHeader(http.StatusServiceUnavailable)
					if _, err := fmt.Fprintln(w, "Loading"); err != nil {
						GinkgoWriter.Printf("Failed to write response: %v\n", err)
					}
				}
			}))

			targetURL, err := url.Parse(server.URL)
			Expect(err).NotTo(HaveOccurred())

			as = &AutoScaler{
				targetURL: targetURL,
				metrics:   stats.NewMetricsRecorder(),
			}

			var cancel context.CancelFunc
			ctx, cancel = context.WithCancel(context.Background())

			// Cancel context after 1 second
			go func() {
				time.Sleep(1 * time.Second)
				cancel()
			}()

			start := time.Now()
			err = as.waitForVLLMHealth(ctx, 10*time.Second)
			elapsed := time.Since(start)

			Expect(err).To(HaveOccurred(), "should fail due to context cancellation")
			Expect(elapsed).To(BeNumerically(">=", 500*time.Millisecond), "should wait at least 500ms")
			Expect(elapsed).To(BeNumerically("<", 2*time.Second), "should cancel around 1 second, not full 10 second timeout")
		})
	})

	Describe("realistic startup simulation", func() {
		It("should handle realistic vLLM startup timing with retries", func() {
			var isReady atomic.Bool
			isReady.Store(false)
			requestCount := atomic.Int32{}
			firstRequestTime := time.Time{}

			// Create a test server that simulates vLLM startup behavior
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requestCount.Add(1)
				if firstRequestTime.IsZero() {
					firstRequestTime = time.Now()
				}

				if r.URL.Path == "/health" {
					if !isReady.Load() {
						// Connection would be refused until server actually starts listening
						// But httptest server is always listening, so we simulate with 503
						w.WriteHeader(http.StatusServiceUnavailable)
						if _, err := fmt.Fprintln(w, "Loading"); err != nil {
							GinkgoWriter.Printf("Failed to write response: %v\n", err)
						}
						return
					}
					w.WriteHeader(http.StatusOK)
					if _, err := fmt.Fprintln(w, "OK"); err != nil {
						GinkgoWriter.Printf("Failed to write response: %v\n", err)
					}
				}
			}))

			// Simulate vLLM becoming ready after 5 seconds (realistic startup time)
			go func() {
				time.Sleep(5 * time.Second)
				isReady.Store(true)
			}()

			targetURL, err := url.Parse(server.URL)
			Expect(err).NotTo(HaveOccurred())

			as = &AutoScaler{
				targetURL: targetURL,
				metrics:   stats.NewMetricsRecorder(),
			}

			ctx = context.Background()
			start := time.Now()

			err = as.waitForVLLMHealth(ctx, 30*time.Second)
			elapsed := time.Since(start)

			Expect(err).NotTo(HaveOccurred())
			Expect(elapsed).To(BeNumerically(">=", 4*time.Second), "should wait at least 4 seconds")
			Expect(elapsed).To(BeNumerically("<", 7*time.Second), "should complete within 7 seconds")

			// Should have made multiple requests with 2 second intervals
			// After 5 seconds: 0s (first), 2s, 4s = 3 requests, then one more at ~6s
			count := requestCount.Load()
			Expect(count).To(BeNumerically(">=", 2), "should make multiple health check requests")

			GinkgoWriter.Printf("Health check completed after %v with %d requests\n", elapsed, count)
		})
	})

	Describe("connection retry behavior", func() {
		Context("when vLLM starts slowly", func() {
			It("should retry connections every 2 seconds until healthy", func() {
				var isReady atomic.Bool
				isReady.Store(false)
				requestTimes := make([]time.Time, 0)
				var requestTimesMutex atomic.Value
				requestTimesMutex.Store(&requestTimes)

				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/health" {
						times := requestTimesMutex.Load().(*[]time.Time)
						*times = append(*times, time.Now())
						requestTimesMutex.Store(times)

						if !isReady.Load() {
							w.WriteHeader(http.StatusServiceUnavailable)
							if _, err := fmt.Fprintln(w, "Loading"); err != nil {
								GinkgoWriter.Printf("Failed to write response: %v\n", err)
							}
							return
						}
						w.WriteHeader(http.StatusOK)
						if _, err := fmt.Fprintln(w, "OK"); err != nil {
							GinkgoWriter.Printf("Failed to write response: %v\n", err)
						}
					}
				}))

				// Become ready after 6 seconds
				go func() {
					time.Sleep(6 * time.Second)
					isReady.Store(true)
				}()

				targetURL, err := url.Parse(server.URL)
				Expect(err).NotTo(HaveOccurred())

				as = &AutoScaler{
					targetURL: targetURL,
					metrics:   stats.NewMetricsRecorder(),
				}

				ctx = context.Background()
				err = as.waitForVLLMHealth(ctx, 15*time.Second)

				Expect(err).NotTo(HaveOccurred())

				// Verify retry intervals are approximately 2 seconds
				times := requestTimesMutex.Load().(*[]time.Time)
				Expect(len(*times)).To(BeNumerically(">=", 3), "should make multiple requests")

				// Check intervals between requests
				for i := 1; i < len(*times); i++ {
					interval := (*times)[i].Sub((*times)[i-1])
					Expect(interval).To(BeNumerically(">=", 1800*time.Millisecond), "retry interval should be at least 1.8 seconds")
					Expect(interval).To(BeNumerically("<=", 2200*time.Millisecond), "retry interval should be at most 2.2 seconds")
				}
			})
		})
	})
})
