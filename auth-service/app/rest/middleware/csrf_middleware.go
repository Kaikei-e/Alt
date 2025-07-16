package middleware

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"auth-service/app/domain"
	"auth-service/app/port"
)

// CSRFConfig contains CSRF middleware configuration
type CSRFConfig struct {
	TokenHeader     string        // Default: "X-CSRF-Token"
	CookieName      string        // Default: "csrf_token"
	ContextKey      string        // Default: "csrf"
	CookiePath      string        // Default: "/"
	CookieDomain    string        // Default: ""
	CookieSecure    bool          // Default: true
	CookieHTTPOnly  bool          // Default: true
	CookieSameSite  http.SameSite // Default: SameSiteLaxMode
	TokenLength     int           // Default: 32
	TokenLookup     string        // Default: "header:X-CSRF-Token"
	IgnoreMethods   []string      // Default: GET, HEAD, OPTIONS, TRACE
}

// DefaultCSRFConfig returns default CSRF configuration
func DefaultCSRFConfig() *CSRFConfig {
	return &CSRFConfig{
		TokenHeader:    "X-CSRF-Token",
		CookieName:     "csrf_token",
		ContextKey:     "csrf",
		CookiePath:     "/",
		CookieSecure:   true,
		CookieHTTPOnly: true,
		CookieSameSite: http.SameSiteLaxMode,
		TokenLength:    32,
		TokenLookup:    "header:X-CSRF-Token",
		IgnoreMethods:  []string{"GET", "HEAD", "OPTIONS", "TRACE"},
	}
}

// EnhancedCSRFMiddleware provides CSRF protection integrated with Ory Kratos sessions
type EnhancedCSRFMiddleware struct {
	authUsecase   port.AuthUsecase
	authGateway   port.AuthGateway
	config        *CSRFConfig
	logger        *slog.Logger
}

// NewEnhancedCSRFMiddleware creates new CSRF middleware with Kratos integration
func NewEnhancedCSRFMiddleware(authUsecase port.AuthUsecase, authGateway port.AuthGateway, config *CSRFConfig, logger *slog.Logger) *EnhancedCSRFMiddleware {
	if config == nil {
		config = DefaultCSRFConfig()
	}

	return &EnhancedCSRFMiddleware{
		authUsecase:   authUsecase,
		authGateway:   authGateway,
		config:        config,
		logger:        logger,
	}
}

// Middleware returns the CSRF middleware function
func (m *EnhancedCSRFMiddleware) Middleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Skip CSRF for ignored methods
			if m.shouldSkip(c.Request().Method) {
				return next(c)
			}

			// Skip CSRF for certain endpoints (like login initiation)
			if m.shouldSkipPath(c.Path()) {
				return next(c)
			}

			// Get session from Kratos
			session, err := m.getSessionFromRequest(c)
			if err != nil {
				m.logger.Error("failed to get session", "error", err, "path", c.Path())
				return echo.NewHTTPError(http.StatusUnauthorized, "invalid session")
			}

			if session == nil {
				return echo.NewHTTPError(http.StatusUnauthorized, "session required")
			}

			// Validate CSRF token
			if err := m.validateCSRFToken(c, session); err != nil {
				m.logger.Error("CSRF validation failed",
					"sessionId", session.SessionID,
					"path", c.Path(),
					"method", c.Request().Method,
					"error", err)
				return echo.NewHTTPError(http.StatusForbidden, "CSRF token validation failed")
			}

			// Store session context
			c.Set("session", session)
			c.Set("user_id", session.UserID.String())
			c.Set("tenant_id", session.TenantID.String())
			c.Set("session_id", session.SessionID)

			return next(c)
		}
	}
}

// getSessionFromRequest extracts and validates session from request
func (m *EnhancedCSRFMiddleware) getSessionFromRequest(c echo.Context) (*domain.SessionContext, error) {
	// Try to get session cookie
	sessionCookie, err := c.Cookie("ory_kratos_session")
	if err != nil {
		// Try Authorization header
		authHeader := c.Request().Header.Get("Authorization")
		if authHeader == "" {
			return nil, echo.NewHTTPError(http.StatusUnauthorized, "session required")
		}

		// Extract Bearer token
		if !strings.HasPrefix(authHeader, "Bearer ") {
			return nil, echo.NewHTTPError(http.StatusUnauthorized, "invalid authorization header")
		}

		sessionToken := strings.TrimPrefix(authHeader, "Bearer ")
		return m.authUsecase.ValidateSession(c.Request().Context(), sessionToken)
	}

	// Validate session with Kratos
	return m.authUsecase.ValidateSession(c.Request().Context(), sessionCookie.Value)
}

