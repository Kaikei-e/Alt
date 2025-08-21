package handlers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/labstack/echo/v4"
	ory "github.com/ory/client-go"

	"auth-service/app/domain"
	"auth-service/app/port"
)

// AuthHandler handles authentication HTTP requests
type AuthHandler struct {
	authUsecase port.AuthUsecase
	oryCli      *ory.APIClient
	logger      *slog.Logger
}

// NewAuthHandler creates a new auth handler
func NewAuthHandler(authUsecase port.AuthUsecase, logger *slog.Logger) *AuthHandler {
	// Initialize Ory client as per memo.md Phase 2.1
	cfg := ory.NewConfiguration()
	cfg.Servers = ory.ServerConfigurations{{URL: os.Getenv("KRATOS_PUBLIC_URL")}} // http://kratos-public:4433
	oryCli := ory.NewAPIClient(cfg)

	return &AuthHandler{
		authUsecase: authUsecase,
		oryCli:      oryCli,
		logger:      logger,
	}
}

// InitiateLoginFlow handles GET request to initiate Kratos browser login flow
// @Summary Initiate login flow (Browser GET)
// @Description Start Kratos browser login flow following Ory specifications
// @Tags authentication
// @Accept json
// @Produce json
// @Param return_to query string false "Return URL after login"
// @Success 200 {object} domain.LoginFlow
// @Success 303 "Redirect to login UI" 
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /v1/auth/login/initiate [get]
func (h *AuthHandler) InitiateLoginFlow(c echo.Context) error {
	ctx := c.Request().Context()
	returnTo := c.QueryParam("return_to")
	if returnTo == "" {
		returnTo = "/"
	}

	h.logger.Info("initiating browser login flow",
		"return_to", returnTo,
		"user_agent", c.Request().Header.Get("User-Agent"),
		"ip", c.RealIP())

	// Ory公式サンプルパターンに従ったKratosクライアント使用
	kratosFlow, kratosResp, err := h.oryCli.FrontendAPI.CreateBrowserLoginFlow(ctx).
		ReturnTo(returnTo).
		Execute()

	if err != nil {
		h.logger.Error("failed to create Kratos browser login flow",
			"error", err,
			"return_to", returnTo,
			"response", kratosResp)
		
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "failed to initiate login flow",
		})
	}

	h.logger.Info("Kratos browser login flow created successfully",
		"flow_id", kratosFlow.Id,
		"return_to", returnTo,
		"expires_at", kratosFlow.ExpiresAt)

	// Ory公式サンプルと同じ構造でレスポンス返却
	return c.JSON(http.StatusOK, kratosFlow)
}

// GetLoginFlow retrieves login flow details by ID
// @Summary Get login flow by ID
// @Description Retrieve details of an existing login flow by its ID
// @Tags authentication
// @Accept json
// @Produce json
// @Param flowId path string true "Flow ID"
// @Success 200 {object} domain.LoginFlow
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 410 {object} ErrorResponse "Flow expired"
// @Failure 500 {object} ErrorResponse
// @Router /v1/auth/login/{flowId} [get]
func (h *AuthHandler) GetLoginFlow(c echo.Context) error {
	ctx := c.Request().Context()
	flowID := c.Param("flowId")
	
	if flowID == "" {
		h.logger.Error("flow ID is required")
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "Flow ID is required",
		})
	}

	h.logger.Info("retrieving login flow details",
		"flow_id", flowID,
		"user_agent", c.Request().Header.Get("User-Agent"),
		"ip", c.RealIP())

	// Ory公式サンプルに従ったKratosフロー取得
	kratosFlow, kratosResp, err := h.oryCli.FrontendAPI.GetLoginFlow(ctx).
		Id(flowID).
		Execute()

	if err != nil {
		h.logger.Error("failed to get Kratos login flow",
			"flow_id", flowID,
			"error", err,
			"response", kratosResp)
		
		// Kratosエラーコードに応じた適切なレスポンス
		if kratosResp != nil {
			switch kratosResp.StatusCode {
			case 400:
				return c.JSON(http.StatusBadRequest, ErrorResponse{
					Error: "Invalid flow ID",
				})
			case 404:
				return c.JSON(http.StatusNotFound, ErrorResponse{
					Error: "Flow not found",
				})
			case 410:
				return c.JSON(http.StatusGone, ErrorResponse{
					Error: "Flow expired",
				})
			}
		}
		
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "failed to retrieve login flow",
		})
	}

	h.logger.Info("Kratos login flow retrieved successfully",
		"flow_id", flowID,
		"expires_at", kratosFlow.ExpiresAt)

	return c.JSON(http.StatusOK, kratosFlow)
}

