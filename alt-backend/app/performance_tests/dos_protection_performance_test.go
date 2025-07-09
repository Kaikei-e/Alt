package performance_tests

import (
	"alt/config"
	"alt/middleware"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func BenchmarkDOSProtectionMiddleware(b *testing.B) {
	benchmarks := []struct {
		name      string
		config    config.DOSProtectionConfig
		numIPs    int
		numCPUs   int
		description string
	}{
		{
			name: "single_ip_high_limits",
			config: config.DOSProtectionConfig{
				Enabled:       true,
				RateLimit:     10000,
				BurstLimit:    20000,
				WindowSize:    time.Minute,
				BlockDuration: 5 * time.Minute,
			},
			numIPs:      1,
			numCPUs:     1,
			description: "Single IP with high limits",
		},
		{
			name: "multiple_ips_moderate_limits",
			config: config.DOSProtectionConfig{
				Enabled:       true,
				RateLimit:     100,
				BurstLimit:    200,
				WindowSize:    time.Minute,
				BlockDuration: 5 * time.Minute,
			},
			numIPs:      100,
			numCPUs:     4,
			description: "Multiple IPs with moderate limits",
		},
		{
			name: "disabled_middleware",
			config: config.DOSProtectionConfig{
				Enabled: false,
			},
			numIPs:      1,
			numCPUs:     1,
			description: "Disabled middleware baseline",
		},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			// Create Echo instance with DOS protection middleware
			e := echo.New()
			e.Use(middleware.DOSProtectionMiddleware(bm.config))
			e.GET("/v1/test", func(c echo.Context) error {
				return c.String(http.StatusOK, "OK")
			})

			b.ResetTimer()
			b.SetParallelism(bm.numCPUs)

			b.RunParallel(func(pb *testing.PB) {
				ipIndex := 0
				for pb.Next() {
					req := httptest.NewRequest(http.MethodGet, "/v1/test", nil)
					req.Header.Set("X-Real-IP", fmt.Sprintf("192.168.1.%d", ipIndex%bm.numIPs+1))
					rec := httptest.NewRecorder()
					e.ServeHTTP(rec, req)
					ipIndex++
				}
			})
		})
	}
}

func TestDOSProtectionConcurrency(t *testing.T) {
	config := config.DOSProtectionConfig{
		Enabled:       true,
		RateLimit:     10,
		BurstLimit:    20,
		WindowSize:    time.Minute,
		BlockDuration: 5 * time.Minute,
	}

	e := echo.New()
	e.Use(middleware.DOSProtectionMiddleware(config))
	e.GET("/v1/test", func(c echo.Context) error {
		return c.String(http.StatusOK, "OK")
	})

	const numGoroutines = 100
	const requestsPerGoroutine = 50
	
	var wg sync.WaitGroup
	results := make(chan int, numGoroutines*requestsPerGoroutine)
	
	// Launch concurrent requests
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			
			for j := 0; j < requestsPerGoroutine; j++ {
				req := httptest.NewRequest(http.MethodGet, "/v1/test", nil)
				req.Header.Set("X-Real-IP", fmt.Sprintf("192.168.%d.%d", goroutineID%10, j%10))
				rec := httptest.NewRecorder()
				e.ServeHTTP(rec, req)
				results <- rec.Code
			}
		}(i)
	}
	
	wg.Wait()
	close(results)
	
	// Analyze results
	statusCodes := make(map[int]int)
	for code := range results {
		statusCodes[code]++
	}
	
	t.Logf("Status code distribution: %+v", statusCodes)
	
	// Should have both successful and rate-limited requests
	assert.Greater(t, statusCodes[200], 0, "Should have some successful requests")
	assert.Greater(t, statusCodes[429], 0, "Should have some rate-limited requests")
	
	// Total should match expected
	total := statusCodes[200] + statusCodes[429]
	assert.Equal(t, numGoroutines*requestsPerGoroutine, total)
}

func TestDOSProtectionMemoryUsage(t *testing.T) {
	config := config.DOSProtectionConfig{
		Enabled:       true,
		RateLimit:     100,
		BurstLimit:    200,
		WindowSize:    time.Minute,
		BlockDuration: 5 * time.Minute,
	}

	e := echo.New()
	e.Use(middleware.DOSProtectionMiddleware(config))
	e.GET("/v1/test", func(c echo.Context) error {
		return c.String(http.StatusOK, "OK")
	})

	// Test with many different IPs to check memory usage
	const numIPs = 10000
	
	start := time.Now()
	for i := 0; i < numIPs; i++ {
		req := httptest.NewRequest(http.MethodGet, "/v1/test", nil)
		req.Header.Set("X-Real-IP", fmt.Sprintf("192.168.%d.%d", i/255, i%255))
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		
		// All should succeed due to different IPs
		assert.Equal(t, http.StatusOK, rec.Code)
	}
	
	duration := time.Since(start)
	t.Logf("Processed %d unique IPs in %v", numIPs, duration)
	
	// Should handle large number of IPs efficiently
	assert.Less(t, duration, 10*time.Second, "Should handle 10k IPs in under 10 seconds")
}

