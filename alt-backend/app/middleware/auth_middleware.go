package middleware

import (
	"errors"
	"fmt"
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

			// JWT validation succeeded - convert to domain.UserContext.
			// The subject claim must be a valid, non-nil UUID; otherwise identity
			// cannot be constructed safely and the request must be rejected.
			userID, err := parseSubjectUUID(jwtUserCtx.ID)
			if err != nil {
				if m.logger != nil {
					m.logger.Warn("JWT subject is not a valid UUID",
						"path", c.Request().URL.Path,
						"remote_addr", c.RealIP(),
						"error", err,
					)
				}
				return echo.NewHTTPError(http.StatusUnauthorized, "invalid subject claim")
			}
			// The tenant_id claim is authoritative for tenant scoping. In
			// single-tenant deployments auth-hub sets tenant_id == subject,
			// but we do not re-derive it here so that multi-tenant migration
			// is a single upstream change.
			tenantID, err := parseTenantUUID(jwtUserCtx.TenantID)
			if err != nil {
				if m.logger != nil {
					m.logger.Warn("JWT tenant_id claim is invalid",
						"path", c.Request().URL.Path,
						"remote_addr", c.RealIP(),
						"error", err,
					)
				}
				return echo.NewHTTPError(http.StatusUnauthorized, "invalid tenant_id claim")
			}
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
					// Valid JWT signature/issuer/audience. The subject must also be
					// a valid, non-nil UUID; otherwise no identity can be built and
					// the request continues as anonymous.
					userID, parseErr := parseSubjectUUID(jwtUserCtx.ID)
					if parseErr != nil {
						if m.logger != nil {
							m.logger.Warn("JWT subject is not a valid UUID, continuing as anonymous",
								"path", c.Request().URL.Path,
								"remote_addr", c.RealIP(),
								"error", parseErr,
							)
						}
						return next(c)
					}
					tenantID, tenantErr := parseTenantUUID(jwtUserCtx.TenantID)
					if tenantErr != nil {
						if m.logger != nil {
							m.logger.Warn("JWT tenant_id claim is invalid, continuing as anonymous",
								"path", c.Request().URL.Path,
								"remote_addr", c.RealIP(),
								"error", tenantErr,
							)
						}
						return next(c)
					}
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

// RequireAdmin ensures that the authenticated user has the admin role.
// Must be chained AFTER RequireAuth so that domain.UserContext is present.
// Returns 401 when no user context is attached, 403 when the user is not admin.
func (m *AuthMiddleware) RequireAdmin() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			user, err := domain.GetUserFromContext(c.Request().Context())
			if err != nil || user == nil {
				return echo.NewHTTPError(http.StatusUnauthorized, "authentication required")
			}
			if !user.IsAdmin() {
				if m.logger != nil {
					m.logger.Warn("admin-only endpoint accessed by non-admin",
						"path", c.Request().URL.Path,
						"user_id", user.UserID,
						"role", user.Role,
					)
				}
				return echo.NewHTTPError(http.StatusForbidden, "admin role required")
			}
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

// parseSubjectUUID parses a JWT subject claim into a UUID and rejects the nil UUID.
// A non-UUID or nil-UUID subject means identity cannot be constructed safely.
func parseSubjectUUID(sub string) (uuid.UUID, error) {
	id, err := uuid.Parse(sub)
	if err != nil {
		return uuid.Nil, fmt.Errorf("subject is not a valid UUID: %w", err)
	}
	if id == uuid.Nil {
		return uuid.Nil, errors.New("subject is the nil UUID")
	}
	return id, nil
}

// parseTenantUUID parses a JWT tenant_id claim into a UUID and rejects the
// nil UUID. The claim is required to be non-empty; absence is a separate
// error so callers can log "missing" distinctly from "malformed".
func parseTenantUUID(tenant string) (uuid.UUID, error) {
	if tenant == "" {
		return uuid.Nil, errors.New("tenant_id claim is missing")
	}
	id, err := uuid.Parse(tenant)
	if err != nil {
		return uuid.Nil, fmt.Errorf("tenant_id is not a valid UUID: %w", err)
	}
	if id == uuid.Nil {
		return uuid.Nil, errors.New("tenant_id is the nil UUID")
	}
	return id, nil
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
