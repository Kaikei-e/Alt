package handler

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeError_502BadGateway(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusBadGateway,
		Header:     make(http.Header),
	}

	normalized := NormalizeError(resp, "test-request-id")

	assert.Equal(t, "BACKEND_UNAVAILABLE", normalized.Code)
	assert.Equal(t, "Backend service is temporarily unavailable", normalized.Message)
	assert.True(t, normalized.IsRetryable)
	assert.Equal(t, 5, normalized.RetryAfter)
	assert.Equal(t, "test-request-id", normalized.RequestID)
}

func TestNormalizeError_503ServiceUnavailable(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusServiceUnavailable,
		Header:     make(http.Header),
	}

	normalized := NormalizeError(resp, "test-request-id")

	assert.Equal(t, "SERVICE_UNAVAILABLE", normalized.Code)
	assert.Equal(t, "Service is temporarily unavailable", normalized.Message)
	assert.True(t, normalized.IsRetryable)
	assert.Equal(t, 10, normalized.RetryAfter)
}

func TestNormalizeError_503WithRetryAfterHeader(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusServiceUnavailable,
		Header:     make(http.Header),
	}
	resp.Header.Set("Retry-After", "30")

	normalized := NormalizeError(resp, "test-request-id")

	assert.Equal(t, "SERVICE_UNAVAILABLE", normalized.Code)
	assert.True(t, normalized.IsRetryable)
	assert.Equal(t, 30, normalized.RetryAfter)
}

func TestNormalizeError_429TooManyRequests(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     make(http.Header),
	}

	normalized := NormalizeError(resp, "test-request-id")

	assert.Equal(t, "RATE_LIMIT_EXCEEDED", normalized.Code)
	assert.Equal(t, "Rate limit exceeded, please slow down", normalized.Message)
	assert.True(t, normalized.IsRetryable)
	assert.Equal(t, 60, normalized.RetryAfter)
}

func TestNormalizeError_429WithRetryAfterHeader(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     make(http.Header),
	}
	resp.Header.Set("Retry-After", "120")

	normalized := NormalizeError(resp, "test-request-id")

	assert.Equal(t, "RATE_LIMIT_EXCEEDED", normalized.Code)
	assert.True(t, normalized.IsRetryable)
	assert.Equal(t, 120, normalized.RetryAfter)
}

func TestNormalizeError_401Unauthorized(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusUnauthorized,
		Header:     make(http.Header),
	}

	normalized := NormalizeError(resp, "test-request-id")

	assert.Equal(t, "INVALID_TOKEN", normalized.Code)
	assert.Equal(t, "Authentication token is invalid or expired", normalized.Message)
	assert.False(t, normalized.IsRetryable)
	assert.Equal(t, 0, normalized.RetryAfter)
}

func TestNormalizeError_403Forbidden(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusForbidden,
		Header:     make(http.Header),
	}

	normalized := NormalizeError(resp, "test-request-id")

	assert.Equal(t, "ACCESS_DENIED", normalized.Code)
	assert.Equal(t, "Access to this resource is denied", normalized.Message)
	assert.False(t, normalized.IsRetryable)
	assert.Equal(t, 0, normalized.RetryAfter)
}

func TestNormalizeError_500InternalServerError(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusInternalServerError,
		Header:     make(http.Header),
	}

	normalized := NormalizeError(resp, "test-request-id")

	assert.Equal(t, "INTERNAL_ERROR", normalized.Code)
	assert.Equal(t, "An internal error occurred", normalized.Message)
	assert.True(t, normalized.IsRetryable)
	assert.Equal(t, 5, normalized.RetryAfter)
}

func TestNormalizeError_504GatewayTimeout(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusGatewayTimeout,
		Header:     make(http.Header),
	}

	normalized := NormalizeError(resp, "test-request-id")

	assert.Equal(t, "GATEWAY_TIMEOUT", normalized.Code)
	assert.Equal(t, "Backend service timed out", normalized.Message)
	assert.True(t, normalized.IsRetryable)
	assert.Equal(t, 10, normalized.RetryAfter)
}

