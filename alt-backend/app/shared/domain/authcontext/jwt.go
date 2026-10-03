package authcontext

import "context"

type jwtContextKey struct{}

// WithJWT stores a JWT string in the context.
func WithJWT(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, jwtContextKey{}, token)
}

// JWTFromContext retrieves the JWT string from the context.
func JWTFromContext(ctx context.Context) (string, bool) {
	token, ok := ctx.Value(jwtContextKey{}).(string)
	return token, ok
}
