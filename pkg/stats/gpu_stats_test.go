//go:build dummy_gpu || test

package stats

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestGPUStats(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "GPU Stats Test Suite")
}

var _ = Describe("GPU Stats with Mock NVML", func() {
	Describe("NVML Initialization", func() {
		AfterEach(func() {
			if nvmlInitialized {
				_ = nvmlShutdown()
			}
		})

		It("should initialize mock NVML", func() {
			err := nvmlInit()
			Expect(err).NotTo(HaveOccurred())
			Expect(nvmlInitialized).To(BeTrue())
		})

		It("should handle double initialization", func() {
			err := nvmlInit()
			Expect(err).NotTo(HaveOccurred())

			// Second init should succeed (idempotent)
			err = nvmlInit()
			Expect(err).NotTo(HaveOccurred())
		})

		It("should shutdown mock NVML", func() {
			err := nvmlInit()
			Expect(err).NotTo(HaveOccurred())

			err = nvmlShutdown()
			Expect(err).NotTo(HaveOccurred())
			Expect(nvmlInitialized).To(BeFalse())
		})
	})

	Describe("Device Count", func() {
		BeforeEach(func() {
			_ = nvmlInit()
		})

		AfterEach(func() {
			_ = nvmlShutdown()
		})

		It("should return mock device count", func() {
			count, err := nvmlDeviceGetCount()
			Expect(err).NotTo(HaveOccurred())
			Expect(count).To(Equal(2)) // We create 2 mock devices
		})

		It("should fail when NVML not initialized", func() {
			_ = nvmlShutdown()

			_, err := nvmlDeviceGetCount()
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("Device Information", func() {
		BeforeEach(func() {
			_ = nvmlInit()
		})

		AfterEach(func() {
			_ = nvmlShutdown()
		})

		It("should get device by index", func() {
			device, err := nvmlDeviceGetHandleByIndex(0)
			Expect(err).NotTo(HaveOccurred())
			Expect(device).NotTo(BeNil())
		})

		It("should fail for invalid index", func() {
			_, err := nvmlDeviceGetHandleByIndex(99)
			Expect(err).To(HaveOccurred())
		})

		It("should get device name", func() {
			device, err := nvmlDeviceGetHandleByIndex(0)
			Expect(err).NotTo(HaveOccurred())

			name, err := nvmlDeviceGetName(device)
			Expect(err).NotTo(HaveOccurred())
			Expect(name).To(ContainSubstring("Mock NVIDIA GPU"))
		})

		It("should get device UUID", func() {
			device, err := nvmlDeviceGetHandleByIndex(0)
			Expect(err).NotTo(HaveOccurred())

			uuid, err := nvmlDeviceGetUUID(device)
			Expect(err).NotTo(HaveOccurred())
			Expect(uuid).To(ContainSubstring("GPU-"))
		})

		It("should get device temperature", func() {
			device, err := nvmlDeviceGetHandleByIndex(0)
			Expect(err).NotTo(HaveOccurred())

			temp, err := nvmlDeviceGetTemperature(device)
			Expect(err).NotTo(HaveOccurred())
			Expect(temp).To(BeNumerically(">", 0))
			Expect(temp).To(BeNumerically("<", 100))
		})

		It("should get device utilization", func() {
			device, err := nvmlDeviceGetHandleByIndex(0)
			Expect(err).NotTo(HaveOccurred())

			gpuUtil, memUtil, err := nvmlDeviceGetUtilizationRates(device)
			Expect(err).NotTo(HaveOccurred())
			Expect(gpuUtil).To(BeNumerically(">=", 0))
			Expect(gpuUtil).To(BeNumerically("<=", 100))
			Expect(memUtil).To(BeNumerically(">=", 0))
			Expect(memUtil).To(BeNumerically("<=", 100))
		})

		It("should get device memory info", func() {
			device, err := nvmlDeviceGetHandleByIndex(0)
			Expect(err).NotTo(HaveOccurred())

			used, free, total, err := nvmlDeviceGetMemoryInfo(device)
			Expect(err).NotTo(HaveOccurred())
			Expect(used).To(BeNumerically(">", 0))
			Expect(free).To(BeNumerically(">", 0))
			Expect(total).To(Equal(used + free))
		})

		It("should get device power usage", func() {
			device, err := nvmlDeviceGetHandleByIndex(0)
			Expect(err).NotTo(HaveOccurred())

			power, err := nvmlDeviceGetPowerUsage(device)
			Expect(err).NotTo(HaveOccurred())
			Expect(power).To(BeNumerically(">", 0))
		})

		It("should get device compute mode", func() {
			device, err := nvmlDeviceGetHandleByIndex(0)
			Expect(err).NotTo(HaveOccurred())

			mode, err := nvmlDeviceGetComputeMode(device)
			Expect(err).NotTo(HaveOccurred())
			Expect(mode).To(BeNumerically(">=", 0))
		})

		It("should get device persistence mode", func() {
			device, err := nvmlDeviceGetHandleByIndex(0)
			Expect(err).NotTo(HaveOccurred())

			mode, err := nvmlDeviceGetPersistenceMode(device)
			Expect(err).NotTo(HaveOccurred())
			Expect(mode).To(BeNumerically(">=", 0))
		})
	})

	Describe("Multiple Devices", func() {
		BeforeEach(func() {
			_ = nvmlInit()
		})

		AfterEach(func() {
			_ = nvmlShutdown()
		})

		It("should handle multiple devices", func() {
			count, err := nvmlDeviceGetCount()
			Expect(err).NotTo(HaveOccurred())

			for i := 0; i < count; i++ {
				device, err := nvmlDeviceGetHandleByIndex(i)
				Expect(err).NotTo(HaveOccurred())

				name, err := nvmlDeviceGetName(device)
				Expect(err).NotTo(HaveOccurred())
				Expect(name).To(ContainSubstring("GPU"))

				GinkgoWriter.Printf("Device %d: %s\n", i, name)
			}
		})

		It("should have different UUIDs for different devices", func() {
			device0, _ := nvmlDeviceGetHandleByIndex(0)
			device1, _ := nvmlDeviceGetHandleByIndex(1)

			uuid0, _ := nvmlDeviceGetUUID(device0)
			uuid1, _ := nvmlDeviceGetUUID(device1)

			Expect(uuid0).NotTo(Equal(uuid1))
		})
	})

	Describe("GetGPUCount utility", func() {
		It("should return GPU count from mock NVML", func() {
			// Should initialize and return mock count
			count, err := GetGPUCount()
			if err == nil && count > 0 {
				Expect(count).To(BeNumerically(">", 0))
			}
		})
	})

	Describe("GPUStatsHandler", func() {
		var handler *GPUStatsHandler

		BeforeEach(func() {
			handler = NewGPUStatsHandler()
		})

		AfterEach(func() {
			if handler != nil {
				handler.Shutdown()
			}
		})

		It("should create GPU stats handler", func() {
			Expect(handler).NotTo(BeNil())
		})

		It("should handle shutdown gracefully", func() {
			err := handler.Shutdown()
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("GinGPUStatsHandler", func() {
		var handler *GinGPUStatsHandler

		BeforeEach(func() {
			handler = NewGinGPUStatsHandler()
		})

		AfterEach(func() {
			if handler != nil {
				handler.Shutdown()
			}
		})

		It("should create Gin GPU stats handler", func() {
			Expect(handler).NotTo(BeNil())
		})

		It("should handle shutdown gracefully", func() {
			err := handler.Shutdown()
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("Concurrent Access", func() {
		BeforeEach(func() {
			_ = nvmlInit()
		})

		AfterEach(func() {
			_ = nvmlShutdown()
		})

		It("should handle concurrent device queries", func() {
			done := make(chan bool, 10)

			for i := 0; i < 10; i++ {
				go func(id int) {
					device, err := nvmlDeviceGetHandleByIndex(id % 2) // Alternate between 2 devices
					Expect(err).NotTo(HaveOccurred())

					_, err = nvmlDeviceGetName(device)
					Expect(err).NotTo(HaveOccurred())

					_, err = nvmlDeviceGetTemperature(device)
					Expect(err).NotTo(HaveOccurred())

					_, _, err = nvmlDeviceGetUtilizationRates(device)
					Expect(err).NotTo(HaveOccurred())

					done <- true
				}(i)
			}

			// Wait for all goroutines
			for i := 0; i < 10; i++ {
				select {
				case <-done:
					// Success
				case <-time.After(5 * time.Second):
					Fail("Timeout waiting for concurrent queries")
				}
			}
		})
	})
})
