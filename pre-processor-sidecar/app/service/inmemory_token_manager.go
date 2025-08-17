// ABOUTME: InMemoryTokenManager - スレッドセーフなOAuth2トークン管理システム
// ABOUTME: 自動リフレッシュ、セキュアな暗号化、バックグラウンド監視を提供

package service

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"pre-processor-sidecar/driver"
	"pre-processor-sidecar/models"
)

// InMemoryTokenManager はメモリー内でOAuth2トークンを安全に管理する
type InMemoryTokenManager struct {
	// 暗号化されたトークン情報
	encryptedAccessToken  []byte
	encryptedRefreshToken []byte
	expiresAt             time.Time
	tokenType             string

	// 認証情報（暗号化）
	encryptedClientID     []byte
	encryptedClientSecret []byte

	// 暗号化キー
	encryptionKey []byte
	gcm           cipher.AEAD

	// 並行制御
	mutex sync.RWMutex

	// 自動更新制御
	refreshTicker *time.Ticker
	stopChan      chan struct{}
	isRunning     bool

	// OAuth2クライアント
	oauth2Client *driver.OAuth2Client

	// ログとメトリクス
	logger           *slog.Logger
	metricsCollector MetricsCollector

	// 設定
	refreshBuffer time.Duration // トークン期限切れ前の更新バッファ（デフォルト5分）
	checkInterval time.Duration // バックグラウンド確認間隔（デフォルト1分）
}

// TokenInfo はトークン情報を表す構造体
type TokenInfo struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
	TokenType    string
}

// MetricsCollector はメトリクス収集インターフェース
type MetricsCollector interface {
	IncrementTokenRefresh(status string)
	RecordTokenExpiry(expiresInSeconds float64)
	IncrementAutoRefresh()
	IncrementAuthenticationError(errorType string)
	IncrementRecoveryAttempt(success bool)
}

// NoOpMetricsCollector はメトリクス収集のデフォルト実装
type NoOpMetricsCollector struct{}

func (n *NoOpMetricsCollector) IncrementTokenRefresh(status string)         {}
func (n *NoOpMetricsCollector) RecordTokenExpiry(expiresInSeconds float64)  {}
func (n *NoOpMetricsCollector) IncrementAutoRefresh()                       {}
func (n *NoOpMetricsCollector) IncrementAuthenticationError(errorType string) {}
func (n *NoOpMetricsCollector) IncrementRecoveryAttempt(success bool)       {}

// NewInMemoryTokenManagerConfig は設定オプション
type InMemoryTokenManagerConfig struct {
	ClientID         string
	ClientSecret     string
	AccessToken      string
	RefreshToken     string
	RefreshBuffer    time.Duration
	CheckInterval    time.Duration
	OAuth2Client     *driver.OAuth2Client
	Logger           *slog.Logger
	MetricsCollector MetricsCollector
}

// NewInMemoryTokenManager は新しいインメモリートークンマネージャーを作成
func NewInMemoryTokenManager(config InMemoryTokenManagerConfig) (*InMemoryTokenManager, error) {
	// 暗号化キー生成（32バイト = AES-256）
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("failed to generate encryption key: %w", err)
	}

	// AES-GCM暗号化の準備
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// デフォルト値設定
	refreshBuffer := config.RefreshBuffer
	if refreshBuffer == 0 {
		refreshBuffer = 5 * time.Minute
	}

	checkInterval := config.CheckInterval
	if checkInterval == 0 {
		checkInterval = 1 * time.Minute
	}

	logger := config.Logger
	if logger == nil {
		logger = slog.Default()
	}

	metricsCollector := config.MetricsCollector
	if metricsCollector == nil {
		metricsCollector = &NoOpMetricsCollector{}
	}

	manager := &InMemoryTokenManager{
		encryptionKey:     key,
		gcm:               gcm,
		refreshBuffer:     refreshBuffer,
		checkInterval:     checkInterval,
		oauth2Client:      config.OAuth2Client,
		logger:            logger,
		metricsCollector:  metricsCollector,
		stopChan:          make(chan struct{}),
	}

	// 初期認証情報を暗号化して保存
	if err := manager.encryptAndStoreCredentials(config.ClientID, config.ClientSecret); err != nil {
		return nil, fmt.Errorf("failed to encrypt credentials: %w", err)
	}

	// 初期アクセストークンがあれば設定（24時間の有効期限）
	if config.AccessToken != "" {
		expiresAt := time.Now().Add(24 * time.Hour) // Inoreaderのアクセストークンは24時間有効
		if err := manager.setInitialAccessToken(config.AccessToken, expiresAt); err != nil {
			return nil, fmt.Errorf("failed to set initial access token: %w", err)
		}
		logger.Info("Initial access token set with 24-hour expiry", "expires_at", expiresAt)
	}

	// 初期リフレッシュトークンがあれば設定
	if config.RefreshToken != "" {
		if err := manager.setEncryptedRefreshToken(config.RefreshToken); err != nil {
			return nil, fmt.Errorf("failed to set initial refresh token: %w", err)
		}

		// アクセストークンが設定されていない場合のみ初回トークン取得を試行
		if config.AccessToken == "" {
			if err := manager.performInitialTokenRefresh(); err != nil {
				logger.Warn("Initial token refresh failed, will retry in background",
					"error", err)
			}
		}
	}

	logger.Info("InMemoryTokenManager initialized successfully",
		"refresh_buffer_minutes", refreshBuffer.Minutes(),
		"check_interval_minutes", checkInterval.Minutes())

	return manager, nil
}