func TestNormalizeError_400BadRequest(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     make(http.Header),
	}

	normalized := NormalizeError(resp, "test-request-id")

	assert.Equal(t, "BAD_REQUEST", normalized.Code)
	assert.Equal(t, "The request was malformed or invalid", normalized.Message)
	assert.False(t, normalized.IsRetryable)
	assert.Equal(t, 0, normalized.RetryAfter)
}

func TestNormalizeError_404NotFound(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusNotFound,
		Header:     make(http.Header),
	}

	normalized := NormalizeError(resp, "test-request-id")

	assert.Equal(t, "NOT_FOUND", normalized.Code)
	assert.Equal(t, "The requested resource was not found", normalized.Message)
	assert.False(t, normalized.IsRetryable)
	assert.Equal(t, 0, normalized.RetryAfter)
}

func TestNormalizeError_UnknownStatusCode(t *testing.T) {
	resp := &http.Response{
		StatusCode: 418, // I'm a teapot
		Header:     make(http.Header),
	}

	normalized := NormalizeError(resp, "test-request-id")

	assert.Equal(t, "UNKNOWN_ERROR", normalized.Code)
	assert.Equal(t, "An unexpected error occurred", normalized.Message)
	assert.False(t, normalized.IsRetryable)
	assert.Equal(t, 0, normalized.RetryAfter)
}

func TestNormalizeNetworkError(t *testing.T) {
	normalized := NormalizeNetworkError("connection refused", "test-request-id")

	assert.Equal(t, "NETWORK_ERROR", normalized.Code)
	assert.Equal(t, "Unable to connect to backend service", normalized.Message)
	assert.True(t, normalized.IsRetryable)
	assert.Equal(t, 5, normalized.RetryAfter)
	assert.Equal(t, "test-request-id", normalized.RequestID)
}

func TestNormalizedError_ToJSON(t *testing.T) {
	normalized := &NormalizedError{
		Code:        "BACKEND_UNAVAILABLE",
		Message:     "Backend service is temporarily unavailable",
		IsRetryable: true,
		RetryAfter:  5,
		RequestID:   "test-request-id",
	}

	jsonBytes, err := normalized.ToJSON()

	assert.NoError(t, err)
	assert.Contains(t, string(jsonBytes), `"code":"BACKEND_UNAVAILABLE"`)
	assert.Contains(t, string(jsonBytes), `"message":"Backend service is temporarily unavailable"`)
	assert.Contains(t, string(jsonBytes), `"is_retryable":true`)
	assert.Contains(t, string(jsonBytes), `"retry_after":5`)
	assert.Contains(t, string(jsonBytes), `"request_id":"test-request-id"`)
}

func TestNormalizedError_ToJSON_OmitsZeroRetryAfter(t *testing.T) {
	normalized := &NormalizedError{
		Code:        "INVALID_TOKEN",
		Message:     "Authentication token is invalid or expired",
		IsRetryable: false,
		RetryAfter:  0,
		RequestID:   "test-request-id",
	}

	jsonBytes, err := normalized.ToJSON()

	assert.NoError(t, err)
	assert.NotContains(t, string(jsonBytes), `"retry_after"`)
}

func TestIsErrorResponse(t *testing.T) {
	tests := []struct {
		statusCode int
		expected   bool
	}{
		{200, false},
		{201, false},
		{204, false},
		{301, false},
		{302, false},
		{400, true},
		{401, true},
		{403, true},
		{404, true},
		{429, true},
		{500, true},
		{502, true},
		{503, true},
		{504, true},
	}

	for _, tt := range tests {
		t.Run(http.StatusText(tt.statusCode), func(t *testing.T) {
			assert.Equal(t, tt.expected, IsErrorResponse(tt.statusCode))
		})
	}
}
