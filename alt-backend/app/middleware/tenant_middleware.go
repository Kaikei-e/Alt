// alt-backend/app/middleware/tenant_middleware.go
package middleware

import (
	"context"
	"net/http"

	"alt/domain"

	"log/slog"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
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
		return func(c *echo.Context) error {
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
		return func(c *echo.Context) error {
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
		return func(c *echo.Context) error {
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

func (m *TenantMiddleware) checkRequestLimits(_ *echo.Context, _ *domain.Tenant) error {
	// TODO: implement feed creation and user invitation limit checks against tenant quotas.
	return nil
}