// encrypt はデータを暗号化する
func (m *InMemoryTokenManager) encrypt(plaintext string) ([]byte, error) {
	nonce := make([]byte, m.gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	ciphertext := m.gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return ciphertext, nil
}

// decrypt は暗号化されたデータを復号化する
func (m *InMemoryTokenManager) decrypt(ciphertext []byte) (string, error) {
	if len(ciphertext) < m.gcm.NonceSize() {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:m.gcm.NonceSize()], ciphertext[m.gcm.NonceSize():]
	plaintext, err := m.gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt: %w", err)
	}

	return string(plaintext), nil
}

// setInitialAccessToken は初期アクセストークンと有効期限を設定
func (m *InMemoryTokenManager) setInitialAccessToken(accessToken string, expiresAt time.Time) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// アクセストークンを暗号化
	encrypted, err := m.encrypt(accessToken)
	if err != nil {
		return fmt.Errorf("failed to encrypt access token: %w", err)
	}

	m.encryptedAccessToken = encrypted
	m.expiresAt = expiresAt
	m.tokenType = "Bearer"

	m.logger.Info("Access token encrypted and stored with expiry", 
		"expires_at", expiresAt,
		"expires_in_hours", time.Until(expiresAt).Hours())

	return nil
}

// encryptAndStoreCredentials は認証情報を暗号化して保存
func (m *InMemoryTokenManager) encryptAndStoreCredentials(clientID, clientSecret string) error {
	encryptedID, err := m.encrypt(clientID)
	if err != nil {
		return fmt.Errorf("failed to encrypt client ID: %w", err)
	}

	encryptedSecret, err := m.encrypt(clientSecret)
	if err != nil {
		return fmt.Errorf("failed to encrypt client secret: %w", err)
	}

	m.mutex.Lock()
	m.encryptedClientID = encryptedID
	m.encryptedClientSecret = encryptedSecret
	m.mutex.Unlock()

	return nil
}

// setEncryptedRefreshToken はリフレッシュトークンを暗号化して保存
func (m *InMemoryTokenManager) setEncryptedRefreshToken(refreshToken string) error {
	encrypted, err := m.encrypt(refreshToken)
	if err != nil {
		return fmt.Errorf("failed to encrypt refresh token: %w", err)
	}

	m.mutex.Lock()
	m.encryptedRefreshToken = encrypted
	m.mutex.Unlock()

	return nil
}

// getDecryptedCredentials は復号化された認証情報を取得
func (m *InMemoryTokenManager) getDecryptedCredentials() (clientID, clientSecret string, err error) {
	m.mutex.RLock()
	encryptedID := make([]byte, len(m.encryptedClientID))
	encryptedSecret := make([]byte, len(m.encryptedClientSecret))
	copy(encryptedID, m.encryptedClientID)
	copy(encryptedSecret, m.encryptedClientSecret)
	m.mutex.RUnlock()

	clientID, err = m.decrypt(encryptedID)
	if err != nil {
		return "", "", fmt.Errorf("failed to decrypt client ID: %w", err)
	}

	clientSecret, err = m.decrypt(encryptedSecret)
	if err != nil {
		return "", "", fmt.Errorf("failed to decrypt client secret: %w", err)
	}

	return clientID, clientSecret, nil
}