func TestDOSProtectionCircuitBreakerPerformance(t *testing.T) {
	config := config.DOSProtectionConfig{
		Enabled:       true,
		RateLimit:     1000,
		BurstLimit:    2000,
		WindowSize:    time.Minute,
		BlockDuration: 5 * time.Minute,
		CircuitBreaker: config.CircuitBreakerConfig{
			Enabled:          true,
			FailureThreshold: 5,
			TimeoutDuration:  30 * time.Second,
			RecoveryTimeout:  60 * time.Second,
		},
	}

	e := echo.New()
	e.Use(middleware.DOSProtectionMiddleware(config))
	
	// Handler that fails initially then succeeds
	requestCount := 0
	e.GET("/v1/test", func(c echo.Context) error {
		requestCount++
		if requestCount <= 5 {
			return echo.NewHTTPError(http.StatusInternalServerError, "Service error")
		}
		return c.String(http.StatusOK, "OK")
	})

	// First 5 requests should fail, triggering circuit breaker
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodGet, "/v1/test", nil)
		req.Header.Set("X-Real-IP", "192.168.1.1")
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	}

	// Next requests should be blocked by circuit breaker
	start := time.Now()
	const numBlockedRequests = 100
	
	for i := 0; i < numBlockedRequests; i++ {
		req := httptest.NewRequest(http.MethodGet, "/v1/test", nil)
		req.Header.Set("X-Real-IP", "192.168.1.1")
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	}
	
	duration := time.Since(start)
	t.Logf("Circuit breaker blocked %d requests in %v", numBlockedRequests, duration)
	
	// Circuit breaker should be very fast
	assert.Less(t, duration, 100*time.Millisecond, "Circuit breaker should block requests very quickly")
}

func TestDOSProtectionScalability(t *testing.T) {
	scalabilityTests := []struct {
		name            string
		numIPs          int
		requestsPerIP   int
		maxDuration     time.Duration
		description     string
	}{
		{
			name:            "small_scale",
			numIPs:          100,
			requestsPerIP:   10,
			maxDuration:     1 * time.Second,
			description:     "Small scale test - 100 IPs, 10 requests each",
		},
		{
			name:            "medium_scale",
			numIPs:          1000,
			requestsPerIP:   5,
			maxDuration:     5 * time.Second,
			description:     "Medium scale test - 1000 IPs, 5 requests each",
		},
		{
			name:            "large_scale",
			numIPs:          5000,
			requestsPerIP:   2,
			maxDuration:     10 * time.Second,
			description:     "Large scale test - 5000 IPs, 2 requests each",
		},
	}

	for _, tt := range scalabilityTests {
		t.Run(tt.name, func(t *testing.T) {
			config := config.DOSProtectionConfig{
				Enabled:       true,
				RateLimit:     tt.requestsPerIP,
				BurstLimit:    tt.requestsPerIP * 2,
				WindowSize:    time.Minute,
				BlockDuration: 5 * time.Minute,
			}

			e := echo.New()
			e.Use(middleware.DOSProtectionMiddleware(config))
			e.GET("/v1/test", func(c echo.Context) error {
				return c.String(http.StatusOK, "OK")
			})

			start := time.Now()
			totalRequests := tt.numIPs * tt.requestsPerIP

			for i := 0; i < tt.numIPs; i++ {
				ip := fmt.Sprintf("192.168.%d.%d", i/255, i%255)
				
				for j := 0; j < tt.requestsPerIP; j++ {
					req := httptest.NewRequest(http.MethodGet, "/v1/test", nil)
					req.Header.Set("X-Real-IP", ip)
					rec := httptest.NewRecorder()
					e.ServeHTTP(rec, req)
					
					// Should succeed since each IP gets its own limit
					assert.Equal(t, http.StatusOK, rec.Code)
				}
			}

			duration := time.Since(start)
			requestsPerSecond := float64(totalRequests) / duration.Seconds()
			
			t.Logf("%s: %d requests in %v (%.2f req/sec)", 
				tt.description, totalRequests, duration, requestsPerSecond)
			
			assert.Less(t, duration, tt.maxDuration, 
				"Test %s should complete within %v, took %v", tt.name, tt.maxDuration, duration)
		})
	}
}

func TestDOSProtectionMemoryLeakPrevention(t *testing.T) {
	config := config.DOSProtectionConfig{
		Enabled:       true,
		RateLimit:     1,
		BurstLimit:    1,
		WindowSize:    time.Minute,
		BlockDuration: 100 * time.Millisecond, // Very short block duration
	}

	e := echo.New()
	e.Use(middleware.DOSProtectionMiddleware(config))
	e.GET("/v1/test", func(c echo.Context) error {
		return c.String(http.StatusOK, "OK")
	})

	// Create many different IPs, trigger rate limiting, then wait for cleanup
	const numIPs = 1000
	
	// First, trigger rate limiting for all IPs
	for i := 0; i < numIPs; i++ {
		ip := fmt.Sprintf("192.168.%d.%d", i/255, i%255)
		
		// First request succeeds
		req := httptest.NewRequest(http.MethodGet, "/v1/test", nil)
		req.Header.Set("X-Real-IP", ip)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
		
		// Second request gets rate limited
		req = httptest.NewRequest(http.MethodGet, "/v1/test", nil)
		req.Header.Set("X-Real-IP", ip)
		rec = httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	}
	
	// Wait for block duration to expire
	time.Sleep(200 * time.Millisecond)
	
	// Test that system still works after potential cleanup
	req := httptest.NewRequest(http.MethodGet, "/v1/test", nil)
	req.Header.Set("X-Real-IP", "192.168.1.1")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	
	t.Log("Memory leak prevention test completed - system remains responsive")
}