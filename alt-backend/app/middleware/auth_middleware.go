package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"alt/config"
	"alt/domain"
	"alt/port/auth_port"
)

type ValidateOKResponse struct {
	Valid      bool   `json:"valid"`
	SessionID  string `json:"session_id,omitempty"`
	IdentityID string `json:"identity_id,omitempty"`
	Email      string `json:"email,omitempty"`
	TenantID   string `json:"tenant_id,omitempty"`
	Role       string `json:"role,omitempty"`
}

type kratosSessionResponse struct {
	ID        string    `json:"id"`
	Active    bool      `json:"active"`
	ExpiresAt time.Time `json:"expires_at"`
	Identity  struct {
		ID     string `json:"id"`
		Traits struct {
			Email string `json:"email"`
		} `json:"traits"`
		MetadataPublic map[string]any `json:"metadata_public"`
	} `json:"identity"`
}

// AuthMiddleware provides authentication middleware functionality
type AuthMiddleware struct {
	authGateway       auth_port.AuthPort
	logger            *slog.Logger
	kratosInternalURL string
	httpClient        *http.Client
	config            *config.Config
}

// NewAuthMiddleware creates a new authentication middleware
func NewAuthMiddleware(authGateway auth_port.AuthPort, logger *slog.Logger, cfg *config.Config) *AuthMiddleware {
	return &AuthMiddleware{
		authGateway:       authGateway,
		logger:            logger,
		kratosInternalURL: cfg.Auth.KratosInternalURL,
		config:            cfg,
		httpClient: &http.Client{
			Timeout: 2 * time.Second,
		},
	}
}

// RequireAuth returns authentication middleware with direct HTTP validation
func (m *AuthMiddleware) RequireAuth() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Generate or extract X-Request-Id for tracing
			requestID := c.Request().Header.Get("X-Request-Id")
			if requestID == "" {
				requestID = uuid.NewString()
				c.Request().Header.Set("X-Request-Id", requestID)
			}

			// Direct HTTP validation with auth-service
			directValid, directErr := m.validateSessionDirect(c.Request().Context(), c.Request(), c)
			valid := directValid
			err := directErr
			fallbackAttempted := false

			if err != nil || !valid {
				fallbackAttempted = true
				fallbackValid, fallbackErr := m.validateWithKratosDirect(c.Request().Context(), c.Request(), c)
				if fallbackErr != nil {
					m.logger.Warn("kratos fallback validation failed",
						"error", fallbackErr,
						"x_request_id", requestID)
					if m.isNetworkError(fallbackErr) || (err != nil && m.isNetworkError(err)) {
						return echo.NewHTTPError(http.StatusBadGateway, "auth unavailable")
					}
					return echo.NewHTTPError(http.StatusUnauthorized, "invalid session")
				}

				valid = fallbackValid
				if !valid {
					if err != nil && m.isNetworkError(err) {
						return echo.NewHTTPError(http.StatusBadGateway, "auth unavailable")
					}
					return echo.NewHTTPError(http.StatusUnauthorized, "invalid session")
				}
			}

			if _, ctxErr := domain.GetUserFromContext(c.Request().Context()); ctxErr != nil {
				if !fallbackAttempted {
					fallbackAttempted = true
					fallbackValid, fallbackErr := m.validateWithKratosDirect(c.Request().Context(), c.Request(), c)
					if fallbackErr != nil {
						m.logger.Warn("kratos fallback could not set user context",
							"error", fallbackErr,
							"x_request_id", requestID)
						if m.isNetworkError(fallbackErr) {
							return echo.NewHTTPError(http.StatusBadGateway, "auth unavailable")
						}
						return echo.NewHTTPError(http.StatusUnauthorized, "invalid session")
					}

					valid = fallbackValid
				}

				if !valid {
					return echo.NewHTTPError(http.StatusUnauthorized, "invalid session")
				}

				if _, verifyErr := domain.GetUserFromContext(c.Request().Context()); verifyErr != nil {
					m.logger.Warn("user context still missing after fallback",
						"error", verifyErr,
						"x_request_id", requestID)
					return echo.NewHTTPError(http.StatusUnauthorized, "invalid session")
				}
			}

			// Set minimal auth context for valid sessions
			c.Set("auth.valid", true)
			c.Set("x_request_id", requestID)
			return next(c)
		}
	}
}

