// ABOUTME: 自己回復システム - 指数バックオフによる障害自動復旧機能
// ABOUTME: 401/403エラー時の自動再試行、レート制限対応、緊急時フォールバック

package service

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"sync"
	"time"

	"pre-processor-sidecar/driver"
)

// RecoveryManager は自己回復機能を提供
type RecoveryManager struct {
	tokenManager     *InMemoryTokenManager
	oauth2Client     *driver.OAuth2Client
	logger           *slog.Logger
	metricsCollector RecoveryMetricsCollector

	// 指数バックオフ設定
	config BackoffConfig

	// 状態管理
	mutex                sync.RWMutex
	consecutiveFailures  int
	lastFailureTime      time.Time
	lastSuccessTime      time.Time
	isInRecoveryMode     bool
	totalRecoveryAttempts int

	// 緊急時フォールバック
	fallbackTokenSource FallbackTokenSource
	
	// 制御チャンネル
	stopChan      chan struct{}
	recoveryChan  chan recoveryRequest
	isRunning     bool
}

// BackoffConfig は指数バックオフの設定
type BackoffConfig struct {
	MaxRetries          int           // 最大リトライ回数: 5
	InitialInterval     time.Duration // 初期間隔: 30秒  
	Multiplier          float64       // 倍率: 2.0
	MaxInterval         time.Duration // 最大間隔: 10分
	Jitter              bool          // ランダム要素: true
	JitterRange         float64       // ジッター範囲: 0.1 (±10%)
	RecoveryTimeout     time.Duration // 回復タイムアウト: 30分
	HealthCheckInterval time.Duration // ヘルスチェック間隔: 1分
}

// RecoveryMetricsCollector は回復システムのメトリクス収集インターフェース  
type RecoveryMetricsCollector interface {
	IncrementRecoveryAttempt(success bool)
	RecordRecoveryDuration(duration time.Duration)
	RecordConsecutiveFailures(count int)
	IncrementFallbackActivation()
	RecordBackoffInterval(interval time.Duration)
}

// FallbackTokenSource は緊急時のトークンソース
type FallbackTokenSource interface {
	GetFallbackToken() (string, error)
	IsAvailable() bool
}

// recoveryRequest は回復要求
type recoveryRequest struct {
	reason      string
	errorType   string
	originalErr error
	responseCh  chan recoveryResponse
}

// recoveryResponse は回復レスポンス
type recoveryResponse struct {
	success bool
	err     error
}

// RecoveryStats は回復統計情報
type RecoveryStats struct {
	ConsecutiveFailures   int       `json:"consecutive_failures"`
	LastFailureTime       time.Time `json:"last_failure_time,omitempty"`
	LastSuccessTime       time.Time `json:"last_success_time,omitempty"`
	IsInRecoveryMode      bool      `json:"is_in_recovery_mode"`
	TotalRecoveryAttempts int       `json:"total_recovery_attempts"`
	NextRetryInterval     time.Duration `json:"next_retry_interval_seconds"`
}

// NoOpRecoveryMetrics はデフォルトのメトリクス実装
type NoOpRecoveryMetrics struct{}

func (n *NoOpRecoveryMetrics) IncrementRecoveryAttempt(success bool)      {}
func (n *NoOpRecoveryMetrics) RecordRecoveryDuration(duration time.Duration) {}
func (n *NoOpRecoveryMetrics) RecordConsecutiveFailures(count int)        {}
func (n *NoOpRecoveryMetrics) IncrementFallbackActivation()               {}
func (n *NoOpRecoveryMetrics) RecordBackoffInterval(interval time.Duration) {}

