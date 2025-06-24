package errors

import (
	"errors"
	"testing"
)

func TestAppError_Error(t *testing.T) {
	tests := []struct {
		name     string
		appError *AppError
		want     string
	}{
		{
			name: "error with cause",
			appError: &AppError{
				Code:    ErrCodeDatabase,
				Message: "failed to insert record",
				Cause:   errors.New("connection timeout"),
			},
			want: "DATABASE_ERROR: failed to insert record (caused by: connection timeout)",
		},
		{
			name: "error without cause",
			appError: &AppError{
				Code:    ErrCodeValidation,
				Message: "invalid input",
				Cause:   nil,
			},
			want: "VALIDATION_ERROR: invalid input",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.appError.Error()
			if got != tt.want {
				t.Errorf("AppError.Error() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAppError_Unwrap(t *testing.T) {
	cause := errors.New("underlying error")
	appError := &AppError{
		Code:    ErrCodeDatabase,
		Message: "database operation failed",
		Cause:   cause,
	}

	got := appError.Unwrap()
	if got != cause {
		t.Errorf("AppError.Unwrap() = %v, want %v", got, cause)
	}
}

func TestDatabaseError(t *testing.T) {
	cause := errors.New("connection lost")
	context := map[string]interface{}{
		"table": "feeds",
		"query": "INSERT INTO feeds...",
	}

	appError := DatabaseError("failed to insert feed", cause, context)

	if appError.Code != ErrCodeDatabase {
		t.Errorf("DatabaseError() Code = %v, want %v", appError.Code, ErrCodeDatabase)
	}
	if appError.Message != "failed to insert feed" {
		t.Errorf("DatabaseError() Message = %v, want %v", appError.Message, "failed to insert feed")
	}
	if appError.Cause != cause {
		t.Errorf("DatabaseError() Cause = %v, want %v", appError.Cause, cause)
	}
	if appError.Context == nil {
		t.Error("DatabaseError() Context should not be nil")
	}
	if len(appError.Context) != 2 {
		t.Errorf("DatabaseError() Context should have 2 entries, got %d", len(appError.Context))
	}
}

func TestValidationError(t *testing.T) {
	context := map[string]interface{}{
		"field": "url",
		"value": "invalid-url",
	}

	appError := ValidationError("invalid URL format", context)

	if appError.Code != ErrCodeValidation {
		t.Errorf("ValidationError() Code = %v, want %v", appError.Code, ErrCodeValidation)
	}
	if appError.Message != "invalid URL format" {
		t.Errorf("ValidationError() Message = %v, want %v", appError.Message, "invalid URL format")
	}
	if appError.Cause != nil {
		t.Errorf("ValidationError() Cause = %v, want nil", appError.Cause)
	}
	if appError.Context == nil {
		t.Error("ValidationError() Context should not be nil")
	}
}

func TestRateLimitError(t *testing.T) {
	cause := errors.New("rate limit exceeded")
	context := map[string]interface{}{
		"host":      "example.com",
		"remaining": 0,
	}

	appError := RateLimitError("rate limit exceeded for host", cause, context)

	if appError.Code != ErrCodeRateLimit {
		t.Errorf("RateLimitError() Code = %v, want %v", appError.Code, ErrCodeRateLimit)
	}
	if appError.Message != "rate limit exceeded for host" {
		t.Errorf("RateLimitError() Message = %v, want %v", appError.Message, "rate limit exceeded for host")
	}
	if appError.Cause != cause {
		t.Errorf("RateLimitError() Cause = %v, want %v", appError.Cause, cause)
	}
	if appError.Context == nil {
		t.Error("RateLimitError() Context should not be nil")
	}
}

func TestExternalAPIError(t *testing.T) {
	cause := errors.New("HTTP 503 Service Unavailable")
	context := map[string]interface{}{
		"url":         "https://example.com/feed.xml",
		"status_code": 503,
	}

	appError := ExternalAPIError("failed to fetch feed", cause, context)

	if appError.Code != ErrCodeExternalAPI {
		t.Errorf("ExternalAPIError() Code = %v, want %v", appError.Code, ErrCodeExternalAPI)
	}
	if appError.Message != "failed to fetch feed" {
		t.Errorf("ExternalAPIError() Message = %v, want %v", appError.Message, "failed to fetch feed")
	}
	if appError.Cause != cause {
		t.Errorf("ExternalAPIError() Cause = %v, want %v", appError.Cause, cause)
	}
}

func TestTimeoutError(t *testing.T) {
	cause := errors.New("context deadline exceeded")
	context := map[string]interface{}{
		"timeout": "30s",
		"operation": "fetch_feed",
	}

	appError := TimeoutError("operation timed out", cause, context)

	if appError.Code != ErrCodeTimeout {
		t.Errorf("TimeoutError() Code = %v, want %v", appError.Code, ErrCodeTimeout)
	}
	if appError.Message != "operation timed out" {
		t.Errorf("TimeoutError() Message = %v, want %v", appError.Message, "operation timed out")
	}
	if appError.Cause != cause {
		t.Errorf("TimeoutError() Cause = %v, want %v", appError.Cause, cause)
	}
}

func TestUnknownError(t *testing.T) {
	cause := errors.New("unexpected error")
	context := map[string]interface{}{
		"component": "feed_processor",
	}

	appError := UnknownError("unexpected error occurred", cause, context)

	if appError.Code != ErrCodeUnknown {
		t.Errorf("UnknownError() Code = %v, want %v", appError.Code, ErrCodeUnknown)
	}
}