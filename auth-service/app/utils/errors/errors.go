package errors

import (
	"errors"
	"fmt"
	"net/http"
)

// ErrorCode represents specific error types
type ErrorCode string

const (
	// Authentication and Authorization errors
	ErrCodeUnauthorized     ErrorCode = "UNAUTHORIZED"
	ErrCodeForbidden        ErrorCode = "FORBIDDEN"
	ErrCodeInvalidCredentials ErrorCode = "INVALID_CREDENTIALS"
	ErrCodeTokenExpired     ErrorCode = "TOKEN_EXPIRED"
	ErrCodeInvalidToken     ErrorCode = "INVALID_TOKEN"
	ErrCodeSessionNotFound  ErrorCode = "SESSION_NOT_FOUND"
	ErrCodeCSRFTokenInvalid ErrorCode = "CSRF_TOKEN_INVALID"
	
	// User Management errors
	ErrCodeUserNotFound     ErrorCode = "USER_NOT_FOUND"
	ErrCodeUserExists       ErrorCode = "USER_EXISTS"
	ErrCodeUserInactive     ErrorCode = "USER_INACTIVE"
	ErrCodeUserSuspended    ErrorCode = "USER_SUSPENDED"
	ErrCodeInvalidUserRole  ErrorCode = "INVALID_USER_ROLE"
	
	// Tenant Management errors
	ErrCodeTenantNotFound   ErrorCode = "TENANT_NOT_FOUND"
	ErrCodeTenantInactive   ErrorCode = "TENANT_INACTIVE"
	ErrCodeTenantSuspended  ErrorCode = "TENANT_SUSPENDED"
	ErrCodeTenantLimitExceeded ErrorCode = "TENANT_LIMIT_EXCEEDED"
	
	// Validation errors
	ErrCodeValidationFailed ErrorCode = "VALIDATION_FAILED"
	ErrCodeInvalidInput     ErrorCode = "INVALID_INPUT"
	ErrCodeMissingField     ErrorCode = "MISSING_FIELD"
	
	// System errors
	ErrCodeInternalError    ErrorCode = "INTERNAL_ERROR"
	ErrCodeDatabaseError    ErrorCode = "DATABASE_ERROR"
	ErrCodeKratosError      ErrorCode = "KRATOS_ERROR"
	ErrCodeConfigError      ErrorCode = "CONFIG_ERROR"
	
	// Rate limiting
	ErrCodeRateLimitExceeded ErrorCode = "RATE_LIMIT_EXCEEDED"
	
	// Generic errors
	ErrCodeBadRequest       ErrorCode = "BAD_REQUEST"
	ErrCodeNotFound         ErrorCode = "NOT_FOUND"
	ErrCodeConflict         ErrorCode = "CONFLICT"
	ErrCodeServiceUnavailable ErrorCode = "SERVICE_UNAVAILABLE"
)

// AppError represents an application error with additional context
type AppError struct {
	Code       ErrorCode              `json:"code"`
	Message    string                 `json:"message"`
	Details    string                 `json:"details,omitempty"`
	StatusCode int                    `json:"-"`
	Cause      error                  `json:"-"`
	Context    map[string]interface{} `json:"context,omitempty"`
}

// Error implements the error interface
func (e *AppError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s (caused by: %v)", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap returns the underlying error for error unwrapping
func (e *AppError) Unwrap() error {
	return e.Cause
}

// WithCause adds a cause to the error
func (e *AppError) WithCause(cause error) *AppError {
	e.Cause = cause
	return e
}

// WithContext adds context to the error
func (e *AppError) WithContext(key string, value interface{}) *AppError {
	if e.Context == nil {
		e.Context = make(map[string]interface{})
	}
	e.Context[key] = value
	return e
}

// WithDetails adds details to the error
func (e *AppError) WithDetails(details string) *AppError {
	e.Details = details
	return e
}

// New creates a new AppError
func New(code ErrorCode, message string) *AppError {
	return &AppError{
		Code:       code,
		Message:    message,
		StatusCode: getHTTPStatusCode(code),
	}
}

// Newf creates a new AppError with formatted message
func Newf(code ErrorCode, format string, args ...interface{}) *AppError {
	return &AppError{
		Code:       code,
		Message:    fmt.Sprintf(format, args...),
		StatusCode: getHTTPStatusCode(code),
	}
}

// Wrap wraps an existing error with AppError
func Wrap(code ErrorCode, message string, cause error) *AppError {
	return &AppError{
		Code:       code,
		Message:    message,
		StatusCode: getHTTPStatusCode(code),
		Cause:      cause,
	}
}

// Wrapf wraps an existing error with AppError and formatted message
func Wrapf(code ErrorCode, cause error, format string, args ...interface{}) *AppError {
	return &AppError{
		Code:       code,
		Message:    fmt.Sprintf(format, args...),
		StatusCode: getHTTPStatusCode(code),
		Cause:      cause,
	}
}

// IsAppError checks if an error is an AppError
func IsAppError(err error) bool {
	var appErr *AppError
	return errors.As(err, &appErr)
}

// AsAppError converts an error to AppError if possible
func AsAppError(err error) (*AppError, bool) {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr, true
	}
	return nil, false
}

// GetErrorCode extracts the error code from an error
func GetErrorCode(err error) ErrorCode {
	if appErr, ok := AsAppError(err); ok {
		return appErr.Code
	}
	return ErrCodeInternalError
}

