// ABOUTME: This file tests the memory-based rate limiter functionality
// ABOUTME: Following TDD principles with time-based testing and concurrent safety

package security

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewMemoryRateLimiter(t *testing.T) {
	limiter := NewMemoryRateLimiter(100, nil)
	defer limiter.Stop()

	assert.NotNil(t, limiter)
	assert.Equal(t, 100, limiter.maxRequestsPerHour)
	assert.Equal(t, 5*time.Minute, limiter.cleanupInterval)
	assert.NotNil(t, limiter.clients)
	assert.NotNil(t, limiter.logger)
	assert.True(t, limiter.isRunning)
}

func TestMemoryRateLimiter_IsAllowed_NewClient(t *testing.T) {
	limiter := NewMemoryRateLimiter(10, nil)
	defer limiter.Stop()

	// New client should be allowed
	allowed := limiter.IsAllowed("192.168.1.1", "/api/test")
	assert.True(t, allowed)

	// Verify client was created
	limiter.mutex.RLock()
	client, exists := limiter.clients["192.168.1.1"]
	limiter.mutex.RUnlock()

	assert.True(t, exists)
	assert.NotNil(t, client)
	assert.Empty(t, client.requests) // IsAllowed doesn't record
}

func TestMemoryRateLimiter_IsAllowed_RateLimitExceeded(t *testing.T) {
	limiter := NewMemoryRateLimiter(2, nil)
	defer limiter.Stop()

	clientIP := "192.168.1.2"
	endpoint := "/api/test"

	// First two requests should be allowed
	assert.True(t, limiter.IsAllowed(clientIP, endpoint))
	limiter.RecordRequest(clientIP, endpoint)

	assert.True(t, limiter.IsAllowed(clientIP, endpoint))
	limiter.RecordRequest(clientIP, endpoint)

	// Third request should be denied
	assert.False(t, limiter.IsAllowed(clientIP, endpoint))
}

func TestMemoryRateLimiter_RecordRequest(t *testing.T) {
	limiter := NewMemoryRateLimiter(100, nil)
	defer limiter.Stop()

	clientIP := "192.168.1.3"
	endpoint := "/api/record"

	// Record a request
	limiter.RecordRequest(clientIP, endpoint)

	// Verify request was recorded
	limiter.mutex.RLock()
	client := limiter.clients[clientIP]
	limiter.mutex.RUnlock()

	assert.NotNil(t, client)
	assert.Len(t, client.requests, 1)
	assert.Equal(t, endpoint, client.requests[0].endpoint)
	assert.WithinDuration(t, time.Now(), client.requests[0].timestamp, 1*time.Second)
}

func TestMemoryRateLimiter_GetClientStats_NewClient(t *testing.T) {
	limiter := NewMemoryRateLimiter(100, nil)
	defer limiter.Stop()

	stats := limiter.GetClientStats("192.168.1.4")

	assert.Equal(t, "192.168.1.4", stats.ClientIP)
	assert.Equal(t, 0, stats.RequestsInLastHour)
	assert.Equal(t, 100, stats.RemainingRequests)
	assert.WithinDuration(t, time.Now().Add(time.Hour), stats.NextResetTime, 1*time.Second)
}

func TestMemoryRateLimiter_GetClientStats_WithRequests(t *testing.T) {
	limiter := NewMemoryRateLimiter(10, nil)
	defer limiter.Stop()

	clientIP := "192.168.1.5"

	// Record some requests
	limiter.RecordRequest(clientIP, "/api/test1")
	limiter.RecordRequest(clientIP, "/api/test2")
	limiter.RecordRequest(clientIP, "/api/test1")

	stats := limiter.GetClientStats(clientIP)

	assert.Equal(t, clientIP, stats.ClientIP)
	assert.Equal(t, 3, stats.RequestsInLastHour)
	assert.Equal(t, 7, stats.RemainingRequests)
	assert.NotEmpty(t, stats.EndpointBreakdown)
	assert.Equal(t, 2, stats.EndpointBreakdown["/api/test1"])
	assert.Equal(t, 1, stats.EndpointBreakdown["/api/test2"])
}

func TestMemoryRateLimiter_GetAllClientsStats(t *testing.T) {
	limiter := NewMemoryRateLimiter(10, nil)
	defer limiter.Stop()

	// Record requests for multiple clients
	limiter.RecordRequest("192.168.1.6", "/api/test")
	limiter.RecordRequest("192.168.1.7", "/api/test")
	limiter.RecordRequest("192.168.1.6", "/api/other")

	allStats := limiter.GetAllClientsStats()

	assert.Len(t, allStats, 2)

	// Find stats by client IP
	var client6Stats, client7Stats *ClientStats
	for i := range allStats {
		if allStats[i].ClientIP == "192.168.1.6" {
			client6Stats = &allStats[i]
		} else if allStats[i].ClientIP == "192.168.1.7" {
			client7Stats = &allStats[i]
		}
	}

	assert.NotNil(t, client6Stats)
	assert.Equal(t, 2, client6Stats.RequestsInLastHour)

	assert.NotNil(t, client7Stats)
	assert.Equal(t, 1, client7Stats.RequestsInLastHour)
}

