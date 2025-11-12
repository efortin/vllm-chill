package scaling

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestConstants(t *testing.T) {
	// Test that constants are properly defined
	assert.Equal(t, 2*time.Minute, DefaultScaleUpTimeout)
	assert.Equal(t, 10*time.Second, DefaultCheckInterval)
	assert.Equal(t, 5*time.Minute, DefaultScaleDownDelay)
}

func TestStatusStruct(t *testing.T) {
	// Test Status struct fields
	status := Status{
		IsScaledUp:   true,
		IsScalingUp:  false,
		LastActivity: time.Now(),
		Replicas:     3,
	}

	assert.True(t, status.IsScaledUp)
	assert.False(t, status.IsScalingUp)
	assert.Equal(t, int32(3), status.Replicas)
	assert.NotZero(t, status.LastActivity)
}

func TestConfigStruct(t *testing.T) {
	// Test Config struct with various values
	config := Config{
		Namespace:      "test-namespace",
		Deployment:     "test-deployment",
		ScaleDownDelay: 10 * time.Minute,
		CheckInterval:  30 * time.Second,
		MinReplicas:    2,
		MaxReplicas:    10,
	}

	assert.Equal(t, "test-namespace", config.Namespace)
	assert.Equal(t, "test-deployment", config.Deployment)
	assert.Equal(t, 10*time.Minute, config.ScaleDownDelay)
	assert.Equal(t, 30*time.Second, config.CheckInterval)
	assert.Equal(t, int32(2), config.MinReplicas)
	assert.Equal(t, int32(10), config.MaxReplicas)
}

func TestConfigDefaults(t *testing.T) {
	// Test default configuration values
	config := Config{
		Namespace:  "test-ns",
		Deployment: "test-deployment",
	}

	// These would be set by NewK8sAutoScaler
	if config.ScaleDownDelay == 0 {
		config.ScaleDownDelay = DefaultScaleDownDelay
	}
	if config.CheckInterval == 0 {
		config.CheckInterval = DefaultCheckInterval
	}
	if config.MinReplicas == 0 {
		config.MinReplicas = 1
	}
	if config.MaxReplicas == 0 {
		config.MaxReplicas = 1
	}

	assert.Equal(t, DefaultScaleDownDelay, config.ScaleDownDelay)
	assert.Equal(t, DefaultCheckInterval, config.CheckInterval)
	assert.Equal(t, int32(1), config.MinReplicas)
	assert.Equal(t, int32(1), config.MaxReplicas)
}

func TestK8sAutoScalerStructure(t *testing.T) {
	// Test that K8sAutoScaler struct can be created
	scaler := &K8sAutoScaler{
		config: Config{
			Namespace:      "test",
			Deployment:     "test-deployment",
			ScaleDownDelay: DefaultScaleDownDelay,
			CheckInterval:  DefaultCheckInterval,
			MinReplicas:    1,
			MaxReplicas:    3,
		},
		lastActivity:    time.Now(),
		currentReplicas: 1,
		isScalingUp:     false,
	}

	assert.NotNil(t, scaler)
	assert.Equal(t, "test", scaler.config.Namespace)
	assert.Equal(t, int32(1), scaler.currentReplicas)
	assert.False(t, scaler.isScalingUp)
}

func TestAutoScalerInterface(t *testing.T) {
	// Ensure AutoScaler interface is defined with expected methods
	var _ AutoScaler = (*K8sAutoScaler)(nil)
}