// NewRecoveryManager は新しい回復マネージャーを作成
func NewRecoveryManager(
	tokenManager *InMemoryTokenManager,
	oauth2Client *driver.OAuth2Client,
	logger *slog.Logger,
	metricsCollector RecoveryMetricsCollector,
	fallbackTokenSource FallbackTokenSource,
) *RecoveryManager {
	// デフォルト設定
	config := BackoffConfig{
		MaxRetries:          5,
		InitialInterval:     30 * time.Second,
		Multiplier:          2.0,
		MaxInterval:         10 * time.Minute,
		Jitter:              true,
		JitterRange:         0.1,
		RecoveryTimeout:     30 * time.Minute,
		HealthCheckInterval: 1 * time.Minute,
	}

	if logger == nil {
		logger = slog.Default()
	}

	if metricsCollector == nil {
		metricsCollector = &NoOpRecoveryMetrics{}
	}

	return &RecoveryManager{
		tokenManager:        tokenManager,
		oauth2Client:        oauth2Client,
		logger:              logger,
		metricsCollector:    metricsCollector,
		config:              config,
		fallbackTokenSource: fallbackTokenSource,
		stopChan:            make(chan struct{}),
		recoveryChan:        make(chan recoveryRequest, 10),
		lastSuccessTime:     time.Now(),
	}
}

// Start は回復マネージャーを開始
func (rm *RecoveryManager) Start() {
	rm.mutex.Lock()
	if rm.isRunning {
		rm.mutex.Unlock()
		return
	}
	rm.isRunning = true
	rm.mutex.Unlock()

	go rm.recoveryLoop()
	go rm.healthCheckLoop()

	rm.logger.Info("Recovery manager started",
		"max_retries", rm.config.MaxRetries,
		"initial_interval_seconds", rm.config.InitialInterval.Seconds(),
		"max_interval_seconds", rm.config.MaxInterval.Seconds())
}

// Stop は回復マネージャーを停止
func (rm *RecoveryManager) Stop() {
	rm.mutex.Lock()
	if !rm.isRunning {
		rm.mutex.Unlock()
		return
	}
	rm.isRunning = false
	rm.mutex.Unlock()

	close(rm.stopChan)
	rm.logger.Info("Recovery manager stopped")
}

// RequestRecovery は回復を要求
func (rm *RecoveryManager) RequestRecovery(reason string, errorType string, originalErr error) error {
	if !rm.isRunning {
		return fmt.Errorf("recovery manager is not running")
	}

	responseCh := make(chan recoveryResponse, 1)
	request := recoveryRequest{
		reason:      reason,
		errorType:   errorType,
		originalErr: originalErr,
		responseCh:  responseCh,
	}

	select {
	case rm.recoveryChan <- request:
		// 要求送信成功
	case <-time.After(5 * time.Second):
		return fmt.Errorf("recovery request timed out")
	}

	// レスポンス待機
	select {
	case response := <-responseCh:
		if response.success {
			return nil
		}
		return response.err
	case <-time.After(rm.config.RecoveryTimeout):
		return fmt.Errorf("recovery operation timed out after %v", rm.config.RecoveryTimeout)
	}
}

// recoveryLoop はメインの回復ループ
func (rm *RecoveryManager) recoveryLoop() {
	for {
		select {
		case request := <-rm.recoveryChan:
			rm.handleRecoveryRequest(request)
		case <-rm.stopChan:
			return
		}
	}
}

// healthCheckLoop はヘルスチェックループ
func (rm *RecoveryManager) healthCheckLoop() {
	ticker := time.NewTicker(rm.config.HealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rm.performHealthCheck()
		case <-rm.stopChan:
			return
		}
	}
}

// handleRecoveryRequest は回復要求を処理
func (rm *RecoveryManager) handleRecoveryRequest(request recoveryRequest) {
	start := time.Now()
	
	rm.logger.Info("Recovery request received",
		"reason", request.reason,
		"error_type", request.errorType,
		"original_error", request.originalErr.Error())

	// 失敗記録を更新
	rm.recordFailure(request.errorType)

	// 指数バックオフによる回復試行
	success := rm.attemptRecoveryWithBackoff(request)

	// 結果記録
	duration := time.Since(start)
	rm.metricsCollector.RecordRecoveryDuration(duration)

	if success {
		rm.recordSuccess()
		rm.logger.Info("Recovery completed successfully",
			"duration_ms", duration.Milliseconds(),
			"attempts", rm.consecutiveFailures+1)
	} else {
		rm.logger.Error("Recovery failed after all attempts",
			"duration_ms", duration.Milliseconds(),
			"max_retries", rm.config.MaxRetries)
	}

	// レスポンス送信
	response := recoveryResponse{
		success: success,
	}
	if !success {
		response.err = fmt.Errorf("recovery failed after %d attempts", rm.config.MaxRetries)
	}

	select {
	case request.responseCh <- response:
	default:
		// チャンネルがクローズされている場合
	}
}

