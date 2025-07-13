// ABOUTME: This file tests JSON file-based Dead Letter Queue implementation
// ABOUTME: Tests failure tracking and message persistence for resilient processing
package dlq

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelError, // Only errors in tests
	}))
}

// TDD RED PHASE: Test DLQ message publishing
func TestFileDLQManager_PublishFailedArticle(t *testing.T) {
	// テスト用一時ディレクトリ
	tempDir := t.TempDir()

	config := FileDLQConfig{
		BasePath:      tempDir,
		MaxFileSize:   1024,
		Retention:     24 * time.Hour,
		EnableCleanup: false,
	}

	dlqManager := NewFileDLQManager(config, testLogger())

	testError := &HTTPError{StatusCode: 500, Message: "Internal Server Error"}

	err := dlqManager.PublishFailedArticle(context.Background(),
		"https://example.com/article", 3, testError)

	require.NoError(t, err)

	// ファイルが作成されたか確認
	dateDir := time.Now().Format("2006-01-02")
	expectedDir := filepath.Join(tempDir, "failed-articles", dateDir)

	files, err := os.ReadDir(expectedDir)
	require.NoError(t, err)
	require.Len(t, files, 1)

	// ファイル内容を確認
	filePath := filepath.Join(expectedDir, files[0].Name())
	content, err := os.ReadFile(filePath)
	require.NoError(t, err)

	var message FailedArticleMessage
	err = json.Unmarshal(content, &message)
	require.NoError(t, err)

	assert.Equal(t, "https://example.com/article", message.URL)
	assert.Equal(t, 3, message.Attempts)
	assert.Equal(t, "HTTPError", message.LastError.Type)
	assert.Equal(t, "pre-processor", message.ServiceName)
	assert.Equal(t, "article_fetch", message.TaskType)
	assert.NotEmpty(t, message.ID)
	assert.False(t, message.Timestamp.IsZero())
}

// TDD RED PHASE: Test multiple message publishing 
func TestFileDLQManager_PublishMultipleMessages(t *testing.T) {
	tempDir := t.TempDir()

	config := FileDLQConfig{
		BasePath:      tempDir,
		MaxFileSize:   1024,
		Retention:     24 * time.Hour,
		EnableCleanup: false,
	}

	dlqManager := NewFileDLQManager(config, testLogger())

	urls := []string{
		"https://example.com/article1",
		"https://example.com/article2",
		"https://example.com/article3",
	}

	for i, url := range urls {
		testError := &HTTPError{StatusCode: 500 + i, Message: "Server Error"}
		err := dlqManager.PublishFailedArticle(context.Background(), url, i+1, testError)
		require.NoError(t, err)
	}

	// ファイル数を確認
	dateDir := time.Now().Format("2006-01-02")
	expectedDir := filepath.Join(tempDir, "failed-articles", dateDir)

	files, err := os.ReadDir(expectedDir)
	require.NoError(t, err)
	assert.Len(t, files, 3)
}

// TDD RED PHASE: Test cleanup functionality
func TestFileDLQManager_Cleanup(t *testing.T) {
	tempDir := t.TempDir()

	config := FileDLQConfig{
		BasePath:      tempDir,
		Retention:     1 * time.Hour,
		EnableCleanup: true,
	}

	dlqManager := NewFileDLQManager(config, testLogger())

	// 古いファイルを作成
	oldTime := time.Now().Add(-2 * time.Hour)
	oldDir := filepath.Join(tempDir, "failed-articles", oldTime.Format("2006-01-02"))
	require.NoError(t, os.MkdirAll(oldDir, 0755))

	oldFile := filepath.Join(oldDir, "old_message.json")
	require.NoError(t, os.WriteFile(oldFile, []byte(`{"test": "data"}`), 0644))
	require.NoError(t, os.Chtimes(oldFile, oldTime, oldTime))

	// 新しいファイルを作成
	newTime := time.Now()
	newDir := filepath.Join(tempDir, "failed-articles", newTime.Format("2006-01-02"))
	require.NoError(t, os.MkdirAll(newDir, 0755))

	newFile := filepath.Join(newDir, "new_message.json")
	require.NoError(t, os.WriteFile(newFile, []byte(`{"test": "data"}`), 0644))

	// クリーンアップ実行
	err := dlqManager.cleanup()
	require.NoError(t, err)

	// 古いファイルが削除され、新しいファイルが残っていることを確認
	_, err = os.Stat(oldFile)
	assert.True(t, os.IsNotExist(err))

	_, err = os.Stat(newFile)
	assert.NoError(t, err)
}

// TDD RED PHASE: Test statistics calculation
func TestFileDLQManager_GetStats(t *testing.T) {
	tempDir := t.TempDir()

	config := FileDLQConfig{
		BasePath: tempDir,
	}

	dlqManager := NewFileDLQManager(config, testLogger())

	// テストファイルを作成
	testDir := filepath.Join(tempDir, "failed-articles", "2024-01-15")
	require.NoError(t, os.MkdirAll(testDir, 0755))

	for i := 0; i < 3; i++ {
		fileName := filepath.Join(testDir, "test_"+string(rune('a'+i))+".json")
		require.NoError(t, os.WriteFile(fileName, []byte(`{"test": "data"}`), 0644))
	}

	stats, err := dlqManager.GetStats()
	require.NoError(t, err)

	assert.Equal(t, 3, stats.TotalFailedItems)
	assert.Greater(t, stats.DiskUsage, int64(0))
}

// TDD RED PHASE: Test error analysis
func TestFileDLQManager_AnalyzeError(t *testing.T) {
	tempDir := t.TempDir()
	config := FileDLQConfig{BasePath: tempDir}
	dlqManager := NewFileDLQManager(config, testLogger())

	tests := map[string]struct {
		err      error
		wantType string
		retryable bool
	}{
		"HTTP 500 error": {
			err:       &HTTPError{StatusCode: 500, Message: "Internal Server Error"},
			wantType:  "HTTPError",
			retryable: true,
		},
		"HTTP 404 error": {
			err:       &HTTPError{StatusCode: 404, Message: "Not Found"},
			wantType:  "HTTPError",
			retryable: false,
		},
		"Generic error": {
			err:       assert.AnError,
			wantType:  "UnknownError",
			retryable: false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			details := dlqManager.analyzeError(tc.err)

			assert.Equal(t, tc.wantType, details.Type)
			assert.Equal(t, tc.retryable, details.IsRetryable)
			assert.Equal(t, tc.err.Error(), details.Message)
		})
	}
}

// HTTPError is defined in file_dlq.go