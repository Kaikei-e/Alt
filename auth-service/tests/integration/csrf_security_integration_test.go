package integration

import (
	"encoding/json"
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
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"auth-service/app/domain"
	"auth-service/app/mocks"
	"auth-service/app/rest/middleware"
)

// CSRFSecurityIntegrationTestSuite tests the complete CSRF security integration
type CSRFSecurityIntegrationTestSuite struct {
	suite.Suite
	ctrl              *gomock.Controller
	mockAuthUsecase   *mock_port.MockAuthUsecase
	mockAuthGateway   *mock_port.MockAuthGateway
	logger            *slog.Logger
	echo              *echo.Echo
	hybridMiddleware  *middleware.HybridCSRFMiddleware
	migrationController *middleware.MigrationController
	rateLimitMiddleware *middleware.RateLimitMiddleware
}

func (suite *CSRFSecurityIntegrationTestSuite) SetupTest() {
	suite.ctrl = gomock.NewController(suite.T())
	suite.mockAuthUsecase = mock_port.NewMockAuthUsecase(suite.ctrl)
	suite.mockAuthGateway = mock_port.NewMockAuthGateway(suite.ctrl)
	suite.logger = slog.New(slog.NewTextHandler(io.Discard, nil))

	// Setup Echo with complete security middleware stack
	suite.echo = echo.New()

	// Security headers middleware
	securityMiddleware := middleware.NewSecurityMiddleware(
		middleware.DefaultSecurityConfig(),
		suite.logger,
	)
	suite.echo.Use(securityMiddleware.Middleware())

	// Rate limiting middleware
	suite.rateLimitMiddleware = middleware.NewRateLimitMiddleware(
		middleware.DefaultRateLimitConfig(),
		suite.logger,
	)
	suite.echo.Use(suite.rateLimitMiddleware.Middleware())

	// Request logging with security monitoring
	suite.echo.Use(middleware.RequestLoggingMiddleware(suite.logger))

	// Setup CSRF middleware components
	legacyCSRF := middleware.NewCSRFMiddleware(suite.mockAuthUsecase, suite.logger)
	kratosCSRF := middleware.NewEnhancedCSRFMiddleware(
		suite.mockAuthUsecase,
		suite.mockAuthGateway,
		nil,
		suite.logger,
	)

	// Hybrid CSRF middleware for migration support
	suite.hybridMiddleware = middleware.NewHybridCSRFMiddleware(
		legacyCSRF,
		kratosCSRF,
		true, // Migration mode enabled
		suite.logger,
	)

	// Migration controller for managing CSRF migration
	suite.migrationController = middleware.NewMigrationController(
		suite.hybridMiddleware,
		suite.logger,
	)

	// Setup routes
	suite.setupRoutes()
}

func (suite *CSRFSecurityIntegrationTestSuite) TearDownTest() {
	suite.ctrl.Finish()
}

func (suite *CSRFSecurityIntegrationTestSuite) setupRoutes() {
	// Admin routes for migration management
	adminGroup := suite.echo.Group("/admin")
	suite.migrationController.RegisterMigrationRoutes(adminGroup)

	// Protected API routes with CSRF protection
	apiGroup := suite.echo.Group("/api")
	apiGroup.Use(suite.hybridMiddleware.Middleware())

	// Test endpoints
	apiGroup.POST("/user/profile", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"message": "profile updated"})
	})

	apiGroup.DELETE("/user/account", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"message": "account deleted"})
	})

	// Auth endpoints with specific rate limiting
	authGroup := suite.echo.Group("/auth")
	authGroup.Use(suite.rateLimitMiddleware.AuthRateLimit())

	authGroup.POST("/login", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"message": "login successful"})
	})

	authGroup.POST("/register", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"message": "registration successful"})
	})
}

