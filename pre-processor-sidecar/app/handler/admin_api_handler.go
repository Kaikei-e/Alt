// ABOUTME: Admin API Handler - セキュアなトークン管理エンドポイント
// ABOUTME: OWASP準拠の入力検証、認証・認可、レート制限を実装

package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"pre-processor-sidecar/models"
	"pre-processor-sidecar/security"
	"pre-processor-sidecar/service"
)

// AdminAPIHandler はAdmin API用のハンドラー
type AdminAPIHandler struct {
	tokenManager     TokenManager
	authenticator    AdminAuthenticator
	rateLimiter      RateLimiter
	inputValidator   InputValidator
	logger           *slog.Logger
	metricsCollector AdminAPIMetricsCollector
}

// TokenManager はトークン管理インターフェース
type TokenManager interface {
	UpdateRefreshToken(ctx context.Context, refreshToken string, clientID, clientSecret string) error
	GetTokenStatus() service.TokenStatus
	GetValidToken(ctx context.Context) (*service.TokenInfo, error)
}

// AdminAuthenticator は管理者認証インターフェース
type AdminAuthenticator interface {
	ValidateKubernetesServiceAccountToken(token string) (*security.ServiceAccountInfo, error)
	HasAdminPermissions(info *security.ServiceAccountInfo) bool
}

// RateLimiter はレート制限インターフェース
type RateLimiter interface {
	IsAllowed(clientIP string, endpoint string) bool
	RecordRequest(clientIP string, endpoint string)
}

// InputValidator は入力検証インターフェース
type InputValidator interface {
	ValidateTokenUpdateRequest(req *models.TokenUpdateRequest) error
	SanitizeString(input string) string
}

// AdminAPIMetricsCollector はAdmin APIメトリクス収集インターフェース
type AdminAPIMetricsCollector interface {
	IncrementAdminAPIRequest(method, endpoint, status string)
	RecordAdminAPIRequestDuration(method, endpoint string, duration time.Duration)
	IncrementAdminAPIRateLimitHit()
	IncrementAdminAPIAuthenticationError(errorType string)
}

// TokenUpdateResponse はトークン更新レスポンス
type TokenUpdateResponse struct {
	Status         string     `json:"status"`
	Message        string     `json:"message"`
	Timestamp      time.Time  `json:"timestamp"`
	TokenExpiresAt *time.Time `json:"token_expires_at,omitempty"`
}

