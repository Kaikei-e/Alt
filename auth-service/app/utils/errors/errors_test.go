package errors

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAppError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *AppError
		expected string
	}{
		{
			name:     "error without cause",
			err:      New(ErrCodeUserNotFound, "user not found"),
			expected: "USER_NOT_FOUND: user not found",
		},
		{
			name:     "error with cause",
			err:      Wrap(ErrCodeDatabaseError, "database error", errors.New("connection failed")),
			expected: "DATABASE_ERROR: database error (caused by: connection failed)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.err.Error())
		})
	}
}

func TestAppError_Unwrap(t *testing.T) {
	cause := errors.New("original error")
	err := Wrap(ErrCodeInternalError, "wrapped error", cause)
	
	unwrapped := errors.Unwrap(err)
	assert.Equal(t, cause, unwrapped)
}

func TestAppError_WithCause(t *testing.T) {
	err := New(ErrCodeUserNotFound, "user not found")
	cause := errors.New("database connection failed")
	
	err.WithCause(cause)
	assert.Equal(t, cause, err.Cause)
}

func TestAppError_WithContext(t *testing.T) {
	err := New(ErrCodeUserNotFound, "user not found")
	err.WithContext("user_id", "123")
	err.WithContext("tenant_id", "456")
	
	assert.Equal(t, "123", err.Context["user_id"])
	assert.Equal(t, "456", err.Context["tenant_id"])
}

func TestAppError_WithDetails(t *testing.T) {
	err := New(ErrCodeValidationFailed, "validation failed")
	err.WithDetails("email field is required")
	
	assert.Equal(t, "email field is required", err.Details)
}

func TestNew(t *testing.T) {
	err := New(ErrCodeUserNotFound, "user not found")
	
	assert.Equal(t, ErrCodeUserNotFound, err.Code)
	assert.Equal(t, "user not found", err.Message)
	assert.Equal(t, http.StatusNotFound, err.StatusCode)
	assert.Nil(t, err.Cause)
}

func TestNewf(t *testing.T) {
	err := Newf(ErrCodeUserNotFound, "user %s not found", "john")
	
	assert.Equal(t, ErrCodeUserNotFound, err.Code)
	assert.Equal(t, "user john not found", err.Message)
	assert.Equal(t, http.StatusNotFound, err.StatusCode)
}

func TestWrap(t *testing.T) {
	cause := errors.New("database connection failed")
	err := Wrap(ErrCodeDatabaseError, "database error", cause)
	
	assert.Equal(t, ErrCodeDatabaseError, err.Code)
	assert.Equal(t, "database error", err.Message)
	assert.Equal(t, http.StatusInternalServerError, err.StatusCode)
	assert.Equal(t, cause, err.Cause)
}

func TestWrapf(t *testing.T) {
	cause := errors.New("connection timeout")
	err := Wrapf(ErrCodeDatabaseError, cause, "database operation failed for user %s", "john")
	
	assert.Equal(t, ErrCodeDatabaseError, err.Code)
	assert.Equal(t, "database operation failed for user john", err.Message)
	assert.Equal(t, cause, err.Cause)
}

func TestIsAppError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "AppError",
			err:      New(ErrCodeUserNotFound, "user not found"),
			expected: true,
		},
		{
			name:     "wrapped AppError",
			err:      fmt.Errorf("wrapped: %w", New(ErrCodeUserNotFound, "user not found")),
			expected: true,
		},
		{
			name:     "standard error",
			err:      errors.New("standard error"),
			expected: false,
		},
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsAppError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAsAppError(t *testing.T) {
	originalErr := New(ErrCodeUserNotFound, "user not found")
	wrappedErr := fmt.Errorf("wrapped: %w", originalErr)
	standardErr := errors.New("standard error")

	tests := []struct {
		name      string
		err       error
		expectOk  bool
		expectErr *AppError
	}{
		{
			name:      "AppError",
			err:       originalErr,
			expectOk:  true,
			expectErr: originalErr,
		},
		{
			name:      "wrapped AppError",
			err:       wrappedErr,
			expectOk:  true,
			expectErr: originalErr,
		},
		{
			name:     "standard error",
			err:      standardErr,
			expectOk: false,
		},
		{
			name:     "nil error",
			err:      nil,
			expectOk: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			appErr, ok := AsAppError(tt.err)
			assert.Equal(t, tt.expectOk, ok)
			if tt.expectOk {
				assert.Equal(t, tt.expectErr, appErr)
			} else {
				assert.Nil(t, appErr)
			}
		})
	}
}

