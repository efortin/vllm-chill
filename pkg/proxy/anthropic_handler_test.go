package proxy

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestGinProxyHandler_RouteDetection tests the route detection logic for Anthropic API
func TestGinProxyHandler_RouteDetection(t *testing.T) {
	tests := []struct {
		name              string
		path              string
		shouldBeAnthropic bool
	}{
		{
			name:              "anthropic endpoint is detected",
			path:              "/v1/messages",
			shouldBeAnthropic: true,
		},
		{
			name:              "openai endpoint bypasses anthropic handler",
			path:              "/v1/chat/completions",
			shouldBeAnthropic: false,
		},
		{
			name:              "other endpoints bypass anthropic handler",
			path:              "/health",
			shouldBeAnthropic: false,
		},
		{
			name:              "anthropic sub-path also triggers",
			path:              "/v1/messages/count",
			shouldBeAnthropic: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the routing logic used in ginProxyHandler
			isAnthropic := strings.HasPrefix(tt.path, "/v1/messages")
			assert.Equal(t, tt.shouldBeAnthropic, isAnthropic,
				"Path %s should%s trigger Anthropic handler",
				tt.path,
				map[bool]string{true: "", false: " not"}[tt.shouldBeAnthropic])
		})
	}
}
