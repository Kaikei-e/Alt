// ABOUTME: This file implements JSON file-based Dead Letter Queue for failed articles
// ABOUTME: Provides persistent storage and management of processing failures
package dlq

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type FailedArticleMessage struct {
	ID          string                 `json:"id"`
	URL         string                 `json:"url"`
	Attempts    int                    `json:"attempts"`
	LastError   ErrorDetails           `json:"last_error"`
	Timestamp   time.Time              `json:"timestamp"`
	ServiceName string                 `json:"service_name"`
	TaskType    string                 `json:"task_type"`
	Context     map[string]interface{} `json:"context"`
	Metadata    map[string]interface{} `json:"metadata"`
}

type ErrorDetails struct {
	Type        string `json:"type"`
	Message     string `json:"message"`
	StackTrace  string `json:"stack_trace,omitempty"`
	IsRetryable bool   `json:"is_retryable"`
}

type FileDLQConfig struct {
	BasePath      string        `json:"base_path" env:"DLQ_BASE_PATH" default:"/var/dlq/pre-processor"`
	MaxFileSize   int64         `json:"max_file_size" env:"DLQ_MAX_FILE_SIZE" default:"10485760"` // 10MB
	Retention     time.Duration `json:"retention" env:"DLQ_RETENTION" default:"720h"`            // 30日
	EnableCleanup bool          `json:"enable_cleanup" env:"DLQ_ENABLE_CLEANUP" default:"true"`
}

type FileDLQManager struct {
	config  FileDLQConfig
	counter uint64
	mu      sync.Mutex
	logger  *slog.Logger
}

func NewFileDLQManager(config FileDLQConfig, logger *slog.Logger) *FileDLQManager {
	return &FileDLQManager{
		config: config,
		logger: logger,
	}
}

func (dlq *FileDLQManager) PublishFailedArticle(ctx context.Context, url string, attempts int, lastError error) error {
	start := time.Now()

	dlq.mu.Lock()
	dlq.counter++
	messageID := fmt.Sprintf("dlq_%s_%03d", time.Now().Format("20060102"), dlq.counter)
	dlq.mu.Unlock()

	domain := extractDomain(url)

	dlq.logger.Info("DLQ publication started",
		"message_id", messageID,
		"url", url,
		"domain", domain,
		"attempts", attempts,
		"error_type", fmt.Sprintf("%T", lastError))

	// エラー詳細の分析
	analysisStart := time.Now()
	errorDetails := dlq.analyzeError(lastError)
	analysisDuration := time.Since(analysisStart)

	dlq.logger.Debug("error analysis completed",
		"message_id", messageID,
		"error_type", errorDetails.Type,
		"is_retryable", errorDetails.IsRetryable,
		"analysis_duration_ms", analysisDuration.Milliseconds())

	message := FailedArticleMessage{
		ID:          messageID,
		URL:         url,
		Attempts:    attempts,
		LastError:   errorDetails,
		Timestamp:   time.Now().UTC(),
		ServiceName: "pre-processor",
		TaskType:    "article_fetch",
		Context: map[string]interface{}{
			"user_agent": "pre-processor/1.0 (+https://alt.example.com/bot)",
			"timeout":    "30s",
		},
		Metadata: map[string]interface{}{
			"domain": domain,
		},
	}

	writeStart := time.Now()
	err := dlq.writeMessageToFile(message)
	writeDuration := time.Since(writeStart)
	totalDuration := time.Since(start)

	if err != nil {
		dlq.logger.Error("DLQ publication failed",
			"message_id", messageID,
			"url", url,
			"domain", domain,
			"error", err,
			"write_duration_ms", writeDuration.Milliseconds(),
			"total_duration_ms", totalDuration.Milliseconds())
		return err
	}

	dlq.logger.Info("DLQ publication completed successfully",
		"message_id", messageID,
		"url", url,
		"domain", domain,
		"attempts", attempts,
		"error_type", errorDetails.Type,
		"is_retryable", errorDetails.IsRetryable,
		"analysis_duration_ms", analysisDuration.Milliseconds(),
		"write_duration_ms", writeDuration.Milliseconds(),
		"total_duration_ms", totalDuration.Milliseconds())

	return nil
}

