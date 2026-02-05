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
