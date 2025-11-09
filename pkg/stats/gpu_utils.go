//go:build cgo

package stats

import (
	"fmt"
	"log"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

// GetGPUCount returns the number of GPUs available
func GetGPUCount() (int, error) {
	// Initialize NVML
	ret := nvml.Init()
	if ret != nvml.SUCCESS {
		return 0, fmt.Errorf("NVML Init failed: %v", nvml.ErrorString(ret))
	}
	defer func() { _ = nvml.Shutdown() }()

	// Get device count
	count, ret := nvml.DeviceGetCount()
	if ret != nvml.SUCCESS {
		return 0, fmt.Errorf("failed to get device count: %v", nvml.ErrorString(ret))
	}

	return count, nil
}

// GetGPUComputeCapability returns the CUDA compute capability of the first GPU
// Returns a string like "8.6" for RTX 3090, "8.0" for A100, etc.
func GetGPUComputeCapability() (string, error) {
	// Initialize NVML
	ret := nvml.Init()
	if ret != nvml.SUCCESS {
		return "", fmt.Errorf("NVML Init failed: %v", nvml.ErrorString(ret))
	}
	defer func() { _ = nvml.Shutdown() }()

	// Get first device
	device, ret := nvml.DeviceGetHandleByIndex(0)
	if ret != nvml.SUCCESS {
		return "", fmt.Errorf("failed to get device 0: %v", nvml.ErrorString(ret))
	}

	// Get compute capability
	major, minor, ret := device.GetCudaComputeCapability()
	if ret != nvml.SUCCESS {
		return "", fmt.Errorf("failed to get compute capability: %v", nvml.ErrorString(ret))
	}

	capability := fmt.Sprintf("%d.%d", major, minor)
	log.Printf("[GPU-UTILS] Detected GPU compute capability: %s", capability)

	return capability, nil
}

// GetGPUInfo returns both GPU count and compute capability
func GetGPUInfo() (count int, computeCapability string, err error) {
	// Initialize NVML once
	ret := nvml.Init()
	if ret != nvml.SUCCESS {
		return 0, "", fmt.Errorf("NVML Init failed: %v", nvml.ErrorString(ret))
	}
	defer func() { _ = nvml.Shutdown() }()

	// Get device count
	count, ret = nvml.DeviceGetCount()
	if ret != nvml.SUCCESS {
		return 0, "", fmt.Errorf("failed to get device count: %v", nvml.ErrorString(ret))
	}

	if count == 0 {
		return 0, "", fmt.Errorf("no GPUs found")
	}

	// Get compute capability from first GPU
	device, ret := nvml.DeviceGetHandleByIndex(0)
	if ret != nvml.SUCCESS {
		return count, "", fmt.Errorf("failed to get device 0: %v", nvml.ErrorString(ret))
	}

	major, minor, ret := device.GetCudaComputeCapability()
	if ret != nvml.SUCCESS {
		return count, "", fmt.Errorf("failed to get compute capability: %v", nvml.ErrorString(ret))
	}

	computeCapability = fmt.Sprintf("%d.%d", major, minor)
	log.Printf("[GPU-UTILS] Detected %d GPU(s) with compute capability: %s", count, computeCapability)

	return count, computeCapability, nil
}
