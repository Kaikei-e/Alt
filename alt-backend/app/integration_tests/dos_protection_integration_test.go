package integration_tests

import (
	"alt/config"
	"alt/middleware"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDOSProtectionIntegration(t *testing.T) {
	tests := []struct {
		name              string
		config            middleware.DOSProtectionConfig
		requests          []IntegrationRequest
		expectedResponses []IntegrationResponse
		description       string
	}{
		{
			name: "basic_dos_protection_flow",
			config: middleware.DOSProtectionConfig{
				Enabled:          true,
				RateLimit:        3,
				BurstLimit:       5,
				WindowSize:       time.Minute,
				BlockDuration:    2 * time.Minute,
				WhitelistedPaths: []string{"/v1/health"},
				CircuitBreaker: middleware.CircuitBreakerConfig{
					Enabled:          false,
					FailureThreshold: 10,
					TimeoutDuration:  30 * time.Second,
					RecoveryTimeout:  60 * time.Second,
				},
			},
			requests: []IntegrationRequest{
				{ip: "192.168.1.1", path: "/v1/feeds", method: "GET", delay: 0},
				{ip: "192.168.1.1", path: "/v1/feeds", method: "GET", delay: 0},
				{ip: "192.168.1.1", path: "/v1/feeds", method: "GET", delay: 0},
				{ip: "192.168.1.1", path: "/v1/feeds", method: "GET", delay: 0},
				{ip: "192.168.1.1", path: "/v1/feeds", method: "GET", delay: 0},
				{ip: "192.168.1.1", path: "/v1/feeds", method: "GET", delay: 0},  // Should be blocked
				{ip: "192.168.1.2", path: "/v1/feeds", method: "GET", delay: 0},  // Different IP
				{ip: "192.168.1.1", path: "/v1/health", method: "GET", delay: 0}, // Whitelisted
			},
			expectedResponses: []IntegrationResponse{
				{statusCode: 200, body: "OK"},
				{statusCode: 200, body: "OK"},
				{statusCode: 200, body: "OK"},
				{statusCode: 200, body: "OK"},
				{statusCode: 200, body: "OK"},
				{statusCode: 429, body: "Too many requests"},
				{statusCode: 200, body: "OK"},
				{statusCode: 200, body: "OK"},
			},
			description: "Should allow burst of 5 requests, then block except for different IPs and whitelisted paths",
		},
		{
			name: "circuit_breaker_integration",
			config: middleware.DOSProtectionConfig{
				Enabled:       true,
				RateLimit:     100,
				BurstLimit:    100,
				WindowSize:    time.Minute,
				BlockDuration: 5 * time.Minute,
				CircuitBreaker: middleware.CircuitBreakerConfig{
					Enabled:          true,
					FailureThreshold: 3,
					TimeoutDuration:  30 * time.Second,
					RecoveryTimeout:  60 * time.Second,
				},
			},
			requests: []IntegrationRequest{
				{ip: "192.168.1.1", path: "/v1/feeds", method: "GET", delay: 0, forceError: true},
				{ip: "192.168.1.1", path: "/v1/feeds", method: "GET", delay: 0, forceError: true},
				{ip: "192.168.1.1", path: "/v1/feeds", method: "GET", delay: 0, forceError: true},
				{ip: "192.168.1.1", path: "/v1/feeds", method: "GET", delay: 0}, // Should be blocked by circuit breaker
				{ip: "192.168.1.2", path: "/v1/feeds", method: "GET", delay: 0}, // Different IP, should also be blocked
			},
			expectedResponses: []IntegrationResponse{
				{statusCode: 500, body: "Internal Server Error"},
				{statusCode: 500, body: "Internal Server Error"},
				{statusCode: 500, body: "Internal Server Error"},
				{statusCode: 503, body: "Service temporarily unavailable"},
				{statusCode: 503, body: "Service temporarily unavailable"},
			},
			description: "Should activate circuit breaker after 3 failures and block all requests",
		},
		{
			name: "disabled_dos_protection",
			config: middleware.DOSProtectionConfig{
				Enabled: false,
			},
			requests: []IntegrationRequest{
				{ip: "192.168.1.1", path: "/v1/feeds", method: "GET", delay: 0},
				{ip: "192.168.1.1", path: "/v1/feeds", method: "GET", delay: 0},
				{ip: "192.168.1.1", path: "/v1/feeds", method: "GET", delay: 0},
				{ip: "192.168.1.1", path: "/v1/feeds", method: "GET", delay: 0},
				{ip: "192.168.1.1", path: "/v1/feeds", method: "GET", delay: 0},
			},
			expectedResponses: []IntegrationResponse{
				{statusCode: 200, body: "OK"},
				{statusCode: 200, body: "OK"},
				{statusCode: 200, body: "OK"},
				{statusCode: 200, body: "OK"},
				{statusCode: 200, body: "OK"},
			},
			description: "Should allow all requests when DoS protection is disabled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create Echo instance with DOS protection middleware
			e := echo.New()

			// Add the DOS protection middleware
			e.Use(middleware.DOSProtectionMiddleware(tt.config))

			// Create test handlers
			e.GET("/v1/feeds", func(c echo.Context) error {
				// Check if this request should force an error
				reqIndex := getRequestIndex(c.Request())
				if reqIndex >= 0 && reqIndex < len(tt.requests) && tt.requests[reqIndex].forceError {
					return echo.NewHTTPError(http.StatusInternalServerError, "Internal Server Error")
				}
				return c.String(http.StatusOK, "OK")
			})

			e.GET("/v1/health", func(c echo.Context) error {
				return c.String(http.StatusOK, "OK")
			})

			// Execute requests and check responses
			for i, req := range tt.requests {
				if req.delay > 0 {
					time.Sleep(req.delay)
				}

				httpReq := httptest.NewRequest(req.method, req.path, nil)
				httpReq.Header.Set("X-Real-IP", req.ip)
				httpReq.Header.Set("X-Request-Index", fmt.Sprintf("%d", i))

				rec := httptest.NewRecorder()
				e.ServeHTTP(rec, httpReq)

				expected := tt.expectedResponses[i]
				assert.Equal(t, expected.statusCode, rec.Code,
					"Request %d failed: expected status %d, got %d. %s",
					i+1, expected.statusCode, rec.Code, tt.description)

				if expected.statusCode != 429 && expected.statusCode != 503 {
					// For non-error responses, check body
					assert.Contains(t, rec.Body.String(), expected.body,
						"Request %d failed: expected body to contain %s, got %s",
						i+1, expected.body, rec.Body.String())
				}
			}
		})
	}
}

