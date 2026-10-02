package authcontext

import (
	"time"
)

// VerifiedClaims represents pure standard library business data.
// Subject carries the authenticated user_id; TenantID carries the tenant_id.
type VerifiedClaims struct {
	Email     string
	Role      string
	Sid       string
	TenantID  string
	Subject   string
	ExpiresAt time.Time
}
