package errors

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

func TestAppContextError_Error(t *testing.T) {
	tests := []struct {
		name           string
		appContextError *AppContextError
		want           string
	}{
		{
			name: "error with cause and full context",
			appContextError: &AppContextError{
				Code:      "DATABASE_ERROR",
				Message:   "failed to fetch feeds",
				Layer:     "gateway",
				Component: "FetchFeedsGateway",
				Operation: "FetchFeedsList",
				Cause:     errors.New("connection timeout"),
				Context: map[string]interface{}{
					"table": "feeds",
					"query": "SELECT * FROM feeds",
				},
			},
			want: "[gateway:FetchFeedsGateway:FetchFeedsList] DATABASE_ERROR: failed to fetch feeds (caused by: connection timeout)",
		},
		{
			name: "error without cause",
			appContextError: &AppContextError{
				Code:      "VALIDATION_ERROR",
				Message:   "invalid input",
				Layer:     "usecase",
				Component: "FetchFeedUsecase",
				Operation: "ValidateInput",
				Cause:     nil,
			},
			want: "[usecase:FetchFeedUsecase:ValidateInput] VALIDATION_ERROR: invalid input",
		},
		{
			name: "error with minimal context",
			appContextError: &AppContextError{
				Code:    "RATE_LIMIT_ERROR",
				Message: "rate limit exceeded",
				Cause:   errors.New("too many requests"),
			},
			want: "RATE_LIMIT_ERROR: rate limit exceeded (caused by: too many requests)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.appContextError.Error()
			if got != tt.want {
				t.Errorf("AppContextError.Error() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAppContextError_Unwrap(t *testing.T) {
	cause := errors.New("underlying error")
	appContextError := &AppContextError{
		Code:      "DATABASE_ERROR",
		Message:   "database operation failed",
		Layer:     "gateway",
		Component: "DatabaseGateway",
		Operation: "Query",
		Cause:     cause,
	}

	got := appContextError.Unwrap()
	if got != cause {
		t.Errorf("AppContextError.Unwrap() = %v, want %v", got, cause)
	}
}

func TestAppContextError_HTTPStatusCode(t *testing.T) {
	tests := []struct {
		name string
		code string
		want int
	}{
		{"validation error", "VALIDATION_ERROR", http.StatusBadRequest},
		{"rate limit error", "RATE_LIMIT_ERROR", http.StatusTooManyRequests},
		{"external API error", "EXTERNAL_API_ERROR", http.StatusBadGateway},
		{"timeout error", "TIMEOUT_ERROR", http.StatusGatewayTimeout},
		{"database error", "DATABASE_ERROR", http.StatusInternalServerError},
		{"unknown error", "UNKNOWN_ERROR", http.StatusInternalServerError},
		{"undefined error", "CUSTOM_ERROR", http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			appErr := &AppContextError{Code: tt.code}
			got := appErr.HTTPStatusCode()
			if got != tt.want {
				t.Errorf("AppContextError.HTTPStatusCode() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAppContextError_ToHTTPResponse(t *testing.T) {
	appErr := &AppContextError{
		Code:      "DATABASE_ERROR",
		Message:   "failed to fetch data",
		Layer:     "gateway",
		Component: "DataGateway",
		Operation: "FetchData",
		Context: map[string]interface{}{
			"table": "feeds",
			"id":    123,
		},
	}

	response := appErr.ToHTTPResponse()

	if response.Error != "error" {
		t.Errorf("ToHTTPResponse().Error = %v, want %v", response.Error, "error")
	}
	if response.Code != "DATABASE_ERROR" {
		t.Errorf("ToHTTPResponse().Code = %v, want %v", response.Code, "DATABASE_ERROR")
	}
	if response.Message != "failed to fetch data" {
		t.Errorf("ToHTTPResponse().Message = %v, want %v", response.Message, "failed to fetch data")
	}
	if response.Layer != "gateway" {
		t.Errorf("ToHTTPResponse().Layer = %v, want %v", response.Layer, "gateway")
	}
	if response.Component != "DataGateway" {
		t.Errorf("ToHTTPResponse().Component = %v, want %v", response.Component, "DataGateway")
	}
	if response.Operation != "FetchData" {
		t.Errorf("ToHTTPResponse().Operation = %v, want %v", response.Operation, "FetchData")
	}
	if response.Context == nil {
		t.Error("ToHTTPResponse().Context should not be nil")
	}
}

func TestNewAppContextError(t *testing.T) {
	ctx := context.Background()
	cause := errors.New("underlying error")
	
	appErr := NewAppContextError(
		"DATABASE_ERROR",
		"operation failed",
		"gateway",
		"TestGateway",
		"TestOperation",
		cause,
		map[string]interface{}{"test": "value"},
	)

	if appErr.Code != "DATABASE_ERROR" {
		t.Errorf("NewAppContextError().Code = %v, want %v", appErr.Code, "DATABASE_ERROR")
	}
	if appErr.Message != "operation failed" {
		t.Errorf("NewAppContextError().Message = %v, want %v", appErr.Message, "operation failed")
	}
	if appErr.Layer != "gateway" {
		t.Errorf("NewAppContextError().Layer = %v, want %v", appErr.Layer, "gateway")
	}
	if appErr.Component != "TestGateway" {
		t.Errorf("NewAppContextError().Component = %v, want %v", appErr.Component, "TestGateway")
	}
	if appErr.Operation != "TestOperation" {
		t.Errorf("NewAppContextError().Operation = %v, want %v", appErr.Operation, "TestOperation")
	}
	if appErr.Cause != cause {
		t.Errorf("NewAppContextError().Cause = %v, want %v", appErr.Cause, cause)
	}
	if appErr.Context == nil {
		t.Error("NewAppContextError().Context should not be nil")
	}

	// Test context value extraction
	_ = ctx // Use ctx if needed for future context-aware features
}

func TestEnrichWithContext(t *testing.T) {
	originalErr := &AppContextError{
		Code:    "DATABASE_ERROR",
		Message: "original error",
		Layer:   "driver",
		Context: map[string]interface{}{"original": "value"},
	}

	enrichedErr := EnrichWithContext(originalErr, "gateway", "TestGateway", "TestOperation", map[string]interface{}{
		"enriched": "value",
	})

	if enrichedErr.Layer != "gateway" {
		t.Errorf("EnrichWithContext().Layer = %v, want %v", enrichedErr.Layer, "gateway")
	}
	if enrichedErr.Component != "TestGateway" {
		t.Errorf("EnrichWithContext().Component = %v, want %v", enrichedErr.Component, "TestGateway")
	}
	if enrichedErr.Operation != "TestOperation" {
		t.Errorf("EnrichWithContext().Operation = %v, want %v", enrichedErr.Operation, "TestOperation")
	}

	// Check that context is merged
	if enrichedErr.Context["original"] != "value" {
		t.Error("EnrichWithContext() should preserve original context")
	}
	if enrichedErr.Context["enriched"] != "value" {
		t.Error("EnrichWithContext() should add new context")
	}
}

func TestAppContextError_IsRetryable(t *testing.T) {
	tests := []struct {
		name string
		code string
		want bool
	}{
		{"rate limit is retryable", "RATE_LIMIT_ERROR", true},
		{"timeout is retryable", "TIMEOUT_ERROR", true},
		{"external API is retryable", "EXTERNAL_API_ERROR", true},
		{"validation is not retryable", "VALIDATION_ERROR", false},
		{"database is not retryable", "DATABASE_ERROR", false},
		{"unknown is not retryable", "UNKNOWN_ERROR", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			appErr := &AppContextError{Code: tt.code}
			got := appErr.IsRetryable()
			if got != tt.want {
				t.Errorf("AppContextError.IsRetryable() = %v, want %v", got, tt.want)
			}
		})
	}
}