// getDecryptedRefreshToken は復号化されたリフレッシュトークンを取得
func (m *InMemoryTokenManager) getDecryptedRefreshToken() (string, error) {
	m.mutex.RLock()
	encrypted := make([]byte, len(m.encryptedRefreshToken))
	copy(encrypted, m.encryptedRefreshToken)
	m.mutex.RUnlock()

	if len(encrypted) == 0 {
		return "", fmt.Errorf("no refresh token available")
	}

	return m.decrypt(encrypted)
}

// GetValidToken はスレッドセーフにアクセストークンを取得
func (m *InMemoryTokenManager) GetValidToken(ctx context.Context) (*models.OAuth2Token, error) {
	m.mutex.RLock()
	
	// トークンの期限確認
	if time.Now().Add(m.refreshBuffer).After(m.expiresAt) {
		m.mutex.RUnlock()
		
		// 期限切れ間近または期限切れ - リフレッシュを試行
		m.logger.Info("Token refresh needed",
			"expires_at", m.expiresAt,
			"buffer_minutes", m.refreshBuffer.Minutes())
		
		if err := m.RefreshTokenIfNeeded(ctx); err != nil {
			m.metricsCollector.IncrementAuthenticationError("refresh_failed")
			return nil, fmt.Errorf("failed to refresh token: %w", err)
		}
		
		m.mutex.RLock()
	}

	// 復号化してトークン情報を取得
	accessToken, err := m.decrypt(m.encryptedAccessToken)
	if err != nil {
		m.mutex.RUnlock()
		return nil, fmt.Errorf("failed to decrypt access token: %w", err)
	}

	token := &models.OAuth2Token{
		AccessToken: accessToken,
		TokenType:   m.tokenType,
		ExpiresAt:   m.expiresAt,
	}

	expiresIn := time.Until(m.expiresAt).Seconds()
	m.metricsCollector.RecordTokenExpiry(expiresIn)

	m.mutex.RUnlock()

	m.logger.Debug("Valid token retrieved",
		"expires_in_seconds", int64(expiresIn),
		"token_type", m.tokenType)

	return token, nil
}

// RefreshTokenIfNeeded は必要に応じてトークンをリフレッシュ
func (m *InMemoryTokenManager) RefreshTokenIfNeeded(ctx context.Context) error {
	refreshToken, err := m.getDecryptedRefreshToken()
	if err != nil {
		return fmt.Errorf("no refresh token available: %w", err)
	}

	// 認証情報は OAuth2Client の初期化時に設定済み

	m.logger.Info("Attempting token refresh",
		"current_expires_at", m.expiresAt)

	// トークンリフレッシュ実行
	newToken, err := m.oauth2Client.RefreshToken(ctx, refreshToken)
	if err != nil {
		m.metricsCollector.IncrementTokenRefresh("failure")
		return fmt.Errorf("OAuth2 token refresh failed: %w", err)
	}

	// 新しいトークン情報をOAuth2Tokenに変換して保存
	currentRefreshToken, err := m.getDecryptedRefreshToken()
	if err != nil {
		return fmt.Errorf("failed to get current refresh token: %w", err)
	}
	
	oauth2Token := models.NewOAuth2Token(*newToken, currentRefreshToken)
	if err := m.storeNewTokenInfo(oauth2Token); err != nil {
		m.metricsCollector.IncrementTokenRefresh("failure")
		return fmt.Errorf("failed to store new token: %w", err)
	}

	m.metricsCollector.IncrementTokenRefresh("success")
	
	m.logger.Info("Token refreshed successfully",
		"new_expires_at", m.expiresAt,
		"expires_in_seconds", int64(time.Until(m.expiresAt).Seconds()))

	return nil
}