func TestDOSProtectionConfigValidation(t *testing.T) {
	tests := []struct {
		name          string
		config        middleware.DOSProtectionConfig
		expectError   bool
		errorContains string
		description   string
	}{
		{
			name: "valid_config",
			config: middleware.DOSProtectionConfig{
				Enabled:       true,
				RateLimit:     10,
				BurstLimit:    20,
				WindowSize:    time.Minute,
				BlockDuration: 5 * time.Minute,
				CircuitBreaker: middleware.CircuitBreakerConfig{
					Enabled:          true,
					FailureThreshold: 5,
					TimeoutDuration:  30 * time.Second,
					RecoveryTimeout:  60 * time.Second,
				},
			},
			expectError: false,
			description: "Valid configuration should pass validation",
		},
		{
			name: "invalid_rate_limit",
			config: middleware.DOSProtectionConfig{
				Enabled:       true,
				RateLimit:     0,
				BurstLimit:    10,
				WindowSize:    time.Minute,
				BlockDuration: 5 * time.Minute,
			},
			expectError:   true,
			errorContains: "rate limit must be greater than 0",
			description:   "Zero rate limit should fail validation",
		},
		{
			name: "burst_less_than_rate",
			config: middleware.DOSProtectionConfig{
				Enabled:       true,
				RateLimit:     10,
				BurstLimit:    5,
				WindowSize:    time.Minute,
				BlockDuration: 5 * time.Minute,
			},
			expectError:   true,
			errorContains: "burst limit must be >= rate limit",
			description:   "Burst limit less than rate limit should fail validation",
		},
		{
			name: "disabled_config",
			config: middleware.DOSProtectionConfig{
				Enabled: false,
				// Other fields can be invalid when disabled
				RateLimit:     0,
				BurstLimit:    0,
				WindowSize:    0,
				BlockDuration: 0,
			},
			expectError: false,
			description: "Disabled configuration should skip validation",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()

			if tt.expectError {
				require.Error(t, err, tt.description)
				assert.Contains(t, err.Error(), tt.errorContains, tt.description)
			} else {
				require.NoError(t, err, tt.description)
			}
		})
	}
}