// RedirectToLogin handles browser-compatible login redirect
// @Summary Redirect to login page (Browser GET)
// @Description Redirects browser requests to the frontend login page with return_to parameter
// @Tags authentication
// @Param return_to query string false "Return URL after login"
// @Success 307 "Redirect to login page"
// @Router /v1/auth/login [get]
func (h *AuthHandler) RedirectToLogin(c echo.Context) error {
	returnTo := c.QueryParam("return_to")
	if returnTo == "" {
		returnTo = "/"
	}

	h.logger.Info("redirecting to login page",
		"return_to", returnTo,
		"user_agent", c.Request().Header.Get("User-Agent"),
		"ip", c.RealIP())

	// Redirect to frontend login page with return_to parameter
	redirectURL := fmt.Sprintf("https://curionoah.com/auth/login?return_to=%s", url.QueryEscape(returnTo))
	return c.Redirect(http.StatusTemporaryRedirect, redirectURL)
}

// generateTempCSRFToken generates a temporary CSRF token for development
// TODO: Replace with proper Kratos CSRF token integration
func (h *AuthHandler) generateTempCSRFToken() string {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		h.logger.Error("failed to generate CSRF token", "error", err)
		return "temp-csrf-token"
	}
	return hex.EncodeToString(bytes)
}

// getStatusCode safely extracts status code from HTTP response
func getStatusCode(resp interface{}) int {
	if resp == nil {
		return 0
	}
	if r, ok := resp.(*http.Response); ok {
		return r.StatusCode
	}
	// For Ory client response type
	if r, ok := resp.(interface{ GetStatusCode() int }); ok {
		return r.GetStatusCode()
	}
	return 0
}

// InitiateLogin starts the login flow (Legacy POST endpoint)
// @Summary Initiate login flow (Legacy POST)
// @Description Start Kratos login flow for user authentication (legacy endpoint)
// @Tags authentication
// @Accept json
// @Produce json
// @Success 200 {object} domain.LoginFlow
// @Failure 500 {object} ErrorResponse
// @Router /v1/auth/login [post]
// @Deprecated Use GET /v1/auth/login/initiate instead
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