// validateCSRFToken validates CSRF token from request
func (m *EnhancedCSRFMiddleware) validateCSRFToken(c echo.Context, session *domain.SessionContext) error {
	// Extract CSRF token from request
	token := m.extractCSRFToken(c)
	if token == "" {
		return echo.NewHTTPError(http.StatusForbidden, "CSRF token required")
	}

	// Validate token with auth service
	return m.authUsecase.ValidateCSRFToken(c.Request().Context(), token, session.SessionID)
}

// extractCSRFToken extracts CSRF token from request based on TokenLookup
func (m *EnhancedCSRFMiddleware) extractCSRFToken(c echo.Context) string {
	// Try header first
	if token := c.Request().Header.Get(m.config.TokenHeader); token != "" {
		return token
	}

	// Try form field
	if token := c.FormValue("csrf_token"); token != "" {
		return token
	}

	// Try query parameter
	if token := c.QueryParam("csrf_token"); token != "" {
		return token
	}

	return ""
}

// shouldSkip checks if CSRF validation should be skipped for the method
func (m *EnhancedCSRFMiddleware) shouldSkip(method string) bool {
	for _, ignoredMethod := range m.config.IgnoreMethods {
		if method == ignoredMethod {
			return true
		}
	}
	return false
}

// shouldSkipPath checks if CSRF should be skipped for certain endpoints
func (m *EnhancedCSRFMiddleware) shouldSkipPath(path string) bool {
	skipPaths := []string{
		"/v1/auth/login",      // Initial login flow
		"/v1/auth/register",   // Initial registration flow
		"/v1/health",          // Health checks
		"/v1/ready",           // Readiness checks
		"/v1/live",            // Liveness checks
	}

	for _, skipPath := range skipPaths {
		if strings.HasPrefix(path, skipPath) {
			return true
		}
	}
	return false
}

// CSRFTokenProvider provides CSRF token generation endpoint
func (m *EnhancedCSRFMiddleware) CSRFTokenProvider() echo.HandlerFunc {
	return func(c echo.Context) error {
		// Get session
		session, err := m.getSessionFromRequest(c)
		if err != nil {
			return echo.NewHTTPError(http.StatusUnauthorized, "session required")
		}

		// Generate CSRF token
		csrfToken, err := m.authUsecase.GenerateCSRFToken(c.Request().Context(), session.SessionID)
		if err != nil {
			m.logger.Error("failed to generate CSRF token", "sessionId", session.SessionID, "error", err)
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to generate CSRF token")
		}

		// Set CSRF cookie
		cookie := &http.Cookie{
			Name:     m.config.CookieName,
			Value:    csrfToken.Token,
			Path:     m.config.CookiePath,
			Domain:   m.config.CookieDomain,
			Secure:   m.config.CookieSecure,
			HttpOnly: m.config.CookieHTTPOnly,
			SameSite: m.config.CookieSameSite,
			Expires:  csrfToken.ExpiresAt,
		}
		c.SetCookie(cookie)

		return c.JSON(http.StatusOK, map[string]interface{}{
			"csrf_token": csrfToken.Token,
			"expires_at": csrfToken.ExpiresAt,
		})
	}
}

// Legacy CSRFMiddleware for backward compatibility during migration
type CSRFMiddleware struct {
	authUsecase port.AuthUsecase
	logger      *slog.Logger
}

// NewCSRFMiddleware creates a new CSRF middleware (legacy)
func NewCSRFMiddleware(authUsecase port.AuthUsecase, logger *slog.Logger) *CSRFMiddleware {
	return &CSRFMiddleware{
		authUsecase: authUsecase,
		logger:      logger,
	}
}

