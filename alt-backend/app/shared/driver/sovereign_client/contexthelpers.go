package sovereign_client

import (
	"alt/shared/domain/authcontext"
	"context"
)

// WithJWT stores a JWT string in the context.
func WithJWT(ctx context.Context, token string) context.Context {
	return authcontext.WithJWT(ctx, token)
}

// JWTFromContext retrieves the JWT string from the context.
func JWTFromContext(ctx context.Context) (string, bool) {
	return authcontext.JWTFromContext(ctx)
}
