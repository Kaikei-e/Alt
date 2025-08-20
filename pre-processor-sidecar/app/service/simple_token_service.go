// ABOUTME: SimpleTokenService - 循環インポート回避のための簡易版統合サービス
// ABOUTME: InMemoryTokenManagerとRecoveryManagerを統合、Admin APIは後で追加

package service

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"pre-processor-sidecar/driver"
	"pre-processor-sidecar/models"
)

// SimpleTokenService は簡易版統合トークンサービス
type SimpleTokenService struct {
	inMemoryManager  *InMemoryTokenManager
	recoveryManager  *RecoveryManager
	oauth2Client     *driver.OAuth2Client
	oauth2SecretSvc  *OAuth2SecretService
	logger           *slog.Logger
	isStarted        bool
	
	// 自律的Secret再読み込み機能 (恒久対応)
	secretWatchEnabled bool
}

// SimpleTokenConfig は簡易版設定
type SimpleTokenConfig struct {
	ClientID            string
	ClientSecret        string
	InitialAccessToken  string
	InitialRefreshToken string
	BaseURL             string
	RefreshBuffer       time.Duration
	CheckInterval       time.Duration
	
	// OAuth2 Secret設定
	OAuth2SecretName string
	KubernetesNamespace string
	
	// 自律的Secret再読み込み設定 (恒久対応)
	EnableSecretWatch bool
}

// NewSimpleTokenService は新しい簡易統合サービスを作成
func NewSimpleTokenService(config SimpleTokenConfig, logger *slog.Logger) (*SimpleTokenService, error) {
	if logger == nil {
		logger = slog.Default()
	}

	// デフォルト値設定
	if config.RefreshBuffer == 0 {
		config.RefreshBuffer = 5 * time.Minute
	}
	if config.CheckInterval == 0 {
		config.CheckInterval = 1 * time.Minute
	}

	// OAuth2クライアントの作成
	oauth2Client := driver.NewOAuth2Client(config.ClientID, config.ClientSecret, config.BaseURL)
	
	// HTTPクライアントの設定（プロキシ対応）
	if httpsProxy := os.Getenv("HTTPS_PROXY"); httpsProxy != "" {
		httpClient := &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				Proxy: http.ProxyFromEnvironment,
			},
		}
		oauth2Client.SetHTTPClient(httpClient)
		logger.Info("OAuth2 client configured with proxy", "proxy", httpsProxy)
	}
	
	// OAuth2 Secretサービスの初期化
	oauth2SecretSvc, err := NewOAuth2SecretService(OAuth2SecretConfig{
		Namespace:  config.KubernetesNamespace,
		SecretName: config.OAuth2SecretName,
		Logger:     logger,
	})
	if err != nil {
		logger.Warn("Failed to initialize OAuth2SecretService, using environment variables only", "error", err)
		oauth2SecretSvc = nil
	}

	// トークンの読み込み - OAuth2 Secretを優先
	initialAccessToken := config.InitialAccessToken
	initialRefreshToken := config.InitialRefreshToken
	var initialExpiresAt time.Time
	
	// OAuth2 Secretからトークンを読み込み（利用可能であれば）
	if oauth2SecretSvc != nil {
		ctx := context.Background()
		secretToken, err := oauth2SecretSvc.LoadOAuth2Token(ctx)
		if err != nil {
			logger.Warn("Failed to load token from OAuth2 Secret, falling back to environment variables", "error", err)
		} else {
			logger.Info("Successfully loaded tokens from OAuth2 Secret - auth-token-manager integration active")
			initialAccessToken = secretToken.AccessToken
			initialRefreshToken = secretToken.RefreshToken
			initialExpiresAt = secretToken.ExpiresAt // 重要: 実際の有効期限を取得
			logger.Info("Using actual token expiry from Secret", "expires_at", initialExpiresAt, "expires_in_hours", time.Until(initialExpiresAt).Hours())
		}
	}

	// InMemoryTokenManagerの作成
	tokenManagerConfig := InMemoryTokenManagerConfig{
		ClientID:         config.ClientID,
		ClientSecret:     config.ClientSecret,
		AccessToken:      initialAccessToken,
		RefreshToken:     initialRefreshToken,
		ExpiresAt:        initialExpiresAt, // 実際の有効期限を渡す
		RefreshBuffer:    config.RefreshBuffer,
		CheckInterval:    config.CheckInterval,
		OAuth2Client:     oauth2Client,
		Logger:           logger,
		MetricsCollector: &NoOpMetricsCollector{},
	}

	inMemoryManager, err := NewInMemoryTokenManager(tokenManagerConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create InMemoryTokenManager: %w", err)
	}

	// RecoveryManagerの作成
	recoveryManager := NewRecoveryManager(
		inMemoryManager,
		oauth2Client,
		logger,
		&NoOpRecoveryMetrics{},
		&EnvironmentFallbackTokenSource{logger: logger},
	)

	service := &SimpleTokenService{
		inMemoryManager:    inMemoryManager,
		recoveryManager:    recoveryManager,
		oauth2Client:       oauth2Client,
		oauth2SecretSvc:    oauth2SecretSvc,
		logger:             logger,
		secretWatchEnabled: config.EnableSecretWatch && oauth2SecretSvc != nil,
	}

	logger.Info("SimpleTokenService created successfully",
		"refresh_buffer_minutes", config.RefreshBuffer.Minutes(),
		"check_interval_minutes", config.CheckInterval.Minutes(),
		"secret_watch_enabled", service.secretWatchEnabled)

	return service, nil
}

