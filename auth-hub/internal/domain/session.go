package domain

import "time"

// Identity represents an authenticated user identity from the identity provider.
type Identity struct {
	UserID    string
	Email     string
	SessionID string
	CreatedAt time.Time
}

// CachedSession holds session data stored in the cache.
type CachedSession struct {
	UserID   string
	TenantID string
	Email    string
}
