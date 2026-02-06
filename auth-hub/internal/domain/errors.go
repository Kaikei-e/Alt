package domain

import "errors"

// Authentication errors.
var (
	ErrSessionNotFound = errors.New("session not found")
	ErrSessionExpired  = errors.New("session expired")
	ErrAuthFailed      = errors.New("authentication failed")
	ErrSessionInactive = errors.New("session is not active")
	ErrMissingIdentity = errors.New("missing identity in session")
)

// Token errors.
var (
	ErrTokenGeneration   = errors.New("token generation failed")
	ErrCSRFSecretMissing = errors.New("CSRF secret not configured")
	ErrBackendSecretWeak = errors.New("backend token secret too weak")
)

// External service errors.
var (
	ErrKratosUnavailable  = errors.New("identity provider unavailable")
	ErrAdminNotConfigured = errors.New("admin API not configured")
	ErrNoIdentitiesFound  = errors.New("no identities found")
)

// Rate limiting errors.
var (
	ErrRateLimited = errors.New("rate limit exceeded")
)
