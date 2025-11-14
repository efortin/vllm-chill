//go:build dummy_gpu || test

package stats

import (
	"fmt"
	"log"
)

// Mock NVML structures and functions for testing without GPU hardware

var mockDevices []*mockNVMLDevice
var nvmlInitialized bool

// Mock NVML initialization
func nvmlInit() error {
	if nvmlInitialized {
		return nil
	}

	log.Println("Initializing mock NVML (no real GPU)")

	// Create mock GPU devices
	mockDevices = []*mockNVMLDevice{
		{
			index:       0,
			name:        "Mock NVIDIA GPU 0",
			uuid:        "GPU-00000000-0000-0000-0000-000000000000",
			temperature: 65,
			utilization: 75,
			memoryUsed:  8 * 1024 * 1024 * 1024,  // 8 GB
			memoryTotal: 16 * 1024 * 1024 * 1024, // 16 GB
			powerUsage:  250,
		},
		{
			index:       1,
			name:        "Mock NVIDIA GPU 1",
			uuid:        "GPU-11111111-1111-1111-1111-111111111111",
			temperature: 60,
			utilization: 50,
			memoryUsed:  4 * 1024 * 1024 * 1024,  // 4 GB
			memoryTotal: 16 * 1024 * 1024 * 1024, // 16 GB
			powerUsage:  200,
		},
	}

	nvmlInitialized = true
	return nil
}

func nvmlShutdown() error {
	if !nvmlInitialized {
		return fmt.Errorf("NVML not initialized")
	}
	log.Println("Shutting down mock NVML")
	nvmlInitialized = false
	return nil
}

func nvmlDeviceGetCount() (int, error) {
	if !nvmlInitialized {
		return 0, fmt.Errorf("NVML not initialized")
	}
	return len(mockDevices), nil
}

func nvmlDeviceGetHandleByIndex(index int) (interface{}, error) {
	if !nvmlInitialized {
		return nil, fmt.Errorf("NVML not initialized")
	}
	if index < 0 || index >= len(mockDevices) {
		return nil, fmt.Errorf("invalid device index: %d", index)
	}
	return mockDevices[index], nil
}

func nvmlDeviceGetName(device interface{}) (string, error) {
	dev, ok := device.(*mockNVMLDevice)
	if !ok {
		return "", fmt.Errorf("invalid device handle")
	}
	return dev.name, nil
}

func nvmlDeviceGetUUID(device interface{}) (string, error) {
	dev, ok := device.(*mockNVMLDevice)
	if !ok {
		return "", fmt.Errorf("invalid device handle")
	}
	return dev.uuid, nil
}

func nvmlDeviceGetTemperature(device interface{}) (uint32, error) {
	dev, ok := device.(*mockNVMLDevice)
	if !ok {
		return 0, fmt.Errorf("invalid device handle")
	}
	// Simulate slight variation
	return dev.temperature + uint32(dev.index*2), nil
}

func nvmlDeviceGetUtilizationRates(device interface{}) (uint32, uint32, error) {
	dev, ok := device.(*mockNVMLDevice)
	if !ok {
		return 0, 0, fmt.Errorf("invalid device handle")
	}
	// Return GPU and memory utilization
	gpuUtil := dev.utilization
	memUtil := uint32(float64(dev.memoryUsed) / float64(dev.memoryTotal) * 100)
	return gpuUtil, memUtil, nil
}

func nvmlDeviceGetMemoryInfo(device interface{}) (uint64, uint64, uint64, error) {
	dev, ok := device.(*mockNVMLDevice)
	if !ok {
		return 0, 0, 0, fmt.Errorf("invalid device handle")
	}
	used := dev.memoryUsed
	free := dev.memoryTotal - dev.memoryUsed
	total := dev.memoryTotal
	return used, free, total, nil
}

func nvmlDeviceGetPowerUsage(device interface{}) (uint32, error) {
	dev, ok := device.(*mockNVMLDevice)
	if !ok {
		return 0, fmt.Errorf("invalid device handle")
	}
	return dev.powerUsage, nil
}

func nvmlDeviceGetComputeMode(device interface{}) (uint32, error) {
	// NVML_COMPUTEMODE_DEFAULT = 0
	return 0, nil
}

func nvmlDeviceGetPersistenceMode(device interface{}) (uint32, error) {
	// NVML_FEATURE_ENABLED = 1
	return 1, nil
}

// Helper functions that match the real NVML API patterns

func getDeviceCount() (int, error) {
	return nvmlDeviceGetCount()
}

func getDeviceByIndex(index int) (interface{}, error) {
	return nvmlDeviceGetHandleByIndex(index)
}