// attemptRecoveryWithBackoff は指数バックオフで回復を試行
func (rm *RecoveryManager) attemptRecoveryWithBackoff(request recoveryRequest) bool {
	for attempt := 1; attempt <= rm.config.MaxRetries; attempt++ {
		rm.logger.Info("Recovery attempt starting",
			"attempt", attempt,
			"max_retries", rm.config.MaxRetries,
			"reason", request.reason)

		// 回復試行
		if rm.attemptSingleRecovery(request) {
			rm.metricsCollector.IncrementRecoveryAttempt(true)
			return true
		}

		rm.metricsCollector.IncrementRecoveryAttempt(false)

		// 最後の試行でなければバックオフ待機
		if attempt < rm.config.MaxRetries {
			interval := rm.calculateBackoffInterval(attempt)
			rm.metricsCollector.RecordBackoffInterval(interval)
			
			rm.logger.Info("Recovery attempt failed, waiting before retry",
				"attempt", attempt,
				"next_retry_in_seconds", interval.Seconds(),
				"error_type", request.errorType)

			select {
			case <-time.After(interval):
				// 正常な待機完了
			case <-rm.stopChan:
				// 停止要求
				return false
			}
		}
	}

	// 全ての試行が失敗した場合、フォールバックを試行
	return rm.attemptFallbackRecovery()
}

// attemptSingleRecovery は単一の回復を試行
func (rm *RecoveryManager) attemptSingleRecovery(request recoveryRequest) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// トークンリフレッシュを試行
	if err := rm.tokenManager.RefreshTokenIfNeeded(ctx); err != nil {
		rm.logger.Warn("Token refresh failed during recovery",
			"error", err,
			"error_type", request.errorType)
		return false
	}

	// トークンの有効性を確認
	token, err := rm.tokenManager.GetValidToken(ctx)
	if err != nil {
		rm.logger.Warn("Failed to get valid token after refresh",
			"error", err)
		return false
	}

	// トークンの期限確認（5分以上残っているか）
	if time.Until(token.ExpiresAt) < 5*time.Minute {
		rm.logger.Warn("Token expires too soon after refresh",
			"expires_at", token.ExpiresAt,
			"expires_in_seconds", time.Until(token.ExpiresAt).Seconds())
		return false
	}

	rm.logger.Info("Recovery attempt succeeded",
		"token_expires_at", token.ExpiresAt,
		"expires_in_seconds", time.Until(token.ExpiresAt).Seconds())

	return true
}

// attemptFallbackRecovery はフォールバック回復を試行
func (rm *RecoveryManager) attemptFallbackRecovery() bool {
	if rm.fallbackTokenSource == nil || !rm.fallbackTokenSource.IsAvailable() {
		rm.logger.Warn("Fallback token source not available")
		return false
	}

	rm.logger.Info("Attempting fallback recovery")
	rm.metricsCollector.IncrementFallbackActivation()

	fallbackToken, err := rm.fallbackTokenSource.GetFallbackToken()
	if err != nil {
		rm.logger.Error("Fallback token retrieval failed", "error", err)
		return false
	}

	// フォールバックトークンを使用して更新を試行
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := rm.tokenManager.UpdateRefreshToken(ctx, fallbackToken, "", ""); err != nil {
		rm.logger.Error("Fallback token update failed", "error", err)
		return false
	}

	rm.logger.Info("Fallback recovery succeeded")
	return true
}

