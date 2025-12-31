// Package errors provides structured error handling for the Alt backend application.
// It defines error types with codes, messages, causes, and contextual information
// to facilitate debugging and error tracking across the application layers.
package errors

import (
	"fmt"
	"log/slog"
)

// ErrorCode represents a categorized error type for structured error handling.
type ErrorCode string

// Error code constants for categorizing application errors.
const (
	ErrCodeDatabase       ErrorCode = "DATABASE_ERROR"
	ErrCodeValidation     ErrorCode = "VALIDATION_ERROR"
	ErrCodeRateLimit      ErrorCode = "RATE_LIMIT_ERROR"
	ErrCodeExternalAPI    ErrorCode = "EXTERNAL_API_ERROR"
	ErrCodeTimeout        ErrorCode = "TIMEOUT_ERROR"
	ErrCodeTLSCertificate ErrorCode = "TLS_CERTIFICATE_ERROR"
	ErrCodeUnknown        ErrorCode = "UNKNOWN_ERROR"
)

// AppError represents a structured application error with code, message, cause, and context.
// It implements the error interface and supports error unwrapping.
type AppError struct {
	Code    ErrorCode
	Message string
	Cause   error
	Context map[string]interface{}
}

// Error returns a string representation of the AppError.
func (e *AppError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s (caused by: %v)", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap returns the underlying cause error for use with errors.Is and errors.As.
func (e *AppError) Unwrap() error {
	return e.Cause
}

// DatabaseError creates an AppError for database-related errors.
func DatabaseError(message string, cause error, context map[string]interface{}) *AppError {
	return &AppError{
		Code:    ErrCodeDatabase,
		Message: message,
		Cause:   cause,
		Context: context,
	}
}

// ValidationError creates an AppError for input validation failures.
func ValidationError(message string, context map[string]interface{}) *AppError {
	return &AppError{
		Code:    ErrCodeValidation,
		Message: message,
		Context: context,
	}
}

// RateLimitError creates an AppError for rate limiting violations.
func RateLimitError(message string, cause error, context map[string]interface{}) *AppError {
	return &AppError{
		Code:    ErrCodeRateLimit,
		Message: message,
		Cause:   cause,
		Context: context,
	}
}

// ExternalAPIError creates an AppError for external API call failures.
func ExternalAPIError(message string, cause error, context map[string]interface{}) *AppError {
	return &AppError{
		Code:    ErrCodeExternalAPI,
		Message: message,
		Cause:   cause,
		Context: context,
	}
}

// TimeoutError creates an AppError for timeout-related errors.
func TimeoutError(message string, cause error, context map[string]interface{}) *AppError {
	return &AppError{
		Code:    ErrCodeTimeout,
		Message: message,
		Cause:   cause,
		Context: context,
	}
}

// TLSCertificateError creates an AppError for TLS certificate validation failures.
func TLSCertificateError(message string, cause error, context map[string]interface{}) *AppError {
	return &AppError{
		Code:    ErrCodeTLSCertificate,
		Message: message,
		Cause:   cause,
		Context: context,
	}
}

// UnknownError creates an AppError for unclassified errors.
func UnknownError(message string, cause error, context map[string]interface{}) *AppError {
	return &AppError{
		Code:    ErrCodeUnknown,
		Message: message,
		Cause:   cause,
		Context: context,
	}
}

// LogError logs an AppError with structured logging and context
func LogError(logger *slog.Logger, err error, operation string) {
	// Handle nil logger gracefully (e.g., during tests)
	if logger == nil {
		return
	}

	if appErr, ok := err.(*AppError); ok {
		args := []interface{}{
			"operation", operation,
			"error_code", string(appErr.Code),
			"error_message", appErr.Message,
		}

		// Add context attributes
		if appErr.Context != nil {
			for key, value := range appErr.Context {
				args = append(args, key, value)
			}
		}

		// Add cause if present
		if appErr.Cause != nil {
			args = append(args, "cause", appErr.Cause.Error())
		}

		logger.Error("application error occurred", args...)
	} else {
		logger.Error("unknown error occurred",
			"operation", operation,
			"error", err.Error(),
		)
	}
}
