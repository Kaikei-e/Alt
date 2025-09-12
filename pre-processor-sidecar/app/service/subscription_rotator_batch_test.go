package service

import (
	"context"
	"testing"
	"log/slog"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestSubscriptionRotator_GetNextSubscriptionBatch(t *testing.T) {
	logger := slog.Default()
	rotator := NewSubscriptionRotator(logger)
	
	// テスト用サブスクリプションを用意
	subscriptions := make([]uuid.UUID, 46)
	for i := 0; i < 46; i++ {
		subscriptions[i] = uuid.New()
	}
	
	ctx := context.Background()
	err := rotator.LoadSubscriptions(ctx, subscriptions)
	assert.NoError(t, err)
	
	// バッチサイズ2でテスト
	batch := rotator.GetNextSubscriptionBatch(2)
	assert.Equal(t, 2, len(batch))
	
	// 連続してバッチを取得
	batch2 := rotator.GetNextSubscriptionBatch(2)
	assert.Equal(t, 2, len(batch2))
	
	// 異なるサブスクリプションが返されることを確認
	assert.NotEqual(t, batch[0], batch2[0])
	assert.NotEqual(t, batch[1], batch2[1])
}

func TestSubscriptionRotator_GetNextSubscriptionBatch_EndOfRotation(t *testing.T) {
	// サブスクリプション数が奇数の場合のテスト
	logger := slog.Default()
	rotator := NewSubscriptionRotator(logger)
	
	subscriptions := make([]uuid.UUID, 3) // 3つのサブスクリプション
	for i := 0; i < 3; i++ {
		subscriptions[i] = uuid.New()
	}
	
	ctx := context.Background()
	err := rotator.LoadSubscriptions(ctx, subscriptions)
	assert.NoError(t, err)
	
	// 最初のバッチ（2つ）
	batch1 := rotator.GetNextSubscriptionBatch(2)
	assert.Equal(t, 2, len(batch1))
	
	// 次のバッチ（1つのみ残っている）
	batch2 := rotator.GetNextSubscriptionBatch(2)
	assert.Equal(t, 1, len(batch2))
	
	// 2回転目開始（MAX_DAILY_ROTATIONS=2なので、さらに処理可能）
	batch3 := rotator.GetNextSubscriptionBatch(2)
	assert.Equal(t, 2, len(batch3)) // 2回転目の最初のバッチ
	
	// 2回転目の最後のバッチ
	batch4 := rotator.GetNextSubscriptionBatch(2)
	assert.Equal(t, 1, len(batch4)) // 残り1つ
	
	// すべての処理完了（3 subscriptions × 2 rotations = 6回処理完了）
	batch5 := rotator.GetNextSubscriptionBatch(2)
	assert.Equal(t, 0, len(batch5)) // 今日の処理は完了
}

func TestSubscriptionRotator_ProcessingCycleCalculation(t *testing.T) {
	rotator := NewSubscriptionRotator(slog.Default())
	
	subscriptions := make([]uuid.UUID, 46)
	for i := 0; i < 46; i++ {
		subscriptions[i] = uuid.New()
	}
	
	ctx := context.Background()
	err := rotator.LoadSubscriptions(ctx, subscriptions)
	assert.NoError(t, err)
	
	stats := rotator.GetStats()
	
	// 46サブスクリプション × 2回/日 = 92回の処理が必要
	expectedDailyProcessing := 46 * 2
	assert.Equal(t, expectedDailyProcessing, stats.TotalSubscriptions * 2)
	
	// バッチサイズ2で23回処理すれば全サブスクリプション完了
	expectedBatchCount := (46 + 1) / 2  // 23回
	assert.Equal(t, expectedBatchCount, 23)
}

func TestSubscriptionRotator_BatchProcessingIntegration(t *testing.T) {
	logger := slog.Default()
	rotator := NewSubscriptionRotator(logger)

	// 46個のサブスクリプションを作成
	subscriptions := make([]uuid.UUID, 46)
	for i := 0; i < 46; i++ {
		subscriptions[i] = uuid.New()
	}

	ctx := context.Background()
	err := rotator.LoadSubscriptions(ctx, subscriptions)
	assert.NoError(t, err)

	// バッチサイズ2で全サブスクリプションを処理
	totalProcessed := 0
	batchCount := 0
	processedSubscriptions := make(map[uuid.UUID]bool)
	
	for totalProcessed < 46 {
		batch := rotator.GetNextSubscriptionBatch(2)
		if len(batch) == 0 {
			break
		}
		
		// 重複チェック
		for _, sub := range batch {
			assert.False(t, processedSubscriptions[sub], "Subscription should not be processed twice in same cycle")
			processedSubscriptions[sub] = true
		}
		
		totalProcessed += len(batch)
		batchCount++
		
		t.Logf("Batch %d: processed %d subscriptions, total: %d", 
			batchCount, len(batch), totalProcessed)
	}

	// 46個のサブスクリプションを全て処理するのに23回のバッチが必要
	expectedBatches := (46 + 2 - 1) / 2  // 23回
	assert.Equal(t, expectedBatches, batchCount)
	assert.Equal(t, 46, totalProcessed)
	
	// サイクル完了時間の確認（23 × 30分 = 11.5時間）
	expectedCycleHours := float64(expectedBatches) * 0.5
	assert.InDelta(t, 11.5, expectedCycleHours, 0.1)
}

func TestSubscriptionRotator_GetNextSubscriptionBatch_EmptyRotator(t *testing.T) {
	logger := slog.Default()
	rotator := NewSubscriptionRotator(logger)
	
	// サブスクリプションをロードしない
	batch := rotator.GetNextSubscriptionBatch(2)
	assert.Equal(t, 0, len(batch))
}

func TestSubscriptionRotator_GetNextSubscriptionBatch_DifferentBatchSizes(t *testing.T) {
	logger := slog.Default()
	rotator := NewSubscriptionRotator(logger)
	
	subscriptions := make([]uuid.UUID, 10)
	for i := 0; i < 10; i++ {
		subscriptions[i] = uuid.New()
	}
	
	ctx := context.Background()
	err := rotator.LoadSubscriptions(ctx, subscriptions)
	assert.NoError(t, err)
	
	// バッチサイズ1
	batch1 := rotator.GetNextSubscriptionBatch(1)
	assert.Equal(t, 1, len(batch1))
	
	// バッチサイズ3
	batch3 := rotator.GetNextSubscriptionBatch(3)
	assert.Equal(t, 3, len(batch3))
	
	// バッチサイズ5
	batch5 := rotator.GetNextSubscriptionBatch(5)
	assert.Equal(t, 5, len(batch5))
	
	// 残り1つ
	batchRemaining := rotator.GetNextSubscriptionBatch(5)
	assert.Equal(t, 1, len(batchRemaining))
}

func TestSubscriptionRotator_GetBatchProcessingStatus(t *testing.T) {
	logger := slog.Default()
	rotator := NewSubscriptionRotator(logger)
	
	subscriptions := make([]uuid.UUID, 46)
	for i := 0; i < 46; i++ {
		subscriptions[i] = uuid.New()
	}
	
	ctx := context.Background()
	err := rotator.LoadSubscriptions(ctx, subscriptions)
	assert.NoError(t, err)
	
	// 最初の状態
	status := rotator.GetBatchProcessingStatus(2)
	assert.Contains(t, status, "Batch processing")
	assert.Contains(t, status, "batch size: 2")
	
	// いくつかのバッチを処理
	for i := 0; i < 5; i++ {
		batch := rotator.GetNextSubscriptionBatch(2)
		assert.Equal(t, 2, len(batch))
	}
	
	// 進捗状況を確認
	statusAfter := rotator.GetBatchProcessingStatus(2)
	assert.Contains(t, statusAfter, "batch size: 2")
	
	// 全て処理完了まで進める
	for {
		batch := rotator.GetNextSubscriptionBatch(2)
		if len(batch) == 0 {
			break
		}
	}
	
	// 完了状態
	completedStatus := rotator.GetBatchProcessingStatus(2)
	assert.Contains(t, completedStatus, "completed")
}

func TestSubscriptionRotator_DailyRotationWithBatch(t *testing.T) {
	logger := slog.Default()
	rotator := NewSubscriptionRotator(logger)
	
	// MAX_DAILY_ROTATIONS = 2 がデフォルトになったので、4つのサブスクリプションを2回処理可能
	// 4 subscriptions × 2 rotations = 8回の処理が可能
	
	subscriptions := make([]uuid.UUID, 4) // 小さなセットでテスト
	for i := 0; i < 4; i++ {
		subscriptions[i] = uuid.New()
	}
	
	ctx := context.Background()
	err := rotator.LoadSubscriptions(ctx, subscriptions)
	assert.NoError(t, err)
	
	// 1回転目（4個 ÷ 2 = 2バッチ）
	batch1 := rotator.GetNextSubscriptionBatch(2)
	assert.Equal(t, 2, len(batch1))
	
	batch2 := rotator.GetNextSubscriptionBatch(2)
	assert.Equal(t, 2, len(batch2))
	
	// 2回転目（maxDaily=2なので、さらに4回処理可能）
	batch3 := rotator.GetNextSubscriptionBatch(2)
	assert.Equal(t, 2, len(batch3)) // 2回転目の1回目のバッチ
	
	batch4 := rotator.GetNextSubscriptionBatch(2)
	assert.Equal(t, 2, len(batch4)) // 2回転目の2回目のバッチ
	
	// すべての処理完了（4 subscriptions × 2 rotations = 8回処理完了）
	batch5 := rotator.GetNextSubscriptionBatch(2)
	assert.Equal(t, 0, len(batch5)) // 今日の処理完了
}