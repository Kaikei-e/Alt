package middleware

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"auth-service/app/port"
)

// AuthMiddleware provides authentication middleware
type AuthMiddleware struct {
	authUsecase port.AuthUsecase
	logger      *slog.Logger
}

// NewAuthMiddleware creates a new auth middleware
func NewAuthMiddleware(authUsecase port.AuthUsecase, logger *slog.Logger) *AuthMiddleware {
	return &AuthMiddleware{
		authUsecase: authUsecase,
		logger:      logger,
	}
}

// RequireAuth middleware that requires authentication
func (m *AuthMiddleware) RequireAuth() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ctx := c.Request().Context()

			// Extract session token
			sessionToken := m.extractSessionToken(c)
			if sessionToken == "" {
				return echo.NewHTTPError(http.StatusUnauthorized, "authentication required")
			}

			// Validate session
			sessionCtx, err := m.authUsecase.ValidateSession(ctx, sessionToken)
			if err != nil {
				m.logger.Error("session validation failed", "error", err)
				return echo.NewHTTPError(http.StatusUnauthorized, "invalid session")
			}

			// Set user context
			c.Set("user_id", sessionCtx.UserID.String())
			c.Set("tenant_id", sessionCtx.TenantID.String())
			c.Set("user_email", sessionCtx.Email)
			c.Set("user_name", sessionCtx.Name)
			c.Set("user_role", string(sessionCtx.Role))
			c.Set("session_id", sessionCtx.SessionID)

			return next(c)
		}
	}
}

// RequireRole middleware that requires specific role
func (m *AuthMiddleware) RequireRole(requiredRole string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			userRole := c.Get("user_role")
			if userRole == nil {
				return echo.NewHTTPError(http.StatusUnauthorized, "authentication required")
			}

			if userRole.(string) != requiredRole {
				return echo.NewHTTPError(http.StatusForbidden, "insufficient privileges")
			}

			return next(c)
		}
	}
}

// RequireAdmin middleware that requires admin role
func (m *AuthMiddleware) RequireAdmin() echo.MiddlewareFunc {
	return m.RequireRole("admin")
}

// OptionalAuth middleware that provides optional authentication
func (m *AuthMiddleware) OptionalAuth() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ctx := c.Request().Context()

			// Extract session token
			sessionToken := m.extractSessionToken(c)
			if sessionToken == "" {
				// No session, continue without auth
				return next(c)
			}

			// Try to validate session
			sessionCtx, err := m.authUsecase.ValidateSession(ctx, sessionToken)
			if err != nil {
				// Invalid session, log but continue
				m.logger.Debug("optional auth failed", "error", err)
				return next(c)
			}

			// Set user context if valid
			c.Set("user_id", sessionCtx.UserID.String())
			c.Set("tenant_id", sessionCtx.TenantID.String())
			c.Set("user_email", sessionCtx.Email)
			c.Set("user_name", sessionCtx.Name)
			c.Set("user_role", string(sessionCtx.Role))
			c.Set("session_id", sessionCtx.SessionID)

			return next(c)
		}
	}
}

// extractSessionToken extracts session token from request
// For browser requests, returns entire Cookie header
// For API requests, returns token from Authorization or X-Session-Token header
func (m *AuthMiddleware) extractSessionToken(c echo.Context) string {
	// Check if request has browser cookies (indicating browser session)
	if cookieHeader := c.Request().Header.Get("Cookie"); cookieHeader != "" && strings.Contains(cookieHeader, "ory_kratos_session") {
		return cookieHeader // Return entire cookie header for browser sessions
	}

	// Try Authorization header (for API clients)
	auth := c.Request().Header.Get("Authorization")
	if auth != "" {
		// Support both "Bearer token" and raw token formats
		if strings.HasPrefix(auth, "Bearer ") {
			return strings.TrimPrefix(auth, "Bearer ")
		}
		return auth
	}

	// Try X-Session-Token header (for API clients)
	return c.Request().Header.Get("X-Session-Token")
}