// validateSessionDirect performs direct HTTP validation with auth-service
func (m *AuthMiddleware) validateSessionDirect(ctx context.Context, req *http.Request, c echo.Context) (bool, error) {
	validateURL := m.config.Auth.ServiceURL + "/v1/auth/validate"

	httpReq, err := http.NewRequestWithContext(ctx, "GET", validateURL, nil)
	if err != nil {
		return false, err
	}

	// Copy headers from original request
	httpReq.Header = req.Header.Clone()

	// Ensure X-Request-Id is propagated
	if httpReq.Header.Get("X-Request-Id") == "" {
		httpReq.Header.Set("X-Request-Id", uuid.NewString())
	}

	resp, err := m.httpClient.Do(httpReq)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return false, err
		}

		// Check for empty body (the problem we're fixing)
		if len(body) == 0 {
			// Feature flag fallback for empty 200 responses
			if m.config.Auth.ValidateEmpty200OK {
				m.logger.Warn("auth validate returned 200 with empty body, but feature flag allows fallback",
					"x_request_id", httpReq.Header.Get("X-Request-Id"))
				return true, nil
			}
			m.logger.Warn("auth validate returned 200 with empty body",
				"x_request_id", httpReq.Header.Get("X-Request-Id"))
			return false, nil
		}

		// Parse JSON response to validate contract
		var validateResp ValidateOKResponse
		if err := json.Unmarshal(body, &validateResp); err != nil {
			m.logger.Warn("auth validate returned invalid JSON",
				"error", err,
				"body_length", len(body),
				"x_request_id", httpReq.Header.Get("X-Request-Id"))
			return false, nil
		}

		if !validateResp.Valid {
			return false, nil
		}

		// TDD GREEN: Create and set UserContext from auth-service response
		if err := m.createAndSetUserContext(c, validateResp); err != nil {
			m.logger.Warn("failed to create user context from auth response",
				"error", err,
				"session_id", validateResp.SessionID,
				"x_request_id", httpReq.Header.Get("X-Request-Id"))
			// Continue with authentication success but without user context
		}

		return true, nil
	}

	if resp.StatusCode == http.StatusUnauthorized {
		return false, nil
	}

	// Other status codes are treated as service errors
	return false, echo.NewHTTPError(resp.StatusCode, "auth unexpected status")
}

func (m *AuthMiddleware) OptionalAuth() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			cookie := c.Request().Header.Get("Cookie")
			if cookie == "" {
				return next(c)
			}

			// Generate or extract X-Request-Id for tracing
			requestID := c.Request().Header.Get("X-Request-Id")
			if requestID == "" {
				requestID = uuid.NewString()
				c.Request().Header.Set("X-Request-Id", requestID)
			}

			// Try direct HTTP validation
			valid, err := m.validateSessionDirect(c.Request().Context(), c.Request(), c)
			fallbackAttempted := false
			if err != nil || !valid {
				fallbackAttempted = true
				var fallbackErr error
				valid, fallbackErr = m.validateWithKratosDirect(c.Request().Context(), c.Request(), c)
				if fallbackErr != nil {
					m.logger.Debug("optional auth kratos fallback error",
						"error", fallbackErr,
						"x_request_id", requestID)
					return next(c)
				}
			}

			if valid {
				if _, ctxErr := domain.GetUserFromContext(c.Request().Context()); ctxErr != nil && !fallbackAttempted {
					if fallbackValid, fallbackErr := m.validateWithKratosDirect(c.Request().Context(), c.Request(), c); fallbackErr == nil && fallbackValid {
						valid = true
					} else {
						valid = false
					}
				}
			}

			if valid {
				c.Set("auth.valid", true)
				c.Set("x_request_id", requestID)
			}

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

// isNetworkError checks if an error is a network/infrastructure error rather than an authentication error
func (m *AuthMiddleware) isNetworkError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()
	// Check for common network/infrastructure error patterns
	networkErrorPatterns := []string{
		"502",
		"503",
		"connection refused",
		"connection timeout",
		"context deadline exceeded",
		"no such host",
		"network is unreachable",
		"connection reset by peer",
		"failed to make request",
	}

	for _, pattern := range networkErrorPatterns {
		if strings.Contains(strings.ToLower(errStr), pattern) {
			return true
		}
	}

	return false
}