func (dlq *FileDLQManager) analyzeError(err error) ErrorDetails {
	details := ErrorDetails{
		Message: err.Error(),
	}

	// エラータイプの判定
	switch e := err.(type) {
	case *HTTPError:
		details.Type = "HTTPError"
		details.IsRetryable = isRetryableHTTPStatus(e.StatusCode)
	default:
		details.Type = "UnknownError"
		details.IsRetryable = false
	}

	return details
}

func (dlq *FileDLQManager) writeMessageToFile(message FailedArticleMessage) error {
	start := time.Now()

	// 日付別ディレクトリ作成
	dateDir := message.Timestamp.Format("2006-01-02")
	dir := filepath.Join(dlq.config.BasePath, "failed-articles", dateDir)

	dirStart := time.Now()
	if err := os.MkdirAll(dir, 0750); err != nil {
		dirDuration := time.Since(dirStart)
		dlq.logger.Error("failed to create DLQ directory",
			"dir", dir,
			"error", err,
			"dir_creation_duration_ms", dirDuration.Milliseconds())
		return fmt.Errorf("create directory failed: %w", err)
	}
	dirDuration := time.Since(dirStart)

	// ファイル名生成
	filename := fmt.Sprintf("%s.json", message.ID)
	targetPath := filepath.Join(dir, filename)
	tempFile := targetPath + ".tmp"

	// JSON マーシャリング
	marshalStart := time.Now()
	messageBytes, err := json.MarshalIndent(message, "", "  ")
	marshalDuration := time.Since(marshalStart)
	messageSize := len(messageBytes)

	if err != nil {
		dlq.logger.Error("failed to marshal DLQ message",
			"message_id", message.ID,
			"error", err,
			"marshal_duration_ms", marshalDuration.Milliseconds())
		return fmt.Errorf("marshal failed: %w", err)
	}

	dlq.logger.Debug("DLQ message marshaled",
		"message_id", message.ID,
		"message_size_bytes", messageSize,
		"marshal_duration_ms", marshalDuration.Milliseconds())

	// 一時ファイルに書き込み（原子性保証）
	writeStart := time.Now()
	if err := os.WriteFile(tempFile, messageBytes, 0600); err != nil {
		writeDuration := time.Since(writeStart)
		dlq.logger.Error("failed to write temp DLQ file",
			"temp_file", tempFile,
			"message_size_bytes", messageSize,
			"error", err,
			"write_duration_ms", writeDuration.Milliseconds())
		return fmt.Errorf("write temp file failed: %w", err)
	}
	writeDuration := time.Since(writeStart)

	// 原子的リネーム
	renameStart := time.Now()
	if err := os.Rename(tempFile, targetPath); err != nil {
		renameDuration := time.Since(renameStart)
		if cleanupErr := os.Remove(tempFile); cleanupErr != nil {
			dlq.logger.Error("failed to cleanup temp file", "temp_file", tempFile, "error", cleanupErr)
		} // クリーンアップ
		dlq.logger.Error("failed to rename DLQ file",
			"temp_file", tempFile,
			"target_file", targetPath,
			"error", err,
			"rename_duration_ms", renameDuration.Milliseconds())
		return fmt.Errorf("rename file failed: %w", err)
	}
	renameDuration := time.Since(renameStart)
	totalDuration := time.Since(start)

	dlq.logger.Info("DLQ message file operation completed",
		"message_id", message.ID,
		"url", message.URL,
		"file_path", targetPath,
		"attempts", message.Attempts,
		"message_size_bytes", messageSize,
		"dir_creation_duration_ms", dirDuration.Milliseconds(),
		"marshal_duration_ms", marshalDuration.Milliseconds(),
		"write_duration_ms", writeDuration.Milliseconds(),
		"rename_duration_ms", renameDuration.Milliseconds(),
		"total_duration_ms", totalDuration.Milliseconds(),
		"write_throughput_bytes_per_second", float64(messageSize)/totalDuration.Seconds())

	return nil
}

