package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"alt/domain"
	"alt/port/auth_port"
)

// AuthMiddleware provides authentication middleware functionality
type AuthMiddleware struct {
	authGateway      auth_port.AuthPort
	logger           *slog.Logger
	kratosInternalURL string
	httpClient       *http.Client
}

// NewAuthMiddleware creates a new authentication middleware
func NewAuthMiddleware(authGateway auth_port.AuthPort, logger *slog.Logger, kratosInternalURL string) *AuthMiddleware {
	return &AuthMiddleware{
		authGateway:       authGateway,
		logger:            logger,
		kratosInternalURL: kratosInternalURL,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// RequireAuth returns authentication middleware per memo.md Phase 2.2
func (m *AuthMiddleware) RequireAuth() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			cookie := c.Request().Header.Get("Cookie")
			if cookie == "" {
				return c.NoContent(http.StatusUnauthorized)
			}

			// Use AuthGateway to validate session
			userContext, err := m.authGateway.ValidateSessionWithCookie(c.Request().Context(), cookie)
			if err != nil {
				// Check if this is a network/infrastructure error
				if m.isNetworkError(err) {
					m.logger.Warn("auth service network error, trying kratos fallback", "error", err)
					
					// Try Kratos direct fallback
					if m.validateWithKratosDirect(c.Request().Context(), cookie) {
						m.logger.Info("kratos direct validation succeeded, allowing request")
						// Continue with degraded auth context (we know session is valid but don't have full user details)
						c.Set("auth_degraded", true)
						return next(c)
					}
					
					// Network error and kratos also failed - return 503 with retry
					m.logger.Error("both auth-service and kratos failed", "error", err)
					return c.JSON(http.StatusServiceUnavailable, map[string]interface{}{
						"message": "auth-indeterminate", 
						"retry_after": 2,
					})
				}
				
				// This is a genuine authentication error (401)
				m.logger.Debug("session validation failed", "error", err)
				return c.NoContent(http.StatusUnauthorized)
			}

			// Set user context in Echo context for downstream handlers
			c.Set("user", userContext)
			c.Set("user_id", userContext.UserID.String())
			c.Set("user_email", userContext.Email)
			c.Set("user_role", string(userContext.Role))
			c.Set("session_id", userContext.SessionID)

			return next(c)
		}
	}
}

func (m *AuthMiddleware) OptionalAuth() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			cookie := c.Request().Header.Get("Cookie")
			if cookie == "" {
				// セッションがない場合は続行（匿名ユーザー）
				return next(c)
			}

			// Try to validate session using AuthGateway
			userContext, err := m.authGateway.ValidateSessionWithCookie(c.Request().Context(), cookie)
			if err != nil {
				// Check if this is a network error - try Kratos fallback for optional auth too
				if m.isNetworkError(err) {
					m.logger.Debug("auth service network error in optional auth, trying kratos fallback", "error", err)
					if m.validateWithKratosDirect(c.Request().Context(), cookie) {
						m.logger.Debug("kratos direct validation succeeded in optional auth")
						// Continue as authenticated but without full context (graceful degradation)
						c.Set("auth_degraded", true)
						return next(c)
					}
				}
				
				// 認証失敗時も続行（匿名ユーザーとして処理）
				m.logger.Debug("optional auth failed", "error", err)
				return next(c)
			}

			// Set user context if valid
			c.Set("user", userContext)
			c.Set("user_id", userContext.UserID.String())
			c.Set("user_email", userContext.Email)
			c.Set("user_role", string(userContext.Role))
			c.Set("session_id", userContext.SessionID)

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
