package domain

import "errors"

// Authentication and session errors
var (
	// Authentication errors
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUserNotFound       = errors.New("user not found")
	ErrUserAlreadyExists  = errors.New("user already exists")
	ErrAccountLocked      = errors.New("account locked")
	ErrAccountDisabled    = errors.New("account disabled")

	// Session errors
	ErrSessionNotFound    = errors.New("session not found")
	ErrSessionExpired     = errors.New("session expired")
	ErrSessionInactive    = errors.New("session inactive")
	ErrInvalidSession     = errors.New("invalid session")

	// CSRF errors
	ErrCSRFTokenRequired  = errors.New("CSRF token required")
	ErrInvalidCSRFToken   = errors.New("invalid CSRF token")
	ErrCSRFTokenExpired   = errors.New("CSRF token expired")
	ErrCSRFTokenMismatch  = errors.New("CSRF token session mismatch")

	// Authorization errors
	ErrUnauthorized       = errors.New("unauthorized")
	ErrForbidden          = errors.New("forbidden")
	ErrInsufficientRole   = errors.New("insufficient role")

	// Tenant errors
	ErrTenantNotFound     = errors.New("tenant not found")
	ErrTenantDisabled     = errors.New("tenant disabled")
	ErrTenantQuotaExceeded = errors.New("tenant quota exceeded")

	// Validation errors
	ErrInvalidInput       = errors.New("invalid input")
	ErrValidationFailed   = errors.New("validation failed")
	ErrInvalidEmail       = errors.New("invalid email")
	ErrPasswordTooWeak    = errors.New("password too weak")

	// Rate limiting errors
	ErrRateLimitExceeded  = errors.New("rate limit exceeded")
	ErrTooManyRequests    = errors.New("too many requests")

	// General errors
	ErrInternal           = errors.New("internal error")
	ErrNotImplemented     = errors.New("not implemented")
	ErrResourceNotFound   = errors.New("resource not found")
	ErrConflict           = errors.New("resource conflict")
)

// AuthError represents authentication-related errors with additional context
type AuthError struct {
	Code    string
	Message string
	Cause   error
}

func (e *AuthError) Error() string {
	if e.Cause != nil {
		return e.Message + ": " + e.Cause.Error()
	}
	return e.Message
}

func (e *AuthError) Unwrap() error {
	return e.Cause
}

// NewAuthError creates a new authentication error
func NewAuthError(code, message string, cause error) *AuthError {
	return &AuthError{
		Code:    code,
		Message: message,
		Cause:   cause,
	}
}

// Common auth error codes
const (
	ErrCodeInvalidCredentials = "INVALID_CREDENTIALS"
	ErrCodeSessionExpired     = "SESSION_EXPIRED" 
	ErrCodeCSRFInvalid        = "CSRF_INVALID"
	ErrCodeUnauthorized       = "UNAUTHORIZED"
	ErrCodeForbidden          = "FORBIDDEN"
	ErrCodeRateLimit          = "RATE_LIMIT"
	ErrCodeInternal           = "INTERNAL_ERROR"
)

// ValidationError represents validation errors with field-specific details
type ValidationError struct {
	Field   string
	Value   interface{}
	Message string
}

func (e *ValidationError) Error() string {
	return e.Message
}

// NewValidationError creates a new validation error
func NewValidationError(field string, value interface{}, message string) *ValidationError {
	return &ValidationError{
		Field:   field,
		Value:   value,
		Message: message,
	}
}

// SecurityError represents security-related errors
type SecurityError struct {
	Type        string
	Description string
	RemoteAddr  string
	UserAgent   string
	Timestamp   int64
}

func (e *SecurityError) Error() string {
	return e.Description
}

// NewSecurityError creates a new security error
func NewSecurityError(errorType, description, remoteAddr, userAgent string, timestamp int64) *SecurityError {
	return &SecurityError{
		Type:        errorType,
		Description: description,
		RemoteAddr:  remoteAddr,
		UserAgent:   userAgent,
		Timestamp:   timestamp,
	}
}

// Security error types
const (
	SecurityErrorCSRFAttack      = "CSRF_ATTACK"
	SecurityErrorRateLimit       = "RATE_LIMIT_EXCEEDED"
	SecurityErrorSuspiciousUA    = "SUSPICIOUS_USER_AGENT"
	SecurityErrorPathTraversal   = "PATH_TRAVERSAL_ATTEMPT"
	SecurityErrorSQLInjection    = "SQL_INJECTION_ATTEMPT"
	SecurityErrorUnauthorized    = "UNAUTHORIZED_ACCESS"
)