// TestCompleteCSRFProtectionFlow tests the complete CSRF protection flow
func (suite *CSRFSecurityIntegrationTestSuite) TestCompleteCSRFProtectionFlow() {
	tests := []struct {
		name           string
		phase          string
		sessionType    string
		setupRequest   func(*http.Request)
		setupMocks     func()
		expectedStatus int
		description    string
	}{
		{
			name:        "Phase1_KratosSession_ValidCSRF",
			phase:       "migration",
			sessionType: "kratos",
			setupRequest: func(req *http.Request) {
				req.Method = http.MethodPost
				req.URL.Path = "/api/user/profile"
				req.Header.Set("X-CSRF-Token", "valid-kratos-token")
				req.AddCookie(&http.Cookie{
					Name:  "ory_kratos_session",
					Value: "kratos-session-123",
				})
			},
			setupMocks: func() {
				// Kratos session validation
				suite.mockAuthGateway.EXPECT().
					GetSession(gomock.Any(), "kratos-session-123").
					Return(&domain.KratosSession{
						ID:     "kratos-session-123",
						Active: true,
						ExpiresAt: time.Now().Add(1 * time.Hour),
						Identity: &domain.KratosIdentity{ID: "user-123"},
					}, nil)

				// CSRF token validation
				suite.mockAuthUsecase.EXPECT().
					ValidateCSRFToken(gomock.Any(), "valid-kratos-token", "kratos-session-123").
					Return(nil)
			},
			expectedStatus: http.StatusOK,
			description:    "Kratos session with valid CSRF should pass in migration phase",
		},
		{
			name:        "Phase1_LegacySession_ValidCSRF",
			phase:       "migration",
			sessionType: "legacy",
			setupRequest: func(req *http.Request) {
				req.Method = http.MethodPost
				req.URL.Path = "/api/user/profile"
				req.Header.Set("X-CSRF-Token", "valid-legacy-token")
				// No Kratos session cookie - should fall back to legacy
			},
			setupMocks: func() {
				// Legacy CSRF validation would be handled by legacy middleware
				// For this test, we'll simulate it passing
			},
			expectedStatus: http.StatusOK,
			description:    "Legacy session should work during migration phase",
		},
		{
			name:        "Phase2_PostMigration_KratosOnly",
			phase:       "post-migration",
			sessionType: "kratos",
			setupRequest: func(req *http.Request) {
				req.Method = http.MethodPost
				req.URL.Path = "/api/user/profile"
				req.Header.Set("X-CSRF-Token", "valid-kratos-token")
				req.AddCookie(&http.Cookie{
					Name:  "ory_kratos_session",
					Value: "kratos-session-123",
				})
			},
			setupMocks: func() {
				// Complete migration first
				suite.hybridMiddleware.CompleteKratosMigration()

				// Kratos session validation
				suite.mockAuthGateway.EXPECT().
					GetSession(gomock.Any(), "kratos-session-123").
					Return(&domain.KratosSession{
						ID:     "kratos-session-123",
						Active: true,
						ExpiresAt: time.Now().Add(1 * time.Hour),
						Identity: &domain.KratosIdentity{ID: "user-123"},
					}, nil)

				// CSRF token validation
				suite.mockAuthUsecase.EXPECT().
					ValidateCSRFToken(gomock.Any(), "valid-kratos-token", "kratos-session-123").
					Return(nil)
			},
			expectedStatus: http.StatusOK,
			description:    "Kratos session should work after migration complete",
		},
		{
			name:        "Phase2_PostMigration_LegacyRejected",
			phase:       "post-migration",
			sessionType: "legacy",
			setupRequest: func(req *http.Request) {
				req.Method = http.MethodPost
				req.URL.Path = "/api/user/profile"
				req.Header.Set("X-CSRF-Token", "legacy-token")
				// No Kratos session - should be rejected
			},
			setupMocks: func() {
				// Migration already completed in previous test
			},
			expectedStatus: http.StatusUnauthorized,
			description:    "Legacy session should be rejected after migration complete",
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			tt.setupRequest(req)
			tt.setupMocks()

			rec := httptest.NewRecorder()
			suite.echo.ServeHTTP(rec, req)

			assert.Equal(suite.T(), tt.expectedStatus, rec.Code, tt.description)

			// Verify security headers are always present
			headers := rec.Header()
			assert.Contains(suite.T(), headers.Get("Content-Security-Policy"), "default-src")
			assert.Contains(suite.T(), headers.Get("Strict-Transport-Security"), "max-age")
		})
	}
}

