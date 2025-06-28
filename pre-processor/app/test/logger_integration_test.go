// ABOUTME: This file contains integration tests for enhanced structured logging
// ABOUTME: Tests context propagation, performance monitoring, and rask-compatible JSON format
package test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"pre-processor/utils/logger"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContextLogger_StructuredLogging(t *testing.T) {
	tests := map[string]struct {
		setupContext func() context.Context
		logOperation func(ctx context.Context, logger *logger.ContextLogger)
		wantFields   []string
	}{
		"logs_with_request_id_context": {
			setupContext: func() context.Context {
				return logger.WithRequestID(context.Background(), "req-123")
			},
			logOperation: func(ctx context.Context, contextLogger *logger.ContextLogger) {
				contextLogger.WithContext(ctx).Info("operation started", "operation", "test")
			},
			wantFields: []string{"request_id", "operation", "service", "time"},
		},
		"logs_with_trace_id_context": {
			setupContext: func() context.Context {
				ctx := logger.WithRequestID(context.Background(), "req-456")
				return logger.WithTraceID(ctx, "trace-789")
			},
			logOperation: func(ctx context.Context, contextLogger *logger.ContextLogger) {
				contextLogger.WithContext(ctx).Error("operation failed", "error", "timeout")
			},
			wantFields: []string{"request_id", "trace_id", "error", "level"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// Capture log output
			buf := &bytes.Buffer{}
			contextLogger := logger.NewContextLogger(buf, "json", "debug")

			ctx := tc.setupContext()
			tc.logOperation(ctx, contextLogger)

			// Parse JSON log output
			var logEntry map[string]interface{}
			err := json.Unmarshal(buf.Bytes(), &logEntry)
			require.NoError(t, err)

			// Verify required fields
			for _, field := range tc.wantFields {
				assert.Contains(t, logEntry, field, "field %s should be present", field)
				assert.NotEmpty(t, logEntry[field], "field %s should not be empty", field)
			}
		})
	}
}

func TestPerformanceLogger_Timing(t *testing.T) {
	tests := map[string]struct {
		operation         string
		duration          time.Duration
		threshold         time.Duration
		expectSlowWarning bool
	}{
		"fast_operation_under_threshold": {
			operation:         "fast_op",
			duration:          10 * time.Millisecond,
			threshold:         100 * time.Millisecond,
			expectSlowWarning: false,
		},
		"slow_operation_over_threshold": {
			operation:         "slow_op",
			duration:          200 * time.Millisecond,
			threshold:         100 * time.Millisecond,
			expectSlowWarning: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			perfLogger := logger.NewPerformanceLogger(buf, tc.threshold)

			// Simulate operation timing
			timer := perfLogger.StartTimer(context.Background(), tc.operation)
			time.Sleep(tc.duration)
			timer.End()

			// Verify log contains timing information
			logLines := bytes.Split(buf.Bytes(), []byte("\n"))

			foundTiming := false
			foundSlowWarning := false

			for _, line := range logLines {
				if len(line) == 0 {
					continue
				}

				var logEntry map[string]interface{}
				if err := json.Unmarshal(line, &logEntry); err != nil {
					continue
				}

				if msg, ok := logEntry["msg"].(string); ok {
					if msg == "operation completed" {
						foundTiming = true
						assert.Contains(t, logEntry, "duration_ms")
						assert.Contains(t, logEntry, "operation")
					}
					if msg == "slow operation detected" {
						foundSlowWarning = true
					}
				}
			}

			assert.True(t, foundTiming, "should log operation completion")
			assert.Equal(t, tc.expectSlowWarning, foundSlowWarning, "slow warning expectation mismatch")
		})
	}
}

func TestLoggerConfiguration_EnvironmentBased(t *testing.T) {
	tests := map[string]struct {
		envVars      map[string]string
		expectLevel  string
		expectFormat string
	}{
		"json_debug_configuration": {
			envVars: map[string]string{
				"LOG_LEVEL":  "debug",
				"LOG_FORMAT": "json",
			},
			expectLevel:  "debug",
			expectFormat: "json",
		},
		"text_info_configuration": {
			envVars: map[string]string{
				"LOG_LEVEL":  "info",
				"LOG_FORMAT": "text",
			},
			expectLevel:  "info",
			expectFormat: "text",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// Set environment variables
			for key, value := range tc.envVars {
				t.Setenv(key, value)
			}

			config := logger.LoadLoggerConfigFromEnv()

			assert.Equal(t, tc.expectLevel, config.Level)
			assert.Equal(t, tc.expectFormat, config.Format)
		})
	}
}