// CompleteLogin completes the login flow (X2.md Phase 2.4.1 強化)
// @Summary Complete login flow
// @Description Complete Kratos login flow with user credentials
// @Tags authentication
// @Accept json
// @Produce json
// @Param flowId path string true "Flow ID"
// @Param body body LoginRequest true "Login credentials"
// @Success 200 {object} domain.SessionContext
// @Failure 400 {object} DetailedErrorResponse
// @Failure 401 {object} DetailedErrorResponse
// @Failure 500 {object} DetailedErrorResponse
// @Router /v1/auth/login/{flowId} [post]
func (h *AuthHandler) CompleteLogin(c echo.Context) error {
	ctx := c.Request().Context()
	flowID := c.Param("flowId")

	var loginReq LoginRequest
	if err := c.Bind(&loginReq); err != nil {
		h.logger.Error("failed to bind login request", 
			"flowId", flowID, 
			"error", err,
			"content_type", c.Request().Header.Get("Content-Type"))
		return c.JSON(http.StatusBadRequest, DetailedErrorResponse{
			Error:   "Invalid request format",
			Code:    "INVALID_REQUEST",
			Details: "Request body could not be parsed as JSON",
		})
	}

	// バリデーション
	if err := h.validateLoginRequest(&loginReq); err != nil {
		h.logger.Error("login request validation failed",
			"flowId", flowID,
			"error", err,
			"email", h.extractEmailFromLoginRequest(&loginReq))
		return c.JSON(http.StatusBadRequest, DetailedErrorResponse{
			Error:   "Validation failed",
			Code:    "VALIDATION_ERROR", 
			Details: err.Error(),
		})
	}

	// X18.md HAR Analysis Fix: Convert LoginRequest struct to map for Kratos compatibility
	loginMap := h.convertLoginRequestToMap(&loginReq)
	sessionCtx, err := h.authUsecase.CompleteLogin(ctx, flowID, loginMap)
	if err != nil {
		h.logger.Error("failed to complete login", 
			"flowId", flowID, 
			"email", h.extractEmailFromLoginRequest(&loginReq),
			"error", err,
			"error_type", fmt.Sprintf("%T", err))
			
		// ドメインエラーに基づく詳細レスポンス
		return h.handleAuthError(c, err)
	}

	// 成功ログ
	h.logger.Info("login completed successfully",
		"flowId", flowID,
		"userId", sessionCtx.UserID,
		"email", h.extractEmailFromLoginRequest(&loginReq))

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

// CompleteRegistration completes the registration flow (X2.md Phase 2.4.1 強化)
// @Summary Complete registration flow
// @Description Complete Kratos registration flow with user details
// @Tags authentication
// @Accept json
// @Produce json
// @Param flowId path string true "Flow ID"
// @Param body body RegistrationRequest true "Registration details"
// @Success 201 {object} domain.SessionContext
// @Failure 400 {object} DetailedErrorResponse
// @Failure 409 {object} DetailedErrorResponse
// @Failure 500 {object} DetailedErrorResponse
// @Router /v1/auth/register/{flowId} [post]
func (h *AuthHandler) CompleteRegistration(c echo.Context) error {
	ctx := c.Request().Context()
	flowID := c.Param("flowId")

	var regReq RegistrationRequest
	if err := c.Bind(&regReq); err != nil {
		h.logger.Error("failed to bind registration request", 
			"flowId", flowID, 
			"error", err,
			"content_type", c.Request().Header.Get("Content-Type"))
		return c.JSON(http.StatusBadRequest, DetailedErrorResponse{
			Error:   "Invalid request format",
			Code:    "INVALID_REQUEST",
			Details: "Request body could not be parsed as JSON",
		})
	}

	// バリデーション
	if err := h.validateRegistrationRequest(&regReq); err != nil {
		h.logger.Error("registration request validation failed",
			"flowId", flowID,
			"error", err,
			"email", regReq.Email)
		return c.JSON(http.StatusBadRequest, DetailedErrorResponse{
			Error:   "Validation failed",
			Code:    "VALIDATION_ERROR", 
			Details: err.Error(),
		})
	}

	// X18.md HAR Analysis Fix: Convert RegistrationRequest struct to map for Kratos compatibility
	regMap := h.convertRegistrationRequestToMap(&regReq)
	sessionCtx, err := h.authUsecase.CompleteRegistration(ctx, flowID, regMap)
	if err != nil {
		h.logger.Error("failed to complete registration",
			"flowId", flowID,
			"email", regReq.Email,
			"error", err,
			"error_type", fmt.Sprintf("%T", err))

		// ドメインエラーに基づく詳細レスポンス
		return h.handleAuthError(c, err)
	}

	// 成功ログ
	h.logger.Info("registration completed successfully",
		"flowId", flowID,
		"userId", sessionCtx.UserID,
		"email", regReq.Email)

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

// Validate validates session for internal services (memo.md Phase 2.1)
// @Summary Validate session
// @Description Validate session via FrontendAPI.ToSession() using cookies or tokens
// @Tags authentication
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 401 {string} string "unauthorized"
// @Router /v1/auth/validate [get]
func (h *AuthHandler) Validate(c echo.Context) error {
	ctx := c.Request().Context()
	cookie := c.Request().Header.Get("Cookie")
	
	// Enhanced logging for debugging redirect loops
	clientIP := c.RealIP()
	userAgent := c.Request().Header.Get("User-Agent")
	
	h.logger.Info("session validation request",
		"client_ip", clientIP,
		"user_agent", userAgent,
		"has_cookie", cookie != "",
		"cookie_preview", truncateCookie(cookie, 50))

	// Use the usecase layer instead of direct Ory client access (Clean Architecture)
	sessionCtx, err := h.authUsecase.ValidateSessionWithCookie(ctx, cookie)
	if err != nil {
		h.logger.Warn("session validation failed",
			"error", err.Error(),
			"client_ip", clientIP)
		return c.JSON(http.StatusUnauthorized, ErrorPayload{Message: "session validation failed"})
	}

	if sessionCtx == nil {
		h.logger.Warn("session validation returned nil context",
			"client_ip", clientIP)
		return c.JSON(http.StatusUnauthorized, ErrorPayload{Message: "invalid session context"})
	}

	if !sessionCtx.IsActive {
		h.logger.Info("session is inactive",
			"session_id", sessionCtx.SessionID,
			"client_ip", clientIP)
		return c.JSON(http.StatusUnauthorized, ErrorPayload{Message: "session inactive"})
	}

	h.logger.Info("session validation successful",
		"session_id", sessionCtx.SessionID,
		"identity_id", sessionCtx.UserID,
		"client_ip", clientIP)

	// Set headers BEFORE creating response to prevent Echo content-length issues
	c.Response().Header().Set("Cache-Control", "no-store")
	c.Response().Header().Set("Content-Type", "application/json")
	
	// Single return with JSON - no other response writes
	return c.JSON(http.StatusOK, ValidateOK{
		Valid:      true,
		SessionID:  sessionCtx.SessionID,
		IdentityID: sessionCtx.UserID.String(),
		Email:      sessionCtx.Email,
		TenantID:   sessionCtx.TenantID.String(),
		Role:       string(sessionCtx.Role),
	})
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

	// 🚀 X26 PERMANENT FIX: CSRF tokens must be generated WITHOUT session requirement
	// CSRF tokens are needed BEFORE session establishment, so we use anonymous session ID
	
	sessionID := h.extractSessionID(c)
	if sessionID == "" {
		// Use anonymous session for CSRF token generation before authentication
		sessionID = "anonymous-" + generateRandomID()
		h.logger.Debug("generating CSRF token for anonymous session", "anonymousSessionId", sessionID)
	}

	csrfToken, err := h.authUsecase.GenerateCSRFToken(ctx, sessionID)
	if err != nil {
		h.logger.Error("failed to generate CSRF token", "sessionId", sessionID, "error", err)
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "failed to generate CSRF token",
		})
	}

	h.logger.Debug("CSRF token generated successfully", 
		"sessionId", sessionID, 
		"tokenLength", len(csrfToken.Token),
		"clientIP", c.RealIP())

	return c.JSON(http.StatusOK, map[string]interface{}{
		"csrf_token": csrfToken.Token,
		"expires_at": csrfToken.ExpiresAt,
	})
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
		Domain:   "curionoah.com", // Updated for production domain
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   86400, // 24 hours
	}
	c.SetCookie(cookie)
	h.logger.Info("session cookie set", "session_id", sessionID)
}