func TestGetErrorCode(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected ErrorCode
	}{
		{
			name:     "AppError",
			err:      New(ErrCodeUserNotFound, "user not found"),
			expected: ErrCodeUserNotFound,
		},
		{
			name:     "wrapped AppError",
			err:      fmt.Errorf("wrapped: %w", New(ErrCodeValidationFailed, "validation failed")),
			expected: ErrCodeValidationFailed,
		},
		{
			name:     "standard error",
			err:      errors.New("standard error"),
			expected: ErrCodeInternalError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code := GetErrorCode(tt.err)
			assert.Equal(t, tt.expected, code)
		})
	}
}

func TestGetHTTPStatusCode(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected int
	}{
		{
			name:     "AppError with known status",
			err:      New(ErrCodeUserNotFound, "user not found"),
			expected: http.StatusNotFound,
		},
		{
			name:     "AppError with unauthorized status",
			err:      New(ErrCodeUnauthorized, "unauthorized"),
			expected: http.StatusUnauthorized,
		},
		{
			name:     "standard error",
			err:      errors.New("standard error"),
			expected: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			statusCode := GetHTTPStatusCode(tt.err)
			assert.Equal(t, tt.expected, statusCode)
		})
	}
}

func TestGetHTTPStatusCodeMapping(t *testing.T) {
	tests := []struct {
		code     ErrorCode
		expected int
	}{
		{ErrCodeUnauthorized, http.StatusUnauthorized},
		{ErrCodeInvalidCredentials, http.StatusUnauthorized},
		{ErrCodeTokenExpired, http.StatusUnauthorized},
		{ErrCodeInvalidToken, http.StatusUnauthorized},
		{ErrCodeForbidden, http.StatusForbidden},
		{ErrCodeUserSuspended, http.StatusForbidden},
		{ErrCodeTenantSuspended, http.StatusForbidden},
		{ErrCodeUserNotFound, http.StatusNotFound},
		{ErrCodeTenantNotFound, http.StatusNotFound},
		{ErrCodeSessionNotFound, http.StatusNotFound},
		{ErrCodeNotFound, http.StatusNotFound},
		{ErrCodeUserExists, http.StatusConflict},
		{ErrCodeConflict, http.StatusConflict},
		{ErrCodeValidationFailed, http.StatusBadRequest},
		{ErrCodeInvalidInput, http.StatusBadRequest},
		{ErrCodeMissingField, http.StatusBadRequest},
		{ErrCodeCSRFTokenInvalid, http.StatusBadRequest},
		{ErrCodeBadRequest, http.StatusBadRequest},
		{ErrCodeRateLimitExceeded, http.StatusTooManyRequests},
		{ErrCodeServiceUnavailable, http.StatusServiceUnavailable},
		{ErrCodeKratosError, http.StatusServiceUnavailable},
		{ErrCodeInternalError, http.StatusInternalServerError},
		{ErrCodeDatabaseError, http.StatusInternalServerError},
		{ErrCodeConfigError, http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(string(tt.code), func(t *testing.T) {
			statusCode := getHTTPStatusCode(tt.code)
			assert.Equal(t, tt.expected, statusCode)
		})
	}
}

func TestPredefinedErrors(t *testing.T) {
	tests := []struct {
		name     string
		err      *AppError
		code     ErrorCode
		httpCode int
	}{
		{"ErrUnauthorized", ErrUnauthorized, ErrCodeUnauthorized, http.StatusUnauthorized},
		{"ErrForbidden", ErrForbidden, ErrCodeForbidden, http.StatusForbidden},
		{"ErrUserNotFound", ErrUserNotFound, ErrCodeUserNotFound, http.StatusNotFound},
		{"ErrUserExists", ErrUserExists, ErrCodeUserExists, http.StatusConflict},
		{"ErrInternalError", ErrInternalError, ErrCodeInternalError, http.StatusInternalServerError},
		{"ErrValidationFailed", ErrValidationFailed, ErrCodeValidationFailed, http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.code, tt.err.Code)
			assert.Equal(t, tt.httpCode, tt.err.StatusCode)
			assert.NotEmpty(t, tt.err.Message)
		})
	}
}