// calculateBackoffInterval は指数バックオフの間隔を計算
func (rm *RecoveryManager) calculateBackoffInterval(attempt int) time.Duration {
	// 指数バックオフ計算: initial * (multiplier ^ (attempt-1))
	interval := float64(rm.config.InitialInterval) * math.Pow(rm.config.Multiplier, float64(attempt-1))
	
	// 最大間隔制限
	if time.Duration(interval) > rm.config.MaxInterval {
		interval = float64(rm.config.MaxInterval)
	}

	// ジッター追加（ランダム要素で衝突回避）
	if rm.config.Jitter {
		jitter := interval * rm.config.JitterRange
		randomOffset := (rand.Float64() - 0.5) * 2 * jitter // -jitter ~ +jitter
		interval += randomOffset
		
		// 負の値にならないよう制限
		if interval < 0 {
			interval = float64(rm.config.InitialInterval)
		}
	}

	return time.Duration(interval)
}

// recordFailure は失敗を記録
func (rm *RecoveryManager) recordFailure(errorType string) {
	rm.mutex.Lock()
	rm.consecutiveFailures++
	rm.lastFailureTime = time.Now()
	rm.isInRecoveryMode = true
	rm.totalRecoveryAttempts++
	rm.mutex.Unlock()

	rm.metricsCollector.RecordConsecutiveFailures(rm.consecutiveFailures)

	rm.logger.Warn("Authentication failure recorded",
		"consecutive_failures", rm.consecutiveFailures,
		"error_type", errorType,
		"recovery_mode", true)
}

// recordSuccess は成功を記録
func (rm *RecoveryManager) recordSuccess() {
	rm.mutex.Lock()
	rm.consecutiveFailures = 0
	rm.lastSuccessTime = time.Now()
	rm.isInRecoveryMode = false
	rm.mutex.Unlock()

	rm.logger.Info("Authentication success recorded, recovery mode disabled",
		"last_success_time", rm.lastSuccessTime)
}

// performHealthCheck はヘルスチェックを実行
func (rm *RecoveryManager) performHealthCheck() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// トークンの有効性を確認
	token, err := rm.tokenManager.GetValidToken(ctx)
	if err != nil {
		rm.logger.Debug("Health check failed to get valid token", "error", err)
		return
	}

	// トークンが期限切れ間近（5分以内）の場合は警告
	timeToExpiry := time.Until(token.ExpiresAt)
	if timeToExpiry < 5*time.Minute {
		rm.logger.Warn("Token expires soon during health check",
			"expires_at", token.ExpiresAt,
			"expires_in_seconds", timeToExpiry.Seconds())
	} else {
		rm.logger.Debug("Health check passed",
			"expires_in_seconds", timeToExpiry.Seconds())
	}
}

// GetRecoveryStats は回復統計情報を取得
func (rm *RecoveryManager) GetRecoveryStats() RecoveryStats {
	rm.mutex.RLock()
	defer rm.mutex.RUnlock()

	stats := RecoveryStats{
		ConsecutiveFailures:   rm.consecutiveFailures,
		LastFailureTime:       rm.lastFailureTime,
		LastSuccessTime:       rm.lastSuccessTime,
		IsInRecoveryMode:      rm.isInRecoveryMode,
		TotalRecoveryAttempts: rm.totalRecoveryAttempts,
	}

	// 次のリトライ間隔を計算（参考値）
	if rm.isInRecoveryMode {
		stats.NextRetryInterval = rm.calculateBackoffInterval(rm.consecutiveFailures + 1)
	}

	return stats
}

// IsHealthy はシステムの健全性を確認
func (rm *RecoveryManager) IsHealthy() bool {
	rm.mutex.RLock()
	defer rm.mutex.RUnlock()

	// 回復モードでない、かつ連続失敗が少ない場合を健全とみなす
	return !rm.isInRecoveryMode && rm.consecutiveFailures < 3
}