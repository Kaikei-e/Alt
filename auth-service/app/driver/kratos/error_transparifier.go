package kratos

import (
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// DetailedAuthError represents a comprehensive authentication error
type DetailedAuthError struct {
	Type           string    `json:"type"`
	Message        string    `json:"message"`
	TechnicalInfo  string    `json:"technical_info"`
	KratosRawError string    `json:"kratos_raw_error"`
	Suggestions    []string  `json:"suggestions"`
	IsRetryable    bool      `json:"is_retryable"`
	RetryAfter     *int      `json:"retry_after,omitempty"` // seconds
	Timestamp      time.Time `json:"timestamp"`
	ErrorCode      string    `json:"error_code,omitempty"`
}

// ErrorTransparifier transparently passes through Kratos errors with detailed information
type ErrorTransparifier struct {
	logger *slog.Logger
}

// NewErrorTransparifier creates a new error transparifier
func NewErrorTransparifier(logger *slog.Logger) *ErrorTransparifier {
	return &ErrorTransparifier{
		logger: logger,
	}
}

// TransparifyKratosError converts a Kratos error into a detailed auth error
func (t *ErrorTransparifier) TransparifyKratosError(kratosErr error) *DetailedAuthError {
	transparifyId := fmt.Sprintf("TRANS-%d", time.Now().UnixNano())
	
	t.logger.Info("🔍 Transparifying Kratos error", 
		"transparify_id", transparifyId,
		"error", kratosErr.Error())

	detailedError := &DetailedAuthError{
		KratosRawError: kratosErr.Error(),
		Timestamp:      time.Now(),
		Suggestions:    make([]string, 0),
	}

	// Classify error type and extract detailed information
	t.classifyError(detailedError, kratosErr)
	t.extractTechnicalInfo(detailedError, kratosErr)
	t.generateSuggestions(detailedError, kratosErr)
	t.determineRetryability(detailedError, kratosErr)

	t.logger.Info("✅ Error transparification completed",
		"transparify_id", transparifyId,
		"error_type", detailedError.Type,
		"is_retryable", detailedError.IsRetryable,
		"suggestions_count", len(detailedError.Suggestions))

	return detailedError
}

// classifyError determines the error type based on Kratos error patterns
func (t *ErrorTransparifier) classifyError(detailedError *DetailedAuthError, kratosErr error) {
	errorStr := strings.ToLower(kratosErr.Error())

	// Specific Kratos error patterns
	if strings.Contains(errorStr, "property email is missing") {
		detailedError.Type = "MISSING_EMAIL_FIELD"
		detailedError.Message = "メールアドレスフィールドが不足しています"
		detailedError.ErrorCode = "KRATOS_MISSING_EMAIL"
		return
	}

	if strings.Contains(errorStr, "user already exists") || 
	   strings.Contains(errorStr, "already registered") ||
	   strings.Contains(errorStr, "409") {
		detailedError.Type = "USER_ALREADY_EXISTS"
		detailedError.Message = "このメールアドレスは既に登録されています"
		detailedError.ErrorCode = "KRATOS_USER_EXISTS"
		return
	}

	if strings.Contains(errorStr, "flow expired") || 
	   strings.Contains(errorStr, "410") ||
	   strings.Contains(errorStr, "gone") {
		detailedError.Type = "FLOW_EXPIRED"
		detailedError.Message = "登録フローの有効期限が切れました。もう一度お試しください"
		detailedError.ErrorCode = "KRATOS_FLOW_EXPIRED"
		return
	}

	if strings.Contains(errorStr, "invalid credentials") ||
	   strings.Contains(errorStr, "401") ||
	   strings.Contains(errorStr, "unauthorized") {
		detailedError.Type = "INVALID_CREDENTIALS"
		detailedError.Message = "認証情報が正しくありません"
		detailedError.ErrorCode = "KRATOS_INVALID_CREDENTIALS"
		return
	}

	if strings.Contains(errorStr, "validation failed") ||
	   strings.Contains(errorStr, "400") ||
	   strings.Contains(errorStr, "bad request") {
		detailedError.Type = "VALIDATION_FAILED"
		detailedError.Message = "入力内容に問題があります"
		detailedError.ErrorCode = "KRATOS_VALIDATION_FAILED"
		return
	}

	if strings.Contains(errorStr, "502") || 
	   strings.Contains(errorStr, "503") ||
	   strings.Contains(errorStr, "bad gateway") ||
	   strings.Contains(errorStr, "service unavailable") {
		detailedError.Type = "KRATOS_SERVICE_ERROR"
		detailedError.Message = "認証サービスに一時的な問題が発生しています"
		detailedError.ErrorCode = "KRATOS_SERVICE_UNAVAILABLE"
		return
	}

	if strings.Contains(errorStr, "timeout") ||
	   strings.Contains(errorStr, "deadline exceeded") {
		detailedError.Type = "TIMEOUT_ERROR"
		detailedError.Message = "リクエストがタイムアウトしました"
		detailedError.ErrorCode = "KRATOS_TIMEOUT"
		return
	}

	if strings.Contains(errorStr, "network") ||
	   strings.Contains(errorStr, "connection") {
		detailedError.Type = "NETWORK_ERROR"
		detailedError.Message = "ネットワーク接続に問題があります"
		detailedError.ErrorCode = "KRATOS_NETWORK_ERROR"
		return
	}

	// Generic classification
	detailedError.Type = "UNKNOWN_KRATOS_ERROR"
	detailedError.Message = "認証処理中に予期しないエラーが発生しました"
	detailedError.ErrorCode = "KRATOS_UNKNOWN"
}

// extractTechnicalInfo extracts technical information from the error
func (t *ErrorTransparifier) extractTechnicalInfo(detailedError *DetailedAuthError, kratosErr error) {
	errorStr := kratosErr.Error()
	
	// Extract HTTP status codes
	if strings.Contains(errorStr, "HTTP") {
		// Find HTTP status code patterns
		for _, status := range []string{"400", "401", "409", "410", "500", "502", "503"} {
			if strings.Contains(errorStr, status) {
				detailedError.TechnicalInfo = fmt.Sprintf("HTTP Status: %s", status)
				break
			}
		}
	}

	// Extract flow ID if present
	if strings.Contains(errorStr, "flow") {
		detailedError.TechnicalInfo += " | Flow-related error"
	}

	// Extract validation specific information
	if strings.Contains(errorStr, "traits") {
		detailedError.TechnicalInfo += " | Traits validation issue"
	}

	if detailedError.TechnicalInfo == "" {
		detailedError.TechnicalInfo = "General Kratos error"
	}
}

// generateSuggestions creates actionable suggestions based on error type
func (t *ErrorTransparifier) generateSuggestions(detailedError *DetailedAuthError, kratosErr error) {
	switch detailedError.Type {
	case "MISSING_EMAIL_FIELD":
		detailedError.Suggestions = []string{
			"メールアドレスフィールドが正しく送信されているか確認してください",
			"フロントエンドのフォームデータ構造を確認してください",
			"Content-Typeヘッダーが正しく設定されているか確認してください",
		}

	case "USER_ALREADY_EXISTS":
		detailedError.Suggestions = []string{
			"別のメールアドレスを使用してください",
			"既にアカウントをお持ちの場合はログインしてください",
			"パスワードリセットが必要な場合は、パスワード復旧機能をご利用ください",
		}

	case "FLOW_EXPIRED":
		detailedError.Suggestions = []string{
			"ページを再読み込みして新しい登録フローを開始してください",
			"登録処理は10分以内に完了する必要があります",
			"ブラウザの戻るボタンは使用せず、フォームから直接操作してください",
		}

	case "INVALID_CREDENTIALS":
		detailedError.Suggestions = []string{
			"メールアドレスとパスワードを再確認してください",
			"パスワードの大文字・小文字が正しいか確認してください",
			"パスワードを忘れた場合は、パスワード復旧機能をご利用ください",
		}

	case "VALIDATION_FAILED":
		detailedError.Suggestions = []string{
			"メールアドレスの形式が正しいか確認してください",
			"パスワードが最小8文字以上であることを確認してください",
			"すべての必須フィールドが入力されていることを確認してください",
		}

	case "KRATOS_SERVICE_ERROR":
		detailedError.Suggestions = []string{
			"しばらく時間をおいてから再試行してください",
			"問題が継続する場合は、サポートにお問い合わせください",
			"システムメンテナンス中の可能性があります",
		}

	case "TIMEOUT_ERROR":
		detailedError.Suggestions = []string{
			"ネットワーク接続を確認してください",
			"再試行してください",
			"ページの読み込みが完了してから操作してください",
		}

	case "NETWORK_ERROR":
		detailedError.Suggestions = []string{
			"インターネット接続を確認してください",
			"VPNを使用している場合は、一時的に無効にしてみてください",
			"ファイアウォール設定を確認してください",
		}

	default:
		detailedError.Suggestions = []string{
			"入力内容を確認してもう一度お試しください",
			"問題が継続する場合は、サポートにお問い合わせください",
			"ブラウザの開発者ツールでエラーの詳細を確認してください",
		}
	}
}

// determineRetryability determines if the error is retryable and when
func (t *ErrorTransparifier) determineRetryability(detailedError *DetailedAuthError, kratosErr error) {
	switch detailedError.Type {
	case "USER_ALREADY_EXISTS":
		detailedError.IsRetryable = false

	case "INVALID_CREDENTIALS":
		detailedError.IsRetryable = false

	case "FLOW_EXPIRED":
		detailedError.IsRetryable = true
		// Immediate retry after flow regeneration

	case "KRATOS_SERVICE_ERROR":
		detailedError.IsRetryable = true
		retryAfter := 30 // 30 seconds
		detailedError.RetryAfter = &retryAfter

	case "TIMEOUT_ERROR":
		detailedError.IsRetryable = true
		retryAfter := 10 // 10 seconds
		detailedError.RetryAfter = &retryAfter

	case "NETWORK_ERROR":
		detailedError.IsRetryable = true
		retryAfter := 15 // 15 seconds
		detailedError.RetryAfter = &retryAfter

	case "VALIDATION_FAILED":
		detailedError.IsRetryable = false

	case "MISSING_EMAIL_FIELD":
		detailedError.IsRetryable = true
		// Immediate retry after fixing payload

	default:
		detailedError.IsRetryable = true
		retryAfter := 5 // 5 seconds
		detailedError.RetryAfter = &retryAfter
	}
}

// GetRetryDelay returns the recommended retry delay in seconds
func (d *DetailedAuthError) GetRetryDelay() time.Duration {
	if d.RetryAfter != nil {
		return time.Duration(*d.RetryAfter) * time.Second
	}
	return 0
}

// IsRetryRecommended returns true if a retry is recommended
func (d *DetailedAuthError) IsRetryRecommended() bool {
	return d.IsRetryable
}

// GetUserFriendlyMessage returns a user-friendly error message
func (d *DetailedAuthError) GetUserFriendlyMessage() string {
	return d.Message
}

// GetTechnicalDetails returns technical details for debugging
func (d *DetailedAuthError) GetTechnicalDetails() map[string]interface{} {
	return map[string]interface{}{
		"error_type":        d.Type,
		"error_code":        d.ErrorCode,
		"technical_info":    d.TechnicalInfo,
		"kratos_raw_error":  d.KratosRawError,
		"timestamp":         d.Timestamp,
		"is_retryable":      d.IsRetryable,
		"retry_after":       d.RetryAfter,
		"suggestions_count": len(d.Suggestions),
	}
}