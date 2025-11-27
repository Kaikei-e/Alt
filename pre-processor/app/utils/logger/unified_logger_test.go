// ABOUTME: This file tests the unified slog-based logger for rask-log-aggregator compatibility
// ABOUTME: Ensures Alt-backend compatible JSON format and proper fields extraction
package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSlogCompatibility(t *testing.T) {
	tests := map[string]struct {
		level        slog.Level
		message      string
		args         []any
		expectedJSON map[string]interface{}
	}{
		"info level with attributes": {
			level:   slog.LevelInfo,
			message: "request completed",
			args:    []any{"method", "GET", "status", 200, "duration_ms", 45},
			expectedJSON: map[string]interface{}{
				"level":       "INFO",
				"msg":         "request completed",
				"method":      "GET",
				"status":      float64(200),
				"duration_ms": float64(45),
			},
		},
		"error level with nested fields": {
			level:   slog.LevelError,
			message: "database connection failed",
			args:    []any{"error", "connection timeout", "retries", 3},
			expectedJSON: map[string]interface{}{
				"level":   "ERROR",
				"msg":     "database connection failed",
				"error":   "connection timeout",
				"retries": float64(3),
			},
		},
		"debug level for tracing": {
			level:   slog.LevelDebug,
			message: "processing item",
			args:    []any{"trace_id", "abc123", "operation", "validate"},
			expectedJSON: map[string]interface{}{
				"level":     "DEBUG",
				"msg":       "processing item",
				"trace_id":  "abc123",
				"operation": "validate",
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			// Capture stdout
			old := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			logger := NewUnifiedLogger("test-service")

			// Use the logger's internal slog logger with the specified level
			logger.logger.Log(context.Background(), test.level, test.message, test.args...)

			// Restore stdout and read captured output
			errors := w.Close()
			require.NoError(t, errors, "Should close writer")
			os.Stdout = old

			var buf bytes.Buffer
			_, _ = buf.ReadFrom(r)
			logOutput := buf.String()

			var result map[string]interface{}
			err := json.Unmarshal([]byte(logOutput), &result)
			require.NoError(t, err, "Should produce valid JSON")

			// Verify all expected fields are present
			for key, expectedValue := range test.expectedJSON {
				assert.Equal(t, expectedValue, result[key], "Field %s should match", key)
			}

			// Verify standard fields are present
			assert.Contains(t, result, "time", "Should have timestamp")
			assert.Equal(t, "test-service", result["service"], "Should have service name")
		})
	}
}

func TestAltBackendCompatibility(t *testing.T) {
	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	logger := NewUnifiedLogger("feed-processor")

	// Simulate typical Alt-backend logging pattern
	logger.Info("Feed search completed successfully",
		"query", "golang news",
		"total_results", 42,
		"duration_ms", 125,
		"cache_hit", true)

	// Restore stdout and read captured output
	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	logOutput := buf.String()

	var logEntry map[string]interface{}
	err := json.Unmarshal([]byte(logOutput), &logEntry)
	require.NoError(t, err, "Should produce valid JSON")

	// Verify Alt-backend field structure exactly
	expectedFields := map[string]interface{}{
		"level":         "INFO",
		"msg":           "Feed search completed successfully",
		"query":         "golang news",
		"total_results": float64(42),
		"duration_ms":   float64(125),
		"cache_hit":     true,
		"service":       "feed-processor",
	}

	for key, expectedValue := range expectedFields {
		assert.Equal(t, expectedValue, logEntry[key], "Alt-backend field %s should match", key)
	}

	// Verify required Alt-backend structure
	assert.Contains(t, logEntry, "time", "Should have RFC3339 timestamp")
	assert.IsType(t, "", logEntry["time"], "Timestamp should be string")

	// Verify timestamp is valid RFC3339
	_, err = time.Parse(time.RFC3339, logEntry["time"].(string))
	assert.NoError(t, err, "Timestamp should be valid RFC3339")
}

func TestContextIntegrationWithSlog(t *testing.T) {
	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	logger := NewUnifiedLogger("context-test")

	// Test context integration with slog patterns
	ctx := WithRequestID(WithTraceID(context.Background(), "trace-slog"), "req-slog")
	contextLogger := logger.WithContext(ctx)

	// Use both logger methods and direct slog calls
	contextLogger.Warn("potential issue detected", "threshold", 0.95, "current", 0.97)

	// Restore stdout and read captured output
	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	logOutput := buf.String()

	var logEntry map[string]interface{}
	err := json.Unmarshal([]byte(logOutput), &logEntry)
	require.NoError(t, err, "Should produce valid JSON")

	// Verify context fields are preserved in slog format
	expectedFields := map[string]interface{}{
		"level":      "WARN",
		"msg":        "potential issue detected",
		"threshold":  0.95,
		"current":    0.97,
		"request_id": "req-slog",
		"trace_id":   "trace-slog",
		"service":    "context-test",
	}

	for key, expectedValue := range expectedFields {
		assert.Equal(t, expectedValue, logEntry[key], "Context field %s should be preserved", key)
	}
}
