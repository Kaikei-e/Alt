package handlers

import (
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"

	"auth-service/app/port"
)

// AuthHandler handles authentication HTTP requests
type AuthHandler struct {
	authUsecase port.AuthUsecase
	logger      *slog.Logger
}

// NewAuthHandler creates a new auth handler
func NewAuthHandler(authUsecase port.AuthUsecase, logger *slog.Logger) *AuthHandler {
	return &AuthHandler{
		authUsecase: authUsecase,
		logger:      logger,
	}
}

// InitiateLogin starts the login flow
// @Summary Initiate login flow
// @Description Start Kratos login flow for user authentication
// @Tags authentication
// @Accept json
// @Produce json
// @Success 200 {object} domain.LoginFlow
// @Failure 500 {object} ErrorResponse
// @Router /v1/auth/login [post]
func (h *AuthHandler) InitiateLogin(c echo.Context) error {
	ctx := c.Request().Context()

	flow, err := h.authUsecase.InitiateLogin(ctx)
	if err != nil {
		h.logger.Error("failed to initiate login flow", "error", err)
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "failed to initiate login flow",
		})
	}

	return c.JSON(http.StatusOK, flow)
}

// CompleteLogin completes the login flow
// @Summary Complete login flow
// @Description Complete Kratos login flow with user credentials
// @Tags authentication
// @Accept json
// @Produce json
// @Param flowId path string true "Flow ID"
// @Param body body LoginRequest true "Login credentials"
// @Success 200 {object} domain.SessionContext
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /v1/auth/login/{flowId} [post]
func (h *AuthHandler) CompleteLogin(c echo.Context) error {
	ctx := c.Request().Context()
	flowID := c.Param("flowId")

	var loginReq LoginRequest
	if err := c.Bind(&loginReq); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "invalid request body",
		})
	}

	sessionCtx, err := h.authUsecase.CompleteLogin(ctx, flowID, &loginReq)
	if err != nil {
		h.logger.Error("failed to complete login", "flowId", flowID, "error", err)
		return c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error: "login failed",
		})
	}

	// Set session cookie
	h.setSessionCookie(c, sessionCtx.SessionID)

	return c.JSON(http.StatusOK, sessionCtx)
}

// InitiateRegistration starts the registration flow
// @Summary Initiate registration flow
// @Description Start Kratos registration flow for user registration
// @Tags authentication
// @Accept json
// @Produce json
// @Success 200 {object} domain.RegistrationFlow
// @Failure 500 {object} ErrorResponse
// @Router /v1/auth/register [post]
func (h *AuthHandler) InitiateRegistration(c echo.Context) error {
	ctx := c.Request().Context()

	flow, err := h.authUsecase.InitiateRegistration(ctx)
	if err != nil {
		h.logger.Error("failed to initiate registration flow", "error", err)
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "failed to initiate registration flow",
		})
	}

	return c.JSON(http.StatusOK, flow)
}

// CompleteRegistration completes the registration flow
// @Summary Complete registration flow
// @Description Complete Kratos registration flow with user details
// @Tags authentication
// @Accept json
// @Produce json
// @Param flowId path string true "Flow ID"
// @Param body body RegistrationRequest true "Registration details"
// @Success 201 {object} domain.SessionContext
// @Failure 400 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /v1/auth/register/{flowId} [post]
func (h *AuthHandler) CompleteRegistration(c echo.Context) error {
	ctx := c.Request().Context()
	flowID := c.Param("flowId")

	var regReq RegistrationRequest
	if err := c.Bind(&regReq); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "invalid request body",
		})
	}

	sessionCtx, err := h.authUsecase.CompleteRegistration(ctx, flowID, &regReq)
	if err != nil {
		h.logger.Error("failed to complete registration", "flowId", flowID, "error", err)
		return c.JSON(http.StatusConflict, ErrorResponse{
			Error: "registration failed",
		})
	}

	// Set session cookie
	h.setSessionCookie(c, sessionCtx.SessionID)

	return c.JSON(http.StatusCreated, sessionCtx)
}

// Logout logs out the user
// @Summary Logout user
// @Description Revoke user session and logout
// @Tags authentication
// @Accept json
// @Produce json
// @Success 200 {object} SuccessResponse
// @Failure 401 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /v1/auth/logout [post]
func (h *AuthHandler) Logout(c echo.Context) error {
	ctx := c.Request().Context()

	sessionID := h.extractSessionID(c)
	if sessionID == "" {
		return c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error: "session required",
		})
	}

	if err := h.authUsecase.Logout(ctx, sessionID); err != nil {
		h.logger.Error("failed to logout", "sessionId", sessionID, "error", err)
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "logout failed",
		})
	}

	// Clear session cookie
	h.clearSessionCookie(c)

	return c.JSON(http.StatusOK, SuccessResponse{
		Message: "logout successful",
	})
}

