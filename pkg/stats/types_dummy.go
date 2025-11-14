//go:build dummy_gpu || test

package stats

// mockNVMLDevice represents a mock GPU device for testing without hardware
type mockNVMLDevice struct {
	index       int
	name        string
	uuid        string
	temperature uint32
	utilization uint32
	memoryUsed  uint64
	memoryTotal uint64
	powerUsage  uint32
}
