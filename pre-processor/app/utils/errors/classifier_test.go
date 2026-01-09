// ABOUTME: Tests for unified error classifier
// ABOUTME: Consolidates retry decision logic from service/error_classifier.go
package errors

import (
	"context"
	"errors"
	"net"
	"syscall"
	"testing"
)

func TestIsRetryable_NilError(t *testing.T) {
	if IsRetryable(nil) {
		t.Error("nil error should not be retryable")
	}
}

func TestIsRetryable_ContextErrors(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		retryable bool
	}{
		{
			name:      "context.Canceled is not retryable",
			err:       context.Canceled,
			retryable: false,
		},
		{
			name:      "context.DeadlineExceeded is retryable",
			err:       context.DeadlineExceeded,
			retryable: true,
		},
		{
			name:      "wrapped context.Canceled is not retryable",
			err:       errors.Join(errors.New("operation failed"), context.Canceled),
			retryable: false,
		},
		{
			name:      "wrapped context.DeadlineExceeded is retryable",
			err:       errors.Join(errors.New("operation failed"), context.DeadlineExceeded),
			retryable: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsRetryable(tt.err); got != tt.retryable {
				t.Errorf("IsRetryable() = %v, want %v", got, tt.retryable)
			}
		})
	}
}

func TestIsRetryable_AppContextError(t *testing.T) {
	tests := []struct {
		name      string
		err       *AppContextError
		retryable bool
	}{
		{
			name:      "TIMEOUT_ERROR is retryable",
			err:       &AppContextError{Code: "TIMEOUT_ERROR"},
			retryable: true,
		},
		{
			name:      "RATE_LIMIT_ERROR is retryable",
			err:       &AppContextError{Code: "RATE_LIMIT_ERROR"},
			retryable: true,
		},
		{
			name:      "EXTERNAL_API_ERROR is retryable",
			err:       &AppContextError{Code: "EXTERNAL_API_ERROR"},
			retryable: true,
		},
		{
			name:      "VALIDATION_ERROR is not retryable",
			err:       &AppContextError{Code: "VALIDATION_ERROR"},
			retryable: false,
		},
		{
			name:      "NOT_FOUND_ERROR is not retryable",
			err:       &AppContextError{Code: "NOT_FOUND_ERROR"},
			retryable: false,
		},
		{
			name:      "DATABASE_ERROR is not retryable",
			err:       &AppContextError{Code: "DATABASE_ERROR"},
			retryable: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsRetryable(tt.err); got != tt.retryable {
				t.Errorf("IsRetryable() = %v, want %v", got, tt.retryable)
			}
		})
	}
}

func TestIsRetryable_OperationError(t *testing.T) {
	tests := []struct {
		name      string
		err       *OperationError
		retryable bool
	}{
		{
			name:      "retryable OperationError",
			err:       NewOperationError("test", errors.New("timeout"), true),
			retryable: true,
		},
		{
			name:      "non-retryable OperationError",
			err:       NewOperationError("test", errors.New("invalid"), false),
			retryable: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsRetryable(tt.err); got != tt.retryable {
				t.Errorf("IsRetryable() = %v, want %v", got, tt.retryable)
			}
		})
	}
}

// mockNetError implements net.Error for testing
type mockNetError struct {
	timeout   bool
	temporary bool
}

func (e *mockNetError) Error() string   { return "mock net error" }
func (e *mockNetError) Timeout() bool   { return e.timeout }
func (e *mockNetError) Temporary() bool { return e.temporary }

func TestIsRetryable_NetError(t *testing.T) {
	tests := []struct {
		name      string
		err       net.Error
		retryable bool
	}{
		{
			name:      "timeout net.Error is retryable",
			err:       &mockNetError{timeout: true, temporary: false},
			retryable: true,
		},
		{
			name:      "non-timeout net.Error is not retryable",
			err:       &mockNetError{timeout: false, temporary: false},
			retryable: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsRetryable(tt.err); got != tt.retryable {
				t.Errorf("IsRetryable() = %v, want %v", got, tt.retryable)
			}
		})
	}
}

func TestIsRetryable_SyscallErrors(t *testing.T) {
	// Test with OpError wrapping syscall errors
	tests := []struct {
		name      string
		errno     syscall.Errno
		retryable bool
	}{
		{
			name:      "ECONNREFUSED is retryable",
			errno:     syscall.ECONNREFUSED,
			retryable: true,
		},
		{
			name:      "ECONNRESET is retryable",
			errno:     syscall.ECONNRESET,
			retryable: true,
		},
		{
			name:      "ETIMEDOUT is retryable",
			errno:     syscall.ETIMEDOUT,
			retryable: true,
		},
		{
			name:      "ENOENT is not retryable",
			errno:     syscall.ENOENT,
			retryable: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opErr := &net.OpError{
				Op:  "dial",
				Net: "tcp",
				Err: tt.errno,
			}
			if got := IsRetryable(opErr); got != tt.retryable {
				t.Errorf("IsRetryable() = %v, want %v", got, tt.retryable)
			}
		})
	}
}

func TestIsRetryable_RegularError(t *testing.T) {
	// Regular errors without specific type should not be retryable by default
	err := errors.New("some random error")
	if IsRetryable(err) {
		t.Error("regular error should not be retryable")
	}
}

func TestIsRetryableHTTPStatus(t *testing.T) {
	tests := []struct {
		status    int
		retryable bool
	}{
		{200, false},
		{201, false},
		{400, false},
		{401, false},
		{403, false},
		{404, false},
		{408, true}, // Request Timeout
		{429, true}, // Too Many Requests
		{500, true}, // Internal Server Error
		{501, true}, // Not Implemented (5xx)
		{502, true}, // Bad Gateway
		{503, true}, // Service Unavailable
		{504, true}, // Gateway Timeout
	}

	for _, tt := range tests {
		t.Run(string(rune(tt.status)), func(t *testing.T) {
			if got := IsRetryableHTTPStatus(tt.status); got != tt.retryable {
				t.Errorf("IsRetryableHTTPStatus(%d) = %v, want %v", tt.status, got, tt.retryable)
			}
		})
	}
}
