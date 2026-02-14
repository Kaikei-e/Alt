package errorhandler

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"connectrpc.com/connect"

	appErrors "alt/utils/errors"
)

func TestHandleInternalError_ReturnsErrorWithMessage(t *testing.T) {
	ctx := context.Background()
	logger := slog.Default()
	originalErr := errors.New("database connection failed")

	connectErr := HandleInternalError(ctx, logger, originalErr, "TestOperation")

	if connectErr == nil {
		t.Fatal("expected connect error, got nil")
	}

	if connectErr.Code() != connect.CodeInternal {
		t.Errorf("expected CodeInternal, got %v", connectErr.Code())
	}

	// Should contain a safe message, not the original error
	msg := connectErr.Message()
	if msg == "" {
		t.Error("expected non-empty message")
	}

	// Should NOT contain the original error message (security)
	if strings.Contains(msg, "database connection failed") {
		t.Error("error message should not contain internal details")
	}

	// Should contain "Error ID:" for traceability
	if !strings.Contains(msg, "Error ID:") {
		t.Error("error message should contain Error ID for traceability")
	}
}

func TestHandleInternalError_WithAppContextError(t *testing.T) {
	ctx := context.Background()
	logger := slog.Default()
	appErr := appErrors.NewDatabaseContextError(
		"failed to query users",
		"connect",
		"TestHandler",
		"TestOperation",
		errors.New("sql: connection refused"),
		nil,
	)

	connectErr := HandleInternalError(ctx, logger, appErr, "TestOperation")

	if connectErr == nil {
		t.Fatal("expected connect error, got nil")
	}

	msg := connectErr.Message()

	// Should return safe message for DATABASE_ERROR
	if !strings.Contains(msg, "temporary service error") {
		t.Errorf("expected safe database error message, got: %s", msg)
	}

	// Should NOT contain the original error details
	if strings.Contains(msg, "sql: connection refused") {
		t.Error("error message should not contain internal details")
	}
}

func TestHandleConnectError_DifferentCodes(t *testing.T) {
	ctx := context.Background()
	logger := slog.Default()
	originalErr := errors.New("some error")

	tests := []struct {
		name     string
		code     connect.Code
		expected connect.Code
	}{
		{"Internal", connect.CodeInternal, connect.CodeInternal},
		{"InvalidArgument", connect.CodeInvalidArgument, connect.CodeInvalidArgument},
		{"NotFound", connect.CodeNotFound, connect.CodeNotFound},
		{"Unauthenticated", connect.CodeUnauthenticated, connect.CodeUnauthenticated},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			connectErr := HandleConnectError(ctx, logger, originalErr, tt.code, "TestOperation")

			if connectErr.Code() != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, connectErr.Code())
			}
		})
	}
}

func TestHandleValidationError_ReturnsValidationMessage(t *testing.T) {
	ctx := context.Background()
	logger := slog.Default()

	connectErr := HandleValidationError(ctx, logger, "email is required", "CreateUser")

	if connectErr == nil {
		t.Fatal("expected connect error, got nil")
	}

	if connectErr.Code() != connect.CodeInvalidArgument {
		t.Errorf("expected CodeInvalidArgument, got %v", connectErr.Code())
	}

	// Validation errors should return the original message (they're designed to be safe)
	msg := connectErr.Message()
	if !strings.Contains(msg, "email is required") {
		t.Errorf("expected validation message to be preserved, got: %s", msg)
	}
}

func TestHandleNotFoundError_ReturnsNotFoundMessage(t *testing.T) {
	ctx := context.Background()
	logger := slog.Default()

	connectErr := HandleNotFoundError(ctx, logger, "user not found", "GetUser")

	if connectErr == nil {
		t.Fatal("expected connect error, got nil")
	}

	if connectErr.Code() != connect.CodeNotFound {
		t.Errorf("expected CodeNotFound, got %v", connectErr.Code())
	}
}

func TestHandleInternalError_DoesNotLeakSensitiveInfo(t *testing.T) {
	ctx := context.Background()
	logger := slog.Default()

	sensitiveErrors := []error{
		errors.New("connection to postgres://user:password@db:5432 failed"),
		errors.New("failed to read /var/lib/app/secrets/api_key.txt"),
		errors.New("SMTP auth failed: invalid credentials for smtp.example.com"),
		errors.New("Redis connection to 10.0.0.5:6379 timed out"),
	}

	for _, sensitiveErr := range sensitiveErrors {
		connectErr := HandleInternalError(ctx, logger, sensitiveErr, "TestOperation")
		msg := connectErr.Message()

		// Should not contain any of the sensitive info
		if strings.Contains(msg, "postgres") ||
			strings.Contains(msg, "password") ||
			strings.Contains(msg, "/var/lib") ||
			strings.Contains(msg, "api_key") ||
			strings.Contains(msg, "smtp") ||
			strings.Contains(msg, "credentials") ||
			strings.Contains(msg, "10.0.0.5") ||
			strings.Contains(msg, "6379") {
			t.Errorf("error message leaked sensitive info: %s", msg)
		}
	}
}

func TestHandleInternalError_SearchServiceUnavailable(t *testing.T) {
	ctx := context.Background()
	logger := slog.Default()

	connectErr := HandleInternalError(ctx, logger, appErrors.ErrSearchServiceUnavailable, "SearchFeeds")

	if connectErr == nil {
		t.Fatal("expected connect error, got nil")
	}

	if connectErr.Code() != connect.CodeInternal {
		t.Errorf("expected CodeInternal, got %v", connectErr.Code())
	}

	msg := connectErr.Message()

	// Should return a user-friendly message for external API errors
	if !strings.Contains(msg, "external service") {
		t.Errorf("expected external service error message, got: %s", msg)
	}

	// Should contain Error ID for traceability
	if !strings.Contains(msg, "Error ID:") {
		t.Error("error message should contain Error ID for traceability")
	}

	// Should NOT contain internal error details
	if strings.Contains(msg, "search service unavailable") {
		t.Error("error message should not contain internal error details")
	}
}

func TestHandleInternalError_SearchTimeout(t *testing.T) {
	ctx := context.Background()
	logger := slog.Default()

	connectErr := HandleInternalError(ctx, logger, appErrors.ErrSearchTimeout, "SearchFeeds")

	if connectErr == nil {
		t.Fatal("expected connect error, got nil")
	}

	if connectErr.Code() != connect.CodeInternal {
		t.Errorf("expected CodeInternal, got %v", connectErr.Code())
	}

	msg := connectErr.Message()

	// Should return a user-friendly message for timeout errors
	if !strings.Contains(msg, "too long") {
		t.Errorf("expected timeout error message, got: %s", msg)
	}

	// Should contain Error ID for traceability
	if !strings.Contains(msg, "Error ID:") {
		t.Error("error message should contain Error ID for traceability")
	}
}

func TestClassifyDriverError_SearchErrors(t *testing.T) {
	tests := []struct {
		name         string
		err          error
		expectedCode string
	}{
		{
			name:         "search service unavailable",
			err:          appErrors.ErrSearchServiceUnavailable,
			expectedCode: "EXTERNAL_API_ERROR",
		},
		{
			name:         "search timeout",
			err:          appErrors.ErrSearchTimeout,
			expectedCode: "TIMEOUT_ERROR",
		},
		{
			name:         "unknown error",
			err:          errors.New("some random error"),
			expectedCode: "UNKNOWN_ERROR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifyDriverError(tt.err, "TestOperation")

			if result.Code != tt.expectedCode {
				t.Errorf("expected code %s, got %s", tt.expectedCode, result.Code)
			}
		})
	}
}
