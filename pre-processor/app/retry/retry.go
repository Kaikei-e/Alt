// ABOUTME: This file implements exponential backoff retry mechanism with jitter
// ABOUTME: Provides resilient error handling for external service calls
package retry

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"crypto/rand"
	"time"
)

type RetryConfig struct {
	MaxAttempts   int
	BaseDelay     time.Duration
	MaxDelay      time.Duration
	BackoffFactor float64
	JitterFactor  float64
}

type ErrorClassifier func(error) bool

type Retrier struct {
	config      RetryConfig
	isRetryable ErrorClassifier
	logger      *slog.Logger
}

func NewRetrier(config RetryConfig, classifier ErrorClassifier, logger *slog.Logger) *Retrier {
	return &Retrier{
		config:      config,
		isRetryable: classifier,
		logger:      logger,
	}
}

func (r *Retrier) Do(ctx context.Context, operation func() error) error {
	start := time.Now()
	var lastErr error
	var totalWaitTime time.Duration

	r.logger.Info("retry operation started",
		"max_attempts", r.config.MaxAttempts,
		"base_delay", r.config.BaseDelay,
		"max_delay", r.config.MaxDelay)

	for attempt := 1; attempt <= r.config.MaxAttempts; attempt++ {
		attemptStart := time.Now()
		lastErr = operation()
		attemptDuration := time.Since(attemptStart)

		if lastErr == nil {
			totalDuration := time.Since(start)
			if attempt > 1 {
				r.logger.Info("operation succeeded after retry",
					"attempt", attempt,
					"total_attempts", r.config.MaxAttempts,
					"attempt_duration_ms", attemptDuration.Milliseconds(),
					"total_duration_ms", totalDuration.Milliseconds(),
					"total_wait_time_ms", totalWaitTime.Milliseconds())
			} else {
				r.logger.Info("operation succeeded on first attempt",
					"attempt_duration_ms", attemptDuration.Milliseconds())
			}
			return nil
		}

		// エラー発生時のパフォーマンスログ
		isRetryable := r.isRetryable != nil && r.isRetryable(lastErr)
		r.logger.Warn("operation attempt failed",
			"attempt", attempt,
			"error", lastErr,
			"retryable", isRetryable,
			"attempt_duration_ms", attemptDuration.Milliseconds())

		// 最後の試行の場合、または、リトライ不可能なエラーの場合
		if attempt == r.config.MaxAttempts || !isRetryable {
			totalDuration := time.Since(start)
			r.logger.Error("operation failed permanently",
				"attempt", attempt,
				"error", lastErr,
				"retryable", isRetryable,
				"total_duration_ms", totalDuration.Milliseconds(),
				"total_wait_time_ms", totalWaitTime.Milliseconds())
			break
		}

		// バックオフ計算
		delay := r.calculateDelay(attempt)
		totalWaitTime += delay

		r.logger.Info("retry backoff wait",
			"attempt", attempt,
			"error", lastErr,
			"retry_delay_ms", delay.Milliseconds(),
			"total_wait_time_ms", totalWaitTime.Milliseconds())

		// コンテキストでキャンセル可能な待機
		waitStart := time.Now()
		select {
		case <-ctx.Done():
			waitDuration := time.Since(waitStart)
			totalDuration := time.Since(start)
			r.logger.Error("retry cancelled by context",
				"attempt", attempt,
				"context_error", ctx.Err(),
				"wait_duration_ms", waitDuration.Milliseconds(),
				"total_duration_ms", totalDuration.Milliseconds())
			return fmt.Errorf("retry cancelled: %w", ctx.Err())
		case <-time.After(delay):
			// 次の試行へ続行
		}
	}

	totalDuration := time.Since(start)
	return fmt.Errorf("operation failed after %d attempts (total: %dms, wait: %dms): %w",
		r.config.MaxAttempts, totalDuration.Milliseconds(), totalWaitTime.Milliseconds(), lastErr)
}

func (r *Retrier) calculateDelay(attempt int) time.Duration {
	// 指数バックオフ
	delay := float64(r.config.BaseDelay) * math.Pow(r.config.BackoffFactor, float64(attempt-1))

	// 最大遅延の制限
	if delay > float64(r.config.MaxDelay) {
		delay = float64(r.config.MaxDelay)
	}

	// ジッター追加（サンダリングハード防止）
	jitterBytes := make([]byte, 8)
	n, err := rand.Read(jitterBytes)
	if err != nil || n != 8 {
		r.logger.Error("failed to read random bytes for jitter", "error", err)
		jitterBytes = []byte{0, 0, 0, 0, 0, 0, 0, 0} // デフォルト値
	}
	jitterInt := int64(jitterBytes[0]) | int64(jitterBytes[1])<<8 | int64(jitterBytes[2])<<16 | int64(jitterBytes[3])<<24 |
		int64(jitterBytes[4])<<32 | int64(jitterBytes[5])<<40 | int64(jitterBytes[6])<<48 | int64(jitterBytes[7])<<56
	if jitterInt < 0 {
		jitterInt = -jitterInt
	}
	jitter := 1.0 + (float64(jitterInt%1000)/1000.0)*r.config.JitterFactor
	delay *= jitter

	return time.Duration(delay)
}