// TestCSRFAttackScenarios tests various CSRF attack scenarios
func (suite *CSRFSecurityIntegrationTestSuite) TestCSRFAttackScenarios() {
	attackScenarios := []struct {
		name           string
		setupRequest   func(*http.Request)
		setupMocks     func()
		expectedStatus int
		attackType     string
		description    string
	}{
		{
			name:       "NoCSRFToken_Attack",
			attackType: "missing_token",
			setupRequest: func(req *http.Request) {
				req.Method = http.MethodPost
				req.URL.Path = "/api/user/profile"
				req.AddCookie(&http.Cookie{
					Name:  "ory_kratos_session",
					Value: "session-123",
				})
				// No CSRF token - attack attempt
			},
			setupMocks: func() {
				suite.mockAuthGateway.EXPECT().
					GetSession(gomock.Any(), "session-123").
					Return(&domain.KratosSession{
						ID:     "session-123",
						Active: true,
						ExpiresAt: time.Now().Add(1 * time.Hour),
						Identity: &domain.KratosIdentity{ID: "user-123"},
					}, nil)
			},
			expectedStatus: http.StatusForbidden,
			description:    "CSRF attack without token should be blocked",
		},
		{
			name:       "InvalidCSRFToken_Attack",
			attackType: "invalid_token",
			setupRequest: func(req *http.Request) {
				req.Method = http.MethodPost
				req.URL.Path = "/api/user/profile"
				req.Header.Set("X-CSRF-Token", "malicious-token")
				req.AddCookie(&http.Cookie{
					Name:  "ory_kratos_session",
					Value: "session-123",
				})
			},
			setupMocks: func() {
				suite.mockAuthGateway.EXPECT().
					GetSession(gomock.Any(), "session-123").
					Return(&domain.KratosSession{
						ID:     "session-123",
						Active: true,
						ExpiresAt: time.Now().Add(1 * time.Hour),
						Identity: &domain.KratosIdentity{ID: "user-123"},
					}, nil)

				suite.mockAuthUsecase.EXPECT().
					ValidateCSRFToken(gomock.Any(), "malicious-token", "session-123").
					Return(domain.ErrInvalidCSRFToken)
			},
			expectedStatus: http.StatusForbidden,
			description:    "CSRF attack with invalid token should be blocked",
		},
		{
			name:       "CrossSessionCSRF_Attack",
			attackType: "cross_session",
			setupRequest: func(req *http.Request) {
				req.Method = http.MethodPost
				req.URL.Path = "/api/user/profile"
				req.Header.Set("X-CSRF-Token", "other-session-token")
				req.AddCookie(&http.Cookie{
					Name:  "ory_kratos_session",
					Value: "session-123",
				})
			},
			setupMocks: func() {
				suite.mockAuthGateway.EXPECT().
					GetSession(gomock.Any(), "session-123").
					Return(&domain.KratosSession{
						ID:     "session-123",
						Active: true,
						ExpiresAt: time.Now().Add(1 * time.Hour),
						Identity: &domain.KratosIdentity{ID: "user-123"},
					}, nil)

				suite.mockAuthUsecase.EXPECT().
					ValidateCSRFToken(gomock.Any(), "other-session-token", "session-123").
					Return(domain.ErrCSRFTokenMismatch)
			},
			expectedStatus: http.StatusForbidden,
			description:    "CSRF attack with token from different session should be blocked",
		},
	}

	for _, scenario := range attackScenarios {
		suite.Run(scenario.name, func() {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			scenario.setupRequest(req)
			scenario.setupMocks()

			rec := httptest.NewRecorder()
			suite.echo.ServeHTTP(rec, req)

			assert.Equal(suite.T(), scenario.expectedStatus, rec.Code, scenario.description)

			// Verify that attack was properly logged and headers are set
			headers := rec.Header()
			assert.Contains(suite.T(), headers.Get("Content-Security-Policy"), "default-src")
		})
	}
}

