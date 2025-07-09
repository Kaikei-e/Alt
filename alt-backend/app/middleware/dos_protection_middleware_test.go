package middleware

import (
	"alt/config"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDOSProtectionMiddleware(t *testing.T) {
	tests := []struct {
		name           string
		config         DOSProtectionConfig
		requests       []testRequest
		expectedStatus []int
		description    string
	}{
		{
			name: "basic_ip_rate_limiting",
			config: DOSProtectionConfig{
				Enabled:     true,
				RateLimit:   5,
				BurstLimit:  10,
				WindowSize:  time.Minute,
				BlockDuration: 5 * time.Minute,
			},
			requests: []testRequest{
				{ip: "192.168.1.1", path: "/v1/feeds", method: "GET"},
				{ip: "192.168.1.1", path: "/v1/feeds", method: "GET"},
				{ip: "192.168.1.1", path: "/v1/feeds", method: "GET"},
				{ip: "192.168.1.1", path: "/v1/feeds", method: "GET"},
				{ip: "192.168.1.1", path: "/v1/feeds", method: "GET"},
				{ip: "192.168.1.1", path: "/v1/feeds", method: "GET"}, // Should be blocked
			},
			expectedStatus: []int{200, 200, 200, 200, 200, 429},
			description:    "Should allow 5 requests per minute per IP, then block",
		},
		{
			name: "different_ips_not_affected",
			config: DOSProtectionConfig{
				Enabled:     true,
				RateLimit:   2,
				BurstLimit:  2,
				WindowSize:  time.Minute,
				BlockDuration: 5 * time.Minute,
			},
			requests: []testRequest{
				{ip: "192.168.1.1", path: "/v1/feeds", method: "GET"},
				{ip: "192.168.1.1", path: "/v1/feeds", method: "GET"},
				{ip: "192.168.1.1", path: "/v1/feeds", method: "GET"}, // Should be blocked
				{ip: "192.168.1.2", path: "/v1/feeds", method: "GET"}, // Different IP, should work
				{ip: "192.168.1.2", path: "/v1/feeds", method: "GET"}, // Different IP, should work
			},
			expectedStatus: []int{200, 200, 429, 200, 200},
			description:    "Different IPs should have separate rate limits",
		},
		{
			name: "burst_protection",
			config: DOSProtectionConfig{
				Enabled:     true,
				RateLimit:   1,
				BurstLimit:  3,
				WindowSize:  time.Minute,
				BlockDuration: 5 * time.Minute,
			},
			requests: []testRequest{
				{ip: "192.168.1.1", path: "/v1/feeds", method: "GET"},
				{ip: "192.168.1.1", path: "/v1/feeds", method: "GET"},
				{ip: "192.168.1.1", path: "/v1/feeds", method: "GET"},
				{ip: "192.168.1.1", path: "/v1/feeds", method: "GET"}, // Should be blocked
			},
			expectedStatus: []int{200, 200, 200, 429},
			description:    "Should allow burst of 3 requests, then block",
		},
		{
			name: "disabled_middleware",
			config: DOSProtectionConfig{
				Enabled: false,
			},
			requests: []testRequest{
				{ip: "192.168.1.1", path: "/v1/feeds", method: "GET"},
				{ip: "192.168.1.1", path: "/v1/feeds", method: "GET"},
				{ip: "192.168.1.1", path: "/v1/feeds", method: "GET"},
			},
			expectedStatus: []int{200, 200, 200},
			description:    "Disabled middleware should allow all requests",
		},
		{
			name: "whitelisted_paths",
			config: DOSProtectionConfig{
				Enabled:     true,
				RateLimit:   1,
				BurstLimit:  1,
				WindowSize:  time.Minute,
				BlockDuration: 5 * time.Minute,
				WhitelistedPaths: []string{"/v1/health"},
			},
			requests: []testRequest{
				{ip: "192.168.1.1", path: "/v1/health", method: "GET"},
				{ip: "192.168.1.1", path: "/v1/health", method: "GET"},
				{ip: "192.168.1.1", path: "/v1/health", method: "GET"},
			},
			expectedStatus: []int{200, 200, 200},
			description:    "Whitelisted paths should not be rate limited",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			middleware := DOSProtectionMiddleware(tt.config)
			
			handler := middleware(func(c echo.Context) error {
				return c.String(http.StatusOK, "OK")
			})

			for i, req := range tt.requests {
				e := echo.New()
				httpReq := httptest.NewRequest(req.method, req.path, nil)
				httpReq.Header.Set("X-Real-IP", req.ip)
				rec := httptest.NewRecorder()
				c := e.NewContext(httpReq, rec)

				err := handler(c)
				
				if tt.expectedStatus[i] != 429 {
					require.NoError(t, err)
				}
				
				assert.Equal(t, tt.expectedStatus[i], rec.Code,
					"Request %d failed: %s", i+1, tt.description)
			}
		})
	}
}

