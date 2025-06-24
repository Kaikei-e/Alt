package rate_limiter_port

import (
	"context"
	"testing"
	"time"
)

// TestRateLimiterPortInterface verifies the interface is properly defined
func TestRateLimiterPortInterface(t *testing.T) {
	// This test ensures the RateLimiterPort interface compiles correctly
	// Actual implementation testing will be done in the gateway layer
	var _ RateLimiterPort = (*mockRateLimiterPort)(nil)
}

// mockRateLimiterPort is a simple mock to verify interface compliance
type mockRateLimiterPort struct{}

func (m *mockRateLimiterPort) WaitForHost(ctx context.Context, host string) error {
	return nil
}

func (m *mockRateLimiterPort) WaitForURL(ctx context.Context, url string) error {
	return nil
}

func (m *mockRateLimiterPort) GetRemainingRequests(host string) int {
	return 0
}

func (m *mockRateLimiterPort) GetNextAvailableTime(host string) time.Time {
	return time.Now()
}