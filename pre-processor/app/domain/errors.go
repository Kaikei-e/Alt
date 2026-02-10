// ABOUTME: Domain-level sentinel errors for the pre-processor service
// ABOUTME: These errors are used with errors.Is() for error type checking
package domain

import "errors"

// Article-related errors
var (
	// ErrArticleNotFound indicates the requested article does not exist
	ErrArticleNotFound = errors.New("article not found")

	// ErrArticleContentEmpty indicates the article exists but has no content
	ErrArticleContentEmpty = errors.New("article content is empty")

	// ErrContentTooShort indicates content is below minimum length for summarization
	ErrContentTooShort = errors.New("content too short for summarization")

	// ErrContentTooLong indicates content exceeds maximum length for summarization
	// Large content (>100KB) causes timeouts on 8GB VRAM GPUs
	ErrContentTooLong = errors.New("content too long for summarization")

	// ErrContentNotProcessable indicates the content cannot be summarized by the model
	// (e.g., model degeneration producing empty/whitespace output). Non-retryable.
	ErrContentNotProcessable = errors.New("content not processable by model")
)

// Job-related errors
var (
	// ErrJobNotFound indicates the requested job does not exist
	ErrJobNotFound = errors.New("job not found")
)

// Validation errors
var (
	// ErrInvalidRequest indicates the request format is invalid
	ErrInvalidRequest = errors.New("invalid request format")

	// ErrMissingArticleID indicates article_id field is required but missing
	ErrMissingArticleID = errors.New("article ID is required")

	// ErrEmptyContent indicates content field is required but empty
	ErrEmptyContent = errors.New("content cannot be empty")
)

// External service errors
var (
	// ErrNewsCreatorUnavailable indicates news-creator service is not reachable
	ErrNewsCreatorUnavailable = errors.New("news-creator service unavailable")

	// ErrServiceOverloaded indicates downstream service returned 429 (queue full)
	ErrServiceOverloaded = errors.New("downstream service overloaded")
)
