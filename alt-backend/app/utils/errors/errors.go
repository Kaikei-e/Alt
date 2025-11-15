package errors

import (
	"fmt"
	"log/slog"
)

type ErrorCode string

const (
	ErrCodeDatabase       ErrorCode = "DATABASE_ERROR"
	ErrCodeValidation     ErrorCode = "VALIDATION_ERROR"
	ErrCodeRateLimit      ErrorCode = "RATE_LIMIT_ERROR"
	ErrCodeExternalAPI    ErrorCode = "EXTERNAL_API_ERROR"
	ErrCodeTimeout        ErrorCode = "TIMEOUT_ERROR"
	ErrCodeTLSCertificate ErrorCode = "TLS_CERTIFICATE_ERROR"
	ErrCodeUnknown        ErrorCode = "UNKNOWN_ERROR"
)

type AppError struct {
	Code    ErrorCode
	Message string
	Cause   error
	Context map[string]interface{}
}

func (e *AppError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s (caused by: %v)", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *AppError) Unwrap() error {
	return e.Cause
}

// Helper functions for common error patterns
func DatabaseError(message string, cause error, context map[string]interface{}) *AppError {
	return &AppError{
		Code:    ErrCodeDatabase,
		Message: message,
		Cause:   cause,
		Context: context,
	}
}

func ValidationError(message string, context map[string]interface{}) *AppError {
	return &AppError{
		Code:    ErrCodeValidation,
		Message: message,
		Context: context,
	}
}

func RateLimitError(message string, cause error, context map[string]interface{}) *AppError {
	return &AppError{
		Code:    ErrCodeRateLimit,
		Message: message,
		Cause:   cause,
		Context: context,
	}
}

func ExternalAPIError(message string, cause error, context map[string]interface{}) *AppError {
	return &AppError{
		Code:    ErrCodeExternalAPI,
		Message: message,
		Cause:   cause,
		Context: context,
	}
}

func TimeoutError(message string, cause error, context map[string]interface{}) *AppError {
	return &AppError{
		Code:    ErrCodeTimeout,
		Message: message,
		Cause:   cause,
		Context: context,
	}
}

func TLSCertificateError(message string, cause error, context map[string]interface{}) *AppError {
	return &AppError{
		Code:    ErrCodeTLSCertificate,
		Message: message,
		Cause:   cause,
		Context: context,
	}
}

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