// ErrorResponse はエラーレスポンス
type ErrorResponse struct {
	Status    string    `json:"status"`
	ErrorCode string    `json:"error_code"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}

// TokenStatusResponse はトークン状態レスポンス
type TokenStatusResponse struct {
	Status           string    `json:"status"`
	HasAccessToken   bool      `json:"has_access_token"`
	HasRefreshToken  bool      `json:"has_refresh_token"`
	ExpiresAt        time.Time `json:"expires_at,omitempty"`
	ExpiresInSeconds int64     `json:"expires_in_seconds,omitempty"`
	TokenType        string    `json:"token_type,omitempty"`
	NeedsRefresh     bool      `json:"needs_refresh"`
	IsAutoRefreshing bool      `json:"is_auto_refreshing"`
	Timestamp        time.Time `json:"timestamp"`
}

// NewAdminAPIHandler は新しいAdmin APIハンドラーを作成
func NewAdminAPIHandler(
	tokenManager TokenManager,
	authenticator AdminAuthenticator,
	rateLimiter RateLimiter,
	inputValidator InputValidator,
	logger *slog.Logger,
	metricsCollector AdminAPIMetricsCollector,
) *AdminAPIHandler {
	return &AdminAPIHandler{
		tokenManager:     tokenManager,
		authenticator:    authenticator,
		rateLimiter:      rateLimiter,
		inputValidator:   inputValidator,
		logger:           logger,
		metricsCollector: metricsCollector,
	}
}

// HandleRefreshTokenUpdate はリフレッシュトークン更新を処理
func (h *AdminAPIHandler) HandleRefreshTokenUpdate(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	clientIP := getClientIP(r)

	defer func() {
		duration := time.Since(start)
		h.metricsCollector.RecordAdminAPIRequestDuration("POST", "/admin/oauth2/refresh-token", duration)
	}()

	// セキュリティヘッダー設定
	h.setSecurityHeaders(w)

	// HTTPメソッド確認
	if r.Method != http.MethodPost {
		h.respondWithError(w, "METHOD_NOT_ALLOWED", "Method not allowed", http.StatusMethodNotAllowed)
		h.metricsCollector.IncrementAdminAPIRequest("POST", "/admin/oauth2/refresh-token", "method_not_allowed")
		return
	}

	// HTTPS強制確認
	if r.TLS == nil && r.Header.Get("X-Forwarded-Proto") != "https" {
		h.respondWithError(w, "HTTPS_REQUIRED", "HTTPS is required for this endpoint", http.StatusBadRequest)
		h.metricsCollector.IncrementAdminAPIRequest("POST", "/admin/oauth2/refresh-token", "https_required")
		return
	}

	// レート制限確認
	if !h.rateLimiter.IsAllowed(clientIP, "/admin/oauth2/refresh-token") {
		h.logger.Warn("Rate limit exceeded for admin API",
			"client_ip", clientIP,
			"endpoint", "/admin/oauth2/refresh-token")

		h.respondWithError(w, "RATE_LIMITED", "Rate limit exceeded", http.StatusTooManyRequests)
		h.metricsCollector.IncrementAdminAPIRateLimitHit()
		h.metricsCollector.IncrementAdminAPIRequest("POST", "/admin/oauth2/refresh-token", "rate_limited")
		return
	}

	// 認証確認
	authToken := extractBearerToken(r)
	if authToken == "" {
		h.respondWithError(w, "MISSING_AUTHORIZATION", "Authorization header with Bearer token is required", http.StatusUnauthorized)
		h.metricsCollector.IncrementAdminAPIAuthenticationError("missing_token")
		h.metricsCollector.IncrementAdminAPIRequest("POST", "/admin/oauth2/refresh-token", "unauthorized")
		return
	}

	// ServiceAccountトークン検証
	serviceAccountInfo, err := h.authenticator.ValidateKubernetesServiceAccountToken(authToken)
	if err != nil {
		h.logger.Error("ServiceAccount token validation failed",
			"error", err,
			"client_ip", clientIP)

		h.respondWithError(w, "INVALID_TOKEN", "Invalid authentication token", http.StatusUnauthorized)
		h.metricsCollector.IncrementAdminAPIAuthenticationError("invalid_token")
		h.metricsCollector.IncrementAdminAPIRequest("POST", "/admin/oauth2/refresh-token", "unauthorized")
		return
	}

	// 管理者権限確認
	if !h.authenticator.HasAdminPermissions(serviceAccountInfo) {
		h.logger.Warn("Insufficient permissions for admin API",
			"subject", serviceAccountInfo.Subject,
			"namespace", serviceAccountInfo.Namespace,
			"client_ip", clientIP)

		h.respondWithError(w, "INSUFFICIENT_PERMISSIONS", "Insufficient permissions for this operation", http.StatusForbidden)
		h.metricsCollector.IncrementAdminAPIAuthenticationError("insufficient_permissions")
		h.metricsCollector.IncrementAdminAPIRequest("POST", "/admin/oauth2/refresh-token", "forbidden")
		return
	}

	// リクエストボディの読み取りと検証
	var req models.TokenUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondWithError(w, "INVALID_JSON", "Invalid JSON in request body", http.StatusBadRequest)
		h.metricsCollector.IncrementAdminAPIRequest("POST", "/admin/oauth2/refresh-token", "invalid_json")
		return
	}

	// 入力検証
	if err := h.inputValidator.ValidateTokenUpdateRequest(&req); err != nil {
		h.logger.Warn("Invalid token update request",
			"error", err,
			"client_ip", clientIP,
			"subject", serviceAccountInfo.Subject)

		h.respondWithError(w, "VALIDATION_ERROR", fmt.Sprintf("Input validation failed: %v", err), http.StatusBadRequest)
		h.metricsCollector.IncrementAdminAPIRequest("POST", "/admin/oauth2/refresh-token", "validation_error")
		return
	}

	// 入力サニタイゼーション
	req.RefreshToken = h.inputValidator.SanitizeString(req.RefreshToken)
	req.ClientID = h.inputValidator.SanitizeString(req.ClientID)
	req.ClientSecret = h.inputValidator.SanitizeString(req.ClientSecret)

	// レート制限記録
	h.rateLimiter.RecordRequest(clientIP, "/admin/oauth2/refresh-token")

	// トークン更新実行
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	h.logger.Info("Processing token update request",
		"subject", serviceAccountInfo.Subject,
		"client_ip", clientIP,
		"has_client_id", req.ClientID != "",
		"has_client_secret", req.ClientSecret != "")

	if err := h.tokenManager.UpdateRefreshToken(ctx, req.RefreshToken, req.ClientID, req.ClientSecret); err != nil {
		h.logger.Error("Token update failed",
			"error", err,
			"subject", serviceAccountInfo.Subject,
			"client_ip", clientIP)

		h.respondWithError(w, "TOKEN_UPDATE_FAILED", "Failed to update token", http.StatusInternalServerError)
		h.metricsCollector.IncrementAdminAPIRequest("POST", "/admin/oauth2/refresh-token", "token_update_failed")
		return
	}

	// 更新されたトークンの状態を取得
	status := h.tokenManager.GetTokenStatus()

	// 成功レスポンス
	response := TokenUpdateResponse{
		Status:    "success",
		Message:   "Token updated successfully",
		Timestamp: time.Now(),
	}

	if !status.ExpiresAt.IsZero() {
		response.TokenExpiresAt = &status.ExpiresAt
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)

	h.metricsCollector.IncrementAdminAPIRequest("POST", "/admin/oauth2/refresh-token", "success")

	h.logger.Info("Token updated successfully via admin API",
		"subject", serviceAccountInfo.Subject,
		"client_ip", clientIP,
		"new_expires_at", status.ExpiresAt,
		"duration_ms", time.Since(start).Milliseconds())
}

// HandleTokenStatus はトークン状態を返す
func (h *AdminAPIHandler) HandleTokenStatus(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	_ = getClientIP(r) // Get client IP but don't use it in this function

	defer func() {
		duration := time.Since(start)
		h.metricsCollector.RecordAdminAPIRequestDuration("GET", "/admin/oauth2/status", duration)
	}()

	// セキュリティヘッダー設定
	h.setSecurityHeaders(w)

	// HTTPメソッド確認
	if r.Method != http.MethodGet {
		h.respondWithError(w, "METHOD_NOT_ALLOWED", "Method not allowed", http.StatusMethodNotAllowed)
		h.metricsCollector.IncrementAdminAPIRequest("GET", "/admin/oauth2/status", "method_not_allowed")
		return
	}

	// 認証確認（簡易版 - 状態確認なので軽い認証）
	authToken := extractBearerToken(r)
	if authToken == "" {
		h.respondWithError(w, "MISSING_AUTHORIZATION", "Authorization header with Bearer token is required", http.StatusUnauthorized)
		h.metricsCollector.IncrementAdminAPIRequest("GET", "/admin/oauth2/status", "unauthorized")
		return
	}

	// トークン状態取得
	status := h.tokenManager.GetTokenStatus()

	response := TokenStatusResponse{
		Status:           "success",
		HasAccessToken:   status.HasAccessToken,
		HasRefreshToken:  status.HasRefreshToken,
		ExpiresAt:        status.ExpiresAt,
		ExpiresInSeconds: status.ExpiresInSeconds,
		TokenType:        status.TokenType,
		NeedsRefresh:     status.NeedsRefresh,
		IsAutoRefreshing: status.IsAutoRefreshing,
		Timestamp:        time.Now(),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)

	h.metricsCollector.IncrementAdminAPIRequest("GET", "/admin/oauth2/status", "success")
}

// setSecurityHeaders はセキュリティヘッダーを設定
func (h *AdminAPIHandler) setSecurityHeaders(w http.ResponseWriter) {
	w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("X-XSS-Protection", "1; mode=block")
	w.Header().Set("Content-Security-Policy", "default-src 'self'")
	w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
}

// respondWithError はエラーレスポンスを送信
func (h *AdminAPIHandler) respondWithError(w http.ResponseWriter, errorCode, message string, statusCode int) {
	response := ErrorResponse{
		Status:    "error",
		ErrorCode: errorCode,
		Message:   message,
		Timestamp: time.Now(),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(response)
}

// extractBearerToken はAuthorizationヘッダーからBearerトークンを抽出
func extractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return ""
	}

	parts := strings.SplitN(auth, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
		return ""
	}

	return parts[1]
}

// getClientIP はクライアントIPアドレスを取得
func getClientIP(r *http.Request) string {
	// プロキシ経由の場合のヘッダーをチェック
	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		// 最初のIPアドレスを使用（プロキシチェーンの場合）
		parts := strings.Split(ip, ",")
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}

	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}

	return r.RemoteAddr
}
