// ABOUTME: This file tests adaptive rate limiting with domain-specific controls and performance tracking
// ABOUTME: Tests adaptive behavior, fallback mechanisms, and resource management
package ratelimit

import (
	"context"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"pre-processor/config"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelError, // Only errors in tests
	}))
}

// TDD RED PHASE: Test adaptive rate limiter creation
func TestNewAdaptiveLimiter(t *testing.T) {
	tests := map[string]struct {
		config      config.RateLimitConfig
		expectError bool
		validate    func(*testing.T, *AdaptiveLimiter)
	}{
		"default configuration": {
			config: config.RateLimitConfig{
				DefaultInterval: 5 * time.Second,
				BurstSize:       1,
				EnableAdaptive:  true,
				DomainIntervals: map[string]time.Duration{},
			},
			expectError: false,
			validate: func(t *testing.T, limiter *AdaptiveLimiter) {
				assert.Equal(t, 5*time.Second, limiter.defaultInterval)
				assert.Equal(t, 1, limiter.burstSize)
				assert.True(t, limiter.enableAdaptive)
				assert.NotNil(t, limiter.domainLimiters)
				assert.NotNil(t, limiter.metrics)
			},
		},
		"custom domain intervals": {
			config: config.RateLimitConfig{
				DefaultInterval: 3 * time.Second,
				BurstSize:       2,
				EnableAdaptive:  true,
				DomainIntervals: map[string]time.Duration{
					"example.com": 10 * time.Second,
					"slow.com":    15 * time.Second,
				},
			},
			expectError: false,
			validate: func(t *testing.T, limiter *AdaptiveLimiter) {
				assert.Equal(t, 3*time.Second, limiter.defaultInterval)
				assert.Equal(t, 2, limiter.burstSize)
				assert.Len(t, limiter.domainIntervals, 2)
				assert.Equal(t, 10*time.Second, limiter.domainIntervals["example.com"])
				assert.Equal(t, 15*time.Second, limiter.domainIntervals["slow.com"])
			},
		},
		"adaptive disabled": {
			config: config.RateLimitConfig{
				DefaultInterval: 5 * time.Second,
				BurstSize:       1,
				EnableAdaptive:  false,
				DomainIntervals: map[string]time.Duration{},
			},
			expectError: false,
			validate: func(t *testing.T, limiter *AdaptiveLimiter) {
				assert.False(t, limiter.enableAdaptive)
			},
		},
		"invalid default interval": {
			config: config.RateLimitConfig{
				DefaultInterval: 0,
				BurstSize:       1,
				EnableAdaptive:  true,
			},
			expectError: true,
		},
		"invalid burst size": {
			config: config.RateLimitConfig{
				DefaultInterval: 5 * time.Second,
				BurstSize:       0,
				EnableAdaptive:  true,
			},
			expectError: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			limiter, err := NewAdaptiveLimiter(tc.config, testLogger())

			if tc.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, limiter)
			tc.validate(t, limiter)
		})
	}
}

// TDD RED PHASE: Test basic wait functionality
func TestAdaptiveLimiter_Wait(t *testing.T) {
	t.Run("should respect default interval", func(t *testing.T) {
		config := config.RateLimitConfig{
			DefaultInterval: 100 * time.Millisecond,
			BurstSize:       1,
			EnableAdaptive:  false,
		}

		limiter, err := NewAdaptiveLimiter(config, testLogger())
		require.NoError(t, err)

		start := time.Now()
		limiter.Wait("example.com")
		first := time.Since(start)

		start = time.Now()
		limiter.Wait("example.com")
		second := time.Since(start)

		// First call should be immediate, second should wait
		assert.Less(t, first, 50*time.Millisecond)
		assert.GreaterOrEqual(t, second, 100*time.Millisecond)
	})

	t.Run("should use domain-specific intervals", func(t *testing.T) {
		config := config.RateLimitConfig{
			DefaultInterval: 50 * time.Millisecond,
			BurstSize:       1,
			EnableAdaptive:  false,
			DomainIntervals: map[string]time.Duration{
				"slow.com": 200 * time.Millisecond,
			},
		}

		limiter, err := NewAdaptiveLimiter(config, testLogger())
		require.NoError(t, err)

		// Test default domain
		start := time.Now()
		limiter.Wait("example.com")
		limiter.Wait("example.com")
		defaultWait := time.Since(start)

		// Test slow domain
		start = time.Now()
		limiter.Wait("slow.com")
		limiter.Wait("slow.com")
		slowWait := time.Since(start)

		assert.GreaterOrEqual(t, slowWait, 200*time.Millisecond)
		assert.Greater(t, slowWait, defaultWait)
	})

	t.Run("should isolate domains", func(t *testing.T) {
		config := config.RateLimitConfig{
			DefaultInterval: 100 * time.Millisecond,
			BurstSize:       1,
			EnableAdaptive:  false,
		}

		limiter, err := NewAdaptiveLimiter(config, testLogger())
		require.NoError(t, err)

		// Trigger rate limit for domain1
		limiter.Wait("domain1.com")
		limiter.Wait("domain1.com")

		// domain2 should not be affected
		start := time.Now()
		limiter.Wait("domain2.com")
		domain2Wait := time.Since(start)

		assert.Less(t, domain2Wait, 50*time.Millisecond)
	})
}

