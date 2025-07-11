package utils

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDomainRateLimiter_NewDomainRateLimiter(t *testing.T) {
	tests := []struct {
		name      string
		rateLimit time.Duration
		burst     int
		want      struct {
			rateLimit time.Duration
			burst     int
		}
	}{
		{
			name:      "should create with default settings",
			rateLimit: 5 * time.Second,
			burst:     1,
			want: struct {
				rateLimit time.Duration
				burst     int
			}{
				rateLimit: 5 * time.Second,
				burst:     1,
			},
		},
		{
			name:      "should create with custom settings",
			rateLimit: 2 * time.Second,
			burst:     3,
			want: struct {
				rateLimit time.Duration
				burst     int
			}{
				rateLimit: 2 * time.Second,
				burst:     3,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limiter := NewDomainRateLimiter(tt.rateLimit, tt.burst)

			require.NotNil(t, limiter)
			assert.Equal(t, tt.want.rateLimit, limiter.rateLimit)
			assert.Equal(t, tt.want.burst, limiter.burst)
		})
	}
}

func TestDomainRateLimiter_Wait(t *testing.T) {
	tests := []struct {
		name     string
		domain   string
		requests int
		want     struct {
			shouldWait    bool
			minWaitTime   time.Duration
			separateDomains bool
		}
	}{
		{
			name:     "should not wait for first request",
			domain:   "example.com",
			requests: 1,
			want: struct {
				shouldWait    bool
				minWaitTime   time.Duration
				separateDomains bool
			}{
				shouldWait:  false,
				minWaitTime: 0,
			},
		},
		{
			name:     "should wait for second request from same domain",
			domain:   "example.com",
			requests: 2,
			want: struct {
				shouldWait    bool
				minWaitTime   time.Duration
				separateDomains bool
			}{
				shouldWait:  true,
				minWaitTime: 5 * time.Second,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limiter := NewDomainRateLimiter(5*time.Second, 1)

			for i := 0; i < tt.requests; i++ {
				start := time.Now()
				limiter.Wait(tt.domain)
				elapsed := time.Since(start)

				if i == 0 && !tt.want.shouldWait {
					// First request should not wait
					assert.Less(t, elapsed, 100*time.Millisecond)
				} else if i > 0 && tt.want.shouldWait {
					// Subsequent requests should wait
					assert.GreaterOrEqual(t, elapsed, tt.want.minWaitTime-100*time.Millisecond)
				}
			}
		})
	}
}

func TestDomainRateLimiter_DifferentDomains(t *testing.T) {
	t.Run("should allow concurrent requests to different domains", func(t *testing.T) {
		limiter := NewDomainRateLimiter(5*time.Second, 1)

		start := time.Now()

		// First request to example.com
		limiter.Wait("example.com")
		elapsed1 := time.Since(start)

		// Immediate request to different domain should not wait
		start2 := time.Now()
		limiter.Wait("another.com")
		elapsed2 := time.Since(start2)

		assert.Less(t, elapsed1, 100*time.Millisecond)
		assert.Less(t, elapsed2, 100*time.Millisecond)
	})
}

func TestDomainRateLimiter_ExtractDomain(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{
			name: "should extract domain from simple URL",
			url:  "https://example.com/path",
			want: "example.com",
		},
		{
			name: "should extract domain from URL with port",
			url:  "http://example.com:8080/path",
			want: "example.com",
		},
		{
			name: "should extract subdomain",
			url:  "https://api.example.com/v1/data",
			want: "api.example.com",
		},
		{
			name: "should handle malformed URL",
			url:  "not-a-url",
			want: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractDomain(tt.url)
			assert.Equal(t, tt.want, result)
		})
	}
}