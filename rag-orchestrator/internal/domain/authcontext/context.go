package authcontext

import "context"

type jwtKey struct{}

// WithJWT stores the JWT token in context.
func WithJWT(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, jwtKey{}, token)
}

// JWTFromContext extracts the JWT token from context.
func JWTFromContext(ctx context.Context) (string, bool) {
	val, ok := ctx.Value(jwtKey{}).(string)
	return val, ok && val != ""
}