func TestMemoryRateLimiter_GetGlobalStats(t *testing.T) {
	limiter := NewMemoryRateLimiter(10, nil)
	defer limiter.Stop()

	// Record some requests
	limiter.RecordRequest("192.168.1.8", "/api/test")
	limiter.RecordRequest("192.168.1.9", "/api/test")
	limiter.RecordRequest("192.168.1.8", "/api/other")

	globalStats := limiter.GetGlobalStats()

	assert.Equal(t, 2, globalStats.TotalActiveClients)
	assert.Equal(t, 3, globalStats.TotalRequestsLastHour)
	assert.Equal(t, 10, globalStats.MaxRequestsPerHour)
	assert.Equal(t, 2, globalStats.EndpointBreakdown["/api/test"])
	assert.Equal(t, 1, globalStats.EndpointBreakdown["/api/other"])
	assert.WithinDuration(t, time.Now(), globalStats.LastCleanup, 1*time.Second)
}

func TestMemoryRateLimiter_FilterValidRequests(t *testing.T) {
	limiter := NewMemoryRateLimiter(10, nil)
	defer limiter.Stop()

	now := time.Now()
	requests := []RequestRecord{
		{timestamp: now.Add(-2 * time.Hour), endpoint: "/old1"},      // Too old
		{timestamp: now.Add(-30 * time.Minute), endpoint: "/recent"}, // Valid
		{timestamp: now.Add(-90 * time.Minute), endpoint: "/old2"},   // Too old
		{timestamp: now.Add(-10 * time.Minute), endpoint: "/new"},    // Valid
	}

	cutoff := now.Add(-time.Hour)
	validRequests := limiter.filterValidRequests(requests, cutoff)

	assert.Len(t, validRequests, 2)
	assert.Equal(t, "/recent", validRequests[0].endpoint)
	assert.Equal(t, "/new", validRequests[1].endpoint)
}

func TestMemoryRateLimiter_GetEndpointBreakdown(t *testing.T) {
	limiter := NewMemoryRateLimiter(10, nil)
	defer limiter.Stop()

	requests := []RequestRecord{
		{endpoint: "/api/test"},
		{endpoint: "/api/other"},
		{endpoint: "/api/test"},
		{endpoint: "/api/third"},
		{endpoint: "/api/test"},
	}

	breakdown := limiter.getEndpointBreakdown(requests)

	assert.Equal(t, 3, breakdown["/api/test"])
	assert.Equal(t, 1, breakdown["/api/other"])
	assert.Equal(t, 1, breakdown["/api/third"])
}

func TestMemoryRateLimiter_Stop(t *testing.T) {
	limiter := NewMemoryRateLimiter(10, nil)

	// Verify it's running
	assert.True(t, limiter.isRunning)

	// Add some data
	limiter.RecordRequest("192.168.1.10", "/api/test")
	
	// Stop the limiter
	limiter.Stop()

	// Verify it's stopped and cleaned
	assert.False(t, limiter.isRunning)
	
	limiter.mutex.RLock()
	clientCount := len(limiter.clients)
	limiter.mutex.RUnlock()
	
	assert.Equal(t, 0, clientCount)
}

func TestMemoryRateLimiter_ConcurrentAccess(t *testing.T) {
	limiter := NewMemoryRateLimiter(100, nil)
	defer limiter.Stop()

	clientIP := "192.168.1.11"
	
	// Test concurrent access doesn't cause data races
	done := make(chan bool, 10)
	
	// Start multiple goroutines
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 10; j++ {
				limiter.IsAllowed(clientIP, "/api/test")
				limiter.RecordRequest(clientIP, "/api/test")
				limiter.GetClientStats(clientIP)
			}
			done <- true
		}(i)
	}
	
	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}
	
	// Verify final state is consistent
	stats := limiter.GetClientStats(clientIP)
	assert.Equal(t, clientIP, stats.ClientIP)
	assert.Equal(t, 100, stats.RequestsInLastHour)
	assert.Equal(t, 0, stats.RemainingRequests)
}

