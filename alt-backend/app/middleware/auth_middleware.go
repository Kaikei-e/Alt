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
	authService auth_port.AuthPort
	logger      *slog.Logger
}

// NewAuthMiddleware creates a new authentication middleware
func NewAuthMiddleware(authService auth_port.AuthPort, logger *slog.Logger) *AuthMiddleware {
	return &AuthMiddleware{
		authService: authService,
		logger:      logger,
	}
}

// RequireAuth returns authentication middleware that requires valid session
func (m *AuthMiddleware) RequireAuth() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			m.logger.Info("AUTH MIDDLEWARE: RequireAuth called", "path", c.Request().URL.Path, "method", c.Request().Method)
			
			// Cookieヘッダ全体を取得（TODO.md修正: Cookie素通しに変更）
			cookieHeader := c.Request().Header.Get("Cookie")
			if cookieHeader == "" {
				m.logger.Warn("AUTH MIDDLEWARE: No cookie header found")
				return echo.NewHTTPError(http.StatusUnauthorized, map[string]interface{}{
					"error":   "authentication_required",
					"message": "Authentication is required",
				})
			}

			m.logger.Info("AUTH MIDDLEWARE: Found cookie header", "cookie_length", len(cookieHeader))

			// auth-serviceでセッション検証（Cookieヘッダ全体を渡す）
			userContext, err := m.authService.ValidateSessionWithCookie(c.Request().Context(), cookieHeader)
			if err != nil {
				m.logger.Warn("AUTH MIDDLEWARE: Session validation failed", "error", err)
				return echo.NewHTTPError(http.StatusUnauthorized, map[string]interface{}{
					"error":   "invalid_session",
					"message": "Session validation failed",
				})
			}

			// コンテキストにユーザー情報を設定
			ctx := domain.SetUserContext(c.Request().Context(), userContext)
			c.SetRequest(c.Request().WithContext(ctx))

			m.logger.Info("AUTH MIDDLEWARE: User authenticated successfully",
				"user_id", userContext.UserID,
				"email", userContext.Email,
				"path", c.Request().URL.Path)

			return next(c)
		}
	}
}

func (m *AuthMiddleware) OptionalAuth() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			cookieHeader := c.Request().Header.Get("Cookie")
			if cookieHeader == "" {
				// セッションがない場合は続行（匿名ユーザー）
				return next(c)
			}

			// TODO.md修正: OptionalAuthでも認証失敗は401を返すべき場合がある
			// しかし、OptionalAuthでは認証失敗時も続行するため、エラーログのみ
			userContext, err := m.authService.ValidateSessionWithCookie(c.Request().Context(), cookieHeader)
			if err != nil {
				m.logger.Debug("optional auth failed, continuing without authentication", "error", err)
				// TODO.md修正: auth-serviceエラーでも続行（匿名ユーザーとして処理）
				return next(c)
			}

			ctx := domain.SetUserContext(c.Request().Context(), userContext)
			c.SetRequest(c.Request().WithContext(ctx))

			return next(c)
		}
	}
}

// GetUserContext extracts user context from request context
func GetUserContext(c echo.Context) (*domain.UserContext, error) {
	return domain.GetUserFromContext(c.Request().Context())
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