func (h *AuthHandler) clearSessionCookie(c echo.Context) {
	cookie := &http.Cookie{
		Name:     "ory_kratos_session",
		Value:    "",
		Path:     "/",
		Domain:   "curionoah.com", // Updated for production domain
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1, // Delete cookie
	}
	c.SetCookie(cookie)
	h.logger.Info("session cookie cleared")
}

func (h *AuthHandler) extractSessionToken(c echo.Context) string {
	// Try cookie first (preferred for browsers)
	if cookie, err := c.Cookie("ory_kratos_session"); err == nil && cookie.Value != "" {
		h.logger.Debug("session token extracted from cookie")
		return cookie.Value
	}

	// Try Authorization header (for API clients)
	authHeader := c.Request().Header.Get("Authorization")
	if authHeader != "" {
		h.logger.Debug("session token extracted from authorization header")
		// Remove "Bearer " prefix if present
		if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
			return authHeader[7:]
		}
		return authHeader
	}

	// Try custom session header (for service-to-service)
	sessionHeader := c.Request().Header.Get("X-Session-Token")
	if sessionHeader != "" {
		h.logger.Debug("session token extracted from X-Session-Token header")
		return sessionHeader
	}

	h.logger.Debug("no session token found in cookie, Authorization, or X-Session-Token headers")
	return ""
}

