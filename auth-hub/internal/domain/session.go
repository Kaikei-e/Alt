package domain

import "time"

// Identity represents an authenticated user identity from the identity provider.
// TenantID is propagated into the JWT as the tenant_id claim so alt-backend
// can scope requests without deriving tenant from the subject. In single-tenant
// deployments TenantID equals UserID; multi-tenant migrations only need to
// change how TenantID is populated upstream.
type Identity struct {
	UserID    string
	TenantID  string
	Email     string
	Role      string
	SessionID string
	CreatedAt time.Time
}

// CachedSession holds session data stored in the cache.
type CachedSession struct {
	UserID   string
	TenantID string
	Email    string
	Role     string
}
