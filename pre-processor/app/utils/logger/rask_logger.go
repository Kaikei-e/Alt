// ABOUTME: This file provides Rask-compatible logging that wraps slog.Handler
// ABOUTME: Outputs JSON logs compatible with rask-log-aggregator EnrichedLogEntry schema
package logger

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"
)

// RaskLogger is a logger that outputs JSON compatible with EnrichedLogEntry
type RaskLogger struct {
	serviceName string
	output      io.Writer
	attrs       map[string]any
}

// EnrichedLogEntry represents the expected schema for rask-log-aggregator
type EnrichedLogEntry struct {
	ServiceType  string            `json:"service_type"`
	LogType      string            `json:"log_type"`
	Message      string            `json:"message"`
	Level        string            `json:"level,omitempty"`
	Timestamp    string            `json:"timestamp"`
	Stream       string            `json:"stream,omitempty"`
	ContainerID  string            `json:"container_id,omitempty"`
	ServiceName  string            `json:"service_name"`
	ServiceGroup string            `json:"service_group,omitempty"`
	Fields       map[string]string `json:"fields,omitempty"`
}

// NewRaskLogger creates a new RaskLogger that outputs JSON compatible with EnrichedLogEntry
func NewRaskLogger(output io.Writer, serviceName string) *RaskLogger {
	return &RaskLogger{
		output:      output,
		serviceName: serviceName,
		attrs:       make(map[string]any),
	}
}

// Log logs a message at the specified level with rask-compatible metadata
func (rl *RaskLogger) Log(level slog.Level, msg string, args ...any) {
	// Build fields map from args and persistent attrs
	fields := make(map[string]string)

	// Add persistent attributes
	for k, v := range rl.attrs {
		fields[k] = fmt.Sprintf("%v", v)
	}

	// Add args as fields
	for i := 0; i < len(args); i += 2 {
		if i+1 < len(args) {
			if key, ok := args[i].(string); ok {
				fields[key] = fmt.Sprintf("%v", args[i+1])
			}
		}
	}

	// Create EnrichedLogEntry
	entry := EnrichedLogEntry{
		ServiceType: "application",
		LogType:     "structured",
		Message:     msg,
		Level:       strings.ToLower(level.String()),
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
		ServiceName: rl.serviceName,
		Fields:      fields,
	}

	// Serialize and write
	jsonData, err := json.Marshal(entry)
	if err != nil {
		// Fallback to simple format
		fmt.Fprintf(rl.output, `{"error":"failed to marshal log entry","message":"%s"}%s`, msg, "\n")
		return
	}

	rl.output.Write(jsonData)
	rl.output.Write([]byte("\n"))
}

// Info logs an info message
func (rl *RaskLogger) Info(msg string, args ...any) {
	rl.Log(slog.LevelInfo, msg, args...)
}

// Error logs an error message
func (rl *RaskLogger) Error(msg string, args ...any) {
	rl.Log(slog.LevelError, msg, args...)
}

// Debug logs a debug message
func (rl *RaskLogger) Debug(msg string, args ...any) {
	rl.Log(slog.LevelDebug, msg, args...)
}

// Warn logs a warning message
func (rl *RaskLogger) Warn(msg string, args ...any) {
	rl.Log(slog.LevelWarn, msg, args...)
}

// With returns a new RaskLogger with additional key-value pairs
func (rl *RaskLogger) With(args ...any) *RaskLogger {
	// Create a copy of existing attributes
	newAttrs := make(map[string]any)
	for k, v := range rl.attrs {
		newAttrs[k] = v
	}

	// Add new attributes
	for i := 0; i < len(args); i += 2 {
		if i+1 < len(args) {
			if key, ok := args[i].(string); ok {
				newAttrs[key] = args[i+1]
			}
		}
	}

	return &RaskLogger{
		output:      rl.output,
		serviceName: rl.serviceName,
		attrs:       newAttrs,
	}
}