func (h *AuthHandler) extractSessionID(c echo.Context) string {
	return h.extractSessionToken(c)
}

// extractBearer extracts bearer token from Authorization header
func (h *AuthHandler) extractBearer(authHeader string) string {
	if authHeader == "" {
		return ""
	}
	// Remove "Bearer " prefix if present
	if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
		return authHeader[7:]
	}
	return authHeader
}

// truncateCookie truncates cookie string for safe logging
func truncateCookie(cookie string, maxLen int) string {
	if cookie == "" {
		return "(empty)"
	}
	if len(cookie) <= maxLen {
		return cookie
	}
	return cookie[:maxLen] + "..."
}

// upsertSessionAsync performs session audit in background (best-effort)
func (h *AuthHandler) upsertSessionAsync(ctx context.Context, sess *ory.Session) {
	// This is a placeholder for session audit logic
	// In actual implementation, this would store session info to database
	// for audit/analytics purposes, but failure here should not affect auth validation
	h.logger.Debug("session audit logged", "session_id", sess.Id)
}

// generateRandomID generates a random ID for anonymous sessions
func generateRandomID() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to simpler method if crypto/rand fails
		return fmt.Sprintf("%d", len(bytes)*1000000)
	}
	return hex.EncodeToString(bytes)
}

// Request/Response types
type LoginRequest struct {
	// HAR分析により判明: フロントエンドは"identifier"フィールドで送信している
	// X17.md Phase 17.1: Login API修正
	Email      string `json:"email,omitempty"`           // 既存フィールド（後方互換性）
	Identifier string `json:"identifier,omitempty"`     // フロントエンドが実際に送信するフィールド
	Password   string `json:"password" validate:"required,min=8"`
	Method     string `json:"method,omitempty"`          // Kratosプロトコル用
	CSRFToken  string `json:"csrf_token,omitempty"`
}

type RegistrationRequest struct {
	Email     string `json:"email" validate:"required,email"`
	Password  string `json:"password" validate:"required,min=8"`
	Name      string `json:"name,omitempty"`
	Method    string `json:"method,omitempty"`           // Kratosプロトコル用
	CSRFToken string `json:"csrf_token,omitempty"`
}

type CSRFValidationRequest struct {
	Token string `json:"token" validate:"required"`
}

type ErrorResponse struct {
	Error   string `json:"error"`
	Details string `json:"details,omitempty"`
}

