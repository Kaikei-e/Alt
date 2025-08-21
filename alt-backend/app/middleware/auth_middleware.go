package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"alt/config"
	"alt/domain"
	"alt/port/auth_port"
)

type ValidateOKResponse struct {
	Valid      bool   `json:"valid"`
	SessionID  string `json:"session_id,omitempty"`
	IdentityID string `json:"identity_id,omitempty"`
	Email      string `json:"email,omitempty"`
	TenantID   string `json:"tenant_id,omitempty"`
	Role       string `json:"role,omitempty"`
}

// AuthMiddleware provides authentication middleware functionality
type AuthMiddleware struct {
	authGateway      auth_port.AuthPort
	logger           *slog.Logger
	kratosInternalURL string
	httpClient       *http.Client
	config           *config.Config
}

// NewAuthMiddleware creates a new authentication middleware
func NewAuthMiddleware(authGateway auth_port.AuthPort, logger *slog.Logger, cfg *config.Config) *AuthMiddleware {
	return &AuthMiddleware{
		authGateway:       authGateway,
		logger:            logger,
		kratosInternalURL: cfg.Auth.KratosInternalURL,
		config:            cfg,
		httpClient: &http.Client{
			Timeout: 2 * time.Second,
		},
	}
}

// RequireAuth returns authentication middleware with direct HTTP validation
func (m *AuthMiddleware) RequireAuth() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Generate or extract X-Request-Id for tracing
			requestID := c.Request().Header.Get("X-Request-Id")
			if requestID == "" {
				requestID = uuid.NewString()
				c.Request().Header.Set("X-Request-Id", requestID)
			}

			// Direct HTTP validation with auth-service
			valid, err := m.validateSessionDirect(c.Request().Context(), c.Request(), c)
			if err != nil {
				m.logger.Warn("auth validate transport error", 
					"error", err,
					"x_request_id", requestID)
				return echo.NewHTTPError(http.StatusBadGateway, "auth unavailable")
			}

			if !valid {
				return echo.NewHTTPError(http.StatusUnauthorized, "invalid session")
			}

			// Set minimal auth context for valid sessions
			c.Set("auth.valid", true)
			c.Set("x_request_id", requestID)
			return next(c)
		}
	}
}

// validateSessionDirect performs direct HTTP validation with auth-service
func (m *AuthMiddleware) validateSessionDirect(ctx context.Context, req *http.Request, c echo.Context) (bool, error) {
	validateURL := m.config.Auth.ServiceURL + "/v1/auth/validate"
	
	httpReq, err := http.NewRequestWithContext(ctx, "GET", validateURL, nil)
	if err != nil {
		return false, err
	}

	// Copy headers from original request
	httpReq.Header = req.Header.Clone()
	
	// Ensure X-Request-Id is propagated
	if httpReq.Header.Get("X-Request-Id") == "" {
		httpReq.Header.Set("X-Request-Id", uuid.NewString())
	}

	resp, err := m.httpClient.Do(httpReq)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return false, err
		}

		// Check for empty body (the problem we're fixing)
		if len(body) == 0 {
			// Feature flag fallback for empty 200 responses
			if m.config.Auth.ValidateEmpty200OK {
				m.logger.Warn("auth validate returned 200 with empty body, but feature flag allows fallback",
					"x_request_id", httpReq.Header.Get("X-Request-Id"))
				return true, nil
			}
			m.logger.Warn("auth validate returned 200 with empty body",
				"x_request_id", httpReq.Header.Get("X-Request-Id"))
			return false, nil
		}

		// Parse JSON response to validate contract
		var validateResp ValidateOKResponse
		if err := json.Unmarshal(body, &validateResp); err != nil {
			m.logger.Warn("auth validate returned invalid JSON",
				"error", err,
				"body_length", len(body),
				"x_request_id", httpReq.Header.Get("X-Request-Id"))
			return false, nil
		}

		if !validateResp.Valid {
			return false, nil
		}

		// TDD GREEN: Create and set UserContext from auth-service response
		if err := m.createAndSetUserContext(c, validateResp); err != nil {
			m.logger.Warn("failed to create user context from auth response",
				"error", err,
				"session_id", validateResp.SessionID,
				"x_request_id", httpReq.Header.Get("X-Request-Id"))
			// Continue with authentication success but without user context
		}

		return true, nil
	}

	if resp.StatusCode == http.StatusUnauthorized {
		return false, nil
	}

	// Other status codes are treated as service errors
	return false, echo.NewHTTPError(resp.StatusCode, "auth unexpected status")
}

