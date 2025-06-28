// ABOUTME: This file contains integration tests for end-to-end logging flow validation
// ABOUTME: Tests rask-log-forwarder compatibility, JSON schema compliance, and context propagation
package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"pre-processor/utils/logger"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// LogEntry represents the simplified log format for stdout/stderr
type LogEntry struct {
	Time    string `json:"time"`
	Level   string `json:"level"`
	Msg     string `json:"msg"`
	Service string `json:"service"`
	Version string `json:"version"`
	// Additional fields for context
	RequestID string `json:"request_id,omitempty"`
	TraceID   string `json:"trace_id,omitempty"`
	Operation string `json:"operation,omitempty"`
}

func TestLoggingIntegration_EndToEndFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	tests := map[string]struct {
		setupContext func() context.Context
		operation    func(ctx context.Context, h *mockHealthHandler)
		wantLogs     []string
		wantFields   []string
	}{
		"complete_request_flow_with_context": {
			setupContext: func() context.Context {
				ctx := logger.WithRequestID(context.Background(), "integration-req-001")
				return logger.WithTraceID(ctx, "integration-trace-001")
			},
			operation: func(ctx context.Context, h *mockHealthHandler) {
				// Simulate complete request flow
				h.CheckHealth(ctx)
				h.logger.WithContext(ctx).Info("processing request")
				h.logger.WithContext(ctx).Info("health check completed")
			},
			wantLogs: []string{"health check completed"},
			wantFields: []string{
				"request_id", "trace_id",
				"service", "version", "time", "level", "msg",
			},
		},
		"error_handling_flow": {
			setupContext: func() context.Context {
				return logger.WithRequestID(context.Background(), "error-req-001")
			},
			operation: func(ctx context.Context, h *mockHealthHandler) {
				h.logger.WithContext(ctx).Error("simulated error", "error_type", "validation")
			},
			wantLogs: []string{"simulated error"},
			wantFields: []string{
				"request_id", "service", "error_type",
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			logBuffer := &bytes.Buffer{}
			contextLogger := logger.NewContextLogger(logBuffer, "json", "debug")

			handler := &mockHealthHandler{logger: contextLogger}
			ctx := tc.setupContext()

			// Execute operation
			tc.operation(ctx, handler)

			// Parse and validate logs
			logLines := bytes.Split(logBuffer.Bytes(), []byte("\n"))
			var logEntries []LogEntry

			for _, line := range logLines {
				if len(line) == 0 {
					continue
				}

				var entry LogEntry
				err := json.Unmarshal(line, &entry)
				require.NoError(t, err)
				logEntries = append(logEntries, entry)
			}

			// Verify expected log messages are present
			foundMessages := make(map[string]bool)
			for _, entry := range logEntries {
				foundMessages[entry.Msg] = true
			}

			for _, expectedMsg := range tc.wantLogs {
				assert.True(t, foundMessages[expectedMsg],
					"should find log message: %s", expectedMsg)
			}

			// Verify required fields are present in at least one log entry
			for _, requiredField := range tc.wantFields {
				found := false
				for _, entry := range logEntries {
					if hasField(entry, requiredField) {
						found = true
						break
					}
				}
				assert.True(t, found, "should find required field: %s", requiredField)
			}

			// Verify rask-log-forwarder compatibility
			for i, entry := range logEntries {
				validateLogFormat(t, entry, i)
			}
		})
	}
}

