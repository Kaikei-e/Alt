package middleware

import (
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"

	"alt/domain"
	"alt/port/auth_port"
)

// AuthMiddleware provides authentication middleware functionality
type AuthMiddleware struct {
	authGateway auth_port.AuthPort
	logger      *slog.Logger
}

// NewAuthMiddleware creates a new authentication middleware
func NewAuthMiddleware(authGateway auth_port.AuthPort, logger *slog.Logger) *AuthMiddleware {
	return &AuthMiddleware{
		authGateway: authGateway,
		logger:      logger,
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
