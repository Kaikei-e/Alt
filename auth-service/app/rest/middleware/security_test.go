package middleware

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"auth-service/app/domain"
	"auth-service/app/port"
)

// TestCSRFAttackPrevention tests various CSRF attack scenarios
func TestCSRFAttackPrevention(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAuthUsecase := port.NewMockAuthUsecase(ctrl)
	mockKratosGateway := port.NewMockKratosGateway(ctrl)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	tests := []struct {
		name           string
		setupRequest   func(*http.Request, *echo.Context)
		setupMocks     func()
		expectedStatus int
		expectBlocked  bool
		description    string
	}{
		{
			name: "CSRF attack without token",
			setupRequest: func(req *http.Request, c *echo.Context) {
				req.Method = http.MethodPost
				// Set session but no CSRF token
				(*c).Set("session_context", &domain.SessionContext{
					SessionID: "session-123",
					IsActive:  true,
				})
			},
			setupMocks: func() {
				// No mocks needed as request should be blocked before validation
			},
			expectedStatus: http.StatusForbidden,
			expectBlocked:  true,
			description:    "POST request without CSRF token should be blocked",
		},
		{
			name: "CSRF attack with invalid token",
			setupRequest: func(req *http.Request, c *echo.Context) {
				req.Method = http.MethodPost
				req.Header.Set("X-CSRF-Token", "invalid-token")
				(*c).Set("session_context", &domain.SessionContext{
					SessionID: "session-123",
					IsActive:  true,
				})
			},
			setupMocks: func() {
				mockAuthUsecase.EXPECT().
					ValidateCSRFToken(gomock.Any(), "invalid-token", "session-123").
					Return(domain.ErrInvalidCSRFToken)
			},
			expectedStatus: http.StatusForbidden,
			expectBlocked:  true,
			description:    "POST request with invalid CSRF token should be blocked",
		},
		{
			name: "CSRF attack with expired token",
			setupRequest: func(req *http.Request, c *echo.Context) {
				req.Method = http.MethodPost
				req.Header.Set("X-CSRF-Token", "expired-token")
				(*c).Set("session_context", &domain.SessionContext{
					SessionID: "session-123",
					IsActive:  true,
				})
			},
			setupMocks: func() {
				mockAuthUsecase.EXPECT().
					ValidateCSRFToken(gomock.Any(), "expired-token", "session-123").
					Return(domain.ErrCSRFTokenExpired)
			},
			expectedStatus: http.StatusForbidden,
			expectBlocked:  true,
			description:    "POST request with expired CSRF token should be blocked",
		},
		{
			name: "CSRF attack with token from different session",
			setupRequest: func(req *http.Request, c *echo.Context) {
				req.Method = http.MethodPost
				req.Header.Set("X-CSRF-Token", "other-session-token")
				(*c).Set("session_context", &domain.SessionContext{
					SessionID: "session-123",
					IsActive:  true,
				})
			},
			setupMocks: func() {
				mockAuthUsecase.EXPECT().
					ValidateCSRFToken(gomock.Any(), "other-session-token", "session-123").
					Return(domain.ErrCSRFTokenMismatch)
			},
			expectedStatus: http.StatusForbidden,
			expectBlocked:  true,
			description:    "POST request with token from different session should be blocked",
		},
		{
			name: "Valid CSRF token should pass",
			setupRequest: func(req *http.Request, c *echo.Context) {
				req.Method = http.MethodPost
				req.Header.Set("X-CSRF-Token", "valid-token")
				(*c).Set("session_context", &domain.SessionContext{
					SessionID: "session-123",
					IsActive:  true,
				})
			},
			setupMocks: func() {
				mockAuthUsecase.EXPECT().
					ValidateCSRFToken(gomock.Any(), "valid-token", "session-123").
					Return(nil)
			},
			expectedStatus: http.StatusOK,
			expectBlocked:  false,
			description:    "POST request with valid CSRF token should pass",
		},
		{
			name: "GET request should pass without CSRF token",
			setupRequest: func(req *http.Request, c *echo.Context) {
				req.Method = http.MethodGet
				// No CSRF token needed for GET
				(*c).Set("session_context", &domain.SessionContext{
					SessionID: "session-123",
					IsActive:  true,
				})
			},
			setupMocks: func() {
				// No CSRF validation for GET requests
			},
			expectedStatus: http.StatusOK,
			expectBlocked:  false,
			description:    "GET request should not require CSRF token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			middleware := NewEnhancedCSRFMiddleware(mockAuthUsecase, mockKratosGateway, nil, logger)

			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			tt.setupRequest(req, &c)
			tt.setupMocks()

			handler := middleware.Middleware()(func(c echo.Context) error {
				return c.String(http.StatusOK, "success")
			})

			err := handler(c)

			if tt.expectBlocked {
				assert.Error(t, err, tt.description)
				httpErr, ok := err.(*echo.HTTPError)
				require.True(t, ok, "Expected HTTP error")
				assert.Equal(t, tt.expectedStatus, httpErr.Code, tt.description)
			} else {
				assert.NoError(t, err, tt.description)
				assert.Equal(t, tt.expectedStatus, rec.Code, tt.description)
			}
		})
	}
}

