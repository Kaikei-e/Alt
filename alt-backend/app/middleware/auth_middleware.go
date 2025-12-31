package middleware

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"alt/config"
	"alt/domain"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

// Header constants for authentication.
// These headers are set by the edge proxy (nginx) after validating the user session.

const (
	userIDHeader       = "X-Alt-User-Id"
	tenantIDHeader     = "X-Alt-Tenant-Id"
	userEmailHeader    = "X-Alt-User-Email"
	userRoleHeader     = "X-Alt-User-Role"
	sessionIDHeader    = "X-Alt-Session-Id"
	sharedSecretHeader = "X-Alt-Shared-Secret"
)

// Authentication error types.
var (
	errMissingHeaders = errors.New("missing authentication headers")
	errInvalidUserID  = errors.New("invalid user identifier")
	errInvalidTenant  = errors.New("invalid tenant identifier")
	errInvalidSecret  = errors.New("invalid shared secret") //nolint:unused // Reserved for future use
)

// AuthMiddleware validates lightweight identity headers provided by the frontend.
// The backend no longer performs calls to external auth services and simply trusts
// the identity information forwarded by the edge components.
// During migration, supports both JWT tokens and shared secret authentication.
type AuthMiddleware struct {
	logger        *slog.Logger
	sharedSecret  string
	jwtMiddleware *JWTAuthMiddleware
	config        *config.Config
}

// NewAuthMiddleware constructs an AuthMiddleware instance.
func NewAuthMiddleware(logger *slog.Logger, sharedSecret string, cfg *config.Config) *AuthMiddleware {
	return &AuthMiddleware{
		logger:        logger,
		sharedSecret:  sharedSecret,
		jwtMiddleware: NewJWTAuthMiddleware(logger, cfg),
		config:        cfg,
	}
}

// RequireAuth ensures that identity headers are present and valid before
// allowing the request to proceed.
// During migration, supports both JWT tokens (preferred) and shared secret (legacy).
func (m *AuthMiddleware) RequireAuth() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Try JWT authentication first (new method)
			if jwtUserCtx, err := m.jwtMiddleware.validateJWT(c); err == nil {
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

			// Fallback to shared secret authentication (legacy method)
			if !m.validateSharedSecret(c) {
				return echo.NewHTTPError(http.StatusUnauthorized, "invalid authentication")
			}

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
			if m.logger != nil {
				m.logger.Debug("request authenticated via shared secret (legacy)",
					"user_id", userContext.UserID,
					"tenant_id", userContext.TenantID,
				)
			}
			return next(c)
		}
	}
}

// OptionalAuth attaches a user context only when identity headers are present.
// Requests without headers continue unauthenticated.
func (m *AuthMiddleware) OptionalAuth() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Verify shared secret first if headers are present?
			// Or always?
			// If we want to prevent direct access even for public endpoints that MIGHT have auth,
			// we should probably enforce it if we want to trust the headers.
			// But OptionalAuth is often used for "if you are logged in, good; if not, also good".
			// If the request comes from Nginx, it should have the secret.
			// If it comes from attacker directly, it won't.
			// If attacker sends no headers, they are anonymous. That's fine for OptionalAuth endpoints (assuming they are public).
			// If attacker sends headers but no secret, we MUST ignore the headers (or reject).
			// If we reject, we protect against "I am admin" lies.
			// So yes, if headers are present, secret MUST be present and valid.
			// If headers are NOT present, secret is optional?
			// Nginx will always send the secret if we configure it globally for /api/backend.
			// So we can enforce secret always.

			if !m.validateSharedSecret(c) {
				// If secret is missing/invalid, we treat as unauthenticated (anonymous)
				// BUT, if they tried to send auth headers, we should probably warn.
				// For safety, let's just return next(c) without context, effectively anonymous.
				// This is safe because extractUserContext won't be called or its result won't be used if we return here.
				// Wait, if we return next(c) here, we skip extractUserContext.
				return next(c)
			}

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

func (m *AuthMiddleware) validateSharedSecret(c echo.Context) bool {
	// If no secret is configured, we default to insecure (open) mode?
	// Or we fail open?
	// Implementation Plan says: "If this secret is not configured, the backend will reject all authenticated requests."
	// So if m.sharedSecret is empty, we should probably fail secure.
	if m.sharedSecret == "" {
		// Log warning?
		return false
	}

	providedSecret := c.Request().Header.Get(sharedSecretHeader)
	return providedSecret == m.sharedSecret
}
