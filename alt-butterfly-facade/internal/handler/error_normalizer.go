// Package handler provides HTTP handlers for the BFF service.
package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
)

// NormalizedError represents a standardized error response for the frontend.
// It provides consistent error information including retry guidance.
type NormalizedError struct {
	Code        string `json:"code"`
	Message     string `json:"message"`
	IsRetryable bool   `json:"is_retryable"`
	RetryAfter  int    `json:"retry_after,omitempty"` // seconds
	RequestID   string `json:"request_id"`
}

// Error codes
const (
	CodeBackendUnavailable = "BACKEND_UNAVAILABLE"
	CodeServiceUnavailable = "SERVICE_UNAVAILABLE"
	CodeRateLimitExceeded  = "RATE_LIMIT_EXCEEDED"
	CodeInvalidToken       = "INVALID_TOKEN"
	CodeAccessDenied       = "ACCESS_DENIED"
	CodeInternalError      = "INTERNAL_ERROR"
	CodeGatewayTimeout     = "GATEWAY_TIMEOUT"
	CodeBadRequest         = "BAD_REQUEST"
	CodeNotFound           = "NOT_FOUND"
	CodeNetworkError       = "NETWORK_ERROR"
	CodeUnknownError       = "UNKNOWN_ERROR"
)

// errorMapping defines how HTTP status codes map to normalized errors
var errorMapping = map[int]struct {
	code        string
	message     string
	isRetryable bool
	retryAfter  int
}{
	http.StatusBadGateway: {
		code:        CodeBackendUnavailable,
		message:     "Backend service is temporarily unavailable",
		isRetryable: true,
		retryAfter:  5,
	},
	http.StatusServiceUnavailable: {
		code:        CodeServiceUnavailable,
		message:     "Service is temporarily unavailable",
		isRetryable: true,
		retryAfter:  10,
	},
	http.StatusTooManyRequests: {
		code:        CodeRateLimitExceeded,
		message:     "Rate limit exceeded, please slow down",
		isRetryable: true,
		retryAfter:  60,
	},
	http.StatusUnauthorized: {
		code:        CodeInvalidToken,
		message:     "Authentication token is invalid or expired",
		isRetryable: false,
		retryAfter:  0,
	},
	http.StatusForbidden: {
		code:        CodeAccessDenied,
		message:     "Access to this resource is denied",
		isRetryable: false,
		retryAfter:  0,
	},
	http.StatusInternalServerError: {
		code:        CodeInternalError,
		message:     "An internal error occurred",
		isRetryable: true,
		retryAfter:  5,
	},
	http.StatusGatewayTimeout: {
		code:        CodeGatewayTimeout,
		message:     "Backend service timed out",
		isRetryable: true,
		retryAfter:  10,
	},
	http.StatusBadRequest: {
		code:        CodeBadRequest,
		message:     "The request was malformed or invalid",
		isRetryable: false,
		retryAfter:  0,
	},
	http.StatusNotFound: {
		code:        CodeNotFound,
		message:     "The requested resource was not found",
		isRetryable: false,
		retryAfter:  0,
	},
}

// NormalizeError converts an HTTP response to a normalized error.
// It extracts retry information from headers when available.
func NormalizeError(resp *http.Response, requestID string) *NormalizedError {
	mapping, ok := errorMapping[resp.StatusCode]
	if !ok {
		return &NormalizedError{
			Code:        CodeUnknownError,
			Message:     "An unexpected error occurred",
			IsRetryable: false,
			RetryAfter:  0,
			RequestID:   requestID,
		}
	}

	retryAfter := mapping.retryAfter

	// Check for Retry-After header
	if retryAfterHeader := resp.Header.Get("Retry-After"); retryAfterHeader != "" {
		if seconds, err := strconv.Atoi(retryAfterHeader); err == nil && seconds > 0 {
			retryAfter = seconds
		}
	}

	return &NormalizedError{
		Code:        mapping.code,
		Message:     mapping.message,
		IsRetryable: mapping.isRetryable,
		RetryAfter:  retryAfter,
		RequestID:   requestID,
	}
}

// NormalizeNetworkError creates a normalized error for network-level failures.
// This is used when the backend cannot be reached at all.
func NormalizeNetworkError(errMsg string, requestID string) *NormalizedError {
	return &NormalizedError{
		Code:        CodeNetworkError,
		Message:     "Unable to connect to backend service",
		IsRetryable: true,
		RetryAfter:  5,
		RequestID:   requestID,
	}
}

// ToJSON serializes the normalized error to JSON bytes.
func (e *NormalizedError) ToJSON() ([]byte, error) {
	return json.Marshal(e)
}

// IsErrorResponse checks if a status code indicates an error.
func IsErrorResponse(statusCode int) bool {
	return statusCode >= 400
}

// IsDependencyFailure reports whether a status code is evidence that the
// dependency itself is unhealthy, and so should charge its circuit-breaker
// failure budget. Only 5xx qualifies. A 4xx means the dependency answered —
// it just answered "no", which for content fetches usually reflects the
// third-party publisher (rate limited, paywalled, removed), not alt-backend.
// This is deliberately narrower than IsErrorResponse, which decides what to
// report to the client.
func IsDependencyFailure(statusCode int) bool {
	return statusCode >= 500
}

const (
	// FailureScopeHeader carries how far a failure reaches. alt-backend stamps
	// it on Connect errors via error metadata, which the unary protocol merges
	// into the response headers, so it arrives here as an ordinary header and
	// travels on to the client untouched.
	//
	// It exists because CodeUnavailable is issued by two parties that mean
	// opposite things by it. This BFF returns it when its own breaker is open
	// against every host; alt-backend returns it when one publisher did not
	// answer. Nothing in the status code separates them.
	FailureScopeHeader = "X-Alt-Failure-Scope"

	// FailureScopeHost means the failure belongs to one third-party publisher.
	FailureScopeHost = "host"

	// FailureScopeGlobal means the failure is a state of this gateway and no
	// host can be routed around it. Only a breaker rejection qualifies.
	FailureScopeGlobal = "global"
)

// IsUpstreamAttributed reports whether the backend named a third-party
// publisher as the party at fault. Such a response says nothing about
// alt-backend's health, so it is neutral for circuit-breaker accounting for
// exactly the reason a 4xx is: charging it opens the class breaker on one dead
// link, and crediting it resets the budget and masks a real outage.
//
// Only the exact host claim counts. The header can excuse a failure from a
// budget but never charge one, so anything absent, unknown, or malformed has
// to read as unattributed and keep counting as ours — a backend that stopped
// stamping must look broken, not healthy.
func IsUpstreamAttributed(header http.Header) bool {
	return header.Get(FailureScopeHeader) == FailureScopeHost
}

// ConnectError is the Connect protocol's unary error body. It is not
// NormalizedError: connect-es resolves the JSON `code` with codeFromString,
// which accepts only the protocol's lowercase snake_case names, so
// NormalizedError's SCREAMING_SNAKE codes resolve to undefined and the client
// silently falls back to the HTTP-status-derived code — discarding every
// non-standard field with it. Retry guidance therefore travels in the
// Retry-After header, which connect-es surfaces as ConnectError.metadata.
type ConnectError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Connect error codes (lowercase snake_case, as they appear on the wire).
const (
	ConnectCodeUnavailable = "unavailable"
)

// ToJSON serializes the Connect error body.
func (e *ConnectError) ToJSON() ([]byte, error) {
	return json.Marshal(e)
}