// GetHTTPStatusCode gets the HTTP status code for an error
func GetHTTPStatusCode(err error) int {
	if appErr, ok := AsAppError(err); ok {
		return appErr.StatusCode
	}
	return http.StatusInternalServerError
}

// getHTTPStatusCode maps error codes to HTTP status codes
func getHTTPStatusCode(code ErrorCode) int {
	switch code {
	case ErrCodeUnauthorized, ErrCodeInvalidCredentials, ErrCodeTokenExpired, ErrCodeInvalidToken:
		return http.StatusUnauthorized
	case ErrCodeForbidden, ErrCodeUserSuspended, ErrCodeTenantSuspended:
		return http.StatusForbidden
	case ErrCodeUserNotFound, ErrCodeTenantNotFound, ErrCodeSessionNotFound, ErrCodeNotFound:
		return http.StatusNotFound
	case ErrCodeUserExists, ErrCodeConflict:
		return http.StatusConflict
	case ErrCodeValidationFailed, ErrCodeInvalidInput, ErrCodeMissingField, ErrCodeCSRFTokenInvalid, ErrCodeBadRequest:
		return http.StatusBadRequest
	case ErrCodeRateLimitExceeded:
		return http.StatusTooManyRequests
	case ErrCodeServiceUnavailable, ErrCodeKratosError:
		return http.StatusServiceUnavailable
	case ErrCodeInternalError, ErrCodeDatabaseError, ErrCodeConfigError:
		return http.StatusInternalServerError
	default:
		return http.StatusInternalServerError
	}
}

// Predefined common errors

// Authentication errors
var (
	ErrUnauthorized       = New(ErrCodeUnauthorized, "authentication required")
	ErrForbidden          = New(ErrCodeForbidden, "access denied")
	ErrInvalidCredentials = New(ErrCodeInvalidCredentials, "invalid credentials")
	ErrTokenExpired       = New(ErrCodeTokenExpired, "token has expired")
	ErrInvalidToken       = New(ErrCodeInvalidToken, "invalid token")
	ErrSessionNotFound    = New(ErrCodeSessionNotFound, "session not found")
	ErrCSRFTokenInvalid   = New(ErrCodeCSRFTokenInvalid, "invalid CSRF token")
)

// User errors
var (
	ErrUserNotFound     = New(ErrCodeUserNotFound, "user not found")
	ErrUserExists       = New(ErrCodeUserExists, "user already exists")
	ErrUserInactive     = New(ErrCodeUserInactive, "user account is inactive")
	ErrUserSuspended    = New(ErrCodeUserSuspended, "user account is suspended")
	ErrInvalidUserRole  = New(ErrCodeInvalidUserRole, "invalid user role")
)

// Tenant errors
var (
	ErrTenantNotFound      = New(ErrCodeTenantNotFound, "tenant not found")
	ErrTenantInactive      = New(ErrCodeTenantInactive, "tenant is inactive")
	ErrTenantSuspended     = New(ErrCodeTenantSuspended, "tenant is suspended")
	ErrTenantLimitExceeded = New(ErrCodeTenantLimitExceeded, "tenant limit exceeded")
)

// System errors
var (
	ErrInternalError      = New(ErrCodeInternalError, "internal server error")
	ErrDatabaseError      = New(ErrCodeDatabaseError, "database error")
	ErrKratosError        = New(ErrCodeKratosError, "kratos service error")
	ErrConfigError        = New(ErrCodeConfigError, "configuration error")
	ErrServiceUnavailable = New(ErrCodeServiceUnavailable, "service temporarily unavailable")
	ErrRateLimitExceeded  = New(ErrCodeRateLimitExceeded, "rate limit exceeded")
)

// Validation errors
var (
	ErrValidationFailed = New(ErrCodeValidationFailed, "validation failed")
	ErrInvalidInput     = New(ErrCodeInvalidInput, "invalid input")
	ErrMissingField     = New(ErrCodeMissingField, "required field is missing")
)

// Generic errors
var (
	ErrBadRequest = New(ErrCodeBadRequest, "bad request")
	ErrNotFound   = New(ErrCodeNotFound, "resource not found")
	ErrConflict   = New(ErrCodeConflict, "resource conflict")
)

// Helper functions for creating contextual errors

// NewUnauthorized creates an unauthorized error with context
func NewUnauthorized(details string) *AppError {
	return New(ErrCodeUnauthorized, "authentication required").WithDetails(details)
}

// NewForbidden creates a forbidden error with context
func NewForbidden(details string) *AppError {
	return New(ErrCodeForbidden, "access denied").WithDetails(details)
}

// NewNotFound creates a not found error with context
func NewNotFound(resource string) *AppError {
	return Newf(ErrCodeNotFound, "%s not found", resource)
}

// NewValidationError creates a validation error with details
func NewValidationError(details string) *AppError {
	return New(ErrCodeValidationFailed, "validation failed").WithDetails(details)
}

// NewInternalError creates an internal error with cause
func NewInternalError(cause error) *AppError {
	return Wrap(ErrCodeInternalError, "internal server error", cause)
}

// NewDatabaseError creates a database error with cause
func NewDatabaseError(cause error) *AppError {
	return Wrap(ErrCodeDatabaseError, "database operation failed", cause)
}

// NewKratosError creates a Kratos service error with cause
func NewKratosError(cause error) *AppError {
	return Wrap(ErrCodeKratosError, "kratos service error", cause)
}