// ValidateSession validates session token for other services
// @Summary Validate session
// @Description Validate session token for internal service authentication
// @Tags authentication
// @Accept json
// @Produce json
// @Header 200 {string} X-User-ID "User ID"
// @Header 200 {string} X-Tenant-ID "Tenant ID"
// @Success 200 {object} domain.SessionContext
// @Failure 401 {object} ErrorResponse
// @Router /v1/auth/validate [get]
func (h *AuthHandler) ValidateSession(c echo.Context) error {
	ctx := c.Request().Context()

	// Extract session token from cookie or header
	sessionToken := h.extractSessionToken(c)
	if sessionToken == "" {
		return c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error: "session token required",
		})
	}

	sessionCtx, err := h.authUsecase.ValidateSession(ctx, sessionToken)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error: "invalid session",
		})
	}

	// Set headers for downstream services
	c.Response().Header().Set("X-User-ID", sessionCtx.UserID.String())
	c.Response().Header().Set("X-Tenant-ID", sessionCtx.TenantID.String())

	return c.JSON(http.StatusOK, sessionCtx)
}

// RefreshSession refreshes the current session
// @Summary Refresh session
// @Description Refresh the current user session
// @Tags authentication
// @Accept json
// @Produce json
// @Success 200 {object} domain.SessionContext
// @Failure 401 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /v1/auth/refresh [post]
func (h *AuthHandler) RefreshSession(c echo.Context) error {
	ctx := c.Request().Context()

	sessionID := h.extractSessionID(c)
	if sessionID == "" {
		return c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error: "session required",
		})
	}

	sessionCtx, err := h.authUsecase.RefreshSession(ctx, sessionID)
	if err != nil {
		h.logger.Error("failed to refresh session", "sessionId", sessionID, "error", err)
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "session refresh failed",
		})
	}

	// Update session cookie
	h.setSessionCookie(c, sessionCtx.SessionID)

	return c.JSON(http.StatusOK, sessionCtx)
}

// GenerateCSRFToken generates CSRF token for session
// @Summary Generate CSRF token
// @Description Generate CSRF token for the current session
// @Tags authentication
// @Accept json
// @Produce json
// @Success 200 {object} domain.CSRFToken
// @Failure 401 {object} ErrorResponse
// @Router /v1/auth/csrf [post]
func (h *AuthHandler) GenerateCSRFToken(c echo.Context) error {
	ctx := c.Request().Context()

	sessionID := h.extractSessionID(c)
	if sessionID == "" {
		return c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error: "session required",
		})
	}

	csrfToken, err := h.authUsecase.GenerateCSRFToken(ctx, sessionID)
	if err != nil {
		h.logger.Error("failed to generate CSRF token", "sessionId", sessionID, "error", err)
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "failed to generate CSRF token",
		})
	}

	return c.JSON(http.StatusOK, csrfToken)
}

// ValidateCSRFToken validates CSRF token
// @Summary Validate CSRF token
// @Description Validate CSRF token for the current session
// @Tags authentication
// @Accept json
// @Produce json
// @Param body body CSRFValidationRequest true "CSRF token validation request"
// @Success 200 {object} SuccessResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Router /v1/auth/csrf/validate [post]
func (h *AuthHandler) ValidateCSRFToken(c echo.Context) error {
	ctx := c.Request().Context()

	var req CSRFValidationRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "invalid request body",
		})
	}

	sessionID := h.extractSessionID(c)
	if sessionID == "" {
		return c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error: "session required",
		})
	}

	if err := h.authUsecase.ValidateCSRFToken(ctx, req.Token, sessionID); err != nil {
		h.logger.Error("CSRF token validation failed", "sessionId", sessionID, "error", err)
		return c.JSON(http.StatusForbidden, ErrorResponse{
			Error: "invalid CSRF token",
		})
	}

	return c.JSON(http.StatusOK, SuccessResponse{
		Message: "CSRF token valid",
	})
}

// Helper methods
func (h *AuthHandler) setSessionCookie(c echo.Context, sessionID string) {
	cookie := &http.Cookie{
		Name:     "ory_kratos_session",
		Value:    sessionID,
		Path:     "/",
		Domain:   "alt.local",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   86400, // 24 hours
	}
	c.SetCookie(cookie)
}

func (h *AuthHandler) clearSessionCookie(c echo.Context) {
	cookie := &http.Cookie{
		Name:     "ory_kratos_session",
		Value:    "",
		Path:     "/",
		Domain:   "alt.local",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1, // Delete cookie
	}
	c.SetCookie(cookie)
}

func (h *AuthHandler) extractSessionToken(c echo.Context) string {
	// Try cookie first
	if cookie, err := c.Cookie("ory_kratos_session"); err == nil {
		return cookie.Value
	}

	// Try Authorization header
	return c.Request().Header.Get("Authorization")
}

func (h *AuthHandler) extractSessionID(c echo.Context) string {
	return h.extractSessionToken(c)
}

// Request/Response types
type LoginRequest struct {
	Email     string `json:"email" validate:"required,email"`
	Password  string `json:"password" validate:"required,min=8"`
	CSRFToken string `json:"csrf_token,omitempty"`
}

type RegistrationRequest struct {
	Email     string `json:"email" validate:"required,email"`
	Password  string `json:"password" validate:"required,min=8"`
	Name      string `json:"name,omitempty"`
	CSRFToken string `json:"csrf_token,omitempty"`
}

type CSRFValidationRequest struct {
	Token string `json:"token" validate:"required"`
}

type ErrorResponse struct {
	Error   string `json:"error"`
	Details string `json:"details,omitempty"`
}

type SuccessResponse struct {
	Message string `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}