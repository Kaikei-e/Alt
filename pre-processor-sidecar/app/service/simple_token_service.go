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
		}
	}

	// InMemoryTokenManagerの作成
	tokenManagerConfig := InMemoryTokenManagerConfig{
		ClientID:         config.ClientID,
		ClientSecret:     config.ClientSecret,
		AccessToken:      initialAccessToken,
		RefreshToken:     initialRefreshToken,
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
		inMemoryManager:  inMemoryManager,
		recoveryManager:  recoveryManager,
		oauth2Client:     oauth2Client,
		oauth2SecretSvc:  oauth2SecretSvc,
		logger:           logger,
	}

	logger.Info("SimpleTokenService created successfully",
		"refresh_buffer_minutes", config.RefreshBuffer.Minutes(),
		"check_interval_minutes", config.CheckInterval.Minutes())

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

	sts.isStarted = true
	sts.logger.Info("SimpleTokenService started successfully")
	return nil
}

// Stop はサービスを停止
func (sts *SimpleTokenService) Stop() error {
	if !sts.isStarted {
		return nil
	}

	sts.logger.Info("Stopping SimpleTokenService...")

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
		// 認証エラーの場合、回復を試行
		if isSimpleAuthenticationError(err) {
			sts.logger.Warn("Authentication error detected, requesting recovery", "error", err)
			
			if recoveryErr := sts.recoveryManager.RequestRecovery(
				"authentication_failed",
				"token_invalid",
				err,
			); recoveryErr != nil {
				sts.logger.Error("Recovery failed", "original_error", err, "recovery_error", recoveryErr)
				return nil, fmt.Errorf("token retrieval failed and recovery failed: %v (recovery: %v)", err, recoveryErr)
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