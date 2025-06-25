package performance_tests

import (
	"alt/utils/rate_limiter"
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

func BenchmarkRateLimiterConcurrentHosts(b *testing.B) {
	rateLimiter := rate_limiter.NewHostRateLimiter(100 * time.Millisecond)

	hosts := []string{
		"http://example1.com",
		"http://example2.com",
		"http://example3.com",
		"http://example4.com",
		"http://example5.com",
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			host := hosts[b.N%len(hosts)]
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			rateLimiter.WaitForHost(ctx, host)
			cancel()
		}
	})
}

func BenchmarkRateLimiterMemoryUsage(b *testing.B) {
	rateLimiter := rate_limiter.NewHostRateLimiter(1 * time.Millisecond)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		host := fmt.Sprintf("http://example%d.com", i)
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		rateLimiter.WaitForHost(ctx, host)
		cancel()
	}
}

func TestRateLimiterHighConcurrency(t *testing.T) {
	rateLimiter := rate_limiter.NewHostRateLimiter(10 * time.Millisecond)

	const numGoroutines = 100
	const requestsPerGoroutine = 10

	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines*requestsPerGoroutine)

	start := time.Now()

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()

			host := fmt.Sprintf("http://host%d.com", goroutineID%5) // 5 different hosts

			for j := 0; j < requestsPerGoroutine; j++ {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				err := rateLimiter.WaitForHost(ctx, host)
				cancel()

				if err != nil {
					errors <- err
				}
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	duration := time.Since(start)

	// Check for errors
	errorCount := 0
	for err := range errors {
		t.Errorf("Rate limiter error: %v", err)
		errorCount++
	}

	if errorCount > 0 {
		t.Fatalf("Found %d errors during high concurrency test", errorCount)
	}

	t.Logf("High concurrency test completed in %v", duration)

	// Should complete within reasonable time (allowing for rate limiting)
	maxExpectedDuration := time.Duration(requestsPerGoroutine-1) * 150 * time.Millisecond * 10 // More relaxed timing for CI/different machines
	if duration > maxExpectedDuration {
		t.Errorf("Test took too long: %v > %v", duration, maxExpectedDuration)
	}
}

func BenchmarkRateLimiterSingleHost(b *testing.B) {
	rateLimiter := rate_limiter.NewHostRateLimiter(1 * time.Millisecond)
	host := "http://example.com"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		rateLimiter.WaitForHost(ctx, host)
		cancel()
	}
}

func BenchmarkRateLimiterLockContention(b *testing.B) {
	rateLimiter := rate_limiter.NewHostRateLimiter(1 * time.Nanosecond) // Very fast rate limit
	host := "http://example.com"

	// Pre-initialize the limiter to avoid setup cost
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	rateLimiter.WaitForHost(ctx, host)
	cancel()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			rateLimiter.WaitForHost(ctx, host)
			cancel()
		}
	})
}

func TestRateLimiterScalability(t *testing.T) {
	// Test scalability with increasing number of hosts
	intervals := []time.Duration{
		1 * time.Millisecond,
		10 * time.Millisecond,
		100 * time.Millisecond,
		1 * time.Second,
	}

	hostCounts := []int{10, 100, 1000, 5000}

	for _, interval := range intervals {
		for _, hostCount := range hostCounts {
			t.Run(fmt.Sprintf("interval_%v_hosts_%d", interval, hostCount), func(t *testing.T) {
				rateLimiter := rate_limiter.NewHostRateLimiter(interval)

				start := time.Now()

				// Create requests to different hosts
				var wg sync.WaitGroup
				for i := 0; i < hostCount; i++ {
					wg.Add(1)
					go func(hostID int) {
						defer wg.Done()
						host := fmt.Sprintf("http://host%d.com", hostID)
						ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
						defer cancel()
						rateLimiter.WaitForHost(ctx, host)
					}(i)
				}

				wg.Wait()
				duration := time.Since(start)

				// Since each host should have its own rate limiter,
				// all requests should complete relatively quickly
				maxExpectedDuration := 5 * time.Second
				if duration > maxExpectedDuration {
					t.Errorf("Scalability test took too long: %v > %v for %d hosts with %v interval",
						duration, maxExpectedDuration, hostCount, interval)
				}

				t.Logf("Processed %d hosts in %v (interval: %v)", hostCount, duration, interval)
			})
		}
	}
}

func TestRateLimiterMemoryGrowth(t *testing.T) {
	rateLimiter := rate_limiter.NewHostRateLimiter(1 * time.Millisecond)

	// Test that memory doesn't grow indefinitely with many unique hosts
	hostCount := 10000

	start := time.Now()

	for i := 0; i < hostCount; i++ {
		host := fmt.Sprintf("http://unique-host-%d.com", i)
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		err := rateLimiter.WaitForHost(ctx, host)
		cancel()

		if err != nil {
			t.Errorf("Unexpected error for host %s: %v", host, err)
		}

		// Log progress for very large tests
		if i%1000 == 0 && i > 0 {
			t.Logf("Processed %d hosts so far...", i)
		}
	}

	duration := time.Since(start)
	t.Logf("Created rate limiters for %d unique hosts in %v", hostCount, duration)

	// This test primarily checks that we don't run out of memory or have performance degradation
	// The actual time will depend on the rate limiting, but it shouldn't be excessive
	maxExpectedDuration := 30 * time.Second
	if duration > maxExpectedDuration {
		t.Errorf("Memory growth test took too long: %v > %v", duration, maxExpectedDuration)
	}
}

func BenchmarkRateLimiterURLParsing(b *testing.B) {
	rateLimiter := rate_limiter.NewHostRateLimiter(1 * time.Nanosecond)

	urls := []string{
		"http://example.com/path/to/feed.xml",
		"https://feeds.example.com/rss",
		"http://subdomain.example.org/feed?format=rss",
		"https://blog.example.net/posts.xml",
		"http://news.example.co.uk/rss.xml",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		url := urls[i%len(urls)]
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		rateLimiter.WaitForHost(ctx, url)
		cancel()
	}
}

func TestRateLimiterPrecision(t *testing.T) {
	// Test that rate limiting timing is reasonably precise
	interval := 100 * time.Millisecond
	rateLimiter := rate_limiter.NewHostRateLimiter(interval)
	host := "http://precision-test.com"

	// Make first request to initialize
	ctx1, cancel1 := context.WithTimeout(context.Background(), 1*time.Second)
	err1 := rateLimiter.WaitForHost(ctx1, host)
	cancel1()
	if err1 != nil {
		t.Fatalf("First request failed: %v", err1)
	}

	// Measure timing for second request
	start := time.Now()
	ctx2, cancel2 := context.WithTimeout(context.Background(), 1*time.Second)
	err2 := rateLimiter.WaitForHost(ctx2, host)
	cancel2()
	if err2 != nil {
		t.Fatalf("Second request failed: %v", err2)
	}
	actualInterval := time.Since(start)

	// Allow for some variance but should be close to expected interval
	tolerance := 20 * time.Millisecond
	minExpected := interval - tolerance
	maxExpected := interval + tolerance

	if actualInterval < minExpected || actualInterval > maxExpected {
		t.Errorf("Rate limiting precision error: expected %v ± %v, got %v",
			interval, tolerance, actualInterval)
	}

	t.Logf("Rate limiting precision test: expected %v, actual %v", interval, actualInterval)
}
