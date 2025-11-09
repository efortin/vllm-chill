package stats

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewMetricsRecorder(t *testing.T) {
	mr := NewMetricsRecorder()
	assert.NotNil(t, mr)
	assert.NotEqual(t, time.Time{}, mr.lastActivityTime)
}

func TestMetricsRecorder_Stop(t *testing.T) {
	mr := NewMetricsRecorder()
	mr.Stop()
	// Just verify it doesn't panic
	assert.NotNil(t, mr)
}

func TestMetricsRecorder_UpdateActivity(t *testing.T) {
	mr := NewMetricsRecorder()

	// Store initial time
	initialTime := mr.lastActivityTime

	// Update activity
	mr.UpdateActivity()

	// Should have updated the time
	assert.NotEqual(t, initialTime, mr.lastActivityTime)
}

func TestMetricsRecorder_SetCurrentModel(t *testing.T) {
	mr := NewMetricsRecorder()

	// Test setting a model
	mr.SetCurrentModel("test-model")

	// Test changing to a different model
	mr.SetCurrentModel("another-model")
}

func TestMetricsRecorder_RecordXMLParsing(t *testing.T) {
	mr := NewMetricsRecorder()

	// Test successful parsing
	mr.RecordXMLParsing(true, 3)

	// Test failed parsing
	mr.RecordXMLParsing(false, 0)
}

func TestMetricsRecorder_RecordProxyLatency(t *testing.T) {
	mr := NewMetricsRecorder()

	// Test recording latency
	mr.RecordProxyLatency("test-operation", 150*time.Millisecond)
}

func TestMetricsRecorder_RecordVLLMStartup(t *testing.T) {
	mr := NewMetricsRecorder()

	// Test recording startup time
	mr.RecordVLLMStartup(200 * time.Millisecond)
}

func TestMetricsRecorder_RecordVLLMShutdown(t *testing.T) {
	mr := NewMetricsRecorder()

	// Test recording shutdown time
	mr.RecordVLLMShutdown(100 * time.Millisecond)
}

func TestMetricsRecorder_SetVLLMState(t *testing.T) {
	mr := NewMetricsRecorder()

	// Test setting different states
	mr.SetVLLMState(0) // stopped
	mr.SetVLLMState(1) // starting
	mr.SetVLLMState(2) // running
	mr.SetVLLMState(3) // stopping
}

func TestParseKVCacheInfo(t *testing.T) {
	// Test with valid data
	logs := `Available KV cache memory: 16.5 GiB
Block size: 128
# GPU blocks: 100
# CPU blocks: 50`

	info := ParseKVCacheInfo(logs)
	assert.Equal(t, 16.5, info.AvailableMemoryGiB)
	assert.Equal(t, 16.5*1024, info.AvailableMemoryMiB)
	assert.Equal(t, 128, info.BlockSize)
	assert.Equal(t, 100, info.NumGPUBlocks)
	assert.Equal(t, 50, info.NumCPUBlocks)

	// Test with empty logs
	emptyInfo := ParseKVCacheInfo("")
	assert.Equal(t, 0.0, emptyInfo.AvailableMemoryGiB)
	assert.Equal(t, 0, emptyInfo.BlockSize)
	assert.Equal(t, 0, emptyInfo.NumGPUBlocks)
	assert.Equal(t, 0, emptyInfo.NumCPUBlocks)
}

func TestKVCacheInfo_IsValid(t *testing.T) {
	// Test valid info with memory
	validInfo1 := &KVCacheInfo{
		AvailableMemoryGiB: 16.5,
	}
	assert.True(t, validInfo1.IsValid())

	// Test valid info with GPU blocks
	validInfo2 := &KVCacheInfo{
		NumGPUBlocks: 100,
	}
	assert.True(t, validInfo2.IsValid())

	// Test invalid info
	invalidInfo := &KVCacheInfo{}
	assert.False(t, invalidInfo.IsValid())
}