// Start はサービスを開始
func (sts *SimpleTokenService) Start() error {
	if sts.isStarted {
		return fmt.Errorf("service already started")
	}

	// InMemoryTokenManagerの自動リフレッシュ開始
	sts.inMemoryManager.StartAutoRefresh()

	// RecoveryManagerの開始
	sts.recoveryManager.Start()

	// Secret監視の開始 (恒久対応: 自律的Secret再読み込み)
	if sts.secretWatchEnabled {
		if err := sts.oauth2SecretSvc.StartWatching(sts.onSecretUpdate); err != nil {
			sts.logger.Warn("Failed to start secret watching", "error", err)
		} else {
			sts.logger.Info("Secret watching started successfully")
		}
	}

	sts.isStarted = true
	sts.logger.Info("SimpleTokenService started successfully",
		"secret_watch_enabled", sts.secretWatchEnabled)
	return nil
}

// Stop はサービスを停止
func (sts *SimpleTokenService) Stop() error {
	if !sts.isStarted {
		return nil
	}

	sts.logger.Info("Stopping SimpleTokenService...")

	// Secret監視の停止 (恒久対応: 自律的Secret再読み込み)
	if sts.secretWatchEnabled && sts.oauth2SecretSvc != nil {
		sts.oauth2SecretSvc.StopWatching()
	}

	// RecoveryManagerの停止
	sts.recoveryManager.Stop()

	// InMemoryTokenManagerの停止
	sts.inMemoryManager.Stop()

	sts.isStarted = false
	sts.logger.Info("SimpleTokenService stopped")
	return nil
}

