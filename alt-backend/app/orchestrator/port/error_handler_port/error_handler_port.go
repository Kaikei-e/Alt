package error_handler_port

import (
	"context"
	"log/slog"
)

//go:generate go run go.uber.org/mock/mockgen -source=error_handler_port.go -destination=../../mocks/mock_error_handler_port.go

// ErrorCode represents the type of error that occurred
type ErrorCode string

const (
	ErrCodeDatabase    ErrorCode = "DATABASE_ERROR"
	ErrCodeValidation  ErrorCode = "VALIDATION_ERROR"
	ErrCodeRateLimit   ErrorCode = "RATE_LIMIT_ERROR"
	ErrCodeExternalAPI ErrorCode = "EXTERNAL_API_ERROR"
	ErrCodeTimeout     ErrorCode = "TIMEOUT_ERROR"
	ErrCodeUnknown     ErrorCode = "UNKNOWN_ERROR"
)

// AppError represents a structured application error
type AppError struct {
	Code    ErrorCode
	Message string
	Cause   error
	Context map[string]interface{}
}

// ErrorHandlerPort defines the interface for error handling operations
type ErrorHandlerPort interface {
	// Error creation methods
	CreateDatabaseError(message string, cause error, context map[string]interface{}) AppError
	CreateValidationError(message string, context map[string]interface{}) AppError
	CreateRateLimitError(message string, cause error, context map[string]interface{}) AppError
	CreateExternalAPIError(message string, cause error, context map[string]interface{}) AppError
	CreateTimeoutError(message string, cause error, context map[string]interface{}) AppError

	// Error logging and handling
	LogError(ctx context.Context, logger *slog.Logger, err error, operation string)
	HandleError(ctx context.Context, err error, operation string) error
}
