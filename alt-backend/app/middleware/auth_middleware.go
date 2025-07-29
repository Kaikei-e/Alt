package middleware

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"alt/domain"
	"alt/port/auth_port"
)

// AuthMiddleware provides authentication middleware functionality
type AuthMiddleware struct {
	authClient auth_port.AuthClient
	logger     *slog.Logger
}

// NewAuthMiddleware creates a new authentication middleware
func NewAuthMiddleware(authClient auth_port.AuthClient, logger *slog.Logger) *AuthMiddleware {
	return &AuthMiddleware{
		authClient: authClient,
		logger:     logger,
	}
}

// Middleware returns authentication middleware that requires valid session
func (m *AuthMiddleware) Middleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Extract session token
			sessionToken := m.extractSessionToken(c)
			if sessionToken == "" {
				m.logger.Warn("missing session token",
					"path", c.Request().URL.Path,
					"method", c.Request().Method,
					"remote_addr", c.Request().RemoteAddr)
				return echo.NewHTTPError(http.StatusUnauthorized, map[string]interface{}{
					"error":   "authentication_required",
					"message": "Session token is required",
				})
			}

			// Extract tenant ID from request (if available)
			tenantID := m.extractTenantID(c)

			// Validate session with auth-service
			response, err := m.authClient.ValidateSession(c.Request().Context(), sessionToken, tenantID)
			if err != nil {
				m.logger.Warn("session validation failed",
					"error", err,
					"path", c.Request().URL.Path,
					"remote_addr", c.Request().RemoteAddr)
				return echo.NewHTTPError(http.StatusUnauthorized, map[string]interface{}{
					"error":   "session_invalid",
					"message": "Invalid or expired session",
				})
			}

			if !response.Valid {
				m.logger.Warn("session validation returned invalid",
					"path", c.Request().URL.Path,
					"remote_addr", c.Request().RemoteAddr)
				return echo.NewHTTPError(http.StatusUnauthorized, map[string]interface{}{
					"error":   "session_invalid",
					"message": "Session is not valid",
				})
			}

			// Store user context in request context
			c.Set("user_context", response.Context)
			c.Set("user_id", response.UserID)
			c.Set("tenant_id", response.Context.TenantID.String())
			c.Set("user_email", response.Email)
			c.Set("user_role", response.Role)

			m.logger.Debug("authentication successful",
				"user_id", response.UserID,
				"user_email", response.Email,
				"path", c.Request().URL.Path)

			return next(c)
		}
	}
}

// OptionalAuth provides optional authentication (for public endpoints)
func (m *AuthMiddleware) OptionalAuth() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			sessionToken := m.extractSessionToken(c)
			if sessionToken != "" {
				tenantID := m.extractTenantID(c)

				response, err := m.authClient.ValidateSession(c.Request().Context(), sessionToken, tenantID)
				if err == nil && response.Valid {
					// Store user context if session is valid
					c.Set("user_context", response.Context)
					c.Set("user_id", response.UserID)
					c.Set("tenant_id", response.Context.TenantID.String())
					c.Set("user_email", response.Email)
					c.Set("user_role", response.Role)

					m.logger.Debug("optional authentication successful",
						"user_id", response.UserID,
						"path", c.Request().URL.Path)
				} else {
					m.logger.Debug("optional authentication failed, continuing without auth",
						"error", err,
						"path", c.Request().URL.Path)
				}
			}

			return next(c)
		}
	}
}

// extractSessionToken extracts session token from cookie or Authorization header
func (m *AuthMiddleware) extractSessionToken(c echo.Context) string {
	// Try cookie first (Kratos default session cookie)
	if cookie, err := c.Cookie("ory_kratos_session"); err == nil {
		return cookie.Value
	}

	// Try Authorization header as fallback
	auth := c.Request().Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}

	return ""
}

// extractTenantID extracts tenant ID from various sources
func (m *AuthMiddleware) extractTenantID(c echo.Context) string {
	// Try header first
	if tenantID := c.Request().Header.Get("X-Tenant-ID"); tenantID != "" {
		return tenantID
	}

	// Try query parameter
	if tenantID := c.QueryParam("tenant_id"); tenantID != "" {
		return tenantID
	}

	// Try path parameter (if defined in route)
	if tenantID := c.Param("tenant_id"); tenantID != "" {
		return tenantID
	}

	return ""
}

// GetUserContext extracts user context from echo context
func GetUserContext(c echo.Context) (*domain.UserContext, error) {
	userCtx, ok := c.Get("user_context").(*domain.UserContext)
	if !ok || userCtx == nil {
		return nil, fmt.Errorf("user context not found")
	}
	return userCtx, nil
}

// RequireAuth checks if user is authenticated and returns user context
func RequireAuth(c echo.Context) (*domain.UserContext, error) {
	userCtx, err := GetUserContext(c)
	if err != nil {
		return nil, echo.NewHTTPError(http.StatusUnauthorized, map[string]interface{}{
			"error":   "authentication_required",
			"message": "Authentication is required",
		})
	}
	return userCtx, nil
}

// RequireRole checks if user has the required role
func RequireRole(c echo.Context, requiredRole domain.UserRole) (*domain.UserContext, error) {
	userCtx, err := RequireAuth(c)
	if err != nil {
		return nil, err
	}

	if userCtx.Role != requiredRole {
		return nil, echo.NewHTTPError(http.StatusForbidden, map[string]interface{}{
			"error":   "insufficient_permissions",
			"message": "Required role: " + string(requiredRole),
		})
	}

	return userCtx, nil
}

// RequirePermission checks if user has the required permission
func RequirePermission(c echo.Context, permission string) (*domain.UserContext, error) {
	userCtx, err := RequireAuth(c)
	if err != nil {
		return nil, err
	}

	if !userCtx.HasPermission(permission) {
		return nil, echo.NewHTTPError(http.StatusForbidden, map[string]interface{}{
			"error":   "insufficient_permissions",
			"message": "Required permission: " + permission,
		})
	}

	return userCtx, nil
}