// GetValidToken は有効なトークンを取得
func (sts *SimpleTokenService) GetValidToken(ctx context.Context) (*models.OAuth2Token, error) {
	token, err := sts.inMemoryManager.GetValidToken(ctx)
	if err != nil {
		// 認証エラーの場合、段階的回復を試行 (恒久対応: 自律的Secret再読み込み)
		if isSimpleAuthenticationError(err) {
			sts.logger.Warn("Authentication error detected, starting recovery process", "error", err)
			
			// 段階1: 既存のRecoveryManagerで内部リフレッシュを試行
			if recoveryErr := sts.recoveryManager.RequestRecovery(
				"authentication_failed",
				"token_invalid",
				err,
			); recoveryErr != nil {
				sts.logger.Warn("Internal recovery failed, trying secret reload", 
					"original_error", err, "recovery_error", recoveryErr)
				
				// 段階2: Secret再読み込みを試行 (恒久対応)
				if sts.oauth2SecretSvc != nil {
					if reloadErr := sts.ReloadFromSecret(ctx); reloadErr != nil {
						sts.logger.Error("Secret reload also failed", 
							"original_error", err, 
							"recovery_error", recoveryErr,
							"reload_error", reloadErr)
						return nil, fmt.Errorf("all recovery attempts failed: original=%v, recovery=%v, reload=%v", 
							err, recoveryErr, reloadErr)
					}
					sts.logger.Info("Secret reload successful, retrying token retrieval")
				} else {
					sts.logger.Error("No secret service available for reload")
					return nil, fmt.Errorf("token retrieval failed and recovery failed: %v (recovery: %v)", err, recoveryErr)
				}
			}

			// 回復後に再試行
			return sts.inMemoryManager.GetValidToken(ctx)
		}
		return nil, err
	}

	return token, nil
}

// EnsureValidToken はトークンの有効性を確保
func (sts *SimpleTokenService) EnsureValidToken(ctx context.Context) (*models.OAuth2Token, error) {
	return sts.GetValidToken(ctx)
}

// RefreshToken は手動でトークンをリフレッシュ
func (sts *SimpleTokenService) RefreshToken(ctx context.Context) error {
	err := sts.inMemoryManager.RefreshTokenIfNeeded(ctx)
	if err != nil {
		// リフレッシュ失敗時は回復を試行
		if recoveryErr := sts.recoveryManager.RequestRecovery(
			"manual_refresh_failed",
			"refresh_failed",
			err,
		); recoveryErr != nil {
			return fmt.Errorf("refresh failed and recovery failed: %v (recovery: %v)", err, recoveryErr)
		}
	}
	return err
}

// GetServiceStatus はサービス全体の状態を取得
func (sts *SimpleTokenService) GetServiceStatus() SimpleServiceStatus {
	tokenStatus := sts.inMemoryManager.GetTokenStatus()
	recoveryStats := sts.recoveryManager.GetRecoveryStats()

	return SimpleServiceStatus{
		IsRunning:     sts.isStarted,
		TokenStatus:   tokenStatus,
		RecoveryStats: recoveryStats,
		IsHealthy:     sts.recoveryManager.IsHealthy(),
	}
}

// UpdateRefreshToken はリフレッシュトークンを更新（Admin API用）
func (sts *SimpleTokenService) UpdateRefreshToken(ctx context.Context, refreshToken string, clientID, clientSecret string) error {
	return sts.inMemoryManager.UpdateRefreshToken(ctx, refreshToken, clientID, clientSecret)
}

// GetTokenStatus はトークン状態を取得（Admin API用）
func (sts *SimpleTokenService) GetTokenStatus() TokenStatus {
	return sts.inMemoryManager.GetTokenStatus()
}

// GetValidTokenForAPI は有効なトークン情報を取得（Admin API用のTokenManagerインターフェース実装）
func (sts *SimpleTokenService) GetValidTokenForAPI(ctx context.Context) (*TokenInfo, error) {
	token, err := sts.GetValidToken(ctx)
	if err != nil {
		return nil, err
	}
	
	// models.OAuth2Token から TokenInfo に変換
	return &TokenInfo{
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		ExpiresAt:    token.ExpiresAt,
		TokenType:    token.TokenType,
	}, nil
}

// SimpleTokenServiceAdapter は AdminAPIHandler の TokenManager インターフェース実装
type SimpleTokenServiceAdapter struct {
	service *SimpleTokenService
}