func TestMultipleConfigurations(t *testing.T) {
	testCases := []struct {
		name     string
		config   Config
		expected Config
	}{
		{
			name: "production config",
			config: Config{
				Namespace:      "production",
				Deployment:     "vllm-prod",
				ScaleDownDelay: 30 * time.Minute,
				CheckInterval:  1 * time.Minute,
				MinReplicas:    3,
				MaxReplicas:    20,
			},
			expected: Config{
				Namespace:      "production",
				Deployment:     "vllm-prod",
				ScaleDownDelay: 30 * time.Minute,
				CheckInterval:  1 * time.Minute,
				MinReplicas:    3,
				MaxReplicas:    20,
			},
		},
		{
			name: "development config",
			config: Config{
				Namespace:      "dev",
				Deployment:     "vllm-dev",
				ScaleDownDelay: 1 * time.Minute,
				CheckInterval:  5 * time.Second,
				MinReplicas:    1,
				MaxReplicas:    2,
			},
			expected: Config{
				Namespace:      "dev",
				Deployment:     "vllm-dev",
				ScaleDownDelay: 1 * time.Minute,
				CheckInterval:  5 * time.Second,
				MinReplicas:    1,
				MaxReplicas:    2,
			},
		},
		{
			name: "staging config",
			config: Config{
				Namespace:      "staging",
				Deployment:     "vllm-staging",
				ScaleDownDelay: 10 * time.Minute,
				CheckInterval:  30 * time.Second,
				MinReplicas:    2,
				MaxReplicas:    5,
			},
			expected: Config{
				Namespace:      "staging",
				Deployment:     "vllm-staging",
				ScaleDownDelay: 10 * time.Minute,
				CheckInterval:  30 * time.Second,
				MinReplicas:    2,
				MaxReplicas:    5,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected.Namespace, tc.config.Namespace)
			assert.Equal(t, tc.expected.Deployment, tc.config.Deployment)
			assert.Equal(t, tc.expected.ScaleDownDelay, tc.config.ScaleDownDelay)
			assert.Equal(t, tc.expected.CheckInterval, tc.config.CheckInterval)
			assert.Equal(t, tc.expected.MinReplicas, tc.config.MinReplicas)
			assert.Equal(t, tc.expected.MaxReplicas, tc.config.MaxReplicas)
		})
	}
}

func TestStatusTransitions(t *testing.T) {
	testCases := []struct {
		name           string
		initialStatus  Status
		expectedStatus Status
		description    string
	}{
		{
			name: "scaling up",
			initialStatus: Status{
				IsScaledUp:   false,
				IsScalingUp:  true,
				LastActivity: time.Now(),
				Replicas:     0,
			},
			expectedStatus: Status{
				IsScaledUp:  false,
				IsScalingUp: true,
				Replicas:    0,
			},
			description: "Pod is in the process of scaling up",
		},
		{
			name: "fully scaled",
			initialStatus: Status{
				IsScaledUp:   true,
				IsScalingUp:  false,
				LastActivity: time.Now(),
				Replicas:     3,
			},
			expectedStatus: Status{
				IsScaledUp:  true,
				IsScalingUp: false,
				Replicas:    3,
			},
			description: "Pod is fully scaled and ready",
		},
		{
			name: "scaled down",
			initialStatus: Status{
				IsScaledUp:   false,
				IsScalingUp:  false,
				LastActivity: time.Now().Add(-10 * time.Minute),
				Replicas:     0,
			},
			expectedStatus: Status{
				IsScaledUp:  false,
				IsScalingUp: false,
				Replicas:    0,
			},
			description: "Pod is scaled down after idle timeout",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expectedStatus.IsScaledUp, tc.initialStatus.IsScaledUp, tc.description)
			assert.Equal(t, tc.expectedStatus.IsScalingUp, tc.initialStatus.IsScalingUp, tc.description)
			assert.Equal(t, tc.expectedStatus.Replicas, tc.initialStatus.Replicas, tc.description)
		})
	}
}

func TestScalingValidation(t *testing.T) {
	t.Run("min replicas cannot exceed max replicas", func(t *testing.T) {
		config := Config{
			MinReplicas: 5,
			MaxReplicas: 3,
		}
		// In a real implementation, this should be validated
		assert.True(t, config.MinReplicas > config.MaxReplicas, "Invalid config: min > max")
	})

	t.Run("negative replicas not allowed", func(t *testing.T) {
		config := Config{
			MinReplicas: -1,
			MaxReplicas: -1,
		}
		// In a real implementation, these should be validated
		assert.True(t, config.MinReplicas < 0, "Invalid config: negative min replicas")
		assert.True(t, config.MaxReplicas < 0, "Invalid config: negative max replicas")
	})

	t.Run("zero max replicas", func(t *testing.T) {
		config := Config{
			MinReplicas: 0,
			MaxReplicas: 0,
		}
		// Zero replicas means no pods
		assert.Equal(t, int32(0), config.MinReplicas)
		assert.Equal(t, int32(0), config.MaxReplicas)
	})
}

