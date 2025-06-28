// ABOUTME: This file contains comprehensive tests for enhanced rate limiter implementation
// ABOUTME: Tests rate limiting with exponential backoff, jitter, and circuit breaker integration
package utils

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRateLimitedHTTPClient_BasicRateLimit(t *testing.T) {
	tests := map[string]struct {
		interval        time.Duration
		requestCount    int
		expectedMinTime time.Duration
	}{
		"should enforce 5 second minimum interval": {
			interval:        5 * time.Second,
			requestCount:    3,
			expectedMinTime: 10 * time.Second, // 2 intervals for 3 requests
		},
		"should enforce custom interval": {
			interval:        2 * time.Second,
			requestCount:    2,
			expectedMinTime: 2 * time.Second, // 1 interval for 2 requests
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// RED PHASE: Test fails because RateLimitedHTTPClient doesn't exist yet
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("OK"))
			}))
			defer server.Close()

			client := NewRateLimitedHTTPClient(tc.interval, 3, 10*time.Second)

			start := time.Now()

			for i := 0; i < tc.requestCount; i++ {
				resp, err := client.Get(server.URL)
				require.NoError(t, err)
				assert.Equal(t, http.StatusOK, resp.StatusCode)
				resp.Body.Close()
			}

			elapsed := time.Since(start)
			assert.GreaterOrEqual(t, elapsed, tc.expectedMinTime)
		})
	}
}

func TestRateLimitedHTTPClient_ExponentialBackoff(t *testing.T) {
	tests := map[string]struct {
		serverErrors  int
		expectBackoff bool
		maxRetries    int
	}{
		"should apply exponential backoff on failures": {
			serverErrors:  2,
			expectBackoff: true,
			maxRetries:    3,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// RED PHASE: Test fails because RateLimitedHTTPClient doesn't exist yet
			errorCount := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if errorCount < tc.serverErrors {
					errorCount++
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			client := NewRateLimitedHTTPClient(100*time.Millisecond, tc.maxRetries, 5*time.Second)

			start := time.Now()
			resp, err := client.GetWithRetry(context.Background(), server.URL)
			elapsed := time.Since(start)

			require.NoError(t, err)
			assert.Equal(t, http.StatusOK, resp.StatusCode)
			resp.Body.Close()

			if tc.expectBackoff {
				// Should take longer due to exponential backoff
				assert.Greater(t, elapsed, 200*time.Millisecond)
			}
		})
	}
}

func TestRateLimitedHTTPClient_CircuitBreakerIntegration(t *testing.T) {
	tests := map[string]struct {
		failureThreshold  int
		consecutiveErrors int
		expectCircuitOpen bool
	}{
		"should open circuit after threshold failures": {
			failureThreshold:  2,
			consecutiveErrors: 2,
			expectCircuitOpen: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// RED PHASE: Test fails because RateLimitedHTTPClient doesn't exist yet
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer server.Close()

			client := NewRateLimitedHTTPClientWithCircuitBreaker(
				100*time.Millisecond,
				tc.failureThreshold,
				5*time.Second,
				tc.failureThreshold,
				time.Second,
			)

			// Make requests to trigger circuit breaker
			for i := 0; i < tc.consecutiveErrors; i++ {
				client.Get(server.URL)
			}

			// Next request should fail due to open circuit
			_, err := client.Get(server.URL)

			if tc.expectCircuitOpen {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "circuit breaker open")
			}
		})
	}
}

func TestRateLimitedHTTPClient_Jitter(t *testing.T) {
	t.Run("should apply jitter to reduce thundering herd", func(t *testing.T) {
		// RED PHASE: Test fails because RateLimitedHTTPClient doesn't exist yet
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client := NewRateLimitedHTTPClient(1*time.Second, 3, 5*time.Second)

		// Make multiple requests and measure intervals
		var intervals []time.Duration
		lastTime := time.Now()

		for i := 0; i < 3; i++ {
			if i > 0 {
				client.Wait() // Wait for rate limit
				current := time.Now()
				intervals = append(intervals, current.Sub(lastTime))
				lastTime = current
			}

			resp, err := client.Get(server.URL)
			require.NoError(t, err)
			resp.Body.Close()
		}

		// Intervals should have some variance due to jitter
		if len(intervals) > 1 {
			assert.NotEqual(t, intervals[0], intervals[1])
		}
	})
}

func TestRateLimitedHTTPClient_ContextCancellation(t *testing.T) {
	t.Run("should respect context cancellation", func(t *testing.T) {
		// RED PHASE: Test fails because RateLimitedHTTPClient doesn't exist yet
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(100 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client := NewRateLimitedHTTPClient(100*time.Millisecond, 3, 5*time.Second)

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		_, err := client.GetWithContext(ctx, server.URL)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "context deadline exceeded")
	})
}

func TestRateLimitedHTTPClient_UserAgent(t *testing.T) {
	t.Run("should set proper User-Agent header", func(t *testing.T) {
		// RED PHASE: Test fails because RateLimitedHTTPClient doesn't exist yet
		var userAgent string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userAgent = r.Header.Get("User-Agent")
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client := NewRateLimitedHTTPClient(100*time.Millisecond, 3, 5*time.Second)

		resp, err := client.Get(server.URL)
		require.NoError(t, err)
		resp.Body.Close()

		assert.Contains(t, userAgent, "pre-processor")
		assert.Contains(t, userAgent, "alt.example.com")
	})
}

func TestRateLimitedHTTPClient_Timeout(t *testing.T) {
	t.Run("should enforce request timeout", func(t *testing.T) {
		// RED PHASE: Test fails because RateLimitedHTTPClient doesn't exist yet
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(2 * time.Second) // Longer than timeout
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client := NewRateLimitedHTTPClient(100*time.Millisecond, 3, 1*time.Second)

		start := time.Now()
		_, err := client.Get(server.URL)
		elapsed := time.Since(start)

		require.Error(t, err)
		assert.Less(t, elapsed, 1500*time.Millisecond) // Should timeout before 1.5s
	})
}

func TestRateLimitedHTTPClient_Metrics(t *testing.T) {
	t.Run("should track request metrics", func(t *testing.T) {
		// RED PHASE: Test fails because RateLimitedHTTPClient doesn't exist yet
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client := NewRateLimitedHTTPClient(100*time.Millisecond, 3, 5*time.Second)

		// Make some requests
		for i := 0; i < 3; i++ {
			resp, err := client.Get(server.URL)
			require.NoError(t, err)
			resp.Body.Close()
		}

		metrics := client.Metrics()
		assert.Equal(t, int64(3), metrics.TotalRequests)
		assert.Equal(t, int64(3), metrics.SuccessfulRequests)
		assert.Equal(t, int64(0), metrics.FailedRequests)
	})
}

func BenchmarkRateLimitedHTTPClient_Get(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewRateLimitedHTTPClient(1*time.Millisecond, 3, 5*time.Second)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resp, err := client.Get(server.URL)
		if err == nil {
			resp.Body.Close()
		}
	}
}