func TestDOSProtectionMiddleware_CircuitBreaker(t *testing.T) {
	config := DOSProtectionConfig{
		Enabled:         true,
		RateLimit:       10,
		BurstLimit:      10,
		WindowSize:      time.Minute,
		BlockDuration:   5 * time.Minute,
		CircuitBreaker: CircuitBreakerConfig{
			Enabled:         true,
			FailureThreshold: 5,
			TimeoutDuration:  time.Second,
			RecoveryTimeout:  30 * time.Second,
		},
	}

	middleware := DOSProtectionMiddleware(config)

	// Handler that simulates failures
	failureCount := 0
	handler := middleware(func(c echo.Context) error {
		failureCount++
		if failureCount <= 5 {
			return echo.NewHTTPError(http.StatusInternalServerError, "Service error")
		}
		return c.String(http.StatusOK, "OK")
	})

	e := echo.New()

	// First 5 requests should return 500 (service errors)
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodGet, "/v1/feeds", nil)
		req.Header.Set("X-Real-IP", "192.168.1.1")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		err := handler(c)
		require.Error(t, err)
		assert.Equal(t, http.StatusInternalServerError, err.(*echo.HTTPError).Code)
	}

	// Next request should trigger circuit breaker (503)
	req := httptest.NewRequest(http.MethodGet, "/v1/feeds", nil)
	req.Header.Set("X-Real-IP", "192.168.1.1")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handler(c)
	require.Error(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, err.(*echo.HTTPError).Code)
}

func TestDOSProtectionMiddleware_ConcurrentRequests(t *testing.T) {
	config := DOSProtectionConfig{
		Enabled:     true,
		RateLimit:   10,
		BurstLimit:  10,
		WindowSize:  time.Minute,
		BlockDuration: 5 * time.Minute,
	}

	middleware := DOSProtectionMiddleware(config)
	handler := middleware(func(c echo.Context) error {
		return c.String(http.StatusOK, "OK")
	})

	// Test concurrent requests from same IP
	const numGoroutines = 20
	const requestsPerGoroutine = 5
	
	results := make(chan int, numGoroutines*requestsPerGoroutine)
	
	for i := 0; i < numGoroutines; i++ {
		go func() {
			for j := 0; j < requestsPerGoroutine; j++ {
				e := echo.New()
				req := httptest.NewRequest(http.MethodGet, "/v1/feeds", nil)
				req.Header.Set("X-Real-IP", "192.168.1.1")
				rec := httptest.NewRecorder()
				c := e.NewContext(req, rec)

				handler(c)
				results <- rec.Code
			}
		}()
	}

	// Collect results
	statusCodes := make([]int, 0, numGoroutines*requestsPerGoroutine)
	for i := 0; i < numGoroutines*requestsPerGoroutine; i++ {
		statusCodes = append(statusCodes, <-results)
	}

	// Count successful and rate limited requests
	successCount := 0
	rateLimitedCount := 0
	for _, code := range statusCodes {
		if code == http.StatusOK {
			successCount++
		} else if code == http.StatusTooManyRequests {
			rateLimitedCount++
		}
	}

	// Should have exactly 10 successful requests (rate limit)
	assert.Equal(t, 10, successCount, "Should have exactly 10 successful requests")
	assert.Equal(t, 90, rateLimitedCount, "Should have 90 rate limited requests")
}

