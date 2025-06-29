package error_handler_port

import (
	"context"
	"log/slog"
	"testing"
)

// TestErrorHandlerPortInterface verifies the interface is properly defined
func TestErrorHandlerPortInterface(t *testing.T) {
	// This test ensures the ErrorHandlerPort interface compiles correctly
	// Actual implementation testing will be done in the gateway layer
	var _ ErrorHandlerPort = (*mockErrorHandlerPort)(nil)
}

// mockErrorHandlerPort is a simple mock to verify interface compliance
type mockErrorHandlerPort struct{}

func (m *mockErrorHandlerPort) CreateDatabaseError(message string, cause error, context map[string]interface{}) AppError {
	return AppError{
		Code:    "DATABASE_ERROR",
		Message: message,
		Cause:   cause,
		Context: context,
	}
}

func (m *mockErrorHandlerPort) CreateValidationError(message string, context map[string]interface{}) AppError {
	return AppError{
		Code:    "VALIDATION_ERROR",
		Message: message,
		Context: context,
	}
}

func (m *mockErrorHandlerPort) CreateRateLimitError(message string, cause error, context map[string]interface{}) AppError {
	return AppError{
		Code:    "RATE_LIMIT_ERROR",
		Message: message,
		Cause:   cause,
		Context: context,
	}
}

func (m *mockErrorHandlerPort) CreateExternalAPIError(message string, cause error, context map[string]interface{}) AppError {
	return AppError{
		Code:    "EXTERNAL_API_ERROR",
		Message: message,
		Cause:   cause,
		Context: context,
	}
}

func (m *mockErrorHandlerPort) CreateTimeoutError(message string, cause error, context map[string]interface{}) AppError {
	return AppError{
		Code:    "TIMEOUT_ERROR",
		Message: message,
		Cause:   cause,
		Context: context,
	}
}

func (m *mockErrorHandlerPort) LogError(ctx context.Context, logger *slog.Logger, err error, operation string) {
	// Mock implementation
}

func (m *mockErrorHandlerPort) HandleError(ctx context.Context, err error, operation string) error {
	return err
}
