// ABOUTME: Tests for AppContextError following alt-backend pattern
// ABOUTME: Verifies error interface, HTTP mapping, retryability, and safe messages
package errors

import (
	"errors"
	"net/http"
	"testing"
)

func TestAppContextError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *AppContextError
		contains []string
	}{
		{
			name: "full context",
			err: NewAppContextError(
				"VALIDATION_ERROR",
				"field is required",
				"handler",
				"SummarizeHandler",
				"HandleSummarize",
				nil,
				nil,
			),
			contains: []string{"handler", "SummarizeHandler", "HandleSummarize", "VALIDATION_ERROR", "field is required"},
		},
		{
			name: "with cause",
			err: NewAppContextError(
				"DATABASE_ERROR",
				"failed to query",
				"repository",
				"ArticleRepository",
				"FindByID",
				errors.New("connection timeout"),
				nil,
			),
			contains: []string{"DATABASE_ERROR", "failed to query", "connection timeout"},
		},
		{
			name: "without layer info",
			err: &AppContextError{
				Code:    "UNKNOWN_ERROR",
				Message: "something went wrong",
			},
			contains: []string{"UNKNOWN_ERROR", "something went wrong"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errStr := tt.err.Error()
			for _, s := range tt.contains {
				if !containsString(errStr, s) {
					t.Errorf("Error() = %q, should contain %q", errStr, s)
				}
			}
		})
	}
}

func TestAppContextError_Unwrap(t *testing.T) {
	cause := errors.New("underlying error")
	err := NewAppContextError(
		"EXTERNAL_API_ERROR",
		"API call failed",
		"gateway",
		"NewsCreator",
		"Summarize",
		cause,
		nil,
	)

	unwrapped := err.Unwrap()
	if unwrapped != cause {
		t.Errorf("Unwrap() = %v, want %v", unwrapped, cause)
	}

	// Verify errors.Is works through the chain
	if !errors.Is(err, cause) {
		t.Error("errors.Is should find the underlying cause")
	}
}

func TestAppContextError_HTTPStatusCode(t *testing.T) {
	tests := []struct {
		code   string
		status int
	}{
		{"VALIDATION_ERROR", http.StatusBadRequest},
		{"NOT_FOUND_ERROR", http.StatusNotFound},
		{"RATE_LIMIT_ERROR", http.StatusTooManyRequests},
		{"EXTERNAL_API_ERROR", http.StatusBadGateway},
		{"TIMEOUT_ERROR", http.StatusGatewayTimeout},
		{"DATABASE_ERROR", http.StatusInternalServerError},
		{"INTERNAL_ERROR", http.StatusInternalServerError},
		{"UNKNOWN_ERROR", http.StatusInternalServerError},
		{"SOME_UNKNOWN_CODE", http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			err := &AppContextError{Code: tt.code}
			if got := err.HTTPStatusCode(); got != tt.status {
				t.Errorf("HTTPStatusCode() = %d, want %d", got, tt.status)
			}
		})
	}
}

func TestAppContextError_IsRetryable(t *testing.T) {
	tests := []struct {
		code      string
		retryable bool
	}{
		{"VALIDATION_ERROR", false},
		{"NOT_FOUND_ERROR", false},
		{"RATE_LIMIT_ERROR", true},
		{"EXTERNAL_API_ERROR", true},
		{"TIMEOUT_ERROR", true},
		{"DATABASE_ERROR", false},
		{"INTERNAL_ERROR", false},
		{"UNKNOWN_ERROR", false},
	}

	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			err := &AppContextError{Code: tt.code}
			if got := err.IsRetryable(); got != tt.retryable {
				t.Errorf("IsRetryable() = %v, want %v", got, tt.retryable)
			}
		})
	}
}

func TestAppContextError_SafeMessage(t *testing.T) {
	tests := []struct {
		name    string
		err     *AppContextError
		want    string
		exactly bool // if true, exact match; if false, just check non-empty
	}{
		{
			name: "validation error exposes message",
			err: &AppContextError{
				Code:    "VALIDATION_ERROR",
				Message: "article ID is required",
			},
			want:    "article ID is required",
			exactly: true,
		},
		{
			name: "not found error exposes message",
			err: &AppContextError{
				Code:    "NOT_FOUND_ERROR",
				Message: "article not found",
			},
			want:    "article not found",
			exactly: true,
		},
		{
			name: "database error hides internal details",
			err: &AppContextError{
				Code:    "DATABASE_ERROR",
				Message: "pq: connection refused to 192.168.1.100:5432",
			},
			want:    "A temporary service error occurred. Please try again later.",
			exactly: true,
		},
		{
			name: "internal error hides details",
			err: &AppContextError{
				Code:    "INTERNAL_ERROR",
				Message: "panic in goroutine: nil pointer dereference",
			},
			want:    "An unexpected error occurred. Please try again later.",
			exactly: true,
		},
		{
			name: "external API error hides details",
			err: &AppContextError{
				Code:    "EXTERNAL_API_ERROR",
				Message: "news-creator: HTTP 500 Internal Server Error",
			},
			want:    "Unable to connect to external service. Please try again.",
			exactly: true,
		},
		{
			name: "timeout error provides safe message",
			err: &AppContextError{
				Code:    "TIMEOUT_ERROR",
				Message: "context deadline exceeded after 30s",
			},
			want:    "The request took too long. Please try again.",
			exactly: true,
		},
		{
			name: "rate limit error provides safe message",
			err: &AppContextError{
				Code:    "RATE_LIMIT_ERROR",
				Message: "rate limit exceeded: 100 requests/min",
			},
			want:    "Too many requests. Please wait before trying again.",
			exactly: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.err.SafeMessage()
			if tt.exactly {
				if got != tt.want {
					t.Errorf("SafeMessage() = %q, want %q", got, tt.want)
				}
			} else {
				if got == "" {
					t.Error("SafeMessage() should not be empty")
				}
			}
		})
	}
}

