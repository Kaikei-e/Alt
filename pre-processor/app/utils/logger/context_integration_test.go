// ABOUTME: This file tests that existing context integration continues to work
// ABOUTME: Ensures request ID, trace ID, and operation context are preserved
package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
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
				return WithOperation(context.Background(), "feed-processing")
			},
			expectedFields: map[string]string{
				"operation": "feed-processing",
			},
		},
		"combined context": {
			setupContext: func() context.Context {
				ctx := WithRequestID(context.Background(), "req-789")
				ctx = WithTraceID(ctx, "trace-789")
				return WithOperation(ctx, "validation")
			},
			expectedFields: map[string]string{
				"request_id": "req-789",
				"trace_id":   "trace-789",
				"operation":  "validation",
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

			ctx := test.setupContext()
			loggerWithCtx := logger.WithContext(ctx)

			loggerWithCtx.Info("operation completed", "status", "success")

			// Restore stdout and read captured output
			w.Close()
			os.Stdout = old

			var buf bytes.Buffer
			buf.ReadFrom(r)
			logOutput := buf.String()

			var logEntry map[string]interface{}
			err := json.Unmarshal([]byte(logOutput), &logEntry)
			require.NoError(t, err, "Should produce valid JSON")

			// Verify basic log structure - should use lowercase levels for rask-log-forwarder compatibility
			assert.Equal(t, "info", logEntry["level"])
			assert.Equal(t, "operation completed", logEntry["msg"])
			assert.Equal(t, "success", logEntry["status"])

			// Verify expected context fields are present
			for key, expectedValue := range test.expectedFields {
				assert.Equal(t, expectedValue, logEntry[key], "Should have correct %s", key)
			}
		})
	}
}

func TestBackwardsCompatibility(t *testing.T) {
	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	logger := NewUnifiedLogger("feed-processor")

	// Test that existing service patterns continue to work
	logger.Info("Feed processing started", "feed_id", "feed-123", "source", "rss")

	// Restore stdout and read captured output
	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	logOutput := buf.String()

	var logEntry map[string]interface{}
	err := json.Unmarshal([]byte(logOutput), &logEntry)
	require.NoError(t, err, "Should produce valid JSON")

	// Verify existing patterns continue to work - should use lowercase levels for rask-log-forwarder compatibility
	assert.Equal(t, "info", logEntry["level"])
	assert.Equal(t, "Feed processing started", logEntry["msg"])
	assert.Equal(t, "feed-123", logEntry["feed_id"])
	assert.Equal(t, "rss", logEntry["source"])
}

func TestServiceLayerIntegration(t *testing.T) {
	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	logger := NewUnifiedLogger("feed-validator")

	// Test service layer error patterns
	logger.Error("validation failed", "feed_url", "https://example.com/feed.xml", "error_type", "malformed_xml")

	// Restore stdout and read captured output
	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	logOutput := buf.String()

	var logEntry map[string]interface{}
	err := json.Unmarshal([]byte(logOutput), &logEntry)
	require.NoError(t, err)

	// Verify service layer patterns work - should use lowercase levels for rask-log-forwarder compatibility
	assert.Equal(t, "error", logEntry["level"])
	assert.Equal(t, "validation failed", logEntry["msg"])
	assert.Equal(t, "https://example.com/feed.xml", logEntry["feed_url"])
	assert.Equal(t, "malformed_xml", logEntry["error_type"])
}