// validateWithKratosDirect validates session directly with Kratos as fallback
func (m *AuthMiddleware) validateWithKratosDirect(ctx context.Context, req *http.Request, c echo.Context) (bool, error) {
	if m.kratosInternalURL == "" {
		return false, nil
	}

	cookieHeader := req.Header.Get("Cookie")
	if cookieHeader == "" {
		return false, nil
	}

	kratosReq, err := http.NewRequestWithContext(ctx, "GET", m.kratosInternalURL+"/sessions/whoami", nil)
	if err != nil {
		return false, err
	}

	kratosReq.Header.Set("Cookie", cookieHeader)
	kratosReq.Header.Set("Accept", "application/json")

	resp, err := m.httpClient.Do(kratosReq)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode >= 500 {
			return false, fmt.Errorf("kratos returned status %d", resp.StatusCode)
		}
		return false, nil
	}

	var session kratosSessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		return false, err
	}

	if !session.Active {
		return false, nil
	}

	if err := m.createAndSetUserContextFromKratos(c, &session); err != nil {
		return false, err
	}

	m.logger.Info("kratos direct validation succeeded")
	return true, nil
}

// createAndSetUserContext creates UserContext from auth-service response and sets it in request context
func (m *AuthMiddleware) createAndSetUserContext(c echo.Context, validateResp ValidateOKResponse) error {
	// Parse UUIDs from auth-service response
	userID, err := uuid.Parse(validateResp.IdentityID)
	if err != nil {
		return fmt.Errorf("invalid user ID format: %w", err)
	}

	tenantID, err := uuid.Parse(validateResp.TenantID)
	if err != nil {
		return fmt.Errorf("invalid tenant ID format: %w", err)
	}

	// Create UserContext
	userContext := &domain.UserContext{
		UserID:      userID,
		Email:       validateResp.Email,
		Role:        domain.UserRole(validateResp.Role),
		TenantID:    tenantID,
		SessionID:   validateResp.SessionID,
		LoginAt:     time.Now(),                     // Best approximation since auth-service doesn't provide it
		ExpiresAt:   time.Now().Add(24 * time.Hour), // Default expiration, could be configurable
		Permissions: []string{},                     // Could be expanded in future
	}

	// Set UserContext in request context for TenantMiddleware and other handlers
	ctx := domain.SetUserContext(c.Request().Context(), userContext)
	c.SetRequest(c.Request().WithContext(ctx))

	// Also set in Echo context for legacy compatibility
	c.Set("user", userContext)

	m.logger.Debug("user context created and set",
		"user_id", userContext.UserID,
		"tenant_id", userContext.TenantID,
		"email", userContext.Email,
		"role", userContext.Role)

	return nil
}

func (m *AuthMiddleware) createAndSetUserContextFromKratos(c echo.Context, session *kratosSessionResponse) error {
	userID, err := uuid.Parse(session.Identity.ID)
	if err != nil {
		return fmt.Errorf("invalid kratos identity id: %w", err)
	}

	email := strings.TrimSpace(session.Identity.Traits.Email)
	if email == "" {
		return fmt.Errorf("kratos session missing email trait")
	}

	tenantID := uuid.Nil
	role := domain.UserRoleUser
	var permissions []string

	if session.Identity.MetadataPublic != nil {
		if tenantRaw, ok := session.Identity.MetadataPublic["tenant_id"].(string); ok && tenantRaw != "" {
			if parsedTenant, parseErr := uuid.Parse(tenantRaw); parseErr == nil {
				tenantID = parsedTenant
			}
		}
		if roleRaw, ok := session.Identity.MetadataPublic["role"].(string); ok && roleRaw != "" {
			role = domain.UserRole(roleRaw)
		}
		if permRaw, ok := session.Identity.MetadataPublic["permissions"].([]any); ok {
			permissions = make([]string, 0, len(permRaw))
			for _, p := range permRaw {
				if str, ok := p.(string); ok {
					permissions = append(permissions, str)
				}
			}
		}
	}

	expiresAt := session.ExpiresAt
	if expiresAt.IsZero() || !expiresAt.After(time.Now()) {
		expiresAt = time.Now().Add(24 * time.Hour)
	}

	userContext := &domain.UserContext{
		UserID:      userID,
		Email:       email,
		Role:        role,
		TenantID:    tenantID,
		SessionID:   session.ID,
		LoginAt:     time.Now(),
		ExpiresAt:   expiresAt,
		Permissions: permissions,
	}

	ctx := domain.SetUserContext(c.Request().Context(), userContext)
	c.SetRequest(c.Request().WithContext(ctx))
	c.Set("user", userContext)

	m.logger.Debug("user context created from kratos",
		"user_id", userContext.UserID,
		"tenant_id", userContext.TenantID,
		"email", userContext.Email,
		"role", userContext.Role)

	return nil
}