// storeNewTokenInfo は新しいトークン情報を保存
func (m *InMemoryTokenManager) storeNewTokenInfo(token *models.OAuth2Token) error {
	encryptedAccess, err := m.encrypt(token.AccessToken)
	if err != nil {
		return fmt.Errorf("failed to encrypt access token: %w", err)
	}

	// リフレッシュトークンが含まれていれば更新
	if token.RefreshToken != "" {
		if err := m.setEncryptedRefreshToken(token.RefreshToken); err != nil {
			return fmt.Errorf("failed to encrypt new refresh token: %w", err)
		}
	}

	m.mutex.Lock()
	m.encryptedAccessToken = encryptedAccess
	m.expiresAt = token.ExpiresAt
	m.tokenType = token.TokenType
	m.mutex.Unlock()

	return nil
}

// performInitialTokenRefresh は初回トークンリフレッシュを実行
func (m *InMemoryTokenManager) performInitialTokenRefresh() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	return m.RefreshTokenIfNeeded(ctx)
}

// UpdateRefreshToken は新しいリフレッシュトークンを設定（Admin API用）
// NOTE: Secret更新の場合はUpdateTokenDirectly()を使用してOAuth2 API競合を回避
func (m *InMemoryTokenManager) UpdateRefreshToken(ctx context.Context, refreshToken string, clientID, clientSecret string) error {
	m.logger.Info("Updating refresh token via admin API (triggers OAuth2 API call)",
		"warning", "Use UpdateTokenDirectly() for Secret updates to avoid token conflicts")

	// 認証情報が提供されていれば更新
	if clientID != "" && clientSecret != "" {
		if err := m.encryptAndStoreCredentials(clientID, clientSecret); err != nil {
			return fmt.Errorf("failed to update credentials: %w", err)
		}
		m.logger.Info("OAuth2 credentials updated")
	}

	// 新しいリフレッシュトークンを設定
	if err := m.setEncryptedRefreshToken(refreshToken); err != nil {
		return fmt.Errorf("failed to set refresh token: %w", err)
	}

	// 即座にアクセストークンを取得
	if err := m.RefreshTokenIfNeeded(ctx); err != nil {
		return fmt.Errorf("failed to refresh with new token: %w", err)
	}

	m.logger.Info("Refresh token updated and access token refreshed successfully",
		"new_expires_at", m.expiresAt)

	return nil
}

// UpdateTokenDirectly は外部から取得したトークンを直接更新（API呼び出しなし）
// auth-token-managerからのSecret更新で使用、OAuth2 API競合を回避
func (m *InMemoryTokenManager) UpdateTokenDirectly(ctx context.Context, token *models.OAuth2Token) error {
	m.logger.Info("Updating token directly from external source (no API call)",
		"expires_at", token.ExpiresAt,
		"scope", token.Scope,
		"expires_in_hours", time.Until(token.ExpiresAt).Hours())

	// バリデーション: 必須フィールドチェック
	if token.AccessToken == "" {
		return fmt.Errorf("access token cannot be empty")
	}
	if token.RefreshToken == "" {
		return fmt.Errorf("refresh token cannot be empty")
	}

	// アクセストークンを暗号化
	encryptedAccess, err := m.encrypt(token.AccessToken)
	if err != nil {
		return fmt.Errorf("failed to encrypt access token: %w", err)
	}

	// リフレッシュトークンを暗号化
	if err := m.setEncryptedRefreshToken(token.RefreshToken); err != nil {
		return fmt.Errorf("failed to set refresh token: %w", err)
	}

	// 内部状態を更新
	m.mutex.Lock()
	m.encryptedAccessToken = encryptedAccess
	m.expiresAt = token.ExpiresAt
	m.tokenType = token.TokenType
	if m.tokenType == "" {
		m.tokenType = "Bearer"
	}
	m.mutex.Unlock()

	m.logger.Info("Token updated directly without API call",
		"new_expires_at", m.expiresAt,
		"expires_in_seconds", int64(time.Until(m.expiresAt).Seconds()),
		"token_type", m.tokenType)

	return nil
}

// StartAutoRefresh はバックグラウンドでの自動リフレッシュを開始
func (m *InMemoryTokenManager) StartAutoRefresh() {
	m.mutex.Lock()
	if m.isRunning {
		m.mutex.Unlock()
		return
	}
	m.isRunning = true
	m.refreshTicker = time.NewTicker(m.checkInterval)
	m.mutex.Unlock()

	go m.autoRefreshLoop()

	m.logger.Info("Auto refresh started",
		"check_interval_minutes", m.checkInterval.Minutes())
}