// DLQ統計情報
type DLQStats struct {
	TotalFailedItems int           `json:"total_failed_items"`
	OldestFailure    time.Time     `json:"oldest_failure"`
	DiskUsage        int64         `json:"disk_usage_bytes"`
	DailyFailureRate float64       `json:"daily_failure_rate"`
}

func (dlq *FileDLQManager) GetStats() (DLQStats, error) {
	stats := DLQStats{}

	failedDir := filepath.Join(dlq.config.BasePath, "failed-articles")

	err := filepath.Walk(failedDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && filepath.Ext(path) == ".json" {
			stats.TotalFailedItems++
			stats.DiskUsage += info.Size()

			if stats.OldestFailure.IsZero() || info.ModTime().Before(stats.OldestFailure) {
				stats.OldestFailure = info.ModTime()
			}
		}

		return nil
	})

	if err != nil {
		return stats, fmt.Errorf("failed to calculate stats: %w", err)
	}

	// 日次失敗率計算
	if !stats.OldestFailure.IsZero() {
		daysSinceOldest := time.Since(stats.OldestFailure).Hours() / 24
		if daysSinceOldest > 0 {
			stats.DailyFailureRate = float64(stats.TotalFailedItems) / daysSinceOldest
		}
	}

	return stats, nil
}

// 古いファイルのクリーンアップ
func (dlq *FileDLQManager) StartCleanup(ctx context.Context) {
	if !dlq.config.EnableCleanup {
		dlq.logger.Info("DLQ cleanup disabled")
		return
	}

	ticker := time.NewTicker(24 * time.Hour) // 1日1回実行
	defer ticker.Stop()

	dlq.logger.Info("DLQ cleanup started",
		"retention", dlq.config.Retention,
		"base_path", dlq.config.BasePath)

	for {
		select {
		case <-ctx.Done():
			dlq.logger.Info("DLQ cleanup stopped")
			return
		case <-ticker.C:
			if err := dlq.cleanup(); err != nil {
				dlq.logger.Error("DLQ cleanup failed", "error", err)
			}
		}
	}
}

func (dlq *FileDLQManager) cleanup() error {
	cutoff := time.Now().Add(-dlq.config.Retention)
	removedCount := 0
	removedSize := int64(0)

	failedDir := filepath.Join(dlq.config.BasePath, "failed-articles")

	err := filepath.Walk(failedDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && info.ModTime().Before(cutoff) {
			size := info.Size()
			if err := os.Remove(path); err != nil {
				dlq.logger.Warn("failed to remove old DLQ file",
					"file", path,
					"error", err)
				return nil // 続行
			}

			removedCount++
			removedSize += size
		}

		return nil
	})

	if removedCount > 0 {
		dlq.logger.Info("DLQ cleanup completed",
			"removed_files", removedCount,
			"removed_size_bytes", removedSize,
			"cutoff", cutoff)
	}

	return err
}

func extractDomain(urlStr string) string {
	if parsed, err := url.Parse(urlStr); err == nil {
		return parsed.Hostname()
	}
	return "unknown"
}

// HTTPError type for compatibility
type HTTPError struct {
	StatusCode int
	Message    string
}

func (e *HTTPError) Error() string {
	return e.Message
}

// isRetryableHTTPStatus determines if HTTP status is retryable
func isRetryableHTTPStatus(status int) bool {
	return status >= 500 && status <= 599 ||
		status == 408 || status == 429 ||
		status == 502 || status == 503 || status == 504
}