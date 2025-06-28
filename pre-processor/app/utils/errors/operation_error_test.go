// ABOUTME: This file tests structured error handling with operation context
package errors

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOperationError_New(t *testing.T) {
	tests := map[string]struct {
		operation string
		err       error
		retryable bool
		expected  *OperationError
	}{
		"basic error creation": {
			operation: "feed_processing",
			err:       errors.New("network timeout"),
			retryable: true,
			expected: &OperationError{
				Operation: "feed_processing",
				Underlying: errors.New("network timeout"),
				Retryable: true,
			},
		},
		"non-retryable error": {
			operation: "validation",
			err:       errors.New("invalid format"),
			retryable: false,
			expected: &OperationError{
				Operation: "validation",
				Underlying: errors.New("invalid format"),
				Retryable: false,
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			opErr := NewOperationError(tc.operation, tc.err, tc.retryable)

			assert.Equal(t, tc.expected.Operation, opErr.Operation)
			assert.Equal(t, tc.expected.Underlying.Error(), opErr.Underlying.Error())
			assert.Equal(t, tc.expected.Retryable, opErr.Retryable)
			assert.WithinDuration(t, time.Now(), opErr.Timestamp, time.Second)
		})
	}
}

func TestOperationError_WithContext(t *testing.T) {
	ctx := context.Background()
	ctx = context.WithValue(ctx, RequestIDKey, "req-12345")
	ctx = context.WithValue(ctx, TraceIDKey, "trace-67890")

	baseErr := errors.New("test error")
	opErr := NewOperationError("test_op", baseErr, true)
	contextErr := opErr.WithContext(ctx)

	assert.Equal(t, "req-12345", contextErr.RequestID)
	assert.Equal(t, "trace-67890", contextErr.TraceID)
	assert.Equal(t, opErr.Operation, contextErr.Operation)
	assert.Equal(t, opErr.Underlying.Error(), contextErr.Underlying.Error())
	assert.Equal(t, opErr.Retryable, contextErr.Retryable)
}

func TestOperationError_Error(t *testing.T) {
	tests := map[string]struct {
		opErr    *OperationError
		expected string
	}{
		"with request and trace ID": {
			opErr: &OperationError{
				Operation:  "feed_processing",
				RequestID:  "req-12345",
				TraceID:    "trace-67890",
				Underlying: errors.New("network timeout"),
				Retryable:  true,
			},
			expected: "operation 'feed_processing' failed (req-12345/trace-67890): network timeout",
		},
		"with request ID only": {
			opErr: &OperationError{
				Operation:  "validation",
				RequestID:  "req-12345",
				Underlying: errors.New("invalid format"),
				Retryable:  false,
			},
			expected: "operation 'validation' failed (req-12345): invalid format",
		},
		"without context IDs": {
			opErr: &OperationError{
				Operation:  "summarization",
				Underlying: errors.New("API error"),
				Retryable:  true,
			},
			expected: "operation 'summarization' failed: API error",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			result := tc.opErr.Error()
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestOperationError_Unwrap(t *testing.T) {
	baseErr := errors.New("underlying error")
	opErr := NewOperationError("test_op", baseErr, true)

	unwrapped := opErr.Unwrap()
	assert.Equal(t, baseErr, unwrapped)
}

func TestOperationError_JSONSerialization(t *testing.T) {
	timestamp := time.Now()
	opErr := &OperationError{
		Operation:  "feed_processing",
		RequestID:  "req-12345",
		TraceID:    "trace-67890",
		Timestamp:  timestamp,
		Underlying: errors.New("network timeout"),
		Retryable:  true,
	}

	// Test JSON marshaling
	jsonData, err := json.Marshal(opErr)
	require.NoError(t, err)

	// Test JSON unmarshaling
	var unmarshaled OperationError
	err = json.Unmarshal(jsonData, &unmarshaled)
	require.NoError(t, err)

	assert.Equal(t, opErr.Operation, unmarshaled.Operation)
	assert.Equal(t, opErr.RequestID, unmarshaled.RequestID)
	assert.Equal(t, opErr.TraceID, unmarshaled.TraceID)
	assert.WithinDuration(t, opErr.Timestamp, unmarshaled.Timestamp, time.Second)
	assert.Equal(t, opErr.Retryable, unmarshaled.Retryable)
	// Note: Underlying error becomes string after JSON roundtrip
}

func TestContextKeys(t *testing.T) {
	ctx := context.Background()

	// Test RequestID
	ctx = WithRequestID(ctx, "req-12345")
	requestID := GetRequestID(ctx)
	assert.Equal(t, "req-12345", requestID)

	// Test TraceID
	ctx = WithTraceID(ctx, "trace-67890")
	traceID := GetTraceID(ctx)
	assert.Equal(t, "trace-67890", traceID)

	// Test empty context
	emptyCtx := context.Background()
	assert.Empty(t, GetRequestID(emptyCtx))
	assert.Empty(t, GetTraceID(emptyCtx))
}

func TestIsRetryable(t *testing.T) {
	tests := map[string]struct {
		err      error
		expected bool
	}{
		"retryable operation error": {
			err:      NewOperationError("test", errors.New("timeout"), true),
			expected: true,
		},
		"non-retryable operation error": {
			err:      NewOperationError("test", errors.New("validation"), false),
			expected: false,
		},
		"regular error": {
			err:      errors.New("regular error"),
			expected: false,
		},
		"nil error": {
			err:      nil,
			expected: false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			result := IsRetryable(tc.err)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestClassifyError(t *testing.T) {
	tests := map[string]struct {
		err      error
		expected ErrorType
	}{
		"network timeout": {
			err:      errors.New("network timeout"),
			expected: ErrorTypeTransient,
		},
		"connection refused": {
			err:      errors.New("connection refused"),
			expected: ErrorTypeTransient,
		},
		"validation error": {
			err:      errors.New("invalid input format"),
			expected: ErrorTypePermanent,
		},
		"unauthorized error": {
			err:      errors.New("unauthorized"),
			expected: ErrorTypePermanent,
		},
		"unknown error": {
			err:      errors.New("unknown error"),
			expected: ErrorTypeUnknown,
		},
		"nil error": {
			err:      nil,
			expected: ErrorTypeUnknown,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			result := ClassifyError(tc.err)
			assert.Equal(t, tc.expected, result)
		})
	}
}