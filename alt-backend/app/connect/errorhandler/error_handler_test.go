package errorhandler

import (
	"errors"
	"log/slog"
	"strings"
	"testing"

	"connectrpc.com/connect"

	appErrors "alt/utils/errors"
)

func TestHandleInternalError_ReturnsErrorWithMessage(t *testing.T) {
	logger := slog.Default()
	originalErr := errors.New("database connection failed")

	connectErr := HandleInternalError(logger, originalErr, "TestOperation")

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
	logger := slog.Default()
	appErr := appErrors.NewDatabaseContextError(
		"failed to query users",
		"connect",
		"TestHandler",
		"TestOperation",
		errors.New("sql: connection refused"),
		nil,
	)

	connectErr := HandleInternalError(logger, appErr, "TestOperation")

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
			connectErr := HandleConnectError(logger, originalErr, tt.code, "TestOperation")

			if connectErr.Code() != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, connectErr.Code())
			}
		})
	}
}

func TestHandleValidationError_ReturnsValidationMessage(t *testing.T) {
	logger := slog.Default()

	connectErr := HandleValidationError(logger, "email is required", "CreateUser")

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
	logger := slog.Default()

	connectErr := HandleNotFoundError(logger, "user not found", "GetUser")

	if connectErr == nil {
		t.Fatal("expected connect error, got nil")
	}

	if connectErr.Code() != connect.CodeNotFound {
		t.Errorf("expected CodeNotFound, got %v", connectErr.Code())
	}
}

func TestHandleInternalError_DoesNotLeakSensitiveInfo(t *testing.T) {
	logger := slog.Default()

	sensitiveErrors := []error{
		errors.New("connection to postgres://user:password@db:5432 failed"),
		errors.New("failed to read /var/lib/app/secrets/api_key.txt"),
		errors.New("SMTP auth failed: invalid credentials for smtp.example.com"),
		errors.New("Redis connection to 10.0.0.5:6379 timed out"),
	}

	for _, sensitiveErr := range sensitiveErrors {
		connectErr := HandleInternalError(logger, sensitiveErr, "TestOperation")
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
