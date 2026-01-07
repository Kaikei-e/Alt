// ABOUTME: This file implements structured error handling with operation context
// ABOUTME: Provides error classification, correlation, and retry information
package errors

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// ContextKey is a type for context keys to avoid collisions
type ContextKey string

const (
	// RequestIDKey is the context key for request ID
	RequestIDKey ContextKey = "request_id"
	// TraceIDKey is the context key for trace ID
	TraceIDKey ContextKey = "trace_id"
)

// ErrorType represents different categories of errors
type ErrorType int

const (
	// ErrorTypeTransient indicates temporary errors that may succeed on retry
	ErrorTypeTransient ErrorType = iota
	// ErrorTypePermanent indicates permanent errors that won't succeed on retry
	ErrorTypePermanent
	// ErrorTypeUnknown indicates errors where retry behavior is uncertain
	ErrorTypeUnknown
)

// OperationError wraps errors with additional context and metadata
type OperationError struct {
	Operation  string    `json:"operation"`
	RequestID  string    `json:"request_id,omitempty"`
	TraceID    string    `json:"trace_id,omitempty"`
	Timestamp  time.Time `json:"timestamp"`
	Underlying error     `json:"error"`
	Retryable  bool      `json:"retryable"`
}

// NewOperationError creates a new operation error with basic information
func NewOperationError(operation string, err error, retryable bool) *OperationError {
	return &OperationError{
		Operation:  operation,
		Timestamp:  time.Now(),
		Underlying: err,
		Retryable:  retryable,
	}
}

// WithContext adds request and trace IDs from context
func (oe *OperationError) WithContext(ctx context.Context) *OperationError {
	newErr := *oe // Copy the struct
	newErr.RequestID = GetRequestID(ctx)
	newErr.TraceID = GetTraceID(ctx)
	return &newErr
}

// Error implements the error interface
func (oe *OperationError) Error() string {
	var contextInfo string

	if oe.RequestID != "" && oe.TraceID != "" {
		contextInfo = fmt.Sprintf(" (%s/%s)", oe.RequestID, oe.TraceID)
	} else if oe.RequestID != "" {
		contextInfo = fmt.Sprintf(" (%s)", oe.RequestID)
	}

	return fmt.Sprintf("operation '%s' failed%s: %v", oe.Operation, contextInfo, oe.Underlying)
}

// Unwrap returns the underlying error for error unwrapping
func (oe *OperationError) Unwrap() error {
	return oe.Underlying
}

// MarshalJSON implements custom JSON marshaling
func (oe *OperationError) MarshalJSON() ([]byte, error) {
	type Alias OperationError
	return json.Marshal(&struct {
		*Alias
		Error string `json:"error"`
	}{
		Alias: (*Alias)(oe),
		Error: oe.Underlying.Error(),
	})
}

// UnmarshalJSON implements custom JSON unmarshaling
func (oe *OperationError) UnmarshalJSON(data []byte) error {
	type Alias OperationError
	aux := &struct {
		*Alias
		Error string `json:"error"`
	}{
		Alias: (*Alias)(oe),
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	oe.Underlying = fmt.Errorf("%s", aux.Error)
	return nil
}

// WithRequestID adds a request ID to the context
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, RequestIDKey, requestID)
}

// GetRequestID retrieves the request ID from the context
func GetRequestID(ctx context.Context) string {
	if requestID, ok := ctx.Value(RequestIDKey).(string); ok {
		return requestID
	}
	return ""
}

// WithTraceID adds a trace ID to the context
func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, TraceIDKey, traceID)
}

// GetTraceID retrieves the trace ID from the context
func GetTraceID(ctx context.Context) string {
	if traceID, ok := ctx.Value(TraceIDKey).(string); ok {
		return traceID
	}
	return ""
}

// ClassifyError categorizes errors based on their nature
// Deprecated: Use AppContextError.IsRetryable() or utils/errors.IsRetryable() instead
func ClassifyError(err error) ErrorType {
	if err == nil {
		return ErrorTypeUnknown
	}

	errMsg := strings.ToLower(err.Error())

	// Transient errors (should retry)
	transientKeywords := []string{
		"timeout", "connection refused", "connection reset",
		"temporary failure", "service unavailable", "too many requests",
		"network", "dns", "socket", "broken pipe",
	}

	for _, keyword := range transientKeywords {
		if strings.Contains(errMsg, keyword) {
			return ErrorTypeTransient
		}
	}

	// Permanent errors (should not retry)
	permanentKeywords := []string{
		"unauthorized", "forbidden", "not found", "invalid",
		"bad request", "validation", "parse", "format",
		"authentication", "permission", "access denied",
	}

	for _, keyword := range permanentKeywords {
		if strings.Contains(errMsg, keyword) {
			return ErrorTypePermanent
		}
	}

	return ErrorTypeUnknown
}
