package domain

import "context"

// SessionValidator validates a session cookie against the identity provider.
type SessionValidator interface {
	ValidateSession(ctx context.Context, cookie string) (*Identity, error)
}

// SessionCache provides read/write access to cached session data.
type SessionCache interface {
	Get(sessionID string) (*CachedSession, bool)
	Set(sessionID string, session CachedSession)
}

// TokenIssuer generates signed backend JWT tokens.
type TokenIssuer interface {
	IssueBackendToken(identity *Identity, sessionID string) (string, error)
}

// CSRFTokenGenerator generates CSRF tokens from session identifiers.
type CSRFTokenGenerator interface {
	Generate(sessionID string) (string, error)
}

// IdentityProvider retrieves identity information from the admin API.
type IdentityProvider interface {
	GetFirstIdentityID(ctx context.Context) (string, error)
}
