// ABOUTME: This file tests the unified slog-based logger for rask-log-aggregator compatibility
// ABOUTME: Ensures Alt-backend compatible JSON format and proper fields extraction
package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
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
		"error level with context": {
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
		"debug level with trace": {
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

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer

			// This will fail initially since UnifiedLogger doesn't exist yet
			logger := NewUnifiedLogger(&buf, "pre-processor")

			// Call the logger method that should produce Alt-backend compatible JSON
			contextLogger := logger.WithContext(context.Background())
			contextLogger.Log(context.Background(), tc.level, tc.message, tc.args...)

			// Parse the JSON output
			var logEntry map[string]interface{}
			err := json.Unmarshal(buf.Bytes(), &logEntry)
			require.NoError(t, err, "JSON output should be valid")

			// Verify Alt-backend compatible structure
			for key, expectedValue := range tc.expectedJSON {
				assert.Equal(t, expectedValue, logEntry[key], "Field %s should match Alt-backend format", key)
			}

			// Must contain service field like Alt-backend
			assert.Equal(t, "pre-processor", logEntry["service"])

			// Must contain time field in Alt-backend format
			assert.Contains(t, logEntry, "time")

			// Time should be parseable as RFC3339
			timeStr, ok := logEntry["time"].(string)
			require.True(t, ok, "time should be a string")
			_, err = time.Parse(time.RFC3339, timeStr)
			assert.NoError(t, err, "time should be in RFC3339 format like Alt-backend")
		})
	}
}

func TestAltBackendFieldStructure(t *testing.T) {
	var buf bytes.Buffer

	// This will fail initially since UnifiedLogger doesn't exist yet
	logger := NewUnifiedLogger(&buf, "pre-processor")
	contextLogger := logger.WithContext(context.Background())

	// Test exact Alt-backend usage pattern
	contextLogger.Info("Feed search completed successfully",
		"query", "golang news",
		"results_count", 10)

	var logEntry map[string]interface{}
	err := json.Unmarshal(buf.Bytes(), &logEntry)
	require.NoError(t, err)

	// Verify Alt-backend field structure exactly
	expectedFields := map[string]interface{}{
		"level":         "INFO",
		"msg":           "Feed search completed successfully",
		"query":         "golang news",
		"results_count": float64(10),
		"service":       "pre-processor",
	}

	for key, expectedValue := range expectedFields {
		assert.Equal(t, expectedValue, logEntry[key], "Field %s must match Alt-backend exactly", key)
	}

	// Should NOT contain rask-specific fields in the JSON
	assert.NotContains(t, logEntry, "service_type")
	assert.NotContains(t, logEntry, "log_type")
	assert.NotContains(t, logEntry, "service_name")
	assert.NotContains(t, logEntry, "fields")
}