// TestSecurityHeaders tests security headers middleware
func TestSecurityHeaders(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	config := DefaultSecurityConfig()
	middleware := NewSecurityMiddleware(config, logger)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	handler := middleware.Middleware()(func(c echo.Context) error {
		return c.String(http.StatusOK, "success")
	})

	err := handler(c)
	require.NoError(t, err)

	// Check security headers are set
	headers := rec.Header()

	assert.Contains(t, headers.Get("Content-Security-Policy"), "default-src 'self'")
	assert.Contains(t, headers.Get("Strict-Transport-Security"), "max-age=31536000")
	assert.Equal(t, "DENY", headers.Get("X-Frame-Options"))
	assert.Equal(t, "nosniff", headers.Get("X-Content-Type-Options"))
	assert.Equal(t, "1; mode=block", headers.Get("X-XSS-Protection"))
	assert.Equal(t, "strict-origin-when-cross-origin", headers.Get("Referrer-Policy"))
	assert.Contains(t, headers.Get("Permissions-Policy"), "camera=()")
	assert.Equal(t, "", headers.Get("Server")) // Server header should be removed
}

// TestRateLimitingAttackPrevention tests rate limiting against attacks
func TestRateLimitingAttackPrevention(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	config := &RateLimitConfig{
		Rate:    1.0,  // Very low rate for testing
		Burst:   2,    // Small burst
		AuthRate: 0.5, // Even lower for auth
	}
	middleware := NewRateLimitMiddleware(config, logger)

	e := echo.New()

	// Apply rate limiting middleware
	e.Use(middleware.Middleware())

	tests := []struct {
		name           string
		requests       int
		expectedPassed int
		description    string
	}{
		{
			name:           "burst allowance",
			requests:       2,
			expectedPassed: 2,
			description:    "First 2 requests should pass (burst allowance)",
		},
		{
			name:           "rate limit exceeded",
			requests:       5,
			expectedPassed: 2, // Only burst should pass
			description:    "Additional requests should be rate limited",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			passed := 0

			for i := 0; i < tt.requests; i++ {
				req := httptest.NewRequest(http.MethodGet, "/test", nil)
				req.Header.Set("X-Real-IP", "192.168.1.100") // Same IP for all requests
				rec := httptest.NewRecorder()

				c := e.NewContext(req, rec)
				c.SetRequest(req)

				handler := func(c echo.Context) error {
					passed++
					return c.String(http.StatusOK, "success")
				}

				// Apply middleware directly
				err := middleware.Middleware()(handler)(c)

				if err == nil {
					// Request passed
					continue
				}

				// Check if it's a rate limit error
				httpErr, ok := err.(*echo.HTTPError)
				if ok && httpErr.Code == http.StatusTooManyRequests {
					// Rate limited, this is expected
					continue
				}

				// Unexpected error
				t.Errorf("Unexpected error: %v", err)
			}

			assert.Equal(t, tt.expectedPassed, passed, tt.description)
		})
	}
}

// TestSuspiciousRequestDetection tests detection of suspicious patterns
func TestSuspiciousRequestDetection(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	tests := []struct {
		name        string
		setupRequest func(*http.Request)
		expectSuspicious bool
		description string
	}{
		{
			name: "normal request",
			setupRequest: func(req *http.Request) {
				req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
			},
			expectSuspicious: false,
			description:     "Normal browser request should not be flagged",
		},
		{
			name: "sqlmap attack tool",
			setupRequest: func(req *http.Request) {
				req.Header.Set("User-Agent", "sqlmap/1.0")
			},
			expectSuspicious: true,
			description:     "SQLMap user agent should be flagged as suspicious",
		},
		{
			name: "path traversal attempt",
			setupRequest: func(req *http.Request) {
				req.URL.Path = "/api/../../../etc/passwd"
			},
			expectSuspicious: true,
			description:     "Path traversal attempt should be flagged",
		},
		{
			name: "sql injection in query",
			setupRequest: func(req *http.Request) {
				req.URL.RawQuery = "id=1' or '1'='1"
			},
			expectSuspicious: true,
			description:     "SQL injection pattern should be flagged",
		},
		{
			name: "burp suite scanner",
			setupRequest: func(req *http.Request) {
				req.Header.Set("User-Agent", "Burp Suite Professional")
			},
			expectSuspicious: true,
			description:     "Burp Suite user agent should be flagged",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			tt.setupRequest(req)
			c.SetRequest(req) // Update context with modified request

			// Test suspicious request detection
			isSuspicious := isSuspiciousRequest(c)
			assert.Equal(t, tt.expectSuspicious, isSuspicious, tt.description)
		})
	}
}

