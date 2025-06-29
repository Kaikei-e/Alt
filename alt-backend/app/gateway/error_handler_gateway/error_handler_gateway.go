package error_handler_gateway

import (
	"alt/port/error_handler_port"
	"alt/utils/errors"
	"context"
	"log/slog"
)

// ErrorHandlerGateway implements the ErrorHandlerPort interface
type ErrorHandlerGateway struct{}

// NewErrorHandlerGateway creates a new error handler gateway
func NewErrorHandlerGateway() *ErrorHandlerGateway {
	return &ErrorHandlerGateway{}
}

// CreateDatabaseError creates a database error
func (e *ErrorHandlerGateway) CreateDatabaseError(message string, cause error, context map[string]interface{}) error_handler_port.AppError {
	appErr := errors.DatabaseError(message, cause, context)
	return error_handler_port.AppError{
		Code:    error_handler_port.ErrCodeDatabase,
		Message: appErr.Message,
		Cause:   appErr.Cause,
		Context: appErr.Context,
	}
}

// CreateValidationError creates a validation error
func (e *ErrorHandlerGateway) CreateValidationError(message string, context map[string]interface{}) error_handler_port.AppError {
	appErr := errors.ValidationError(message, context)
	return error_handler_port.AppError{
		Code:    error_handler_port.ErrCodeValidation,
		Message: appErr.Message,
		Cause:   appErr.Cause,
		Context: appErr.Context,
	}
}

// CreateRateLimitError creates a rate limit error
func (e *ErrorHandlerGateway) CreateRateLimitError(message string, cause error, context map[string]interface{}) error_handler_port.AppError {
	appErr := errors.RateLimitError(message, cause, context)
	return error_handler_port.AppError{
		Code:    error_handler_port.ErrCodeRateLimit,
		Message: appErr.Message,
		Cause:   appErr.Cause,
		Context: appErr.Context,
	}
}

// CreateExternalAPIError creates an external API error
func (e *ErrorHandlerGateway) CreateExternalAPIError(message string, cause error, context map[string]interface{}) error_handler_port.AppError {
	appErr := errors.ExternalAPIError(message, cause, context)
	return error_handler_port.AppError{
		Code:    error_handler_port.ErrCodeExternalAPI,
		Message: appErr.Message,
		Cause:   appErr.Cause,
		Context: appErr.Context,
	}
}

// CreateTimeoutError creates a timeout error
func (e *ErrorHandlerGateway) CreateTimeoutError(message string, cause error, context map[string]interface{}) error_handler_port.AppError {
	appErr := errors.TimeoutError(message, cause, context)
	return error_handler_port.AppError{
		Code:    error_handler_port.ErrCodeTimeout,
		Message: appErr.Message,
		Cause:   appErr.Cause,
		Context: appErr.Context,
	}
}

// LogError logs an error with structured logging
func (e *ErrorHandlerGateway) LogError(ctx context.Context, logger *slog.Logger, err error, operation string) {
	errors.LogError(logger, err, operation)
}

// HandleError processes and potentially transforms an error
func (e *ErrorHandlerGateway) HandleError(ctx context.Context, err error, operation string) error {
	// For now, just return the error as-is
	// In the future, this could add context transformation or error classification
	return err
}