func TestHelperFunctions(t *testing.T) {
	t.Run("NewUnauthorized", func(t *testing.T) {
		err := NewUnauthorized("invalid session")
		assert.Equal(t, ErrCodeUnauthorized, err.Code)
		assert.Equal(t, "authentication required", err.Message)
		assert.Equal(t, "invalid session", err.Details)
		assert.Equal(t, http.StatusUnauthorized, err.StatusCode)
	})

	t.Run("NewForbidden", func(t *testing.T) {
		err := NewForbidden("insufficient permissions")
		assert.Equal(t, ErrCodeForbidden, err.Code)
		assert.Equal(t, "access denied", err.Message)
		assert.Equal(t, "insufficient permissions", err.Details)
		assert.Equal(t, http.StatusForbidden, err.StatusCode)
	})

	t.Run("NewNotFound", func(t *testing.T) {
		err := NewNotFound("user")
		assert.Equal(t, ErrCodeNotFound, err.Code)
		assert.Equal(t, "user not found", err.Message)
		assert.Equal(t, http.StatusNotFound, err.StatusCode)
	})

	t.Run("NewValidationError", func(t *testing.T) {
		err := NewValidationError("email is required")
		assert.Equal(t, ErrCodeValidationFailed, err.Code)
		assert.Equal(t, "validation failed", err.Message)
		assert.Equal(t, "email is required", err.Details)
		assert.Equal(t, http.StatusBadRequest, err.StatusCode)
	})

	t.Run("NewInternalError", func(t *testing.T) {
		cause := errors.New("database connection failed")
		err := NewInternalError(cause)
		assert.Equal(t, ErrCodeInternalError, err.Code)
		assert.Equal(t, "internal server error", err.Message)
		assert.Equal(t, cause, err.Cause)
		assert.Equal(t, http.StatusInternalServerError, err.StatusCode)
	})

	t.Run("NewDatabaseError", func(t *testing.T) {
		cause := errors.New("query timeout")
		err := NewDatabaseError(cause)
		assert.Equal(t, ErrCodeDatabaseError, err.Code)
		assert.Equal(t, "database operation failed", err.Message)
		assert.Equal(t, cause, err.Cause)
		assert.Equal(t, http.StatusInternalServerError, err.StatusCode)
	})

	t.Run("NewKratosError", func(t *testing.T) {
		cause := errors.New("kratos service unavailable")
		err := NewKratosError(cause)
		assert.Equal(t, ErrCodeKratosError, err.Code)
		assert.Equal(t, "kratos service error", err.Message)
		assert.Equal(t, cause, err.Cause)
		assert.Equal(t, http.StatusServiceUnavailable, err.StatusCode)
	})
}

func TestErrorChaining(t *testing.T) {
	// Test error chaining and unwrapping
	originalErr := errors.New("database connection failed")
	databaseErr := NewDatabaseError(originalErr)
	wrappedErr := fmt.Errorf("operation failed: %w", databaseErr)

	// Test that we can extract the original AppError
	var appErr *AppError
	require.True(t, errors.As(wrappedErr, &appErr))
	assert.Equal(t, ErrCodeDatabaseError, appErr.Code)

	// Test that we can extract the original cause
	assert.True(t, errors.Is(wrappedErr, originalErr))
}

func TestErrorWithContext(t *testing.T) {
	err := New(ErrCodeUserNotFound, "user not found")
	err.WithContext("user_id", "123")
	err.WithContext("operation", "GetProfile")
	err.WithDetails("user lookup failed during profile retrieval")

	assert.Equal(t, "123", err.Context["user_id"])
	assert.Equal(t, "GetProfile", err.Context["operation"])
	assert.Equal(t, "user lookup failed during profile retrieval", err.Details)

	// Verify error message includes context in string representation
	errorStr := err.Error()
	assert.Contains(t, errorStr, "USER_NOT_FOUND")
	assert.Contains(t, errorStr, "user not found")
}