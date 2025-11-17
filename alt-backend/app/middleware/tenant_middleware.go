// alt-backend/app/middleware/tenant_middleware.go
package middleware

import (
	"context"
	"net/http"
	"strings"

	"alt/domain"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"log/slog"
)

type TenantMiddleware struct {
	tenantService TenantService
	logger        *slog.Logger
}

type TenantService interface {
	GetTenant(ctx context.Context, tenantID uuid.UUID) (*domain.Tenant, error)
	GetTenantBySlug(ctx context.Context, slug string) (*domain.Tenant, error)
	GetTenantUsage(ctx context.Context, tenantID uuid.UUID) (*domain.TenantUsage, error)
}

func NewTenantMiddleware(tenantService TenantService, logger *slog.Logger) *TenantMiddleware {
	return &TenantMiddleware{
		tenantService: tenantService,
		logger:        logger,
	}
}

func (m *TenantMiddleware) ExtractTenant() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			user, err := domain.GetUserFromContext(c.Request().Context())
			if err != nil {
				m.logger.Warn("user context not found in tenant middleware", "error", err)
				return echo.NewHTTPError(http.StatusUnauthorized, "User context required")
			}

			// テナント情報を取得
			tenant, err := m.tenantService.GetTenant(c.Request().Context(), user.TenantID)
			if err != nil {
				m.logger.Error("failed to get tenant",
					"error", err,
					"tenant_id", user.TenantID,
					"user_id", user.UserID)
				return echo.NewHTTPError(http.StatusForbidden, "Invalid tenant")
			}

			// テナント状態チェック
			if tenant.Status != domain.TenantStatusActive {
				m.logger.Warn("inactive tenant access attempted",
					"tenant_id", tenant.ID,
					"tenant_status", tenant.Status,
					"user_id", user.UserID)
				return echo.NewHTTPError(http.StatusForbidden, "Tenant not active")
			}

			// コンテキストにテナント情報を設定
			ctx := domain.SetTenantContext(c.Request().Context(), tenant)
			c.SetRequest(c.Request().WithContext(ctx))

			m.logger.Debug("tenant context set successfully",
				"tenant_id", tenant.ID,
				"tenant_name", tenant.Name,
				"user_id", user.UserID)

			return next(c)
		}
	}
}

func (m *TenantMiddleware) RequireTenantAdmin() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			user, err := domain.GetUserFromContext(c.Request().Context())
			if err != nil {
				return echo.NewHTTPError(http.StatusUnauthorized, "Authentication required")
			}

			if user.Role != domain.UserRoleTenantAdmin {
				m.logger.Warn("non-admin tenant access attempted",
					"user_id", user.UserID,
					"tenant_id", user.TenantID,
					"user_role", user.Role)
				return echo.NewHTTPError(http.StatusForbidden, "Tenant admin role required")
			}

			return next(c)
		}
	}
}

func (m *TenantMiddleware) CheckTenantLimits() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			tenant, err := domain.GetTenantFromContext(c.Request().Context())
			if err != nil {
				return echo.NewHTTPError(http.StatusForbidden, "Tenant context required")
			}

			// リクエストに応じた制限チェック
			if err := m.checkRequestLimits(c, tenant); err != nil {
				m.logger.Warn("tenant limit exceeded",
					"tenant_id", tenant.ID,
					"limit_type", err.Error())
				return echo.NewHTTPError(http.StatusTooManyRequests, err.Error())
			}

			return next(c)
		}
	}
}

func (m *TenantMiddleware) checkRequestLimits(c echo.Context, tenant *domain.Tenant) error {
	path := c.Request().URL.Path
	method := c.Request().Method

	// フィード作成制限チェック
	if method == "POST" && strings.Contains(path, "/feeds") {
		// TODO: 実際の制限チェックロジックを実装
		// 現在のフィード数を取得して MaxFeeds と比較
	}

	// ユーザー招待制限チェック
	if method == "POST" && strings.Contains(path, "/users/invite") {
		// TODO: 実際の制限チェックロジックを実装
		// 現在のユーザー数を取得して MaxUsers と比較
	}

	return nil
}
