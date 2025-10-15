package middleware

import (
	"context"
	"net/http"
	"strings"

	"pre-processor/internal/auth"

	"github.com/labstack/echo/v4"
)

type AuthMiddleware struct {
	authClient *auth.Client
}

func NewAuthMiddleware(authClient *auth.Client) *AuthMiddleware {
	return &AuthMiddleware{
		authClient: authClient,
	}
}

func (m *AuthMiddleware) RequireAuth() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			authHeader := c.Request().Header.Get("Authorization")
			if authHeader == "" {
				return echo.NewHTTPError(http.StatusUnauthorized, "Authorization header required")
			}

			tokenString := strings.TrimPrefix(authHeader, "Bearer ")
			userContext, err := m.authClient.ValidateUserToken(c.Request().Context(), tokenString)
			if err != nil {
				return echo.NewHTTPError(http.StatusUnauthorized, "Invalid token")
			}

			// コンテキストにユーザー情報を設定
			ctx := context.WithValue(c.Request().Context(), "user", userContext)
			c.SetRequest(c.Request().WithContext(ctx))

			return next(c)
		}
	}
}

func (m *AuthMiddleware) RequireServiceAuth() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// サービス間通信用認証
			serviceHeader := c.Request().Header.Get("X-Service-Token")
			if serviceHeader == "" {
				return echo.NewHTTPError(http.StatusUnauthorized, "Service token required")
			}

			// サービストークン検証ロジック
			// 簡易実装：実際にはJWT検証が必要
			if serviceHeader == "" {
				return echo.NewHTTPError(http.StatusUnauthorized, "Invalid service token")
			}

			return next(c)
		}
	}
}

// オプショナル認証ミドルウェア
func (m *AuthMiddleware) OptionalAuth() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			authHeader := c.Request().Header.Get("Authorization")
			if authHeader == "" {
				// 認証なしで続行
				return next(c)
			}

			tokenString := strings.TrimPrefix(authHeader, "Bearer ")
			userContext, err := m.authClient.ValidateUserToken(c.Request().Context(), tokenString)
			if err != nil {
				// 認証失敗でも続行
				return next(c)
			}

			// コンテキストにユーザー情報を設定
			ctx := context.WithValue(c.Request().Context(), "user", userContext)
			c.SetRequest(c.Request().WithContext(ctx))

			return next(c)
		}
	}
}
