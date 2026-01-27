// ABOUTME: フォールバックトークンソース - 緊急時の認証トークン取得
// ABOUTME: 環境変数、Kubernetesシークレット、外部APIからのトークン取得

package service

import (
	"fmt"
	"log/slog"
	"os"
	"time"
)

// EnvironmentFallbackTokenSource は環境変数からフォールバックトークンを取得
type EnvironmentFallbackTokenSource struct {
	logger *slog.Logger
}

// NewEnvironmentFallbackTokenSource は環境変数フォールバックソースを作成
func NewEnvironmentFallbackTokenSource(logger *slog.Logger) *EnvironmentFallbackTokenSource {
	if logger == nil {
		logger = slog.Default()
	}

	return &EnvironmentFallbackTokenSource{
		logger: logger,
	}
}

// GetFallbackToken はフォールバックトークンを取得
func (e *EnvironmentFallbackTokenSource) GetFallbackToken() (string, error) {
	// 環境変数からリフレッシュトークンを取得
	fallbackTokens := []string{
		"INOREADER_FALLBACK_REFRESH_TOKEN",
		"INOREADER_REFRESH_TOKEN", // 既存の環境変数
		"INOREADER_EMERGENCY_TOKEN",
		"EMERGENCY_REFRESH_TOKEN",
	}

	for _, envVar := range fallbackTokens {
		if token := os.Getenv(envVar); token != "" {
			e.logger.Info("Fallback token found in environment variable",
				"env_var", envVar,
				"token_length", len(token))
			return token, nil
		}
	}

	return "", fmt.Errorf("no fallback token found in environment variables")
}

// IsAvailable はフォールバックトークンが利用可能か確認
func (e *EnvironmentFallbackTokenSource) IsAvailable() bool {
	fallbackTokens := []string{
		"INOREADER_FALLBACK_REFRESH_TOKEN",
		"INOREADER_REFRESH_TOKEN",
		"INOREADER_EMERGENCY_TOKEN",
		"EMERGENCY_REFRESH_TOKEN",
	}

	for _, envVar := range fallbackTokens {
		if token := os.Getenv(envVar); token != "" {
			return true
		}
	}

	return false
}

// NoOpAdminAPIMetrics はデフォルトのAdmin APIメトリクス実装
type NoOpAdminAPIMetrics struct{}

func (n *NoOpAdminAPIMetrics) IncrementAdminAPIRequest(method, endpoint, status string) {}
func (n *NoOpAdminAPIMetrics) RecordAdminAPIRequestDuration(method, endpoint string, duration time.Duration) {
}
func (n *NoOpAdminAPIMetrics) IncrementAdminAPIRateLimitHit()                        {}
func (n *NoOpAdminAPIMetrics) IncrementAdminAPIAuthenticationError(errorType string) {}
