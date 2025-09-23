package middleware

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"alt/domain"
)

const (
	userIDHeader    = "X-Alt-User-Id"
	tenantIDHeader  = "X-Alt-Tenant-Id"
	userEmailHeader = "X-Alt-User-Email"
	userRoleHeader  = "X-Alt-User-Role"
	sessionIDHeader = "X-Alt-Session-Id"
)

var (
	errMissingHeaders = errors.New("missing authentication headers")
	errInvalidUserID  = errors.New("invalid user identifier")
	errInvalidTenant  = errors.New("invalid tenant identifier")
)

// AuthMiddleware validates lightweight identity headers provided by the frontend.
// The backend no longer performs calls to external auth services and simply trusts
// the identity information forwarded by the edge components.
type AuthMiddleware struct {
	logger *slog.Logger
}

// NewAuthMiddleware constructs an AuthMiddleware instance.
func NewAuthMiddleware(logger *slog.Logger) *AuthMiddleware {
	return &AuthMiddleware{logger: logger}
}

// RequireAuth ensures that identity headers are present and valid before
// allowing the request to proceed.
func (m *AuthMiddleware) RequireAuth() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			userContext, err := m.extractUserContext(c)
			if err != nil {
				switch {
				case errors.Is(err, errMissingHeaders):
					return echo.NewHTTPError(http.StatusUnauthorized, "missing authentication headers")
				case errors.Is(err, errInvalidUserID), errors.Is(err, errInvalidTenant):
					return echo.NewHTTPError(http.StatusBadRequest, err.Error())
				default:
					return echo.NewHTTPError(http.StatusUnauthorized, "invalid authentication headers")
				}
			}

			m.attachContext(c, userContext)
			return next(c)
		}
	}
}

// OptionalAuth attaches a user context only when identity headers are present.
// Requests without headers continue unauthenticated.
func (m *AuthMiddleware) OptionalAuth() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			userContext, err := m.extractUserContext(c)
			if err != nil {
				if errors.Is(err, errMissingHeaders) {
					return next(c)
				}

				if m.logger != nil {
					m.logger.Debug("optional auth rejected identity headers", "error", err)
				}
				return echo.NewHTTPError(http.StatusUnauthorized, "invalid authentication headers")
			}

			m.attachContext(c, userContext)
			return next(c)
		}
	}
}

func (m *AuthMiddleware) extractUserContext(c echo.Context) (*domain.UserContext, error) {
	requestHeaders := c.Request().Header
	userIDValue := strings.TrimSpace(requestHeaders.Get(userIDHeader))
	tenantIDValue := strings.TrimSpace(requestHeaders.Get(tenantIDHeader))

	if userIDValue == "" && tenantIDValue == "" {
		return nil, errMissingHeaders
	}

	if userIDValue == "" || tenantIDValue == "" {
		return nil, errMissingHeaders
	}

	userID, err := uuid.Parse(userIDValue)
	if err != nil {
		return nil, errInvalidUserID
	}

	tenantID, err := uuid.Parse(tenantIDValue)
	if err != nil {
		return nil, errInvalidTenant
	}

	email := strings.TrimSpace(requestHeaders.Get(userEmailHeader))
	role := parseRole(strings.TrimSpace(requestHeaders.Get(userRoleHeader)))
	sessionID := strings.TrimSpace(requestHeaders.Get(sessionIDHeader))

	return &domain.UserContext{
		UserID:    userID,
		Email:     email,
		Role:      role,
		TenantID:  tenantID,
		SessionID: sessionID,
		LoginAt:   time.Now().UTC(),
		ExpiresAt: time.Now().UTC().Add(24 * time.Hour),
	}, nil
}

func (m *AuthMiddleware) attachContext(c echo.Context, user *domain.UserContext) {
	ctx := domain.SetUserContext(c.Request().Context(), user)
	c.SetRequest(c.Request().WithContext(ctx))
	c.Set("user", user)
	c.Set("auth.valid", true)

	if m.logger != nil {
		m.logger.Debug("request authenticated via headers",
			"user_id", user.UserID,
			"tenant_id", user.TenantID,
			"role", user.Role,
		)
	}
}

func parseRole(raw string) domain.UserRole {
	if raw == "" {
		return domain.UserRoleUser
	}

	switch strings.ToLower(raw) {
	case string(domain.UserRoleAdmin):
		return domain.UserRoleAdmin
	case string(domain.UserRoleTenantAdmin):
		return domain.UserRoleTenantAdmin
	case string(domain.UserRoleReadOnly):
		return domain.UserRoleReadOnly
	default:
		return domain.UserRoleUser
	}
}