// NewSimpleTokenServiceAdapter は新しいアダプターを作成
func NewSimpleTokenServiceAdapter(service *SimpleTokenService) *SimpleTokenServiceAdapter {
	return &SimpleTokenServiceAdapter{
		service: service,
	}
}

// UpdateRefreshToken は TokenManager インターフェースの実装
func (adapter *SimpleTokenServiceAdapter) UpdateRefreshToken(ctx context.Context, refreshToken string, clientID, clientSecret string) error {
	return adapter.service.UpdateRefreshToken(ctx, refreshToken, clientID, clientSecret)
}

// GetTokenStatus は TokenManager インターフェースの実装
func (adapter *SimpleTokenServiceAdapter) GetTokenStatus() TokenStatus {
	return adapter.service.GetTokenStatus()
}

// GetValidToken は TokenManager インターフェースの実装
func (adapter *SimpleTokenServiceAdapter) GetValidToken(ctx context.Context) (*TokenInfo, error) {
	return adapter.service.GetValidTokenForAPI(ctx)
}

// SimpleServiceStatus は簡易サービス状態情報
type SimpleServiceStatus struct {
	IsRunning     bool          `json:"is_running"`
	TokenStatus   TokenStatus   `json:"token_status"`
	RecoveryStats RecoveryStats `json:"recovery_stats"`
	IsHealthy     bool          `json:"is_healthy"`
}

// isSimpleAuthenticationError は認証エラーかどうか判定
func isSimpleAuthenticationError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return containsSimpleAny(errStr, []string{
		"401",
		"403",
		"unauthorized",
		"forbidden",
		"authentication failed",
		"invalid token",
		"token expired",
	})
}

// containsSimpleAny は文字列に指定された単語のいずれかが含まれているか確認
func containsSimpleAny(s string, keywords []string) bool {
	for _, keyword := range keywords {
		if len(s) >= len(keyword) {
			for i := 0; i <= len(s)-len(keyword); i++ {
				if s[i:i+len(keyword)] == keyword {
					return true
				}
			}
		}
	}
	return false
}

// onSecretUpdate はSecret更新時のコールバック関数 (恒久対応: 自律的Secret再読み込み)
func (sts *SimpleTokenService) onSecretUpdate(newToken *models.OAuth2Token) error {
	sts.logger.Info("Secret update detected, updating tokens directly (no API call)",
		"new_expires_at", newToken.ExpiresAt,
		"new_scope", newToken.Scope)

	// InMemoryTokenManagerに新しいトークンを設定（API呼び出しなし）
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// OAuth2 API競合を回避: 直接トークン更新（refreshなし）
	err := sts.inMemoryManager.UpdateTokenDirectly(ctx, newToken)
	if err != nil {
		sts.logger.Error("Failed to update token directly from secret", "error", err)
		return err
	}

	// 競合回避メトリクス: auth-token-managerとの協調動作をログ記録
	sts.logger.Info("OAuth2 conflict avoided - token updated from Secret without API call",
		"source", "auth-token-manager",
		"method", "direct_update",
		"previous_scope_conflict_fixed", true)

	sts.logger.Info("Tokens updated successfully from secret without API call",
		"expires_at", newToken.ExpiresAt,
		"time_until_expiry_hours", time.Until(newToken.ExpiresAt).Hours())

	return nil
}

// ReloadFromSecret は手動でSecretから再読み込み (恒久対応: 403エラー時の回復)
func (sts *SimpleTokenService) ReloadFromSecret(ctx context.Context) error {
	if sts.oauth2SecretSvc == nil {
		return fmt.Errorf("OAuth2SecretService not available")
	}

	sts.logger.Info("Manual secret reload requested")

	// Secretから最新のトークンを読み込み
	token, err := sts.oauth2SecretSvc.LoadOAuth2Token(ctx)
	if err != nil {
		return fmt.Errorf("failed to reload OAuth2 token from secret: %w", err)
	}

	// Secret更新コールバックを呼び出し
	return sts.onSecretUpdate(token)
}