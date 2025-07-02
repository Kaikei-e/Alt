// ABOUTME: This file tests the Rask-compatible logger implementation
// ABOUTME: Ensures proper JSON format and schema compatibility with rask-log-aggregator
package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRaskLogger_BasicLogging(t *testing.T) {
	tests := map[string]struct {
		level       slog.Level
		message     string
		expectedMsg string
		expectedLvl string
	}{
		"info level": {
			level:       slog.LevelInfo,
			message:     "test info message",
			expectedMsg: "test info message",
			expectedLvl: "info",
		},
		"error level": {
			level:       slog.LevelError,
			message:     "test error message",
			expectedMsg: "test error message",
			expectedLvl: "error",
		},
		"debug level": {
			level:       slog.LevelDebug,
			message:     "test debug message",
			expectedMsg: "test debug message",
			expectedLvl: "debug",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := NewRaskLogger(&buf, "pre-processor")

			// This should fail initially since RaskLogger doesn't exist yet
			logger.Log(tc.level, tc.message)

			// Parse the JSON output
			var logEntry map[string]interface{}
			err := json.Unmarshal(buf.Bytes(), &logEntry)
			require.NoError(t, err, "JSON output should be valid")

			// Verify required fields for rask-log-aggregator
			assert.Equal(t, tc.expectedMsg, logEntry["message"])
			assert.Equal(t, tc.expectedLvl, logEntry["level"])
			assert.Equal(t, "pre-processor", logEntry["service_name"])
			assert.Equal(t, "application", logEntry["service_type"])
			assert.Equal(t, "structured", logEntry["log_type"])
			assert.Contains(t, logEntry, "timestamp")
		})
	}
}

func TestRaskLogger_RequiredFields(t *testing.T) {
	var buf bytes.Buffer
	logger := NewRaskLogger(&buf, "test-service")

	logger.Info("test message")

	var logEntry map[string]interface{}
	err := json.Unmarshal(buf.Bytes(), &logEntry)
	require.NoError(t, err)

	// Verify all required fields from EnrichedLogEntry schema
	requiredFields := []string{
		"service_type",
		"log_type", 
		"message",
		"level",
		"timestamp",
		"service_name",
	}

	for _, field := range requiredFields {
		assert.Contains(t, logEntry, field, "Missing required field: %s", field)
		assert.NotEmpty(t, logEntry[field], "Field %s should not be empty", field)
	}
}

func TestRaskLogger_TimestampFormat(t *testing.T) {
	var buf bytes.Buffer
	logger := NewRaskLogger(&buf, "pre-processor")

	logger.Info("timestamp test")

	var logEntry map[string]interface{}
	err := json.Unmarshal(buf.Bytes(), &logEntry)
	require.NoError(t, err)

	timestampStr, ok := logEntry["timestamp"].(string)
	require.True(t, ok, "timestamp should be a string")

	// Verify RFC3339 format
	_, err = time.Parse(time.RFC3339, timestampStr)
	assert.NoError(t, err, "timestamp should be in RFC3339 format")
}

func TestRaskLogger_WithAttributes(t *testing.T) {
	var buf bytes.Buffer
	logger := NewRaskLogger(&buf, "pre-processor")

	logger.With("operation", "test_op", "trace_id", "abc123").Info("test with attributes")

	var logEntry map[string]interface{}
	err := json.Unmarshal(buf.Bytes(), &logEntry)
	require.NoError(t, err)

	// Check that attributes are in the fields object
	fields, ok := logEntry["fields"].(map[string]interface{})
	require.True(t, ok, "fields should be a map")
	
	assert.Equal(t, "test_op", fields["operation"])
	assert.Equal(t, "abc123", fields["trace_id"])
}

