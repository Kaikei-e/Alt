// ABOUTME: This file tests error classification for retry decisions
// ABOUTME: Tests temporary vs permanent error categorization
package service

import (
	"context"
	"errors"
	"net"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TDD RED PHASE: Test error classification logic
func TestIsRetryableError(t *testing.T) {
	tests := map[string]struct {
		err    error
		expect bool
	}{
		"nil error":               {nil, false},
		"context cancelled":       {context.Canceled, false},
		"context timeout":         {context.DeadlineExceeded, true},
		"500 server error":        {&HTTPError{StatusCode: 500, Message: "Internal Server Error"}, true},
		"502 bad gateway":         {&HTTPError{StatusCode: 502, Message: "Bad Gateway"}, true},
		"503 service unavailable": {&HTTPError{StatusCode: 503, Message: "Service Unavailable"}, true},
		"504 gateway timeout":     {&HTTPError{StatusCode: 504, Message: "Gateway Timeout"}, true},
		"408 request timeout":     {&HTTPError{StatusCode: 408, Message: "Request Timeout"}, true},
		"429 rate limit":          {&HTTPError{StatusCode: 429, Message: "Too Many Requests"}, true},
		"404 not found":           {&HTTPError{StatusCode: 404, Message: "Not Found"}, false},
		"400 bad request":         {&HTTPError{StatusCode: 400, Message: "Bad Request"}, false},
		"401 unauthorized":        {&HTTPError{StatusCode: 401, Message: "Unauthorized"}, false},
		"403 forbidden":           {&HTTPError{StatusCode: 403, Message: "Forbidden"}, false},
		"connection refused":      {&net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}, true},
		"connection reset":        {&net.OpError{Op: "read", Net: "tcp", Err: syscall.ECONNRESET}, true},
		"timeout error":           {&net.OpError{Op: "dial", Net: "tcp", Err: syscall.ETIMEDOUT}, true},
		"generic error":           {errors.New("generic error"), false},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			result := IsRetryableError(tc.err)
			assert.Equal(t, tc.expect, result, "Error classification mismatch for %s", name)
		})
	}
}

// TDD RED PHASE: Test HTTP status code retryability
func TestIsRetryableHTTPStatus(t *testing.T) {
	tests := map[string]struct {
		status int
		expect bool
	}{
		"200 OK":                         {200, false},
		"201 Created":                    {201, false},
		"400 Bad Request":                {400, false},
		"401 Unauthorized":               {401, false},
		"403 Forbidden":                  {403, false},
		"404 Not Found":                  {404, false},
		"408 Request Timeout":            {408, true},
		"429 Too Many Requests":          {429, true},
		"500 Internal Server Error":      {500, true},
		"501 Not Implemented":            {501, true},
		"502 Bad Gateway":                {502, true},
		"503 Service Unavailable":        {503, true},
		"504 Gateway Timeout":            {504, true},
		"505 HTTP Version Not Supported": {505, true},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			result := isRetryableHTTPStatus(tc.status)
			assert.Equal(t, tc.expect, result, "HTTP status retryability mismatch for %s", name)
		})
	}
}

// TDD RED PHASE: Test HTTPError creation and methods
func TestHTTPError(t *testing.T) {
	t.Run("should create HTTPError with status and message", func(t *testing.T) {
		err := &HTTPError{
			StatusCode: 500,
			Message:    "Internal Server Error",
		}

		assert.Equal(t, 500, err.StatusCode)
		assert.Equal(t, "Internal Server Error", err.Message)
		assert.Equal(t, "HTTP 500: Internal Server Error", err.Error())
	})
}

// TDD RED PHASE: Test error extraction from wrapped errors
func TestExtractHTTPError(t *testing.T) {
	t.Run("should extract direct HTTPError", func(t *testing.T) {
		originalErr := &HTTPError{StatusCode: 404, Message: "Not Found"}

		extracted := extractHTTPError(originalErr)

		assert.NotNil(t, extracted)
		assert.Equal(t, 404, extracted.StatusCode)
		assert.Equal(t, "Not Found", extracted.Message)
	})

	t.Run("should return nil for non-HTTP errors", func(t *testing.T) {
		genericErr := errors.New("generic error")

		extracted := extractHTTPError(genericErr)

		assert.Nil(t, extracted)
	})
}

// TDD RED PHASE: Test network error handling
func TestNetworkErrorClassification(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		expect bool
	}{
		{
			name:   "connection refused should be retryable",
			err:    &net.OpError{Err: syscall.ECONNREFUSED},
			expect: true,
		},
		{
			name:   "connection reset should be retryable",
			err:    &net.OpError{Err: syscall.ECONNRESET},
			expect: true,
		},
		{
			name:   "timeout should be retryable",
			err:    &net.OpError{Err: syscall.ETIMEDOUT},
			expect: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := IsRetryableError(tc.err)
			assert.Equal(t, tc.expect, result)
		})
	}
}