func TestMemoryRateLimiter_TimeBasedExpiry(t *testing.T) {
	limiter := NewMemoryRateLimiter(5, nil)
	defer limiter.Stop()

	clientIP := "192.168.1.12"

	// Manually add old requests to test expiry
	now := time.Now()
	limiter.mutex.Lock()
	limiter.clients[clientIP] = &ClientRateLimit{
		requests: []RequestRecord{
			{timestamp: now.Add(-2 * time.Hour), endpoint: "/old"},  // Should be filtered out
			{timestamp: now.Add(-30 * time.Minute), endpoint: "/recent"}, // Should remain
		},
		lastCleanup: now,
	}
	limiter.mutex.Unlock()

	// Check that old requests are filtered
	allowed := limiter.IsAllowed(clientIP, "/api/test")
	assert.True(t, allowed) // Should be allowed because old request was filtered

	stats := limiter.GetClientStats(clientIP)
	assert.Equal(t, 1, stats.RequestsInLastHour) // Only the recent request should count
}

func TestMemoryRateLimiter_MultipleEndpoints(t *testing.T) {
	limiter := NewMemoryRateLimiter(3, nil)
	defer limiter.Stop()

	clientIP := "192.168.1.13"

	// Record requests to different endpoints
	limiter.RecordRequest(clientIP, "/api/login")
	limiter.RecordRequest(clientIP, "/api/refresh")
	limiter.RecordRequest(clientIP, "/api/login")

	// Should be at limit now (3 requests, limit is 3)
	assert.False(t, limiter.IsAllowed(clientIP, "/api/status"))

	stats := limiter.GetClientStats(clientIP)
	assert.Equal(t, 3, stats.RequestsInLastHour)
	assert.Equal(t, 0, stats.RemainingRequests)
	assert.Equal(t, 2, stats.EndpointBreakdown["/api/login"])
	assert.Equal(t, 1, stats.EndpointBreakdown["/api/refresh"])
	// "/api/status" is not in breakdown because the request was denied and not recorded
}

func TestClientStats_Structure(t *testing.T) {
	now := time.Now()
	stats := ClientStats{
		ClientIP:           "test-ip",
		RequestsInLastHour: 5,
		RemainingRequests:  15,
		NextResetTime:      now,
		EndpointBreakdown:  map[string]int{"test": 3, "other": 2},
	}

	assert.Equal(t, "test-ip", stats.ClientIP)
	assert.Equal(t, 5, stats.RequestsInLastHour)
	assert.Equal(t, 15, stats.RemainingRequests)
	assert.Equal(t, now, stats.NextResetTime)
	assert.Len(t, stats.EndpointBreakdown, 2)
}

func TestGlobalStats_Structure(t *testing.T) {
	now := time.Now()
	stats := GlobalStats{
		TotalActiveClients:    10,
		TotalRequestsLastHour: 150,
		MaxRequestsPerHour:    100,
		EndpointBreakdown:     map[string]int{"api1": 100, "api2": 50},
		LastCleanup:           now,
	}

	assert.Equal(t, 10, stats.TotalActiveClients)
	assert.Equal(t, 150, stats.TotalRequestsLastHour)
	assert.Equal(t, 100, stats.MaxRequestsPerHour)
	assert.Len(t, stats.EndpointBreakdown, 2)
	assert.Equal(t, now, stats.LastCleanup)
}

func TestRequestRecord_Structure(t *testing.T) {
	now := time.Now()
	record := RequestRecord{
		timestamp: now,
		endpoint:  "/api/test",
	}

	assert.Equal(t, now, record.timestamp)
	assert.Equal(t, "/api/test", record.endpoint)
}

func TestMemoryRateLimiter_EdgeCases(t *testing.T) {
	t.Run("zero_rate_limit", func(t *testing.T) {
		limiter := NewMemoryRateLimiter(0, nil)
		defer limiter.Stop()

		// Should immediately deny all requests
		assert.False(t, limiter.IsAllowed("test", "/api"))
	})

	t.Run("empty_client_ip", func(t *testing.T) {
		limiter := NewMemoryRateLimiter(10, nil)
		defer limiter.Stop()

		assert.True(t, limiter.IsAllowed("", "/api"))
		limiter.RecordRequest("", "/api")
		
		stats := limiter.GetClientStats("")
		assert.Equal(t, "", stats.ClientIP)
		assert.Equal(t, 1, stats.RequestsInLastHour)
	})

	t.Run("empty_endpoint", func(t *testing.T) {
		limiter := NewMemoryRateLimiter(10, nil)
		defer limiter.Stop()

		assert.True(t, limiter.IsAllowed("test", ""))
		limiter.RecordRequest("test", "")
		
		stats := limiter.GetClientStats("test")
		assert.Equal(t, 1, stats.EndpointBreakdown[""])
	})
}