func TestDOSProtectionMiddleware_EdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		setupFunc   func() (echo.Context, *httptest.ResponseRecorder)
		expectError bool
		description string
	}{
		{
			name: "missing_ip_header",
			setupFunc: func() (echo.Context, *httptest.ResponseRecorder) {
				e := echo.New()
				req := httptest.NewRequest(http.MethodGet, "/v1/feeds", nil)
				// No X-Real-IP header
				rec := httptest.NewRecorder()
				return e.NewContext(req, rec), rec
			},
			expectError: false,
			description: "Should handle missing IP header gracefully",
		},
		{
			name: "invalid_ip_address",
			setupFunc: func() (echo.Context, *httptest.ResponseRecorder) {
				e := echo.New()
				req := httptest.NewRequest(http.MethodGet, "/v1/feeds", nil)
				req.Header.Set("X-Real-IP", "invalid.ip.address")
				rec := httptest.NewRecorder()
				return e.NewContext(req, rec), rec
			},
			expectError: false,
			description: "Should handle invalid IP addresses gracefully",
		},
		{
			name: "empty_ip_header",
			setupFunc: func() (echo.Context, *httptest.ResponseRecorder) {
				e := echo.New()
				req := httptest.NewRequest(http.MethodGet, "/v1/feeds", nil)
				req.Header.Set("X-Real-IP", "")
				rec := httptest.NewRecorder()
				return e.NewContext(req, rec), rec
			},
			expectError: false,
			description: "Should handle empty IP header gracefully",
		},
	}

	config := DOSProtectionConfig{
		Enabled:     true,
		RateLimit:   10,
		BurstLimit:  10,
		WindowSize:  time.Minute,
		BlockDuration: 5 * time.Minute,
	}

	middleware := DOSProtectionMiddleware(config)
	handler := middleware(func(c echo.Context) error {
		return c.String(http.StatusOK, "OK")
	})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, rec := tt.setupFunc()
			
			err := handler(c)
			
			if tt.expectError {
				assert.Error(t, err, tt.description)
			} else {
				assert.NoError(t, err, tt.description)
				assert.Equal(t, http.StatusOK, rec.Code, tt.description)
			}
		})
	}
}

// Helper struct for test requests
type testRequest struct {
	ip     string
	path   string
	method string
}

func TestDOSProtectionConfig_Validation(t *testing.T) {
	tests := []struct {
		name          string
		config        DOSProtectionConfig
		expectValid   bool
		description   string
	}{
		{
			name: "valid_config",
			config: DOSProtectionConfig{
				Enabled:     true,
				RateLimit:   10,
				BurstLimit:  20,
				WindowSize:  time.Minute,
				BlockDuration: 5 * time.Minute,
			},
			expectValid: true,
			description: "Valid configuration should pass validation",
		},
		{
			name: "invalid_rate_limit",
			config: DOSProtectionConfig{
				Enabled:     true,
				RateLimit:   0,
				BurstLimit:  10,
				WindowSize:  time.Minute,
				BlockDuration: 5 * time.Minute,
			},
			expectValid: false,
			description: "Zero rate limit should be invalid",
		},
		{
			name: "invalid_burst_limit",
			config: DOSProtectionConfig{
				Enabled:     true,
				RateLimit:   10,
				BurstLimit:  0,
				WindowSize:  time.Minute,
				BlockDuration: 5 * time.Minute,
			},
			expectValid: false,
			description: "Zero burst limit should be invalid",
		},
		{
			name: "burst_less_than_rate",
			config: DOSProtectionConfig{
				Enabled:     true,
				RateLimit:   10,
				BurstLimit:  5,
				WindowSize:  time.Minute,
				BlockDuration: 5 * time.Minute,
			},
			expectValid: false,
			description: "Burst limit should be >= rate limit",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.expectValid {
				assert.NoError(t, err, tt.description)
			} else {
				assert.Error(t, err, tt.description)
			}
		})
	}
}