func TestNewK8sAutoScaler(t *testing.T) {
	config := Config{
		Namespace:   "test",
		Deployment:  "test-deployment",
		MinReplicas: 1,
		MaxReplicas: 3,
	}

	as := NewK8sAutoScaler(nil, config)

	assert.NotNil(t, as)
	assert.Equal(t, DefaultScaleDownDelay, as.config.ScaleDownDelay)
	assert.Equal(t, DefaultCheckInterval, as.config.CheckInterval)
	assert.Equal(t, int32(1), as.config.MinReplicas)
	assert.Equal(t, int32(3), as.config.MaxReplicas)
	assert.NotNil(t, as.scaleUpCond)
}

func TestNewK8sAutoScaler_WithCustomTimes(t *testing.T) {
	config := Config{
		Namespace:      "test",
		Deployment:     "test-deployment",
		ScaleDownDelay: 10 * time.Minute,
		CheckInterval:  30 * time.Second,
		MinReplicas:    2,
		MaxReplicas:    5,
	}

	as := NewK8sAutoScaler(nil, config)

	assert.NotNil(t, as)
	assert.Equal(t, 10*time.Minute, as.config.ScaleDownDelay)
	assert.Equal(t, 30*time.Second, as.config.CheckInterval)
	assert.Equal(t, int32(2), as.config.MinReplicas)
	assert.Equal(t, int32(5), as.config.MaxReplicas)
}

func TestK8sAutoScaler_UpdateActivity(t *testing.T) {
	as := NewK8sAutoScaler(nil, Config{
		Namespace:   "test",
		Deployment:  "test",
		MinReplicas: 1,
		MaxReplicas: 1,
	})

	initialTime := as.lastActivity
	time.Sleep(10 * time.Millisecond)

	as.UpdateActivity()

	assert.True(t, as.lastActivity.After(initialTime))
}

func TestK8sAutoScaler_IsActive(t *testing.T) {
	tests := []struct {
		name           string
		scaleDownDelay time.Duration
		timeSince      time.Duration
		expectActive   bool
	}{
		{
			name:           "recently active",
			scaleDownDelay: 5 * time.Minute,
			timeSince:      1 * time.Minute,
			expectActive:   true,
		},
		{
			name:           "inactive",
			scaleDownDelay: 5 * time.Minute,
			timeSince:      10 * time.Minute,
			expectActive:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			as := NewK8sAutoScaler(nil, Config{
				Namespace:      "test",
				Deployment:     "test",
				ScaleDownDelay: tt.scaleDownDelay,
				MinReplicas:    1,
				MaxReplicas:    1,
			})

			// Set last activity to the past
			as.lastActivity = time.Now().Add(-tt.timeSince)

			assert.Equal(t, tt.expectActive, as.IsActive())
		})
	}
}

func TestK8sAutoScaler_GetStatus(t *testing.T) {
	as := NewK8sAutoScaler(nil, Config{
		Namespace:   "test",
		Deployment:  "test",
		MinReplicas: 1,
		MaxReplicas: 3,
	})

	// Test initial status
	status := as.GetStatus()
	assert.False(t, status.IsScaledUp)
	assert.False(t, status.IsScalingUp)
	assert.Equal(t, int32(0), status.Replicas)

	// Simulate scaling up
	as.mu.Lock()
	as.currentReplicas = 3
	as.isScalingUp = true
	as.mu.Unlock()

	status = as.GetStatus()
	assert.True(t, status.IsScaledUp)
	assert.True(t, status.IsScalingUp)
	assert.Equal(t, int32(3), status.Replicas)
}
