package errors

import (
	"errors"
	"fmt"
)

// Sentinel errors following 2025 Go best practices
// These are base errors that can be used with errors.Is() and errors.As()
var (
	ErrFeedNotFound               = errors.New("feed not found")
	ErrDatabaseUnavailable        = errors.New("database unavailable")
	ErrRateLimitExceeded          = errors.New("rate limit exceeded")
	ErrExternalServiceUnavailable = errors.New("external service unavailable")
	ErrOperationTimeout           = errors.New("operation timeout")
	ErrInvalidInput               = errors.New("invalid input")

	// Search service errors (used across layers)
	ErrSearchServiceUnavailable = errors.New("search service unavailable")
	ErrSearchTimeout            = errors.New("search request timed out")
)

// Error checking helper functions using errors.Is() for 2025 Go patterns

// IsFeedNotFound checks if an error represents a "feed not found" condition
func IsFeedNotFound(err error) bool {
	return errors.Is(err, ErrFeedNotFound)
}

// IsDatabaseError checks if an error represents a database-related problem
func IsDatabaseError(err error) bool {
	return errors.Is(err, ErrDatabaseUnavailable)
}

// IsRateLimitError checks if an error represents a rate limiting issue
func IsRateLimitError(err error) bool {
	return errors.Is(err, ErrRateLimitExceeded)
}

// IsExternalServiceError checks if an error represents an external service issue
func IsExternalServiceError(err error) bool {
	return errors.Is(err, ErrExternalServiceUnavailable)
}

// IsTimeoutError checks if an error represents a timeout condition
func IsTimeoutError(err error) bool {
	return errors.Is(err, ErrOperationTimeout)
}

// IsValidationError checks if an error represents invalid input
func IsValidationError(err error) bool {
	return errors.Is(err, ErrInvalidInput)
}

// IsRetryableError determines if an error represents a condition that can be retried
func IsRetryableError(err error) bool {
	return IsRateLimitError(err) ||
		IsTimeoutError(err) ||
		IsExternalServiceError(err)
}

// Helper functions to create AppContextErrors that wrap sentinel errors
// This provides the best of both worlds: sentinel error checking AND rich context

// NewFeedNotFoundError creates an AppContextError that wraps ErrFeedNotFound
func NewFeedNotFoundError(layer, component, operation string, context map[string]interface{}) *AppContextError {
	return NewAppContextError(
		"FEED_NOT_FOUND",
		"feed not found",
		layer,
		component,
		operation,
		fmt.Errorf("%w", ErrFeedNotFound), // Wrap sentinel error
		context,
	)
}

// NewDatabaseUnavailableError creates an AppContextError that wraps ErrDatabaseUnavailable
func NewDatabaseUnavailableError(layer, component, operation string, cause error, context map[string]interface{}) *AppContextError {
	// Create proper error chain that preserves both sentinel error and original cause
	var wrappedCause error
	if cause != nil {
		// Wrap original cause with sentinel error: cause -> ErrDatabaseUnavailable
		wrappedCause = fmt.Errorf("%w: %w", ErrDatabaseUnavailable, cause)
	} else {
		wrappedCause = fmt.Errorf("%w", ErrDatabaseUnavailable)
	}

	return NewAppContextError(
		"DATABASE_ERROR",
		"database unavailable",
		layer,
		component,
		operation,
		wrappedCause,
		context,
	)
}

// NewRateLimitExceededError creates an AppContextError that wraps ErrRateLimitExceeded
func NewRateLimitExceededError(layer, component, operation string, cause error, context map[string]interface{}) *AppContextError {
	var wrappedCause error
	if cause != nil {
		wrappedCause = fmt.Errorf("%w: %w", ErrRateLimitExceeded, cause)
	} else {
		wrappedCause = fmt.Errorf("%w", ErrRateLimitExceeded)
	}

	return NewAppContextError(
		"RATE_LIMIT_ERROR",
		"rate limit exceeded",
		layer,
		component,
		operation,
		wrappedCause,
		context,
	)
}

// NewExternalServiceUnavailableError creates an AppContextError that wraps ErrExternalServiceUnavailable
func NewExternalServiceUnavailableError(layer, component, operation string, cause error, context map[string]interface{}) *AppContextError {
	var wrappedCause error
	if cause != nil {
		wrappedCause = fmt.Errorf("%w: %w", ErrExternalServiceUnavailable, cause)
	} else {
		wrappedCause = fmt.Errorf("%w", ErrExternalServiceUnavailable)
	}

	return NewAppContextError(
		"EXTERNAL_API_ERROR",
		"external service unavailable",
		layer,
		component,
		operation,
		wrappedCause,
		context,
	)
}

// NewOperationTimeoutError creates an AppContextError that wraps ErrOperationTimeout
func NewOperationTimeoutError(layer, component, operation string, cause error, context map[string]interface{}) *AppContextError {
	var wrappedCause error
	if cause != nil {
		wrappedCause = fmt.Errorf("%w: %w", ErrOperationTimeout, cause)
	} else {
		wrappedCause = fmt.Errorf("%w", ErrOperationTimeout)
	}

	return NewAppContextError(
		"TIMEOUT_ERROR",
		"operation timeout",
		layer,
		component,
		operation,
		wrappedCause,
		context,
	)
}

// NewInvalidInputError creates an AppContextError that wraps ErrInvalidInput
func NewInvalidInputError(layer, component, operation string, context map[string]interface{}) *AppContextError {
	return NewAppContextError(
		"VALIDATION_ERROR",
		"invalid input",
		layer,
		component,
		operation,
		fmt.Errorf("%w", ErrInvalidInput), // Wrap sentinel error
		context,
	)
}
