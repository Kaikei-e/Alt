// ABOUTME: Structured error type following alt-backend AppContextError pattern
// ABOUTME: Provides rich context, HTTP mapping, retryability, and secure client responses
package errors

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
)

// AppContextError represents an error with rich context information
// following 2025 Go best practices for error handling
type AppContextError struct {
	Code      string                 `json:"code"`
	Message   string                 `json:"message"`
	Layer     string                 `json:"layer,omitempty"`     // Clean Architecture layer (handler, service, repository, driver)
	Component string                 `json:"component,omitempty"` // Specific component/service name
	Operation string                 `json:"operation,omitempty"` // Specific operation/method name
	Cause     error                  `json:"-"`                   // Underlying error (not serialized)
	Context   map[string]interface{} `json:"context,omitempty"`   // Additional context information
	ErrorID   string                 `json:"-"`                   // Unique ID for log correlation (not serialized to client)
}

// Error implements the error interface
func (e *AppContextError) Error() string {
	var prefix string
	if e.Layer != "" && e.Component != "" && e.Operation != "" {
		prefix = fmt.Sprintf("[%s:%s:%s] ", e.Layer, e.Component, e.Operation)
	}

	if e.Cause != nil {
		return fmt.Sprintf("%s%s: %s (caused by: %v)", prefix, e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s%s: %s", prefix, e.Code, e.Message)
}

// Unwrap returns the underlying error for error chain unwrapping
func (e *AppContextError) Unwrap() error {
	return e.Cause
}

// HTTPStatusCode maps error codes to HTTP status codes
func (e *AppContextError) HTTPStatusCode() int {
	switch e.Code {
	case "VALIDATION_ERROR":
		return http.StatusBadRequest
	case "NOT_FOUND_ERROR":
		return http.StatusNotFound
	case "RATE_LIMIT_ERROR":
		return http.StatusTooManyRequests
	case "EXTERNAL_API_ERROR":
		return http.StatusBadGateway
	case "TIMEOUT_ERROR":
		return http.StatusGatewayTimeout
	case "DATABASE_ERROR":
		return http.StatusInternalServerError
	case "INTERNAL_ERROR":
		return http.StatusInternalServerError
	case "UNKNOWN_ERROR":
		return http.StatusInternalServerError
	default:
		return http.StatusInternalServerError
	}
}

// IsRetryable determines if the error represents a retryable condition
func (e *AppContextError) IsRetryable() bool {
	switch e.Code {
	case "RATE_LIMIT_ERROR", "TIMEOUT_ERROR", "EXTERNAL_API_ERROR":
		return true
	default:
		return false
	}
}

// safeMessages maps error codes to user-friendly, non-leaking messages
var safeMessages = map[string]string{
	"DATABASE_ERROR":     "A temporary service error occurred. Please try again later.",
	"EXTERNAL_API_ERROR": "Unable to connect to external service. Please try again.",
	"VALIDATION_ERROR":   "", // Use original message (safe by design)
	"NOT_FOUND_ERROR":    "", // Use original message (safe by design)
	"RATE_LIMIT_ERROR":   "Too many requests. Please wait before trying again.",
	"TIMEOUT_ERROR":      "The request took too long. Please try again.",
	"INTERNAL_ERROR":     "An unexpected error occurred. Please try again later.",
	"UNKNOWN_ERROR":      "An unexpected error occurred. Please try again later.",
}

// SafeMessage returns a user-friendly message that does not leak internal details.
// For VALIDATION_ERROR and NOT_FOUND_ERROR, the original message is returned as it is designed to be safe.
// For other error types, a generic safe message is returned.
func (e *AppContextError) SafeMessage() string {
	if msg, ok := safeMessages[e.Code]; ok && msg != "" {
		return msg
	}
	// VALIDATION_ERROR and NOT_FOUND_ERROR use original message (safe by design)
	if e.Code == "VALIDATION_ERROR" || e.Code == "NOT_FOUND_ERROR" {
		return e.Message
	}
	return "An error occurred."
}

// SecureHTTPResponse represents a secure HTTP error response that does not leak internal details
type SecureHTTPResponse struct {
	Error SecureErrorDetail `json:"error"`
}

// SecureErrorDetail contains the error details for SecureHTTPResponse
type SecureErrorDetail struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	ErrorID   string `json:"error_id,omitempty"`
	Retryable bool   `json:"retryable,omitempty"`
}

// ToSecureHTTPResponse converts an AppContextError to a secure HTTP response
// that does not expose internal error details to the client
func (e *AppContextError) ToSecureHTTPResponse() SecureHTTPResponse {
	return SecureHTTPResponse{
		Error: SecureErrorDetail{
			Code:      e.Code,
			Message:   e.SafeMessage(),
			ErrorID:   e.ErrorID,
			Retryable: e.IsRetryable(),
		},
	}
}

// generateErrorID generates a short unique error ID for log correlation
func generateErrorID() string {
	b := make([]byte, 4) // 4 bytes = 8 hex characters
	if _, err := rand.Read(b); err != nil {
		// Fallback to a fixed ID if random generation fails
		return "00000000"
	}
	return hex.EncodeToString(b)
}

// NewAppContextError creates a new AppContextError with full context
func NewAppContextError(
	code, message, layer, component, operation string,
	cause error,
	context map[string]interface{},
) *AppContextError {
	if context == nil {
		context = make(map[string]interface{})
	}

	return &AppContextError{
		Code:      code,
		Message:   message,
		Layer:     layer,
		Component: component,
		Operation: operation,
		Cause:     cause,
		Context:   context,
		ErrorID:   generateErrorID(),
	}
}

// Helper functions for common error patterns

// NewValidationContextError creates a validation error with context
func NewValidationContextError(message, layer, component, operation string, context map[string]interface{}) *AppContextError {
	if context == nil {
		context = make(map[string]interface{})
	}
	context["error_type"] = "validation"
	return NewAppContextError("VALIDATION_ERROR", message, layer, component, operation, nil, context)
}

// NewNotFoundContextError creates a not found error with context
func NewNotFoundContextError(message, layer, component, operation string, context map[string]interface{}) *AppContextError {
	if context == nil {
		context = make(map[string]interface{})
	}
	context["error_type"] = "not_found"
	return NewAppContextError("NOT_FOUND_ERROR", message, layer, component, operation, nil, context)
}

// NewInternalContextError creates an internal error with context
func NewInternalContextError(message, layer, component, operation string, cause error, context map[string]interface{}) *AppContextError {
	if context == nil {
		context = make(map[string]interface{})
	}
	context["error_type"] = "internal"
	return NewAppContextError("INTERNAL_ERROR", message, layer, component, operation, cause, context)
}

// NewDatabaseContextError creates a database error with context
func NewDatabaseContextError(message, layer, component, operation string, cause error, context map[string]interface{}) *AppContextError {
	if context == nil {
		context = make(map[string]interface{})
	}
	context["error_type"] = "database"
	return NewAppContextError("DATABASE_ERROR", message, layer, component, operation, cause, context)
}

// NewExternalAPIContextError creates an external API error with context
func NewExternalAPIContextError(message, layer, component, operation string, cause error, context map[string]interface{}) *AppContextError {
	if context == nil {
		context = make(map[string]interface{})
	}
	context["error_type"] = "external_api"
	return NewAppContextError("EXTERNAL_API_ERROR", message, layer, component, operation, cause, context)
}

// NewTimeoutContextError creates a timeout error with context
func NewTimeoutContextError(message, layer, component, operation string, cause error, context map[string]interface{}) *AppContextError {
	if context == nil {
		context = make(map[string]interface{})
	}
	context["error_type"] = "timeout"
	return NewAppContextError("TIMEOUT_ERROR", message, layer, component, operation, cause, context)
}

// NewRateLimitContextError creates a rate limit error with context
func NewRateLimitContextError(message, layer, component, operation string, cause error, context map[string]interface{}) *AppContextError {
	if context == nil {
		context = make(map[string]interface{})
	}
	context["error_type"] = "rate_limit"
	return NewAppContextError("RATE_LIMIT_ERROR", message, layer, component, operation, cause, context)
}
