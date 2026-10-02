package authport

import (
	"context"
	"knowledge-sovereign/domain/authcontext"
)

// TokenVerifier validates user delegation tokens.
type TokenVerifier interface {
	ValidateToken(ctx context.Context, tokenStr string) (*authcontext.VerifiedClaims, error)
}