// RequireCSRF middleware that requires CSRF token validation (legacy)
func (m *CSRFMiddleware) RequireCSRF() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ctx := c.Request().Context()

			// Skip CSRF for safe methods
			if m.isSafeMethod(c.Request().Method) {
				return next(c)
			}

			// Skip CSRF for certain endpoints (like login initiation)
			if m.shouldSkipCSRF(c.Path()) {
				return next(c)
			}

			// Extract session ID from context
			sessionID := c.Get("session_id")
			if sessionID == nil {
				return echo.NewHTTPError(http.StatusUnauthorized, "session required for CSRF validation")
			}

			// Extract CSRF token
			csrfToken := m.extractCSRFToken(c)
			if csrfToken == "" {
				return echo.NewHTTPError(http.StatusForbidden, "CSRF token required")
			}

			// Validate CSRF token
			if err := m.authUsecase.ValidateCSRFToken(ctx, csrfToken, sessionID.(string)); err != nil {
				m.logger.Error("CSRF validation failed",
					"sessionId", sessionID,
					"path", c.Path(),
					"method", c.Request().Method,
					"error", err)
				return echo.NewHTTPError(http.StatusForbidden, "invalid CSRF token")
			}

			return next(c)
		}
	}
}

// OptionalCSRF middleware that provides optional CSRF validation (legacy)
func (m *CSRFMiddleware) OptionalCSRF() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ctx := c.Request().Context()

			// Skip CSRF for safe methods
			if m.isSafeMethod(c.Request().Method) {
				return next(c)
			}

			// Extract session ID from context
			sessionID := c.Get("session_id")
			if sessionID == nil {
				// No session, skip CSRF
				return next(c)
			}

			// Extract CSRF token
			csrfToken := m.extractCSRFToken(c)
			if csrfToken == "" {
				// No CSRF token, log warning but continue
				m.logger.Warn("CSRF token missing for state-changing operation",
					"sessionId", sessionID,
					"path", c.Path(),
					"method", c.Request().Method)
				return next(c)
			}

			// Validate CSRF token if present
			if err := m.authUsecase.ValidateCSRFToken(ctx, csrfToken, sessionID.(string)); err != nil {
				m.logger.Error("CSRF validation failed",
					"sessionId", sessionID,
					"path", c.Path(),
					"method", c.Request().Method,
					"error", err)
				return echo.NewHTTPError(http.StatusForbidden, "invalid CSRF token")
			}

			return next(c)
		}
	}
}

// Helper methods for legacy middleware
func (m *CSRFMiddleware) isSafeMethod(method string) bool {
	safeMethods := []string{"GET", "HEAD", "OPTIONS", "TRACE"}
	for _, safe := range safeMethods {
		if method == safe {
			return true
		}
	}
	return false
}

func (m *CSRFMiddleware) shouldSkipCSRF(path string) bool {
	skipPaths := []string{
		"/v1/auth/login",      // Initial login flow
		"/v1/auth/register",   // Initial registration flow
		"/v1/health",          // Health checks
		"/v1/ready",           // Readiness checks
		"/v1/live",            // Liveness checks
	}

	for _, skipPath := range skipPaths {
		if strings.HasPrefix(path, skipPath) {
			return true
		}
	}
	return false
}

func (m *CSRFMiddleware) extractCSRFToken(c echo.Context) string {
	// Try X-CSRF-Token header first
	if token := c.Request().Header.Get("X-CSRF-Token"); token != "" {
		return token
	}

	// Try form field
	if token := c.FormValue("csrf_token"); token != "" {
		return token
	}

	return ""
}

// HybridCSRFMiddleware supports both legacy and Kratos CSRF during migration
type HybridCSRFMiddleware struct {
	legacyCSRF    *CSRFMiddleware
	kratosCSRF    *EnhancedCSRFMiddleware
	migrationMode bool
	logger        *slog.Logger
}

// NewHybridCSRFMiddleware creates a hybrid CSRF middleware for migration
func NewHybridCSRFMiddleware(
	legacyCSRF *CSRFMiddleware,
	kratosCSRF *EnhancedCSRFMiddleware,
	migrationMode bool,
	logger *slog.Logger,
) *HybridCSRFMiddleware {
	return &HybridCSRFMiddleware{
		legacyCSRF:    legacyCSRF,
		kratosCSRF:    kratosCSRF,
		migrationMode: migrationMode,
		logger:        logger,
	}
}

