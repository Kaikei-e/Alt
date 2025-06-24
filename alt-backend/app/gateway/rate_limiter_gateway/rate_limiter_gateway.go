package rate_limiter_gateway

import (
	"alt/utils/rate_limiter"
	"context"
	"fmt"
	"net/url"
	"time"
)

// RateLimiterGateway implements the RateLimiterPort interface
type RateLimiterGateway struct {
	hostLimiter *rate_limiter.HostRateLimiter
}

// NewRateLimiterGateway creates a new rate limiter gateway
func NewRateLimiterGateway(hostLimiter *rate_limiter.HostRateLimiter) *RateLimiterGateway {
	return &RateLimiterGateway{
		hostLimiter: hostLimiter,
	}
}

// WaitForHost blocks until the rate limit allows a request to the given host
func (r *RateLimiterGateway) WaitForHost(ctx context.Context, host string) error {
	if host == "" {
		return &url.Error{Op: "validate", URL: host, Err: fmt.Errorf("host cannot be empty")}
	}
	
	// Create a mock URL to use the existing WaitForHost method
	mockURL := "https://" + host
	return r.hostLimiter.WaitForHost(ctx, mockURL)
}

// WaitForURL blocks until the rate limit allows a request to the given URL
func (r *RateLimiterGateway) WaitForURL(ctx context.Context, urlStr string) error {
	return r.hostLimiter.WaitForHost(ctx, urlStr)
}

// GetRemainingRequests returns the number of requests available for the host
func (r *RateLimiterGateway) GetRemainingRequests(host string) int {
	// The current rate limiter implementation doesn't expose remaining requests
	// This is a simplified implementation - in a real scenario, you'd need to
	// extend the rate_limiter.HostRateLimiter to expose this information
	return 1 // Default to 1 available request
}

// GetNextAvailableTime returns when the next request can be made to the host
func (r *RateLimiterGateway) GetNextAvailableTime(host string) time.Time {
	// The current rate limiter implementation doesn't expose next available time
	// This is a simplified implementation - in a real scenario, you'd need to
	// extend the rate_limiter.HostRateLimiter to expose this information
	return time.Now() // Default to now
}