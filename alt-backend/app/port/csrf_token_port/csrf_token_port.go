package csrf_token_port

import (
	"context"
	"time"
)

// CSRFTokenUsecase defines the interface for CSRF token business logic
type CSRFTokenUsecase interface {
	GenerateToken(ctx context.Context) (string, error)
	ValidateToken(ctx context.Context, token string) (bool, error)
	InvalidateToken(ctx context.Context, token string) error
}

// CSRFTokenGateway defines the interface for CSRF token gateway operations
type CSRFTokenGateway interface {
	GenerateToken(ctx context.Context) (string, error)
	ValidateToken(ctx context.Context, token string) (bool, error)
	InvalidateToken(ctx context.Context, token string) error
}

// CSRFTokenDriver defines the interface for CSRF token driver operations
type CSRFTokenDriver interface {
	StoreToken(ctx context.Context, token string, expiration time.Time) error
	GetToken(ctx context.Context, token string) (time.Time, error)
	DeleteToken(ctx context.Context, token string) error
	GenerateRandomToken() (string, error)
}