// X2.md Phase 2.4.1: 詳細エラーレスポンス型
type DetailedErrorResponse struct {
	Error   string      `json:"error"`
	Code    string      `json:"code"`
	Details string      `json:"details"`
	Field   string      `json:"field,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

type ValidateOK struct {
	Valid      bool   `json:"valid"`
	SessionID  string `json:"session_id,omitempty"`
	IdentityID string `json:"identity_id,omitempty"`
	Email      string `json:"email,omitempty"`
	TenantID   string `json:"tenant_id,omitempty"`
	Role       string `json:"role,omitempty"`
}

type ErrorPayload struct {
	Message string `json:"message"`
}

type SuccessResponse struct {
	Message string `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// X2.md Phase 2.4.1: 詳細エラーハンドリング
func (h *AuthHandler) handleAuthError(c echo.Context, err error) error {
	// ドメインエラーの詳細処理
	if authErr, ok := err.(*domain.AuthError); ok {
		switch authErr.Code {
		case domain.ErrCodeUserExists:
			return c.JSON(http.StatusConflict, DetailedErrorResponse{
				Error:   "User already exists",
				Code:    authErr.Code,
				Details: "A user with this email address is already registered. Please use the login flow instead.",
			})
		case domain.ErrCodeFlowExpired:
			return c.JSON(http.StatusGone, DetailedErrorResponse{
				Error:   "Flow expired",
				Code:    authErr.Code,
				Details: "The registration flow has expired. Please start a new registration.",
			})
		case domain.ErrCodeValidation:
			return c.JSON(http.StatusBadRequest, DetailedErrorResponse{
				Error:   "Validation error",
				Code:    authErr.Code,
				Details: authErr.Message,
			})
		case domain.ErrCodeInvalidCredentials:
			return c.JSON(http.StatusUnauthorized, DetailedErrorResponse{
				Error:   "Invalid credentials",
				Code:    authErr.Code,
				Details: "The provided email or password is incorrect.",
			})
		case domain.ErrCodeSessionExpired:
			return c.JSON(http.StatusUnauthorized, DetailedErrorResponse{
				Error:   "Session expired",
				Code:    authErr.Code,
				Details: "Your session has expired. Please log in again.",
			})
		case domain.ErrCodeServiceUnavailable:
			return c.JSON(http.StatusServiceUnavailable, DetailedErrorResponse{
				Error:   "Service temporarily unavailable",
				Code:    authErr.Code,
				Details: "The authentication service is temporarily unavailable. Please try again later.",
			})
		}
	}

	// バリデーションエラーの処理
	if valErr, ok := err.(*domain.ValidationError); ok {
		return c.JSON(http.StatusBadRequest, DetailedErrorResponse{
			Error:   "Field validation error",
			Code:    "FIELD_VALIDATION",
			Details: fmt.Sprintf("Field '%s': %s", valErr.Field, valErr.Message),
			Field:   valErr.Field,
		})
	}

	// 汎用エラー
	return c.JSON(http.StatusInternalServerError, DetailedErrorResponse{
		Error:   "Internal error",
		Code:    "INTERNAL_ERROR",
		Details: "An internal error occurred. Please try again later.",
	})
}

// X2.md Phase 2.4.1: バリデーション関数

// validateRegistrationRequest validates registration request
func (h *AuthHandler) validateRegistrationRequest(req *RegistrationRequest) error {
	if req.Email == "" {
		return domain.NewValidationError("email", req.Email, "email is required")
	}
	if req.Password == "" {
		return domain.NewValidationError("password", nil, "password is required")
	}
	if len(req.Password) < 8 {
		return domain.NewValidationError("password", nil, "password must be at least 8 characters")
	}
	
	// 基本的なメール形式検証
	if !h.isValidEmail(req.Email) {
		return domain.NewValidationError("email", req.Email, "invalid email format")
	}
	
	return nil
}

// validateLoginRequest validates login request
// X17.md Phase 17.1: identifier/email フィールド両方対応
func (h *AuthHandler) validateLoginRequest(req *LoginRequest) error {
	// identifierまたはemailフィールドからemail値を取得
	email := h.extractEmailFromLoginRequest(req)
	if email == "" {
		return domain.NewValidationError("email", email, "email is required")
	}
	if req.Password == "" {
		return domain.NewValidationError("password", nil, "password is required")
	}
	
	// 基本的なメール形式検証
	if !h.isValidEmail(email) {
		return domain.NewValidationError("email", email, "invalid email format")
	}
	
	return nil
}

// extractEmailFromLoginRequest extracts email from either identifier or email field
// X17.md Phase 17.1: HAR分析に基づくフィールドマッピング
func (h *AuthHandler) extractEmailFromLoginRequest(req *LoginRequest) string {
	// 優先順位: identifier > email (フロントエンドが実際に送信するのはidentifier)
	if req.Identifier != "" {
		return req.Identifier
	}
	return req.Email
}

// isValidEmail performs basic email format validation
func (h *AuthHandler) isValidEmail(email string) bool {
	// 非常に基本的なメール検証 - 実際のプロダクションではより厳密な検証を使用
	return len(email) > 3 && 
		   len(email) < 255 && 
		   len(email) > len("a@b") &&
		   email[0] != '@' && 
		   email[len(email)-1] != '@' &&
		   countChar(email, '@') == 1 &&
		   countChar(email, '.') >= 1
}

// countChar counts occurrences of character in string
func countChar(s string, c byte) int {
	count := 0
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			count++
		}
	}
	return count
}

