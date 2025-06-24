package rate_limiter_port

import (
	"context"
	"time"
)

//go:generate go run go.uber.org/mock/mockgen -source=rate_limiter_port.go -destination=../../mocks/mock_rate_limiter_port.go

// RateLimiterPort defines the interface for rate limiting operations
type RateLimiterPort interface {
	// WaitForHost blocks until the rate limit allows a request to the given host
	WaitForHost(ctx context.Context, host string) error
	
	// WaitForURL blocks until the rate limit allows a request to the given URL
	WaitForURL(ctx context.Context, url string) error
	
	// GetRemainingRequests returns the number of requests available for the host
	GetRemainingRequests(host string) int
	
	// GetNextAvailableTime returns when the next request can be made to the host
	GetNextAvailableTime(host string) time.Time
}

// RateLimiterConfig holds configuration for the rate limiter
type RateLimiterConfig struct {
	DefaultInterval time.Duration
	BurstSize       int
	PerHostLimit    bool
}