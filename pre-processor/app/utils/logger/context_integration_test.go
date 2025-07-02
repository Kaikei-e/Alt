// ABOUTME: This file tests that existing context integration continues to work
// ABOUTME: Ensures request ID, trace ID, and operation context are preserved
package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContextIntegration(t *testing.T) {
	tests := map[string]struct {
		setupContext   func() context.Context
		expectedFields map[string]string
	}{
		"request ID context": {
			setupContext: func() context.Context {
				return WithRequestID(context.Background(), "req-123")
			},
			expectedFields: map[string]string{
				"request_id": "req-123",
			},
		},
		"trace ID context": {
			setupContext: func() context.Context {
				return WithTraceID(context.Background(), "trace-456")
			},
			expectedFields: map[string]string{
				"trace_id": "trace-456",
			},
		},
		"operation context": {
			setupContext: func() context.Context {
				return WithOperation(context.Background(), "process_feed")
			},
			expectedFields: map[string]string{
				"operation": "process_feed",
			},
		},
		"full context chain": {
			setupContext: func() context.Context {
				ctx := context.Background()
				ctx = WithRequestID(ctx, "req-789")
				ctx = WithTraceID(ctx, "trace-abc")
				ctx = WithOperation(ctx, "validate_input")
				return ctx
			},
			expectedFields: map[string]string{
				"request_id": "req-789",
				"trace_id":   "trace-abc",
				"operation":  "validate_input",
			},
		},
		"empty context": {
			setupContext: func() context.Context {
				return context.Background()
			},
			expectedFields: map[string]string{},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			ctx := tc.setupContext()

			// This will fail initially since UnifiedLogger doesn't exist yet
			logger := NewUnifiedLogger(&buf, "pre-processor")
			contextLogger := logger.WithContext(ctx)

			contextLogger.Info("operation completed", "status", "success")

			// Parse the JSON output
			var logEntry map[string]interface{}
			err := json.Unmarshal(buf.Bytes(), &logEntry)
			require.NoError(t, err, "JSON output should be valid")

			// Verify all expected context fields are present
			for key, expectedValue := range tc.expectedFields {
				assert.Equal(t, expectedValue, logEntry[key], "Context field %s should be preserved", key)
			}

			// Verify basic log structure
			assert.Equal(t, "INFO", logEntry["level"])
			assert.Equal(t, "operation completed", logEntry["msg"])
			assert.Equal(t, "success", logEntry["status"])
			assert.Equal(t, "pre-processor", logEntry["service"])
		})
	}
}

func TestExistingLoggerCompatibility(t *testing.T) {
	// Test that the new logger produces output compatible with existing usage patterns
	var buf bytes.Buffer

	// This will fail initially since UnifiedLogger doesn't exist yet
	logger := NewUnifiedLogger(&buf, "pre-processor")

	// Test existing usage patterns from the codebase
	ctx := WithRequestID(WithTraceID(context.Background(), "trace-test"), "req-test")
	contextLogger := logger.WithContext(ctx)

	// Pattern 1: Simple info logging (from service layer)
	contextLogger.Info("Feed processing started", "feed_id", "feed-123")

	// Parse first log entry
	lines := bytes.Split(buf.Bytes(), []byte("\n"))
	require.Greater(t, len(lines), 0, "Should produce log output")

	var logEntry map[string]interface{}
	err := json.Unmarshal(lines[0], &logEntry)
	require.NoError(t, err, "Should produce valid JSON")

	// Verify existing patterns continue to work
	assert.Equal(t, "INFO", logEntry["level"])
	assert.Equal(t, "Feed processing started", logEntry["msg"])
	assert.Equal(t, "feed-123", logEntry["feed_id"])
	assert.Equal(t, "req-test", logEntry["request_id"])
	assert.Equal(t, "trace-test", logEntry["trace_id"])
	assert.Equal(t, "pre-processor", logEntry["service"])
}

func TestMiddlewareLoggingCompatibility(t *testing.T) {
	// Test compatibility with existing middleware logging patterns
	var buf bytes.Buffer

	// This will fail initially since UnifiedLogger doesn't exist yet
	logger := NewUnifiedLogger(&buf, "pre-processor")

	ctx := WithRequestID(context.Background(), "middleware-test")
	contextLogger := logger.WithContext(ctx)

	// Simulate middleware logging pattern
	contextLogger.Info("request completed",
		"method", "POST",
		"path", "/api/feeds",
		"status", 200,
		"duration_ms", 150,
		"response_size", 1024)

	var logEntry map[string]interface{}
	err := json.Unmarshal(buf.Bytes(), &logEntry)
	require.NoError(t, err)

	// Verify middleware fields are preserved
	expectedFields := map[string]interface{}{
		"method":        "POST",
		"path":          "/api/feeds",
		"status":        float64(200),
		"duration_ms":   float64(150),
		"response_size": float64(1024),
		"request_id":    "middleware-test",
	}

	for key, expectedValue := range expectedFields {
		assert.Equal(t, expectedValue, logEntry[key], "Middleware field %s should be preserved", key)
	}
}

func TestServiceLayerLoggingCompatibility(t *testing.T) {
	// Test compatibility with existing service layer logging patterns
	var buf bytes.Buffer

	// This will fail initially since UnifiedLogger doesn't exist yet
	logger := NewUnifiedLogger(&buf, "pre-processor")

	ctx := WithOperation(WithTraceID(context.Background(), "svc-trace"), "validate_feed")
	contextLogger := logger.WithContext(ctx)

	// Simulate service layer error logging
	contextLogger.Error("validation failed",
		"feed_url", "https://example.com/feed.xml",
		"error", "invalid XML structure",
		"line_number", 42)

	var logEntry map[string]interface{}
	err := json.Unmarshal(buf.Bytes(), &logEntry)
	require.NoError(t, err)

	// Verify service layer patterns work
	assert.Equal(t, "ERROR", logEntry["level"])
	assert.Equal(t, "validation failed", logEntry["msg"])
	assert.Equal(t, "https://example.com/feed.xml", logEntry["feed_url"])
	assert.Equal(t, "invalid XML structure", logEntry["error"])
	assert.Equal(t, float64(42), logEntry["line_number"])
	assert.Equal(t, "svc-trace", logEntry["trace_id"])
	assert.Equal(t, "validate_feed", logEntry["operation"])
}