// convertLoginRequestToMap converts LoginRequest struct to map[string]interface{}
// X18.md HAR Analysis Fix: Required for Kratos client compatibility
func (h *AuthHandler) convertLoginRequestToMap(req *LoginRequest) map[string]interface{} {
	result := make(map[string]interface{})
	
	// Extract email from either identifier or email field (X17.md compatibility)
	email := h.extractEmailFromLoginRequest(req)
	if email != "" {
		result["identifier"] = email  // Kratos expects 'identifier' field
	}
	
	// Password is required
	if req.Password != "" {
		result["password"] = req.Password
	}
	
	// Method for Kratos protocol
	if req.Method != "" {
		result["method"] = req.Method
	} else {
		result["method"] = "password"  // Default to password method
	}
	
	// CSRF token if provided
	if req.CSRFToken != "" {
		result["csrf_token"] = req.CSRFToken
	}
	
	h.logger.Debug("converted LoginRequest to map",
		"has_identifier", result["identifier"] != nil,
		"has_password", result["password"] != nil, 
		"method", result["method"])
	
	return result
}

// convertRegistrationRequestToMap converts RegistrationRequest struct to map[string]interface{}
// X18.md HAR Analysis Fix: Required for Kratos client compatibility
func (h *AuthHandler) convertRegistrationRequestToMap(req *RegistrationRequest) map[string]interface{} {
	result := make(map[string]interface{})
	
	// Password is required
	if req.Password != "" {
		result["password"] = req.Password
	}
	
	// Method for Kratos protocol
	if req.Method != "" {
		result["method"] = req.Method
	} else {
		result["method"] = "password"  // Default to password method
	}
	
	// Traits for user data
	traits := make(map[string]interface{})
	if req.Email != "" {
		traits["email"] = req.Email
	}
	
	// Handle name structure
	if req.Name != "" {
		// Split name into first and last parts
		nameParts := strings.Fields(req.Name)
		nameMap := make(map[string]interface{})
		if len(nameParts) > 0 {
			nameMap["first"] = nameParts[0]
			if len(nameParts) > 1 {
				nameMap["last"] = strings.Join(nameParts[1:], " ")
			}
		}
		traits["name"] = nameMap
	}
	
	result["traits"] = traits
	
	// CSRF token if provided
	if req.CSRFToken != "" {
		result["csrf_token"] = req.CSRFToken
	}
	
	h.logger.Debug("converted RegistrationRequest to map",
		"has_traits", result["traits"] != nil,
		"has_password", result["password"] != nil,
		"method", result["method"])
	
	return result
}