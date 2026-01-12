// Package domain provides domain types for the BFF service.
package domain

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

type contextKey string

const userContextKey contextKey = "user_context"

// ErrNoUserContext is returned when no user context is found in the context.
var ErrNoUserContext = errors.New("no user context in context")

// UserContext represents the authenticated user's context.
type UserContext struct {
	UserID    uuid.UUID
	Email     string
	Role      string
	TenantID  uuid.UUID
	SessionID string
	LoginAt   time.Time
	ExpiresAt time.Time
}

// SetUserContext attaches a user context to the given context.
func SetUserContext(ctx context.Context, uc *UserContext) context.Context {
	return context.WithValue(ctx, userContextKey, uc)
}

// GetUserContext retrieves the user context from the given context.
func GetUserContext(ctx context.Context) (*UserContext, error) {
	uc, ok := ctx.Value(userContextKey).(*UserContext)
	if !ok || uc == nil {
		return nil, ErrNoUserContext
	}
	return uc, nil
}

// IsValid checks if the user context is valid and not expired.
func (uc *UserContext) IsValid() bool {
	return uc.UserID != uuid.Nil &&
		uc.Email != "" &&
		uc.ExpiresAt.After(time.Now())
}
