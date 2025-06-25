package integration_tests

import (
	"alt/utils/rate_limiter"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRateLimitingIntegration(t *testing.T) {
	tests := []struct {
		name             string
		requests         int
		interval         time.Duration
		expectedDuration time.Duration
		tolerancePercent float64
	}{
		{
			name:             "respects 5 second intervals",
			requests:         3,
			interval:         5 * time.Second,
			expectedDuration: 10 * time.Second, // 2 intervals for 3 requests
			tolerancePercent: 10.0,
		},
		{
			name:             "handles concurrent requests to same host",
			requests:         5,
			interval:         5 * time.Second,
			expectedDuration: 20 * time.Second, // 4 intervals for 5 requests
			tolerancePercent: 15.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`<?xml version="1.0"?>
<rss version="2.0">
    <channel>
        <title>Test Feed</title>
        <item><title>Test Item</title></item>
    </channel>
</rss>`))
			}))
			defer server.Close()

			// Create rate limiter
			rateLimiter := rate_limiter.NewHostRateLimiter(tt.interval)

			// Measure time for multiple requests
			start := time.Now()

			for i := 0; i < tt.requests; i++ {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				err := rateLimiter.WaitForHost(ctx, server.URL)
				cancel()

				require.NoError(t, err, "Rate limiter should not error")

				// Make actual HTTP request to verify it works
				resp, err := http.Get(server.URL)
				require.NoError(t, err, "HTTP request should succeed")
				resp.Body.Close()
			}

			actualDuration := time.Since(start)

			// Verify timing is within tolerance
			tolerance := time.Duration(float64(tt.expectedDuration) * tt.tolerancePercent / 100.0)
			minDuration := tt.expectedDuration - tolerance
			maxDuration := tt.expectedDuration + tolerance

			assert.True(t, actualDuration >= minDuration,
				"Duration %v should be at least %v", actualDuration, minDuration)
			assert.True(t, actualDuration <= maxDuration,
				"Duration %v should be at most %v", actualDuration, maxDuration)
		})
	}
}

func TestRateLimitingDifferentHosts(t *testing.T) {
	// Test that different hosts don't interfere with each other's rate limits
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server1.Close()

	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server2.Close()

	rateLimiter := rate_limiter.NewHostRateLimiter(5 * time.Second)

	start := time.Now()

	// These should execute immediately since they're different hosts
	ctx1, cancel1 := context.WithTimeout(context.Background(), 1*time.Second)
	err1 := rateLimiter.WaitForHost(ctx1, server1.URL)
	cancel1()

	ctx2, cancel2 := context.WithTimeout(context.Background(), 1*time.Second)
	err2 := rateLimiter.WaitForHost(ctx2, server2.URL)
	cancel2()

	duration := time.Since(start)

	require.NoError(t, err1)
	require.NoError(t, err2)
	assert.Less(t, duration, 2*time.Second, "Different hosts should not block each other")
}

func TestRateLimitingErrorHandling(t *testing.T) {
	rateLimiter := rate_limiter.NewHostRateLimiter(1 * time.Second)

	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{
			name:    "invalid URL returns error",
			url:     "not-a-url",
			wantErr: true,
		},
		{
			name:    "empty URL returns error",
			url:     "",
			wantErr: true,
		},
		{
			name:    "URL without host returns error",
			url:     "http://",
			wantErr: true,
		},
		{
			name:    "valid URL succeeds",
			url:     "http://example.com",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			defer cancel()

			err := rateLimiter.WaitForHost(ctx, tt.url)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestRateLimitingContextCancellation(t *testing.T) {
	rateLimiter := rate_limiter.NewHostRateLimiter(10 * time.Second)

	// Make first request to initialize rate limiter for host
	ctx1, cancel1 := context.WithTimeout(context.Background(), 1*time.Second)
	err1 := rateLimiter.WaitForHost(ctx1, "http://example.com")
	cancel1()
	require.NoError(t, err1)

	// Second request should be rate limited and should respect context cancellation
	ctx2, cancel2 := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel2()

	start := time.Now()
	err2 := rateLimiter.WaitForHost(ctx2, "http://example.com")
	duration := time.Since(start)

	assert.Error(t, err2, "Context should have been cancelled due to timeout")
	assert.Less(t, duration, 1*time.Second, "Should have returned quickly due to context cancellation")
}