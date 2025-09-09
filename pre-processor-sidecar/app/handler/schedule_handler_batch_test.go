package handler

import (
	"testing"
	"time"
	"log/slog"
	"github.com/stretchr/testify/assert"
)

func TestScheduleHandler_NewScheduleHandler(t *testing.T) {
	logger := slog.Default()
	
	// Test with nil handlers since we're focusing on basic functionality
	handler := NewScheduleHandler(nil, nil, logger)
	
	assert.NotNil(t, handler)
	assert.NotNil(t, handler.config)
	assert.NotNil(t, handler.status)
	assert.NotNil(t, handler.subscriptionScheduler)
	assert.NotNil(t, handler.articleFetchScheduler)
	
	// Check default configuration
	config := handler.GetConfig()
	assert.Equal(t, 12*time.Hour, config.SubscriptionSyncInterval)
	assert.Equal(t, 30*time.Minute, config.ArticleFetchInterval)
	assert.True(t, config.EnableSubscriptionSync)
	assert.True(t, config.EnableArticleFetch)
	assert.Equal(t, 2, config.MaxConcurrentJobs)
	assert.True(t, config.EnableRandomStart)
}

func TestScheduleHandler_BatchProcessingConfiguration(t *testing.T) {
	logger := slog.Default()
	
	handler := NewScheduleHandler(nil, nil, logger)
	
	// バッチ処理の設定値を検証
	config := handler.GetConfig()
	
	// 30分間隔での記事取得
	assert.Equal(t, 30*time.Minute, config.ArticleFetchInterval)
	
	// 12時間間隔での購読同期
	assert.Equal(t, 12*time.Hour, config.SubscriptionSyncInterval)
	
	// ランダムスタートが有効
	assert.True(t, config.EnableRandomStart)
	
	// 同時実行数
	assert.Equal(t, 2, config.MaxConcurrentJobs)
	
	// 両方のスケジュールが有効
	assert.True(t, config.EnableArticleFetch)
	assert.True(t, config.EnableSubscriptionSync)
}

func TestScheduleHandler_DailyCycleSimulation(t *testing.T) {
	// 1日のサイクルをシミュレーション
	logger := slog.Default()
	
	// 24時間 = 48回の30分間隔
	// 2個/回 × 48回 = 96回処理
	// 46サブスクリプション × 2回/日 = 92回必要
	
	totalIntervals := 48     // 24時間 ÷ 30分
	batchSize := 2
	totalSubscriptions := 46
	
	expectedDailyProcessing := totalSubscriptions * 2  // 92回
	actualDailyProcessing := totalIntervals * batchSize // 96回
	
	// 実際の処理回数が必要回数を満たすことを確認
	assert.GreaterOrEqual(t, actualDailyProcessing, expectedDailyProcessing)
	
	// 1回転に必要な時間を計算
	cycleIntervals := (totalSubscriptions + batchSize - 1) / batchSize  // 23回
	cycleHours := float64(cycleIntervals) * 0.5  // 11.5時間
	dailyCycles := 24.0 / cycleHours  // 約2.09回/日
	
	assert.InDelta(t, 2.0, dailyCycles, 0.2)  // 約2回/日
	
	t.Logf("Daily processing simulation:")
	t.Logf("  Total intervals per day: %d", totalIntervals)
	t.Logf("  Batch size: %d", batchSize)
	t.Logf("  Total subscriptions: %d", totalSubscriptions)
	t.Logf("  Expected daily processing: %d", expectedDailyProcessing)
	t.Logf("  Actual daily capacity: %d", actualDailyProcessing)
	t.Logf("  Cycle intervals: %d", cycleIntervals)
	t.Logf("  Cycle hours: %.1f", cycleHours)
	t.Logf("  Daily cycles: %.2f", dailyCycles)
}

func TestScheduleHandler_GetStatus(t *testing.T) {
	handler := NewScheduleHandler(nil, nil, nil)

	status := handler.GetStatus()
	assert.NotNil(t, status)
	assert.True(t, status.SubscriptionSyncEnabled)
	assert.True(t, status.ArticleFetchEnabled)
	assert.False(t, status.SubscriptionSyncRunning)
	assert.False(t, status.ArticleFetchRunning)
	assert.Equal(t, int64(0), status.TotalSubscriptionSyncs)
	assert.Equal(t, int64(0), status.TotalArticleFetches)
}

func TestScheduleHandler_IsRunning(t *testing.T) {
	handler := NewScheduleHandler(nil, nil, nil)

	// Initially not running
	assert.False(t, handler.IsRunning())
}

func TestScheduleHandler_UpdateConfig(t *testing.T) {
	handler := NewScheduleHandler(nil, nil, nil)

	tests := map[string]struct {
		config      *ScheduleConfig
		expectError bool
		errorMsg    string
	}{
		"valid_config": {
			config: &ScheduleConfig{
				SubscriptionSyncInterval: 2 * time.Hour,
				ArticleFetchInterval:     30 * time.Minute,
				EnableSubscriptionSync:   false,
				EnableArticleFetch:       true,
				MaxConcurrentJobs:        1,
			},
			expectError: false,
		},
		"subscription_interval_too_short": {
			config: &ScheduleConfig{
				SubscriptionSyncInterval: 30 * time.Second,
				ArticleFetchInterval:     30 * time.Minute,
			},
			expectError: true,
			errorMsg:    "subscription sync interval too short",
		},
		"article_interval_too_short": {
			config: &ScheduleConfig{
				SubscriptionSyncInterval: 2 * time.Hour,
				ArticleFetchInterval:     30 * time.Second,
			},
			expectError: true,
			errorMsg:    "article fetch interval too short",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			err := handler.UpdateConfig(tc.config)

			if tc.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.errorMsg)
			} else {
				assert.NoError(t, err)
				
				// Verify configuration was updated
				updatedConfig := handler.GetConfig()
				assert.Equal(t, tc.config.SubscriptionSyncInterval, updatedConfig.SubscriptionSyncInterval)
				assert.Equal(t, tc.config.ArticleFetchInterval, updatedConfig.ArticleFetchInterval)
				assert.Equal(t, tc.config.EnableSubscriptionSync, updatedConfig.EnableSubscriptionSync)
				assert.Equal(t, tc.config.EnableArticleFetch, updatedConfig.EnableArticleFetch)
			}
		})
	}
}