func TestAppContextError_ToSecureHTTPResponse(t *testing.T) {
	err := NewAppContextError(
		"DATABASE_ERROR",
		"pq: SSL certificate verify failed",
		"repository",
		"ArticleRepository",
		"Create",
		errors.New("x509: certificate signed by unknown authority"),
		map[string]interface{}{"article_id": "123"},
	)

	resp := err.ToSecureHTTPResponse()

	// Code should be preserved
	if resp.Error.Code != "DATABASE_ERROR" {
		t.Errorf("Code = %q, want DATABASE_ERROR", resp.Error.Code)
	}

	// Message should be safe (not contain internal details)
	if containsString(resp.Error.Message, "SSL") {
		t.Error("Message should not contain internal details like 'SSL'")
	}
	if containsString(resp.Error.Message, "x509") {
		t.Error("Message should not contain internal details like 'x509'")
	}
	if containsString(resp.Error.Message, "pq:") {
		t.Error("Message should not contain internal details like 'pq:'")
	}

	// ErrorID should be present
	if resp.Error.ErrorID == "" {
		t.Error("ErrorID should be generated")
	}
}

func TestNewValidationContextError(t *testing.T) {
	err := NewValidationContextError(
		"field X is required",
		"handler",
		"TestHandler",
		"TestOp",
		map[string]interface{}{"field": "X"},
	)

	if err.Code != "VALIDATION_ERROR" {
		t.Errorf("Code = %q, want VALIDATION_ERROR", err.Code)
	}
	if err.HTTPStatusCode() != http.StatusBadRequest {
		t.Errorf("HTTPStatusCode() = %d, want %d", err.HTTPStatusCode(), http.StatusBadRequest)
	}
	if err.IsRetryable() {
		t.Error("Validation error should not be retryable")
	}
}

func TestNewNotFoundContextError(t *testing.T) {
	err := NewNotFoundContextError(
		"article not found",
		"repository",
		"ArticleRepository",
		"FindByID",
		nil,
	)

	if err.Code != "NOT_FOUND_ERROR" {
		t.Errorf("Code = %q, want NOT_FOUND_ERROR", err.Code)
	}
	if err.HTTPStatusCode() != http.StatusNotFound {
		t.Errorf("HTTPStatusCode() = %d, want %d", err.HTTPStatusCode(), http.StatusNotFound)
	}
}

func TestNewInternalContextError(t *testing.T) {
	cause := errors.New("nil pointer")
	err := NewInternalContextError(
		"unexpected error",
		"service",
		"SummarizeService",
		"Process",
		cause,
		nil,
	)

	if err.Code != "INTERNAL_ERROR" {
		t.Errorf("Code = %q, want INTERNAL_ERROR", err.Code)
	}
	if !errors.Is(err, cause) {
		t.Error("Should wrap the cause")
	}
}

func TestNewExternalAPIContextError(t *testing.T) {
	err := NewExternalAPIContextError(
		"news-creator unavailable",
		"gateway",
		"NewsCreatorGateway",
		"Summarize",
		nil,
		nil,
	)

	if err.Code != "EXTERNAL_API_ERROR" {
		t.Errorf("Code = %q, want EXTERNAL_API_ERROR", err.Code)
	}
	if !err.IsRetryable() {
		t.Error("External API error should be retryable")
	}
}

func TestNewTimeoutContextError(t *testing.T) {
	err := NewTimeoutContextError(
		"request timed out",
		"gateway",
		"HTTPClient",
		"Do",
		nil,
		nil,
	)

	if err.Code != "TIMEOUT_ERROR" {
		t.Errorf("Code = %q, want TIMEOUT_ERROR", err.Code)
	}
	if !err.IsRetryable() {
		t.Error("Timeout error should be retryable")
	}
	if err.HTTPStatusCode() != http.StatusGatewayTimeout {
		t.Errorf("HTTPStatusCode() = %d, want %d", err.HTTPStatusCode(), http.StatusGatewayTimeout)
	}
}

func TestGenerateErrorID(t *testing.T) {
	ids := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := generateErrorID()
		if len(id) != 8 {
			t.Errorf("ErrorID length = %d, want 8", len(id))
		}
		if ids[id] {
			t.Errorf("Duplicate ErrorID generated: %s", id)
		}
		ids[id] = true
	}
}

// Helper function
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStringHelper(s, substr))
}

func containsStringHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
