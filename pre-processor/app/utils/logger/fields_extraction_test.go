// ABOUTME: This file tests fields extraction logic that rask-log-forwarder uses
// ABOUTME: Simulates how rask-log-forwarder parses slog JSON and extracts fields
package logger

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// extractFieldsFromSlogJSON simulates rask-log-forwarder's field extraction logic
func extractFieldsFromSlogJSON(logEntry string) map[string]string {
	var jsonData map[string]interface{}
	if err := json.Unmarshal([]byte(logEntry), &jsonData); err != nil {
		return nil
	}

	fields := make(map[string]string)

	// Skip standard slog fields that are handled separately
	skipFields := map[string]bool{
		"time":    true,
		"level":   true,
		"msg":     true,
		"message": true,
		"service": true,
	}

	for key, value := range jsonData {
		if !skipFields[key] {
			fields[key] = toString(value)
		}
	}

	return fields
}

func toString(value interface{}) string {
	switch v := value.(type) {
	case string:
		return v
	case float64:
		return fmt.Sprintf("%.0f", v)
	case int:
		return fmt.Sprintf("%d", v)
	case bool:
		return fmt.Sprintf("%t", v)
	default:
		return fmt.Sprintf("%v", v)
	}
}

func TestFieldsExtraction(t *testing.T) {
	tests := map[string]struct {
		logEntry       string
		expectedFields map[string]string
	}{
		"standard slog format with fields": {
			logEntry: `{"time":"2024-01-01T12:00:00Z","level":"INFO","msg":"test","operation":"fetch","trace_id":"abc123","service":"pre-processor"}`,
			expectedFields: map[string]string{
				"operation": "fetch",
				"trace_id":  "abc123",
			},
		},
		"request logging pattern": {
			logEntry: `{"time":"2024-01-01T12:00:00Z","level":"INFO","msg":"request completed","method":"GET","path":"/feeds","status":"200","duration_ms":"45","service":"pre-processor"}`,
			expectedFields: map[string]string{
				"method":      "GET",
				"path":        "/feeds",
				"status":      "200",
				"duration_ms": "45",
			},
		},
		"error logging with context": {
			logEntry: `{"time":"2024-01-01T12:00:00Z","level":"ERROR","msg":"database error","error":"connection timeout","table":"feeds","retries":"3","service":"pre-processor"}`,
			expectedFields: map[string]string{
				"error":   "connection timeout",
				"table":   "feeds",
				"retries": "3",
			},
		},
		"empty additional fields": {
			logEntry:       `{"time":"2024-01-01T12:00:00Z","level":"INFO","msg":"simple message","service":"pre-processor"}`,
			expectedFields: map[string]string{},
		},
		"numeric values conversion": {
			logEntry: `{"time":"2024-01-01T12:00:00Z","level":"INFO","msg":"metrics","count":10,"rate":95.5,"enabled":true,"service":"pre-processor"}`,
			expectedFields: map[string]string{
				"count":   "10",
				"rate":    "96", // Should convert to string
				"enabled": "true",
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// Test the extraction logic that rask-log-forwarder would use
			fields := extractFieldsFromSlogJSON(tc.logEntry)
			assert.Equal(t, tc.expectedFields, fields, "Fields extraction should match expected format")

			// Verify fields can be stored in ClickHouse Map(String, String) format
			for key, value := range fields {
				assert.IsType(t, "", key, "Field keys must be strings")
				assert.IsType(t, "", value, "Field values must be strings")
			}
		})
	}
}

func TestRaskLogForwarderCompatibility(t *testing.T) {
	// Test that our expected slog output is compatible with rask-log-forwarder parsing

	// This is what UnifiedLogger should produce (will fail initially)
	expectedSlogOutput := `{"time":"2024-01-01T12:00:00Z","level":"INFO","msg":"feed processed","feed_id":"123","status":"success","duration_ms":"150","service":"pre-processor"}`

	fields := extractFieldsFromSlogJSON(expectedSlogOutput)

	expectedFields := map[string]string{
		"feed_id":     "123",
		"status":      "success",
		"duration_ms": "150",
	}

	assert.Equal(t, expectedFields, fields, "Extracted fields should be ready for ClickHouse Map(String, String)")

	// Verify no standard fields leak into the fields map
	assert.NotContains(t, fields, "time")
	assert.NotContains(t, fields, "level")
	assert.NotContains(t, fields, "msg")
	assert.NotContains(t, fields, "service")
}

func TestClickHouseFieldsSchema(t *testing.T) {
	// Test that extracted fields match ClickHouse Map(String, String) requirements

	testLogEntry := `{"time":"2024-01-01T12:00:00Z","level":"INFO","msg":"test","request_id":"req-123","operation":"process_feed","items_count":"5","success_rate":"98.5","service":"pre-processor"}`

	fields := extractFieldsFromSlogJSON(testLogEntry)

	// All fields should be string key-value pairs for ClickHouse compatibility
	for key, value := range fields {
		require.IsType(t, "", key, "ClickHouse Map keys must be strings")
		require.IsType(t, "", value, "ClickHouse Map values must be strings")
		require.NotEmpty(t, key, "ClickHouse Map keys cannot be empty")
	}

	// Verify expected fields are present
	expectedFields := []string{"request_id", "operation", "items_count", "success_rate"}
	for _, fieldName := range expectedFields {
		assert.Contains(t, fields, fieldName, "Field %s should be extracted", fieldName)
	}
}