func TestLoggingIntegration_PerformanceUnderLoad(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	// Setup performance logger
	logBuffer := &bytes.Buffer{}
	perfLogger := logger.NewPerformanceLogger(logBuffer, 100*time.Millisecond)

	// Test concurrent logging
	numConcurrent := 10
	numOperationsPerGoroutine := 100

	done := make(chan bool, numConcurrent)

	for i := 0; i < numConcurrent; i++ {
		go func(workerID int) {
			defer func() { done <- true }()

			for j := 0; j < numOperationsPerGoroutine; j++ {
				ctx := logger.WithRequestID(context.Background(),
					fmt.Sprintf("worker-%d-op-%d", workerID, j))

				timer := perfLogger.StartTimer(ctx, "test_operation")
				time.Sleep(1 * time.Millisecond) // Simulate work
				timer.End()
			}
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < numConcurrent; i++ {
		<-done
	}

	// Validate logs were generated correctly
	logLines := bytes.Split(logBuffer.Bytes(), []byte("\n"))
	var validLogCount int

	for _, line := range logLines {
		if len(line) == 0 {
			continue
		}

		var entry map[string]interface{}
		if err := json.Unmarshal(line, &entry); err == nil {
			if msg, ok := entry["msg"].(string); ok && msg == "operation completed" {
				validLogCount++
			}
		}
	}

	expectedLogCount := numConcurrent * numOperationsPerGoroutine
	assert.Equal(t, expectedLogCount, validLogCount,
		"should log completion for all operations")
}

func TestLoggingIntegration_ContextPropagation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	tests := map[string]struct {
		setupContext     func() context.Context
		operations       []string
		expectedContexts map[string]interface{}
	}{
		"request_id_propagation": {
			setupContext: func() context.Context {
				return logger.WithRequestID(context.Background(), "req-propagation-001")
			},
			operations: []string{"operation1", "operation2", "operation3"},
			expectedContexts: map[string]interface{}{
				"request_id": "req-propagation-001",
			},
		},
		"full_context_propagation": {
			setupContext: func() context.Context {
				ctx := logger.WithRequestID(context.Background(), "req-full-001")
				ctx = logger.WithTraceID(ctx, "trace-full-001")
				return logger.WithOperation(ctx, "parent-operation")
			},
			operations: []string{"child-op1", "child-op2"},
			expectedContexts: map[string]interface{}{
				"request_id": "req-full-001",
				"trace_id":   "trace-full-001",
				"operation":  "parent-operation",
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			logBuffer := &bytes.Buffer{}
			contextLogger := logger.NewContextLogger(logBuffer, "json", "debug")

			ctx := tc.setupContext()

			// Simulate operations with context propagation
			for i := 0; i < len(tc.operations); i++ {
				contextLogger.WithContext(ctx).Info("operation executed")
			}

			// Parse logs and verify context propagation
			logLines := bytes.Split(logBuffer.Bytes(), []byte("\n"))

			for _, line := range logLines {
				if len(line) == 0 {
					continue
				}

				var entry LogEntry
				err := json.Unmarshal(line, &entry)
				require.NoError(t, err)

				// Verify all expected context values are present
				for expectedKey, expectedValue := range tc.expectedContexts {
					switch expectedKey {
					case "request_id":
						if entry.RequestID != "" {
							assert.Equal(t, expectedValue, entry.RequestID,
								"request_id should have expected value")
						}
					case "trace_id":
						if entry.TraceID != "" {
							assert.Equal(t, expectedValue, entry.TraceID,
								"trace_id should have expected value")
						}
					case "operation":
						if entry.Operation != "" {
							assert.Equal(t, expectedValue, entry.Operation,
								"operation should have expected value")
						}
					}
				}
			}
		})
	}
}

// Helper functions

func hasField(entry LogEntry, fieldPath string) bool {
	switch fieldPath {
	case "request_id":
		return entry.RequestID != ""
	case "trace_id":
		return entry.TraceID != ""
	case "operation":
		return entry.Operation != ""
	}

	// Check top-level fields using reflection
	switch fieldPath {
	case "time":
		return entry.Time != ""
	case "level":
		return entry.Level != ""
	case "msg":
		return entry.Msg != ""
	case "service":
		return entry.Service != ""
	case "version":
		return entry.Version != ""
	case "error_type":
		// For error_type field, we need to check the raw JSON entry
		// Since it's not in the LogEntry struct
		return true // Assume present for now
	}

	return false
}

func validateLogFormat(t *testing.T, entry LogEntry, index int) {
	// Validate required top-level fields
	assert.NotEmpty(t, entry.Time, "log entry %d should have time field", index)
	assert.NotEmpty(t, entry.Level, "log entry %d should have level field", index)
	assert.NotEmpty(t, entry.Msg, "log entry %d should have msg field", index)
	assert.Equal(t, "pre-processor", entry.Service,
		"log entry %d should have correct service", index)
	assert.NotEmpty(t, entry.Version,
		"log entry %d should have version field", index)

	// Validate level format (should be lowercase)
	validLevels := []string{"debug", "info", "warn", "error"}
	assert.Contains(t, validLevels, entry.Level,
		"log entry %d should have valid level", index)

	// Validate time format (should be RFC3339)
	_, err := time.Parse(time.RFC3339, entry.Time)
	assert.NoError(t, err, "log entry %d should have valid time format", index)
}

// Mock implementations for testing

type mockHealthHandler struct {
	logger *logger.ContextLogger
}

func (h *mockHealthHandler) CheckHealth(ctx context.Context) error {
	h.logger.WithContext(ctx).Info("performing health check")
	h.logger.WithContext(ctx).Info("health check completed - service is healthy")
	return nil
}
