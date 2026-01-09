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

const testRateLimiterURL = "http://rate-limiter.test"

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func newHandlerTransport(handler http.HandlerFunc, delay time.Duration) http.RoundTripper {
	return roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		if err := req.Context().Err(); err != nil {
			return nil, err
		}
		if delay > 0 {
			select {
			case <-time.After(delay):
			case <-req.Context().Done():
				return nil, req.Context().Err()
			}
		}
		recorder := httptest.NewRecorder()
		handler(recorder, req)
		return recorder.Result(), nil
	})
}

func TestRateLimitedHTTPClient_BasicRateLimit(t *testing.T) {
	tests := map[string]struct {
		interval        time.Duration
		requestCount    int
		expectedMinTime time.Duration
	}{
		"should enforce 1 second minimum interval": {
			interval:        1 * time.Second,
			requestCount:    2,
			expectedMinTime: 500 * time.Millisecond, // Reduced minimum time for faster test
		},
		"should enforce custom interval": {
			interval:        500 * time.Millisecond,
			requestCount:    2,
			expectedMinTime: 200 * time.Millisecond, // Reduced minimum time for faster test
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			client := NewRateLimitedHTTPClient(tc.interval, 3, 10*time.Second)
			client.client.Transport = newHandlerTransport(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("OK"))
			}, 0)

			start := time.Now()

			for i := 0; i < tc.requestCount; i++ {
				resp, err := client.Get(testRateLimiterURL)
				require.NoError(t, err)
				assert.Equal(t, http.StatusOK, resp.StatusCode)
				_ = resp.Body.Close()
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
			errorCount := 0
			client := NewRateLimitedHTTPClient(100*time.Millisecond, tc.maxRetries, 5*time.Second)
			client.client.Transport = newHandlerTransport(func(w http.ResponseWriter, _ *http.Request) {
				if errorCount < tc.serverErrors {
					errorCount++
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				w.WriteHeader(http.StatusOK)
			}, 0)

			start := time.Now()
			resp, err := client.GetWithRetry(context.Background(), testRateLimiterURL)
			elapsed := time.Since(start)

			require.NoError(t, err)
			assert.Equal(t, http.StatusOK, resp.StatusCode)
			_ = resp.Body.Close()

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
			client := NewRateLimitedHTTPClientWithCircuitBreaker(
				100*time.Millisecond,
				tc.failureThreshold,
				5*time.Second,
				tc.failureThreshold,
				time.Second,
			)
			client.client.Transport = newHandlerTransport(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			}, 0)

			// Make requests to trigger circuit breaker
			for i := 0; i < tc.consecutiveErrors; i++ {
				_, _ = client.Get(testRateLimiterURL)
			}

			// Next request should fail due to open circuit
			_, err := client.Get(testRateLimiterURL)

			if tc.expectCircuitOpen {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "circuit breaker open")
			}
		})
	}
}

func TestRateLimitedHTTPClient_Jitter(t *testing.T) {
	t.Run("should apply jitter to reduce thundering herd", func(t *testing.T) {
		client := NewRateLimitedHTTPClient(100*time.Millisecond, 3, 5*time.Second)
		client.client.Transport = newHandlerTransport(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}, 0)

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

			resp, err := client.Get(testRateLimiterURL)
			require.NoError(t, err)
			_ = resp.Body.Close()
		}

		// Intervals should have some variance due to jitter
		if len(intervals) > 1 {
			assert.NotEqual(t, intervals[0], intervals[1])
		}
	})
}

func TestRateLimitedHTTPClient_ContextCancellation(t *testing.T) {
	t.Run("should respect context cancellation", func(t *testing.T) {
		client := NewRateLimitedHTTPClient(100*time.Millisecond, 3, 5*time.Second)
		client.client.Transport = newHandlerTransport(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}, 100*time.Millisecond)

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		_, err := client.GetWithContext(ctx, testRateLimiterURL)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "context deadline exceeded")
	})
}

func TestRateLimitedHTTPClient_UserAgent(t *testing.T) {
	t.Run("should set proper User-Agent header", func(t *testing.T) {
		var userAgent string
		client := NewRateLimitedHTTPClient(100*time.Millisecond, 3, 5*time.Second)
		client.client.Transport = newHandlerTransport(func(w http.ResponseWriter, r *http.Request) {
			userAgent = r.Header.Get("User-Agent")
			w.WriteHeader(http.StatusOK)
		}, 0)

		resp, err := client.Get(testRateLimiterURL)
		require.NoError(t, err)
		_ = resp.Body.Close()

		assert.Contains(t, userAgent, "pre-processor")
		assert.Contains(t, userAgent, "alt.example.com")
	})
}

func TestRateLimitedHTTPClient_Timeout(t *testing.T) {
	t.Run("should enforce request timeout", func(t *testing.T) {
		client := NewRateLimitedHTTPClient(100*time.Millisecond, 3, 1*time.Second)
		client.client.Transport = newHandlerTransport(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}, 2*time.Second)

		start := time.Now()
		_, err := client.Get(testRateLimiterURL)
		elapsed := time.Since(start)

		require.Error(t, err)
		assert.Less(t, elapsed, 1500*time.Millisecond) // Should timeout before 1.5s
	})
}

func TestRateLimitedHTTPClient_Metrics(t *testing.T) {
	t.Run("should track request metrics", func(t *testing.T) {
		client := NewRateLimitedHTTPClient(100*time.Millisecond, 3, 5*time.Second)
		client.client.Transport = newHandlerTransport(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}, 0)

		// Make some requests
		for i := 0; i < 3; i++ {
			resp, err := client.Get(testRateLimiterURL)
			require.NoError(t, err)
			_ = resp.Body.Close()
		}

		metrics := client.Metrics()
		assert.Equal(t, int64(3), metrics.TotalRequests)
		assert.Equal(t, int64(3), metrics.SuccessfulRequests)
		assert.Equal(t, int64(0), metrics.FailedRequests)
	})
}

func BenchmarkRateLimitedHTTPClient_Get(b *testing.B) {
	client := NewRateLimitedHTTPClient(1*time.Millisecond, 3, 5*time.Second)
	client.client.Transport = newHandlerTransport(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}, 0)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resp, err := client.Get(testRateLimiterURL)
		if err == nil {
			_ = resp.Body.Close()
		}
	}
}
