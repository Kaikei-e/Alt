// ABOUTME: This file tests fields extraction logic that rask-log-forwarder uses
// ABOUTME: Simulates how rask-log-forwarder parses slog JSON and extracts fields
package logger

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
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
		"service": true,
	}

	for key, value := range jsonData {
		if !skipFields[key] {
			fields[key] = fmt.Sprintf("%v", value)
		}
	}

	return fields
}

func TestFieldsExtractionCompatibility(t *testing.T) {
	tests := map[string]struct {
		logEntry       string
		expectedFields map[string]string
	}{
		"request processing log": {
			logEntry: `{"time":"2024-01-15T10:30:00Z","level":"info","msg":"request completed","method":"GET","path":"/api/feeds","status":200,"duration_ms":45,"request_id":"req-123"}`,
			expectedFields: map[string]string{
				"method":      "GET",
				"path":        "/api/feeds",
				"status":      "200",
				"duration_ms": "45",
				"request_id":  "req-123",
			},
		},
		"feed processing log": {
			logEntry: `{"time":"2024-01-15T10:31:00Z","level":"info","msg":"feed processed","feed_id":"feed-456","feed_url":"https://example.com/rss","items_count":15,"trace_id":"trace-789"}`,
			expectedFields: map[string]string{
				"feed_id":     "feed-456",
				"feed_url":    "https://example.com/rss",
				"items_count": "15",
				"trace_id":    "trace-789",
			},
		},
		"error log with context": {
			logEntry: `{"time":"2024-01-15T10:32:00Z","level":"error","msg":"validation failed","error":"malformed XML","feed_url":"https://bad.example.com/feed","operation":"validate_feed","request_id":"req-error"}`,
			expectedFields: map[string]string{
				"error":      "malformed XML",
				"feed_url":   "https://bad.example.com/feed",
				"operation":  "validate_feed",
				"request_id": "req-error",
			},
		},
		"debug log with trace": {
			logEntry: `{"time":"2024-01-15T10:33:00Z","level":"debug","msg":"processing item","item_id":"item-999","trace_id":"debug-trace","step":"validation","position":3}`,
			expectedFields: map[string]string{
				"item_id":  "item-999",
				"trace_id": "debug-trace",
				"step":     "validation",
				"position": "3",
			},
		},
		"warning with metrics": {
			logEntry: `{"time":"2024-01-15T10:34:00Z","level":"warn","msg":"high memory usage","memory_mb":512,"threshold_mb":400,"process":"feed-parser","alert_sent":true}`,
			expectedFields: map[string]string{
				"memory_mb":    "512",
				"threshold_mb": "400",
				"process":      "feed-parser",
				"alert_sent":   "true",
			},
		},
		"log with no custom fields": {
			logEntry:       `{"time":"2024-01-15T10:35:00Z","level":"info","msg":"system started","service":"pre-processor"}`,
			expectedFields: map[string]string{},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			fields := extractFieldsFromSlogJSON(test.logEntry)

			// Verify all expected fields are extracted
			assert.Equal(t, test.expectedFields, fields,
				"Extracted fields should match expected for %s", name)
		})
	}
}

func TestFieldsExtractionEdgeCases(t *testing.T) {
	tests := map[string]struct {
		logEntry       string
		expectedFields map[string]string
	}{
		"nested object field": {
			logEntry: `{"time":"2024-01-15T10:30:00Z","level":"info","msg":"complex data","user":{"id":123,"name":"test"},"status":"ok"}`,
			expectedFields: map[string]string{
				"user":   "map[id:123 name:test]",
				"status": "ok",
			},
		},
		"array field": {
			logEntry: `{"time":"2024-01-15T10:30:00Z","level":"info","msg":"list data","tags":["news","tech","go"],"count":3}`,
			expectedFields: map[string]string{
				"tags":  "[news tech go]",
				"count": "3",
			},
		},
		"special characters in values": {
			logEntry: `{"time":"2024-01-15T10:30:00Z","level":"info","msg":"special chars","url":"https://example.com/path?q=test&type=rss","description":"Test: 'quotes' and \"escapes\""}`,
			expectedFields: map[string]string{
				"url":         "https://example.com/path?q=test&type=rss",
				"description": "Test: 'quotes' and \"escapes\"",
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			fields := extractFieldsFromSlogJSON(test.logEntry)
			assert.Equal(t, test.expectedFields, fields,
				"Should handle edge case: %s", name)
		})
	}
}

func TestFieldsExtractionWithInvalidJSON(t *testing.T) {
	invalidCases := []string{
		"not json at all",
		`{"incomplete": json`,
		`{"time":"2024-01-15T10:30:00Z"`, // incomplete
		"",                               // empty
	}

	for i, invalidJSON := range invalidCases {
		t.Run(fmt.Sprintf("invalid_case_%d", i), func(t *testing.T) {
			fields := extractFieldsFromSlogJSON(invalidJSON)
			assert.Nil(t, fields, "Should return nil for invalid JSON")
		})
	}
}

func TestActualUnifiedLoggerOutput(t *testing.T) {
	// This test verifies that our extraction works with actual UnifiedLogger output
	// This will fail initially until UnifiedLogger is implemented

	// TODO: Uncomment when UnifiedLogger is ready
	// var buf bytes.Buffer
	// logger := NewUnifiedLogger(&buf, "test-service")
	//
	// logger.Info("test message", "custom_field", "custom_value", "number", 42)
	//
	// fields := extractFieldsFromSlogJSON(buf.String())
	// expected := map[string]string{
	// 	"custom_field": "custom_value",
	// 	"number":       "42",
	// }
	//
	// assert.Equal(t, expected, fields, "Should extract fields from actual logger output")
}
