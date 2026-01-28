// Package constants provides shared constants used across the alt-backend codebase.
// This package centralizes magic numbers and configuration values for better maintainability.
package constants

import "time"

// Retry configuration constants for HTTP requests and network operations.
const (
	// DefaultMaxRetries is the maximum number of retry attempts for HTTP requests.
	DefaultMaxRetries = 3

	// DefaultInitialDelay is the initial delay for exponential backoff.
	DefaultInitialDelay = 2 * time.Second

	// DefaultMaxDelay is the maximum delay cap for exponential backoff.
	DefaultMaxDelay = 30 * time.Second

	// DefaultRequestTimeout is the default timeout for HTTP requests.
	DefaultRequestTimeout = 60 * time.Second

	// DefaultFeedFetchTimeout is the timeout for RSS feed fetching operations.
	DefaultFeedFetchTimeout = 60 * time.Second
)
