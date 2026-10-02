package port

import "context"

type TokenIntrospection struct {
	Active   bool   `json:"active"`
	Sub      string `json:"sub"`
	TenantID string `json:"tenant_id"`
	Exp      int64  `json:"exp"`
}

type AuthHub interface {
	IntrospectToken(ctx context.Context, token string) (*TokenIntrospection, error)
}