// TestSecurityMiddlewareIntegration tests the complete security middleware stack
func TestSecurityMiddlewareIntegration(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAuthUsecase := port.NewMockAuthUsecase(ctrl)
	mockKratosGateway := port.NewMockKratosGateway(ctrl)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Setup middleware stack
	e := echo.New()

	// Security headers
	securityMiddleware := NewSecurityMiddleware(DefaultSecurityConfig(), logger)
	e.Use(securityMiddleware.Middleware())

	// Rate limiting
	rateLimitMiddleware := NewRateLimitMiddleware(DefaultRateLimitConfig(), logger)
	e.Use(rateLimitMiddleware.Middleware())

	// Request logging with security monitoring
	e.Use(RequestLoggingMiddleware(logger))

	// CSRF protection
	csrfMiddleware := NewEnhancedCSRFMiddleware(mockAuthUsecase, mockKratosGateway, nil, logger)

	// Mock session validation for CSRF test
	sessionCtx := &domain.SessionContext{
		SessionID: "session-123",
		IsActive:  true,
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}

	mockKratosGateway.EXPECT().
		GetSession(gomock.Any(), "session-token").
		Return(&domain.KratosSession{
			ID:     "session-123",
			Active: true,
			ExpiresAt: time.Now().Add(1 * time.Hour),
			Identity: domain.KratosIdentity{
				ID: "user-123",
			},
		}, nil).
		AnyTimes()

	mockAuthUsecase.EXPECT().
		ValidateCSRFToken(gomock.Any(), "valid-token", "session-123").
		Return(nil).
		AnyTimes()

	// Setup protected route
	protectedGroup := e.Group("/api")
	protectedGroup.Use(csrfMiddleware.Middleware())
	protectedGroup.POST("/protected", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"message": "success"})
	})

	tests := []struct {
		name           string
		setupRequest   func(*http.Request)
		expectedStatus int
		checkHeaders   func(*testing.T, http.Header)
		description    string
	}{
		{
			name: "valid protected request",
			setupRequest: func(req *http.Request) {
				req.Method = http.MethodPost
				req.URL.Path = "/api/protected"
				req.Header.Set("X-CSRF-Token", "valid-token")
				req.AddCookie(&http.Cookie{
					Name:  "ory_kratos_session",
					Value: "session-token",
				})
			},
			expectedStatus: http.StatusOK,
			checkHeaders: func(t *testing.T, headers http.Header) {
				// Verify security headers are present
				assert.Contains(t, headers.Get("Content-Security-Policy"), "default-src")
				assert.Contains(t, headers.Get("Strict-Transport-Security"), "max-age")
				assert.Equal(t, "DENY", headers.Get("X-Frame-Options"))
			},
			description: "Valid protected request should succeed with security headers",
		},
		{
			name: "malicious request blocked",
			setupRequest: func(req *http.Request) {
				req.Method = http.MethodPost
				req.URL.Path = "/api/protected"
				req.Header.Set("User-Agent", "sqlmap/1.0")
				// No CSRF token - should be blocked
				req.AddCookie(&http.Cookie{
					Name:  "ory_kratos_session",
					Value: "session-token",
				})
			},
			expectedStatus: http.StatusForbidden,
			checkHeaders: func(t *testing.T, headers http.Header) {
				// Security headers should still be present even on blocked requests
				assert.Contains(t, headers.Get("Content-Security-Policy"), "default-src")
			},
			description: "Malicious request without CSRF should be blocked",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			tt.setupRequest(req)
			rec := httptest.NewRecorder()

			e.ServeHTTP(rec, req)

			assert.Equal(t, tt.expectedStatus, rec.Code, tt.description)
			if tt.checkHeaders != nil {
				tt.checkHeaders(t, rec.Header())
			}
		})
	}
}

// Benchmark tests for performance under security load
func BenchmarkSecurityMiddleware(b *testing.B) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	middleware := NewSecurityMiddleware(DefaultSecurityConfig(), logger)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		handler := middleware.Middleware()(func(c echo.Context) error {
			return c.String(http.StatusOK, "ok")
		})

		_ = handler(c)
	}
}

func BenchmarkSuspiciousRequestDetection(b *testing.B) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/test?id=1", nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (normal browser)")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = isSuspiciousRequest(c)
	}
}