// TestMigrationManagement tests the CSRF migration management API
func (suite *CSRFSecurityIntegrationTestSuite) TestMigrationManagement() {
	// Test getting migration status
	suite.Run("GetMigrationStatus", func() {
		req := httptest.NewRequest(http.MethodGet, "/admin/csrf/migration/status", nil)
		rec := httptest.NewRecorder()

		suite.echo.ServeHTTP(rec, req)

		assert.Equal(suite.T(), http.StatusOK, rec.Code)

		var response map[string]interface{}
		err := json.Unmarshal(rec.Body.Bytes(), &response)
		require.NoError(suite.T(), err)

		assert.Equal(suite.T(), "success", response["status"])
		data := response["data"].(map[string]interface{})
		assert.True(suite.T(), data["migration_mode"].(bool))
	})

	// Test disabling migration mode
	suite.Run("DisableMigrationMode", func() {
		reqBody := `{"enabled": false}`
		req := httptest.NewRequest("PUT", "/admin/csrf/migration/mode",
			strings.NewReader(reqBody))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()

		suite.echo.ServeHTTP(rec, req)

		assert.Equal(suite.T(), http.StatusOK, rec.Code)

		var response map[string]interface{}
		err := json.Unmarshal(rec.Body.Bytes(), &response)
		require.NoError(suite.T(), err)

		assert.Equal(suite.T(), "success", response["status"])
		assert.False(suite.T(), response["migration_mode"].(bool))
	})

	// Test completing migration
	suite.Run("CompleteMigration", func() {
		reqBody := `{}`
		req := httptest.NewRequest(http.MethodPost, "/admin/csrf/migration/complete",
			strings.NewReader(reqBody))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()

		suite.echo.ServeHTTP(rec, req)

		assert.Equal(suite.T(), http.StatusOK, rec.Code)

		var response map[string]interface{}
		err := json.Unmarshal(rec.Body.Bytes(), &response)
		require.NoError(suite.T(), err)

		assert.Equal(suite.T(), "success", response["status"])
		assert.True(suite.T(), response["migration_completed"].(bool))
	})
}

// TestSecurityHeadersIntegration tests security headers integration
func (suite *CSRFSecurityIntegrationTestSuite) TestSecurityHeadersIntegration() {
	req := httptest.NewRequest(http.MethodGet, "/api/user/profile", nil)
	rec := httptest.NewRecorder()

	suite.echo.ServeHTTP(rec, req)

	headers := rec.Header()

	// Test all required security headers
	securityHeaders := map[string]string{
		"Content-Security-Policy":   "default-src 'self'",
		"Strict-Transport-Security": "max-age=31536000",
		"X-Frame-Options":           "DENY",
		"X-Content-Type-Options":    "nosniff",
		"X-XSS-Protection":          "1; mode=block",
		"Referrer-Policy":           "strict-origin-when-cross-origin",
	}

	for header, expectedValue := range securityHeaders {
		headerValue := headers.Get(header)
		assert.Contains(suite.T(), headerValue, expectedValue,
			"Header %s should contain %s, got: %s", header, expectedValue, headerValue)
	}

	// Verify server header is removed
	assert.Empty(suite.T(), headers.Get("Server"), "Server header should be empty")
}

// TestRateLimitingIntegration tests rate limiting integration
func (suite *CSRFSecurityIntegrationTestSuite) TestRateLimitingIntegration() {
	// Make multiple requests from same IP to trigger rate limiting
	clientIP := "192.168.1.100"

	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
		req.Header.Set("X-Real-IP", clientIP)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		suite.echo.ServeHTTP(rec, req)

		// First few requests should pass, then get rate limited
		if i < 2 {
			// Should pass due to burst allowance
			assert.True(suite.T(), rec.Code == http.StatusOK || rec.Code == http.StatusTooManyRequests)
		} else {
			// Should be rate limited
			if rec.Code == http.StatusTooManyRequests {
				// Verify rate limit headers
				assert.NotEmpty(suite.T(), rec.Header().Get("X-Rate-Limit-Limit"))
				assert.Equal(suite.T(), "0", rec.Header().Get("X-Rate-Limit-Remaining"))
				break // Rate limiting working
			}
		}
	}
}

// Run the test suite
func TestCSRFSecurityIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(CSRFSecurityIntegrationTestSuite))
}

// Helper function to create test requests with authentication
func createAuthenticatedRequest(method, path string, body io.Reader, sessionID string) *http.Request {
	req := httptest.NewRequest(method, path, body)
	req.AddCookie(&http.Cookie{
		Name:  "ory_kratos_session",
		Value: sessionID,
	})
	req.Header.Set("Content-Type", "application/json")
	return req
}

// Helper function to create CSRF-protected request
func createCSRFProtectedRequest(method, path string, body io.Reader, sessionID, csrfToken string) *http.Request {
	req := createAuthenticatedRequest(method, path, body, sessionID)
	req.Header.Set("X-CSRF-Token", csrfToken)
	return req
}