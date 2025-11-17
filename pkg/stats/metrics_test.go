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

func TestMetricsRecorder_RecordRequest(t *testing.T) {
	mr := NewMetricsRecorder()
	defer mr.Stop()

	// Test recording a successful request
	mr.RecordRequest("POST", "/v1/chat/completions", 200, 500*time.Millisecond, 1024, 2048)

	// Test recording a failed request
	mr.RecordRequest("POST", "/v1/chat/completions", 500, 100*time.Millisecond, 512, 0)

	// Test recording without payload sizes
	mr.RecordRequest("GET", "/health", 200, 10*time.Millisecond, 0, 0)
}

func TestMetricsRecorder_RecordManagedOperation(t *testing.T) {
	mr := NewMetricsRecorder()
	defer mr.Stop()

	// Test successful model switch
	mr.RecordManagedOperation("model1", "model2", true, 30*time.Second)
	assert.Equal(t, "model2", mr.currentModelName)

	// Test failed model switch
	mr.RecordManagedOperation("model2", "model3", false, 15*time.Second)
	// Current model should still be model2
	assert.Equal(t, "model2", mr.currentModelName)

	// Test switching from empty model
	mr.currentModelName = ""
	mr.RecordManagedOperation("", "model1", true, 25*time.Second)
	assert.Equal(t, "model1", mr.currentModelName)
}

func TestMetricsRecorder_RecordScaleOp(t *testing.T) {
	mr := NewMetricsRecorder()
	defer mr.Stop()

	// Test successful scale up
	mr.RecordScaleOp("up", true, 5*time.Second)

	// Test failed scale down
	mr.RecordScaleOp("down", false, 2*time.Second)
}

func TestMetricsRecorder_UpdateReplicas(t *testing.T) {
	mr := NewMetricsRecorder()
	defer mr.Stop()

	// Test updating replicas
	mr.UpdateReplicas(1)
	mr.UpdateReplicas(0)
	mr.UpdateReplicas(2)
}

func TestMetricsRecorder_IdleTimeUpdate(t *testing.T) {
	mr := NewMetricsRecorder()
	defer mr.Stop()

	// Wait a bit and check that idle time is being tracked
	time.Sleep(100 * time.Millisecond)

	mr.mu.RLock()
	idleTime := time.Since(mr.lastActivityTime)
	mr.mu.RUnlock()

	assert.True(t, idleTime > 0)

	// Update activity and verify idle time resets
	mr.UpdateActivity()

	mr.mu.RLock()
	newIdleTime := time.Since(mr.lastActivityTime)
	mr.mu.RUnlock()

	assert.True(t, newIdleTime < idleTime)
}