// autoRefreshLoop はバックグラウンド自動リフレッシュループ
func (m *InMemoryTokenManager) autoRefreshLoop() {
	for {
		select {
		case <-m.refreshTicker.C:
			m.checkAndRefreshToken()
		case <-m.stopChan:
			m.logger.Info("Auto refresh stopped")
			return
		}
	}
}

// checkAndRefreshToken はトークンの期限をチェックし、必要に応じてリフレッシュ
func (m *InMemoryTokenManager) checkAndRefreshToken() {
	m.mutex.RLock()
	needsRefresh := time.Now().Add(m.refreshBuffer).After(m.expiresAt)
	m.mutex.RUnlock()

	if needsRefresh {
		m.logger.Info("Auto refresh triggered",
			"expires_at", m.expiresAt)

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := m.RefreshTokenIfNeeded(ctx); err != nil {
			m.logger.Error("Auto refresh failed", "error", err)
			m.metricsCollector.IncrementAuthenticationError("auto_refresh_failed")
		} else {
			m.metricsCollector.IncrementAutoRefresh()
			m.logger.Info("Auto refresh completed successfully")
		}
	}
}

// Stop はバックグラウンド処理を停止し、リソースをクリーンアップ
func (m *InMemoryTokenManager) Stop() {
	m.mutex.Lock()
	if !m.isRunning {
		m.mutex.Unlock()
		return
	}
	
	m.isRunning = false
	if m.refreshTicker != nil {
		m.refreshTicker.Stop()
	}
	m.mutex.Unlock()

	// 停止シグナルを送信
	close(m.stopChan)

	// メモリークリア
	m.clearSensitiveData()

	m.logger.Info("InMemoryTokenManager stopped and sensitive data cleared")
}

// clearSensitiveData はメモリー内の機密データをクリア
func (m *InMemoryTokenManager) clearSensitiveData() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// バイトスライスをゼロクリア
	if m.encryptedAccessToken != nil {
		for i := range m.encryptedAccessToken {
			m.encryptedAccessToken[i] = 0
		}
		m.encryptedAccessToken = nil
	}

	if m.encryptedRefreshToken != nil {
		for i := range m.encryptedRefreshToken {
			m.encryptedRefreshToken[i] = 0
		}
		m.encryptedRefreshToken = nil
	}

	if m.encryptedClientID != nil {
		for i := range m.encryptedClientID {
			m.encryptedClientID[i] = 0
		}
		m.encryptedClientID = nil
	}

	if m.encryptedClientSecret != nil {
		for i := range m.encryptedClientSecret {
			m.encryptedClientSecret[i] = 0
		}
		m.encryptedClientSecret = nil
	}

	if m.encryptionKey != nil {
		for i := range m.encryptionKey {
			m.encryptionKey[i] = 0
		}
		m.encryptionKey = nil
	}
}

// GetTokenStatus はトークンの現在状態を取得（管理用）
func (m *InMemoryTokenManager) GetTokenStatus() TokenStatus {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	status := TokenStatus{
		HasAccessToken:   len(m.encryptedAccessToken) > 0,
		HasRefreshToken:  len(m.encryptedRefreshToken) > 0,
		ExpiresAt:        m.expiresAt,
		TokenType:        m.tokenType,
		IsAutoRefreshing: m.isRunning,
	}

	if !m.expiresAt.IsZero() {
		status.ExpiresInSeconds = int64(time.Until(m.expiresAt).Seconds())
		status.NeedsRefresh = time.Now().Add(m.refreshBuffer).After(m.expiresAt)
	}

	return status
}

// TokenStatus はトークンの状態情報
type TokenStatus struct {
	HasAccessToken    bool      `json:"has_access_token"`
	HasRefreshToken   bool      `json:"has_refresh_token"`
	ExpiresAt         time.Time `json:"expires_at"`
	ExpiresInSeconds  int64     `json:"expires_in_seconds"`
	TokenType         string    `json:"token_type"`
	NeedsRefresh      bool      `json:"needs_refresh"`
	IsAutoRefreshing  bool      `json:"is_auto_refreshing"`
}