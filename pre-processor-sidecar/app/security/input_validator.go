// ABOUTME: OWASP準拠の入力検証・サニタイゼーション機能
// ABOUTME: SQLインジェクション、XSS、パストラバーサル対策を実装

package security

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"pre-processor-sidecar/models"
)

// OWASPInputValidator はOWASP準拠の入力検証器
type OWASPInputValidator struct {
	// 正規表現パターン
	refreshTokenPattern *regexp.Regexp
	clientIDPattern     *regexp.Regexp
	clientSecretPattern *regexp.Regexp

	// 危険な文字列パターン
	sqlInjectionPatterns []*regexp.Regexp
	xssPatterns          []*regexp.Regexp
	pathTraversalPattern *regexp.Regexp

	// ホワイトリスト
	safeCharacters *regexp.Regexp
}

// ValidationError は検証エラーを表す
type ValidationError struct {
	Field   string
	Message string
	Value   string // デバッグ用（本番では空にする）
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("validation failed for field '%s': %s", e.Field, e.Message)
}

// NewOWASPInputValidator は新しい入力検証器を作成
func NewOWASPInputValidator() *OWASPInputValidator {
	validator := &OWASPInputValidator{
		// Inoreaderリフレッシュトークン: 32-512文字、英数字とアンダースコア
		refreshTokenPattern: regexp.MustCompile(`^[a-zA-Z0-9_]{32,512}$`),

		// InoreaderクライアントID: 数字10桁
		clientIDPattern: regexp.MustCompile(`^[0-9]{10}$`),

		// Inoreaderクライアントシークレット: 32-128文字、安全な文字セット
		clientSecretPattern: regexp.MustCompile(`^[a-zA-Z0-9_\-]{32,128}$`),

		// 安全な文字セット（英数字、ハイフン、アンダースコア）
		safeCharacters: regexp.MustCompile(`^[a-zA-Z0-9_\-]*$`),

		// パストラバーサル対策
		pathTraversalPattern: regexp.MustCompile(`\.\.[\\/]|[\\/]\.\.|^\.\.[\\/]|[\\/]\.\.$`),
	}

	// SQLインジェクション検出パターン
	validator.sqlInjectionPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)(union\s+(all\s+)?select|insert\s+into|update\s+\w+\s+set|delete\s+from)`),
		regexp.MustCompile(`(?i)(exec\s*\(|sp_executesql|xp_cmdshell|sp_addsrvrolemember)`),
		regexp.MustCompile(`(?i)(drop\s+(table|database|schema|view|trigger|procedure|function))`),
		regexp.MustCompile(`(?i)(alter\s+(table|database|schema|view))`),
		regexp.MustCompile(`(?i)(create\s+(table|database|schema|view|trigger|procedure|function))`),
		regexp.MustCompile(`(?i)(\'\s*(or|and)\s*\'\s*=\s*\'|\'\s*(or|and)\s*\'.*\')`),
		regexp.MustCompile(`(?i)(\'\s*;\s*(update|insert|delete|drop|create|alter))`),
		regexp.MustCompile(`(?i)(benchmark\s*\(|sleep\s*\(|waitfor\s+delay)`),
	}

	// XSS検出パターン
	validator.xssPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)<script[^>]*>.*</script>`),
		regexp.MustCompile(`(?i)<script[^>]*>`),
		regexp.MustCompile(`(?i)javascript:`),
		regexp.MustCompile(`(?i)on\w+\s*=`),
		regexp.MustCompile(`(?i)<iframe[^>]*>`),
		regexp.MustCompile(`(?i)<object[^>]*>`),
		regexp.MustCompile(`(?i)<embed[^>]*>`),
		regexp.MustCompile(`(?i)<link[^>]*>`),
		regexp.MustCompile(`(?i)<meta[^>]*>`),
		regexp.MustCompile(`(?i)expression\s*\(`),
		regexp.MustCompile(`(?i)@import`),
		regexp.MustCompile(`(?i)vbscript:`),
		regexp.MustCompile(`(?i)data:text/html`),
	}

	return validator
}

// ValidateTokenUpdateRequest はトークン更新リクエストを検証
func (v *OWASPInputValidator) ValidateTokenUpdateRequest(req *models.TokenUpdateRequest) error {
	// リフレッシュトークンは必須
	if req.RefreshToken == "" {
		return &ValidationError{
			Field:   "refresh_token",
			Message: "refresh token is required",
		}
	}

	// リフレッシュトークンの形式検証
	if err := v.validateRefreshToken(req.RefreshToken); err != nil {
		return err
	}

	// クライアントIDが提供されている場合の検証
	if req.ClientID != "" {
		if err := v.validateClientID(req.ClientID); err != nil {
			return err
		}
	}

	// クライアントシークレットが提供されている場合の検証
	if req.ClientSecret != "" {
		if err := v.validateClientSecret(req.ClientSecret); err != nil {
			return err
		}
	}

	// セキュリティ脅威の検証
	if err := v.validateSecurityThreats(req); err != nil {
		return err
	}

	return nil
}

// validateRefreshToken はリフレッシュトークンを検証
func (v *OWASPInputValidator) validateRefreshToken(token string) error {
	// 長さ制限
	if len(token) < 32 {
		return &ValidationError{
			Field:   "refresh_token",
			Message: "refresh token must be at least 32 characters",
		}
	}

	if len(token) > 512 {
		return &ValidationError{
			Field:   "refresh_token",
			Message: "refresh token must not exceed 512 characters",
		}
	}

	// パターンマッチング
	if !v.refreshTokenPattern.MatchString(token) {
		return &ValidationError{
			Field:   "refresh_token",
			Message: "refresh token contains invalid characters (only alphanumeric and underscore allowed)",
		}
	}

	// 制御文字チェック
	if containsControlCharacters(token) {
		return &ValidationError{
			Field:   "refresh_token",
			Message: "refresh token contains invalid control characters",
		}
	}

	return nil
}

// validateClientID はクライアントIDを検証
func (v *OWASPInputValidator) validateClientID(clientID string) error {
	// Inoreaderクライアント形式：数字10桁
	if !v.clientIDPattern.MatchString(clientID) {
		return &ValidationError{
			Field:   "client_id",
			Message: "client ID must be exactly 10 digits",
		}
	}

	return nil
}

// validateClientSecret はクライアントシークレットを検証
func (v *OWASPInputValidator) validateClientSecret(secret string) error {
	// 長さ制限
	if len(secret) < 32 {
		return &ValidationError{
			Field:   "client_secret",
			Message: "client secret must be at least 32 characters",
		}
	}

	if len(secret) > 128 {
		return &ValidationError{
			Field:   "client_secret",
			Message: "client secret must not exceed 128 characters",
		}
	}

	// パターンマッチング
	if !v.clientSecretPattern.MatchString(secret) {
		return &ValidationError{
			Field:   "client_secret",
			Message: "client secret contains invalid characters (only alphanumeric, underscore, and hyphen allowed)",
		}
	}

	// 制御文字チェック
	if containsControlCharacters(secret) {
		return &ValidationError{
			Field:   "client_secret",
			Message: "client secret contains invalid control characters",
		}
	}

	return nil
}

// validateSecurityThreats はセキュリティ脅威を検証
func (v *OWASPInputValidator) validateSecurityThreats(req *models.TokenUpdateRequest) error {
	// 全フィールドをチェック
	fields := map[string]string{
		"refresh_token": req.RefreshToken,
		"client_id":     req.ClientID,
		"client_secret": req.ClientSecret,
	}

	for fieldName, value := range fields {
		if value == "" {
			continue
		}

		// SQLインジェクション検査
		if err := v.checkSQLInjection(fieldName, value); err != nil {
			return err
		}

		// XSS検査
		if err := v.checkXSS(fieldName, value); err != nil {
			return err
		}

		// パストラバーサル検査
		if err := v.checkPathTraversal(fieldName, value); err != nil {
			return err
		}

		// 危険なバイト検査
		if err := v.checkDangerousBytes(fieldName, value); err != nil {
			return err
		}
	}

	return nil
}

// checkSQLInjection はSQLインジェクションパターンをチェック
func (v *OWASPInputValidator) checkSQLInjection(fieldName, value string) error {
	for _, pattern := range v.sqlInjectionPatterns {
		if pattern.MatchString(value) {
			return &ValidationError{
				Field:   fieldName,
				Message: "potential SQL injection detected",
			}
		}
	}
	return nil
}

// checkXSS はXSSパターンをチェック
func (v *OWASPInputValidator) checkXSS(fieldName, value string) error {
	for _, pattern := range v.xssPatterns {
		if pattern.MatchString(value) {
			return &ValidationError{
				Field:   fieldName,
				Message: "potential XSS attack detected",
			}
		}
	}
	return nil
}

// checkPathTraversal はパストラバーサルパターンをチェック
func (v *OWASPInputValidator) checkPathTraversal(fieldName, value string) error {
	if v.pathTraversalPattern.MatchString(value) {
		return &ValidationError{
			Field:   fieldName,
			Message: "potential path traversal attack detected",
		}
	}
	return nil
}

// checkDangerousBytes は危険なバイト値をチェック
func (v *OWASPInputValidator) checkDangerousBytes(fieldName, value string) error {
	// NULL文字チェック
	if strings.Contains(value, "\x00") {
		return &ValidationError{
			Field:   fieldName,
			Message: "null bytes are not allowed",
		}
	}

	// 危険な制御文字チェック
	dangerousChars := []rune{
		'\x01', '\x02', '\x03', '\x04', '\x05', '\x06', '\x07', '\x08',
		'\x0B', '\x0C', '\x0E', '\x0F', '\x10', '\x11', '\x12', '\x13',
		'\x14', '\x15', '\x16', '\x17', '\x18', '\x19', '\x1A', '\x1B',
		'\x1C', '\x1D', '\x1E', '\x1F', '\x7F',
	}

	for _, char := range dangerousChars {
		if strings.ContainsRune(value, char) {
			return &ValidationError{
				Field:   fieldName,
				Message: fmt.Sprintf("dangerous control character detected: 0x%02X", char),
			}
		}
	}

	return nil
}

// SanitizeString は文字列をサニタイズ
func (v *OWASPInputValidator) SanitizeString(input string) string {
	if input == "" {
		return ""
	}

	// 改行文字除去
	input = strings.ReplaceAll(input, "\r", "")
	input = strings.ReplaceAll(input, "\n", "")

	// タブ文字除去
	input = strings.ReplaceAll(input, "\t", "")

	// 前後の空白除去
	input = strings.TrimSpace(input)

	// NULL文字除去
	input = strings.ReplaceAll(input, "\x00", "")

	// 連続する空白を単一の空白に変換
	spaceRegex := regexp.MustCompile(`\s+`)
	input = spaceRegex.ReplaceAllString(input, " ")

	// HTMLエンティティエスケープ（念のため）
	input = strings.ReplaceAll(input, "<", "&lt;")
	input = strings.ReplaceAll(input, ">", "&gt;")
	input = strings.ReplaceAll(input, "\"", "&quot;")
	input = strings.ReplaceAll(input, "'", "&#39;")
	input = strings.ReplaceAll(input, "&", "&amp;")

	return input
}

// containsControlCharacters は制御文字の存在をチェック
func containsControlCharacters(s string) bool {
	for _, r := range s {
		// 印刷可能文字以外（制御文字）をチェック
		// タブ(\t)、改行(\n)、復帰(\r)、空白は除外
		if unicode.IsControl(r) && r != '\t' && r != '\n' && r != '\r' {
			return true
		}
	}
	return false
}

// ValidateAndSanitizeInput は入力の検証とサニタイゼーションを同時実行
func (v *OWASPInputValidator) ValidateAndSanitizeInput(input string, fieldName string) (string, error) {
	// まず基本的なサニタイゼーション
	sanitized := v.SanitizeString(input)

	// 長さ制限（一般的な制限）
	if len(sanitized) > 1024 {
		return "", &ValidationError{
			Field:   fieldName,
			Message: "input exceeds maximum length of 1024 characters",
		}
	}

	// セキュリティ脅威チェック
	tempReq := &models.TokenUpdateRequest{}
	switch fieldName {
	case "refresh_token":
		tempReq.RefreshToken = sanitized
	case "client_id":
		tempReq.ClientID = sanitized
	case "client_secret":
		tempReq.ClientSecret = sanitized
	}

	if err := v.validateSecurityThreats(tempReq); err != nil {
		return "", err
	}

	return sanitized, nil
}

// GetValidationRules は検証ルールの説明を返す（開発用）
func (v *OWASPInputValidator) GetValidationRules() map[string]string {
	return map[string]string{
		"refresh_token":  "32-512 characters, alphanumeric and underscore only",
		"client_id":      "exactly 10 digits",
		"client_secret":  "32-128 characters, alphanumeric, underscore, and hyphen only",
		"security_rules": "No SQL injection, XSS, path traversal, or dangerous control characters allowed",
		"sanitization":   "Removes control characters, trims whitespace, escapes HTML entities",
	}
}