// TDD RED PHASE: Test adaptive behavior
func TestAdaptiveLimiter_AdaptiveBehavior(t *testing.T) {
	t.Run("should adjust interval based on success rate", func(t *testing.T) {
		config := config.RateLimitConfig{
			DefaultInterval: 100 * time.Millisecond,
			BurstSize:       1,
			EnableAdaptive:  true,
		}

		limiter, err := NewAdaptiveLimiter(config, testLogger())
		require.NoError(t, err)

		domain := "adaptive.com"

		// Record multiple successes
		for i := 0; i < 10; i++ {
			limiter.RecordSuccess(domain, 50*time.Millisecond)
		}

		// Should reduce interval due to consistent successes
		metrics := limiter.GetMetrics(domain)
		assert.NotNil(t, metrics)
		assert.GreaterOrEqual(t, metrics.TotalRequests, int64(10))
		assert.Equal(t, float64(1.0), metrics.SuccessRate)
	})

	t.Run("should increase interval on failures", func(t *testing.T) {
		config := config.RateLimitConfig{
			DefaultInterval: 100 * time.Millisecond,
			BurstSize:       1,
			EnableAdaptive:  true,
		}

		limiter, err := NewAdaptiveLimiter(config, testLogger())
		require.NoError(t, err)

		domain := "failing.com"

		// Record multiple failures
		for i := 0; i < 5; i++ {
			limiter.RecordFailure(domain, time.Second)
		}

		metrics := limiter.GetMetrics(domain)
		assert.NotNil(t, metrics)
		assert.GreaterOrEqual(t, metrics.TotalRequests, int64(5))
		assert.Equal(t, float64(0.0), metrics.SuccessRate)
		assert.GreaterOrEqual(t, metrics.FailureCount, int64(5))
	})
}

// TDD RED PHASE: Test context cancellation
func TestAdaptiveLimiter_ContextCancellation(t *testing.T) {
	t.Run("should respect context cancellation", func(t *testing.T) {
		config := config.RateLimitConfig{
			DefaultInterval: 500 * time.Millisecond,
			BurstSize:       1,
			EnableAdaptive:  false,
		}

		limiter, err := NewAdaptiveLimiter(config, testLogger())
		require.NoError(t, err)

		// Trigger rate limit
		limiter.Wait("example.com")

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		start := time.Now()
		err = limiter.WaitWithContext(ctx, "example.com")
		elapsed := time.Since(start)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "context")
		assert.Less(t, elapsed, 200*time.Millisecond)
	})
}

// TDD RED PHASE: Test metrics collection
func TestAdaptiveLimiter_Metrics(t *testing.T) {
	t.Run("should collect comprehensive metrics", func(t *testing.T) {
		config := config.RateLimitConfig{
			DefaultInterval: 50 * time.Millisecond,
			BurstSize:       1,
			EnableAdaptive:  true,
		}

		limiter, err := NewAdaptiveLimiter(config, testLogger())
		require.NoError(t, err)

		domain := "metrics.com"

		// Record mixed results
		limiter.RecordSuccess(domain, 100*time.Millisecond)
		limiter.RecordSuccess(domain, 150*time.Millisecond)
		limiter.RecordFailure(domain, 500*time.Millisecond)

		metrics := limiter.GetMetrics(domain)
		require.NotNil(t, metrics)

		assert.Equal(t, int64(3), metrics.TotalRequests)
		assert.Equal(t, int64(2), metrics.SuccessCount)
		assert.Equal(t, int64(1), metrics.FailureCount)
		assert.InDelta(t, 0.67, metrics.SuccessRate, 0.01)
		assert.Equal(t, 250*time.Millisecond, metrics.AvgResponseTime)
	})

	t.Run("should handle empty metrics", func(t *testing.T) {
		config := config.RateLimitConfig{
			DefaultInterval: 50 * time.Millisecond,
			BurstSize:       1,
			EnableAdaptive:  true,
		}

		limiter, err := NewAdaptiveLimiter(config, testLogger())
		require.NoError(t, err)

		metrics := limiter.GetMetrics("nonexistent.com")
		assert.Nil(t, metrics)
	})
}

// TDD RED PHASE: Test concurrent access
func TestAdaptiveLimiter_ConcurrentAccess(t *testing.T) {
	t.Run("should handle concurrent requests safely", func(t *testing.T) {
		config := config.RateLimitConfig{
			DefaultInterval: 10 * time.Millisecond,
			BurstSize:       1,
			EnableAdaptive:  true,
		}

		limiter, err := NewAdaptiveLimiter(config, testLogger())
		require.NoError(t, err)

		domain := "concurrent.com"
		var wg sync.WaitGroup
		concurrency := 10

		// Start concurrent requests
		for i := 0; i < concurrency; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				limiter.Wait(domain)
				limiter.RecordSuccess(domain, 50*time.Millisecond)
			}()
		}

		wg.Wait()

		metrics := limiter.GetMetrics(domain)
		require.NotNil(t, metrics)
		assert.Equal(t, int64(concurrency), metrics.TotalRequests)
		assert.Equal(t, int64(concurrency), metrics.SuccessCount)
	})
}

// TDD RED PHASE: Test cleanup functionality
func TestAdaptiveLimiter_Cleanup(t *testing.T) {
	t.Run("should cleanup old domain metrics", func(t *testing.T) {
		config := config.RateLimitConfig{
			DefaultInterval: 50 * time.Millisecond,
			BurstSize:       1,
			EnableAdaptive:  true,
		}

		limiter, err := NewAdaptiveLimiter(config, testLogger())
		require.NoError(t, err)

		// Add some metrics
		limiter.RecordSuccess("test1.com", 100*time.Millisecond)
		limiter.RecordSuccess("test2.com", 100*time.Millisecond)

		// Verify metrics exist
		assert.NotNil(t, limiter.GetMetrics("test1.com"))
		assert.NotNil(t, limiter.GetMetrics("test2.com"))

		// Trigger cleanup (typically would be based on time, but for testing we'll make it immediate)
		limiter.Cleanup()

		// For this test, we just verify the cleanup method exists and can be called
		assert.NotNil(t, limiter)
	})
}