func TestRaskLogger_SchemaCompatibility(t *testing.T) {
	var buf bytes.Buffer
	logger := NewRaskLogger(&buf, "pre-processor")

	logger.Error("schema compatibility test")

	// Verify the output matches rask-log-aggregator EnrichedLogEntry structure
	var logEntry struct {
		ServiceType  string            `json:"service_type"`
		LogType      string            `json:"log_type"`
		Message      string            `json:"message"`
		Level        string            `json:"level"`
		Timestamp    string            `json:"timestamp"`
		ServiceName  string            `json:"service_name"`
		Stream       string            `json:"stream,omitempty"`
		ContainerID  string            `json:"container_id,omitempty"`
		ServiceGroup string            `json:"service_group,omitempty"`
		Fields       map[string]string `json:"fields,omitempty"`
	}

	err := json.Unmarshal(buf.Bytes(), &logEntry)
	require.NoError(t, err, "Should unmarshal into EnrichedLogEntry-compatible struct")

	assert.Equal(t, "application", logEntry.ServiceType)
	assert.Equal(t, "structured", logEntry.LogType)
	assert.Equal(t, "schema compatibility test", logEntry.Message)
	assert.Equal(t, "error", logEntry.Level)
	assert.Equal(t, "pre-processor", logEntry.ServiceName)
}

func TestRaskLogger_CompareWithExistingLogger(t *testing.T) {
	// Test that basic functionality remains consistent
	var raskBuf, existingBuf bytes.Buffer
	
	raskLogger := NewRaskLogger(&raskBuf, "pre-processor")
	existingLogger := NewContextLogger(&existingBuf, "json", "info")

	testMessage := "comparison test message"
	
	raskLogger.Info(testMessage)
	existingLogger.logger.Info(testMessage)

	// Both should produce valid JSON
	var raskEntry, existingEntry map[string]interface{}
	
	err := json.Unmarshal(raskBuf.Bytes(), &raskEntry)
	require.NoError(t, err, "Rask logger should produce valid JSON")
	
	err = json.Unmarshal(existingBuf.Bytes(), &existingEntry)
	require.NoError(t, err, "Existing logger should produce valid JSON")

	// Both should contain the message (though field names may differ)
	assert.True(t, strings.Contains(raskBuf.String(), testMessage))
	assert.True(t, strings.Contains(existingBuf.String(), testMessage))
}

func TestRaskLogger_Integration_WithConfig(t *testing.T) {
	tests := map[string]struct {
		useRask      bool
		expectFields []string
	}{
		"rask enabled": {
			useRask: true,
			expectFields: []string{"service_type", "log_type", "service_name", "message", "level", "timestamp"},
		},
		"rask disabled": {
			useRask: false,
			expectFields: []string{"msg", "level", "time"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			
			config := &LoggerConfig{
				Level:       "info",
				Format:      "json",
				ServiceName: "test-service",
				UseRask:     tc.useRask,
			}
			
			contextLogger := NewContextLoggerWithConfig(config, &buf)
			logger := contextLogger.WithContext(context.Background())
			
			logger.Info("integration test message")
			
			// Verify output contains expected fields
			output := buf.String()
			assert.NotEmpty(t, output, "Should produce log output")
			
			for _, field := range tc.expectFields {
				assert.Contains(t, output, field, "Should contain field: %s", field)
			}
		})
	}
}

func TestRaskLogger_NoRegression_ExistingBehavior(t *testing.T) {
	// Test that existing behavior is unchanged when USE_RASK_LOGGER=false
	var buf bytes.Buffer
	
	config := &LoggerConfig{
		Level:       "info",
		Format:      "json", 
		ServiceName: "pre-processor",
		UseRask:     false, // Explicitly disable Rask
	}
	
	contextLogger := NewContextLoggerWithConfig(config, &buf)
	logger := contextLogger.WithContext(context.Background())
	
	testMessage := "regression test log"
	logger.Info(testMessage)
	
	output := buf.String()
	assert.Contains(t, output, testMessage)
	
	// Parse JSON to check field names properly
	var logEntry map[string]interface{}
	err := json.Unmarshal(buf.Bytes(), &logEntry)
	require.NoError(t, err)
	
	// Should use existing field names, not Rask field names
	assert.Contains(t, logEntry, "msg")
	assert.Contains(t, logEntry, "time")
	assert.NotContains(t, logEntry, "message")
	assert.NotContains(t, logEntry, "timestamp")
	assert.NotContains(t, logEntry, "service_type")
}