func (m *AuthMiddleware) OptionalAuth() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			cookie := c.Request().Header.Get("Cookie")
			if cookie == "" {
				return next(c)
			}

			// Generate or extract X-Request-Id for tracing
			requestID := c.Request().Header.Get("X-Request-Id")
			if requestID == "" {
				requestID = uuid.NewString()
				c.Request().Header.Set("X-Request-Id", requestID)
			}

			// Try direct HTTP validation
			valid, err := m.validateSessionDirect(c.Request().Context(), c.Request(), c)
			if err != nil {
				m.logger.Debug("optional auth validation transport error",
					"error", err,
					"x_request_id", requestID)
				// Continue as anonymous user on transport errors
				return next(c)
			}

			if valid {
				c.Set("auth.valid", true)
				c.Set("x_request_id", requestID)
			}

			return next(c)
		}
	}
}

// Helper functions for backward compatibility (simplified)
func GetUserContext(c echo.Context) (*domain.UserContext, error) {
	user := c.Get("user")
	if user == nil {
		return nil, echo.NewHTTPError(http.StatusUnauthorized, "authentication required")
	}

	userContext, ok := user.(*domain.UserContext)
	if !ok {
		return nil, echo.NewHTTPError(http.StatusInternalServerError, "invalid user context")
	}

	return userContext, nil
}

// isNetworkError checks if an error is a network/infrastructure error rather than an authentication error
func (m *AuthMiddleware) isNetworkError(err error) bool {
	if err == nil {
		return false
	}
	
	errStr := err.Error()
	// Check for common network/infrastructure error patterns
	networkErrorPatterns := []string{
		"502",
		"503", 
		"connection refused",
		"connection timeout",
		"context deadline exceeded",
		"no such host",
		"network is unreachable",
		"connection reset by peer",
		"failed to make request",
	}
	
	for _, pattern := range networkErrorPatterns {
		if strings.Contains(strings.ToLower(errStr), pattern) {
			return true
		}
	}
	
	return false
}

// validateWithKratosDirect validates session directly with Kratos as fallback
func (m *AuthMiddleware) validateWithKratosDirect(ctx context.Context, cookieHeader string) bool {
	if m.kratosInternalURL == "" {
		return false
	}
	
	req, err := http.NewRequestWithContext(ctx, "GET", m.kratosInternalURL+"/sessions/whoami", nil)
	if err != nil {
		m.logger.Debug("failed to create kratos request", "error", err)
		return false
	}
	
	req.Header.Set("Cookie", cookieHeader)
	
	resp, err := m.httpClient.Do(req)
	if err != nil {
		m.logger.Debug("kratos direct request failed", "error", err)
		return false
	}
	defer resp.Body.Close()
	
	if resp.StatusCode == 200 {
		m.logger.Info("kratos direct validation succeeded")
		return true
	}
	
	m.logger.Debug("kratos direct validation failed", "status", resp.StatusCode)
	return false
}

// createAndSetUserContext creates UserContext from auth-service response and sets it in request context
func (m *AuthMiddleware) createAndSetUserContext(c echo.Context, validateResp ValidateOKResponse) error {
	// Parse UUIDs from auth-service response
	userID, err := uuid.Parse(validateResp.IdentityID)
	if err != nil {
		return fmt.Errorf("invalid user ID format: %w", err)
	}

	tenantID, err := uuid.Parse(validateResp.TenantID)
	if err != nil {
		return fmt.Errorf("invalid tenant ID format: %w", err)
	}

	// Create UserContext
	userContext := &domain.UserContext{
		UserID:      userID,
		Email:       validateResp.Email,
		Role:        domain.UserRole(validateResp.Role),
		TenantID:    tenantID,
		SessionID:   validateResp.SessionID,
		LoginAt:     time.Now(), // Best approximation since auth-service doesn't provide it
		ExpiresAt:   time.Now().Add(24 * time.Hour), // Default expiration, could be configurable
		Permissions: []string{}, // Could be expanded in future
	}

	// Set UserContext in request context for TenantMiddleware and other handlers
	ctx := domain.SetUserContext(c.Request().Context(), userContext)
	c.SetRequest(c.Request().WithContext(ctx))

	// Also set in Echo context for legacy compatibility
	c.Set("user", userContext)

	m.logger.Debug("user context created and set",
		"user_id", userContext.UserID,
		"tenant_id", userContext.TenantID,
		"email", userContext.Email,
		"role", userContext.Role)

	return nil
}