// Middleware returns the hybrid CSRF middleware function
func (h *HybridCSRFMiddleware) Middleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Check if request has Kratos session
			if h.hasKratosSession(c) {
				// Use Kratos CSRF
				return h.kratosCSRF.Middleware()(next)(c)
			}

			// Fall back to legacy CSRF during migration
			if h.migrationMode {
				return h.legacyCSRF.RequireCSRF()(next)(c)
			}

			// Force Kratos session after migration
			return echo.NewHTTPError(http.StatusUnauthorized, "Kratos session required")
		}
	}
}

// hasKratosSession checks if request has a Kratos session
func (h *HybridCSRFMiddleware) hasKratosSession(c echo.Context) bool {
	// Check for Kratos session cookie
	if _, err := c.Cookie("ory_kratos_session"); err == nil {
		return true
	}

	// Check for Authorization header with Bearer token
	authHeader := c.Request().Header.Get("Authorization")
	return strings.HasPrefix(authHeader, "Bearer ")
}

// SetMigrationMode enables or disables migration mode
func (h *HybridCSRFMiddleware) SetMigrationMode(enabled bool) {
	h.migrationMode = enabled
	h.logger.Info("migration mode changed", "enabled", enabled)
}

// GetMigrationStatus returns current migration status
func (h *HybridCSRFMiddleware) GetMigrationStatus() map[string]interface{} {
	return map[string]interface{}{
		"migration_mode": h.migrationMode,
		"legacy_enabled": h.legacyCSRF != nil,
		"kratos_enabled": h.kratosCSRF != nil,
	}
}

// CompleteKratosMigration disables legacy support
func (h *HybridCSRFMiddleware) CompleteKratosMigration() {
	h.migrationMode = false
	h.legacyCSRF = nil // Remove legacy support
	h.logger.Info("CSRF migration to Kratos completed")
}

// MigrationController provides REST API endpoints to manage CSRF migration
type MigrationController struct {
	hybridMiddleware *HybridCSRFMiddleware
	logger           *slog.Logger
}

// NewMigrationController creates a new migration controller
func NewMigrationController(hybridMiddleware *HybridCSRFMiddleware, logger *slog.Logger) *MigrationController {
	return &MigrationController{
		hybridMiddleware: hybridMiddleware,
		logger:           logger,
	}
}

// GetMigrationStatus returns the current migration status
func (m *MigrationController) GetMigrationStatus(c echo.Context) error {
	status := m.hybridMiddleware.GetMigrationStatus()

	m.logger.Info("migration status requested", "status", status)
	return c.JSON(http.StatusOK, map[string]interface{}{
		"status": "success",
		"data":   status,
	})
}

// SetMigrationMode enables or disables migration mode
func (m *MigrationController) SetMigrationMode(c echo.Context) error {
	var req struct {
		Enabled bool `json:"enabled" validate:"required"`
	}

	if err := c.Bind(&req); err != nil {
		m.logger.Error("invalid migration mode request", "error", err)
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body")
	}

	m.hybridMiddleware.SetMigrationMode(req.Enabled)

	response := map[string]interface{}{
		"status":         "success",
		"migration_mode": req.Enabled,
		"message":        "Migration mode updated successfully",
	}

	m.logger.Info("migration mode updated", "enabled", req.Enabled, "by", c.Get("user_id"))
	return c.JSON(http.StatusOK, response)
}

// CompleteMigration completes the migration to Kratos CSRF
func (m *MigrationController) CompleteMigration(c echo.Context) error {
	// Check if migration can be completed safely
	status := m.hybridMiddleware.GetMigrationStatus()
	if !status["kratos_enabled"].(bool) {
		m.logger.Error("cannot complete migration: Kratos CSRF not enabled")
		return echo.NewHTTPError(http.StatusPreconditionFailed, "Kratos CSRF must be enabled before completing migration")
	}

	m.hybridMiddleware.CompleteKratosMigration()

	response := map[string]interface{}{
		"status":              "success",
		"migration_completed": true,
		"message":             "Migration to Kratos CSRF completed successfully",
	}

	m.logger.Info("Kratos migration completed", "by", c.Get("user_id"))
	return c.JSON(http.StatusOK, response)
}

// RegisterMigrationRoutes registers migration management routes
func (m *MigrationController) RegisterMigrationRoutes(adminGroup *echo.Group) {
	adminGroup.GET("/csrf/migration/status", m.GetMigrationStatus)
	adminGroup.PUT("/csrf/migration/mode", m.SetMigrationMode)
	adminGroup.POST("/csrf/migration/complete", m.CompleteMigration)
}