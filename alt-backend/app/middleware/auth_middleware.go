package middleware

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"alt/config"
	"alt/domain"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

// Header constants for authentication.
// These headers are set by the edge proxy (nginx) after validating the user session.

const (
	userIDHeader    = "X-Alt-User-Id"
	tenantIDHeader  = "X-Alt-Tenant-Id"
	userEmailHeader = "X-Alt-User-Email"
	userRoleHeader  = "X-Alt-User-Role"
	sessionIDHeader = "X-Alt-Session-Id"
)

// Authentication error types.
var (
	errMissingHeaders = errors.New("missing authentication headers")
	errInvalidUserID  = errors.New("invalid user identifier")
	errInvalidTenant  = errors.New("invalid tenant identifier")
)

// AuthMiddleware validates JWT tokens provided by the edge proxy or frontend.
// JWT is the sole authentication method.
type AuthMiddleware struct {
	logger        *slog.Logger
	jwtMiddleware *JWTAuthMiddleware
	config        *config.Config
}

// NewAuthMiddleware constructs an AuthMiddleware instance.
func NewAuthMiddleware(logger *slog.Logger, cfg *config.Config) *AuthMiddleware {
	return &AuthMiddleware{
		logger:        logger,
		jwtMiddleware: NewJWTAuthMiddleware(logger, cfg),
		config:        cfg,
	}
}

// RequireAuth ensures that a valid JWT token is present before allowing the request to proceed.
func (m *AuthMiddleware) RequireAuth() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			jwtUserCtx, err := m.jwtMiddleware.validateJWT(c)
			if err != nil {
				return echo.NewHTTPError(http.StatusUnauthorized, "invalid authentication")
			}

			// JWT validation succeeded - convert to domain.UserContext
			userID, _ := uuid.Parse(jwtUserCtx.ID)
			tenantID, _ := uuid.Parse(jwtUserCtx.ID) // Single-tenant: use userID as tenantID
			domainCtx := &domain.UserContext{
				UserID:    userID,
				Email:     jwtUserCtx.Email,
				Role:      parseRole(jwtUserCtx.Role),
				TenantID:  tenantID,
				SessionID: jwtUserCtx.Sid,
				LoginAt:   time.Now().UTC(),
				ExpiresAt: time.Now().UTC().Add(24 * time.Hour),
			}
			m.attachContext(c, domainCtx)
			if m.logger != nil {
				m.logger.Debug("request authenticated via JWT",
					"user_id", domainCtx.UserID,
					"tenant_id", domainCtx.TenantID,
				)
			}
			return next(c)
		}
	}
}

// OptionalAuth attaches a user context only when a valid JWT token is present.
// Requests without tokens continue unauthenticated.
// V-005 Security: If a JWT token is present but invalid, the request continues as anonymous.
func (m *AuthMiddleware) OptionalAuth() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Check if JWT token is present
			if c.Request().Header.Get(backendTokenHeader) != "" {
				if jwtUserCtx, err := m.jwtMiddleware.validateJWT(c); err == nil {
					// Valid JWT - attach user context
					userID, _ := uuid.Parse(jwtUserCtx.ID)
					tenantID, _ := uuid.Parse(jwtUserCtx.ID) // Single-tenant
					domainCtx := &domain.UserContext{
						UserID:    userID,
						Email:     jwtUserCtx.Email,
						Role:      parseRole(jwtUserCtx.Role),
						TenantID:  tenantID,
						SessionID: jwtUserCtx.Sid,
						LoginAt:   time.Now().UTC(),
						ExpiresAt: time.Now().UTC().Add(24 * time.Hour),
					}
					m.attachContext(c, domainCtx)
					return next(c)
				}
				// Invalid JWT - continue as anonymous (V-005 equivalent)
				if m.logger != nil {
					m.logger.Warn("invalid JWT token, continuing as anonymous",
						"path", c.Request().URL.Path,
						"remote_addr", c.RealIP(),
					)
				}
				return next(c)
			}

			// Check if identity headers are present without JWT (possible injection attempt)
			if m.hasAuthHeaders(c) {
				if m.logger != nil {
					m.logger.Warn("auth headers present without JWT token",
						"path", c.Request().URL.Path,
						"remote_addr", c.RealIP(),
					)
				}
				// Continue as anonymous - do NOT trust the headers
				return next(c)
			}

			// No auth headers - anonymous request
			return next(c)
		}
	}
}

func (m *AuthMiddleware) attachContext(c echo.Context, user *domain.UserContext) {
	ctx := domain.SetUserContext(c.Request().Context(), user)
	c.SetRequest(c.Request().WithContext(ctx))
	c.Set("user", user)
	c.Set("auth.valid", true)

	if m.logger != nil {
		m.logger.Debug("request authenticated via JWT",
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

	switch raw {
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

// hasAuthHeaders checks if any authentication identity headers are present.
// V-005: This is used to detect if someone is trying to inject identity headers.
func (m *AuthMiddleware) hasAuthHeaders(c echo.Context) bool {
	h := c.Request().Header
	return h.Get(userIDHeader) != "" ||
		h.Get(tenantIDHeader) != "" ||
		h.Get(userEmailHeader) != "" ||
		h.Get(userRoleHeader) != ""
}