func TestDOSProtectionEnvironmentConfiguration(t *testing.T) {
	// Test environment variable loading
	originalEnv := map[string]string{
		"DOS_PROTECTION_ENABLED":            "true",
		"DOS_PROTECTION_RATE_LIMIT":         "50",
		"DOS_PROTECTION_BURST_LIMIT":        "100",
		"DOS_PROTECTION_WINDOW_SIZE":        "2m",
		"DOS_PROTECTION_BLOCK_DURATION":     "10m",
		"CIRCUIT_BREAKER_ENABLED":           "true",
		"CIRCUIT_BREAKER_FAILURE_THRESHOLD": "15",
		"CIRCUIT_BREAKER_TIMEOUT_DURATION":  "45s",
		"CIRCUIT_BREAKER_RECOVERY_TIMEOUT":  "2m",
	}

	// Set environment variables
	for key, value := range originalEnv {
		t.Setenv(key, value)
	}

	// Load configuration
	cfg, err := config.NewConfig()
	require.NoError(t, err)

	// Verify DOS protection configuration
	dosConfig := cfg.RateLimit.DOSProtection
	assert.Equal(t, true, dosConfig.Enabled)
	assert.Equal(t, 50, dosConfig.RateLimit)
	assert.Equal(t, 100, dosConfig.BurstLimit)
	assert.Equal(t, 2*time.Minute, dosConfig.WindowSize)
	assert.Equal(t, 10*time.Minute, dosConfig.BlockDuration)

	// Verify circuit breaker configuration
	cbConfig := dosConfig.CircuitBreaker
	assert.Equal(t, true, cbConfig.Enabled)
	assert.Equal(t, 15, cbConfig.FailureThreshold)
	assert.Equal(t, 45*time.Second, cbConfig.TimeoutDuration)
	assert.Equal(t, 2*time.Minute, cbConfig.RecoveryTimeout)
}

func TestDOSProtectionPerformance(t *testing.T) {
	config := middleware.DOSProtectionConfig{
		Enabled:       true,
		RateLimit:     1000,
		BurstLimit:    2000,
		WindowSize:    time.Minute,
		BlockDuration: 5 * time.Minute,
	}

	e := echo.New()
	e.Use(middleware.DOSProtectionMiddleware(config))
	e.GET("/v1/test", func(c echo.Context) error {
		return c.String(http.StatusOK, "OK")
	})

	// Benchmark the middleware performance
	start := time.Now()
	const numRequests = 1000

	for i := 0; i < numRequests; i++ {
		req := httptest.NewRequest(http.MethodGet, "/v1/test", nil)
		req.Header.Set("X-Real-IP", fmt.Sprintf("192.168.1.%d", i%100)) // Simulate 100 different IPs
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		// All requests should succeed due to high limits
		assert.Equal(t, http.StatusOK, rec.Code)
	}

	duration := time.Since(start)
	requestsPerSecond := float64(numRequests) / duration.Seconds()

	// Should handle at least 1000 requests per second
	assert.Greater(t, requestsPerSecond, 1000.0,
		"DOS protection middleware should handle at least 1000 requests/second, got %.2f", requestsPerSecond)

	t.Logf("DOS protection middleware performance: %.2f requests/second", requestsPerSecond)
}

// Helper types and functions
type IntegrationRequest struct {
	ip         string
	path       string
	method     string
	delay      time.Duration
	forceError bool
}

type IntegrationResponse struct {
	statusCode int
	body       string
}

func getRequestIndex(req *http.Request) int {
	indexStr := req.Header.Get("X-Request-Index")
	if indexStr == "" {
		return -1
	}

	var index int
	if err := json.Unmarshal([]byte(indexStr), &index); err != nil {
		return -1
	}

	return index
}
