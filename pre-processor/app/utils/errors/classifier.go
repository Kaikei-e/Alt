// ABOUTME: Unified error classifier for retry decisions
// ABOUTME: Consolidates logic from service/error_classifier.go and operation_error.go
package errors

import (
	"context"
	"errors"
	"net"
	"syscall"
)

// IsRetryable determines if an error should trigger a retry.
// This is the unified implementation consolidating:
// - service/error_classifier.go (network/HTTP classification)
// - operation_error.go (OperationError retryable flag)
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}

	// Context cancellation is never retryable (user initiated)
	if errors.Is(err, context.Canceled) {
		return false
	}

	// Context deadline exceeded is retryable (timeout)
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	// AppContextError with explicit retryable flag
	var appErr *AppContextError
	if errors.As(err, &appErr) {
		return appErr.IsRetryable()
	}

	// OperationError with explicit retryable flag (legacy support)
	var opErr *OperationError
	if errors.As(err, &opErr) {
		return opErr.Retryable
	}

	// Network OpError with syscall errors
	var opNetErr *net.OpError
	if errors.As(err, &opNetErr) {
		if opNetErr.Err != nil {
			// Check for specific syscall errors
			if errno, ok := opNetErr.Err.(syscall.Errno); ok {
				switch errno {
				case syscall.ECONNREFUSED, syscall.ECONNRESET, syscall.ETIMEDOUT:
					return true
				}
			}
		}
		// Check OpError's own timeout/temporary status
		if opNetErr.Timeout() {
			return true
		}
	}

	// Generic net.Error (timeout only, Temporary() is deprecated)
	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout()
	}

	// Default: not retryable
	return false
}

// IsRetryableHTTPStatus determines if an HTTP status code indicates a retryable condition.
// This consolidates the HTTP status classification from service/error_classifier.go.
func IsRetryableHTTPStatus(status int) bool {
	switch {
	case status >= 500 && status <= 599:
		// 5xx server errors are generally retryable
		return true
	case status == 408: // Request Timeout
		return true
	case status == 429: // Too Many Requests
		return true
	default:
		// 4xx client errors are not retryable
		return false
	}
}
