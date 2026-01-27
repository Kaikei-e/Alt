// ABOUTME: This file provides an in-memory implementation of OAuth2TokenRepository
// ABOUTME: Used for testing and development when Kubernetes Secret storage is not available

package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"pre-processor-sidecar/models"
)

// InMemoryTokenRepository is an in-memory implementation for OAuth2TokenRepository
type InMemoryTokenRepository struct {
	token *models.OAuth2Token
}

// NewInMemoryTokenRepository creates a new in-memory token repository
func NewInMemoryTokenRepository() OAuth2TokenRepository {
	return &InMemoryTokenRepository{
		token: nil, // Start with no token
	}
}

// NewInMemoryTokenRepositoryWithToken creates a new in-memory token repository with a test token
func NewInMemoryTokenRepositoryWithToken() OAuth2TokenRepository {
	return &InMemoryTokenRepository{
		token: &models.OAuth2Token{
			AccessToken:  "test_access_token_" + uuid.New().String(),
			RefreshToken: "test_refresh_token_" + uuid.New().String(),
			TokenType:    "Bearer",
			ExpiresIn:    3600,
			ExpiresAt:    time.Now().Add(1 * time.Hour),
			Scope:        "read",
			IssuedAt:     time.Now(),
		},
	}
}

func (r *InMemoryTokenRepository) GetCurrentToken(ctx context.Context) (*models.OAuth2Token, error) {
	if r.token == nil {
		return nil, ErrTokenNotFound
	}
	return r.token, nil
}

func (r *InMemoryTokenRepository) SaveToken(ctx context.Context, token *models.OAuth2Token) error {
	if token == nil {
		return ErrInvalidToken
	}
	if token.AccessToken == "" {
		return ErrInvalidToken
	}
	r.token = token
	return nil
}

func (r *InMemoryTokenRepository) UpdateToken(ctx context.Context, token *models.OAuth2Token) error {
	if token == nil {
		return ErrInvalidToken
	}
	if token.AccessToken == "" {
		return ErrInvalidToken
	}
	r.token = token
	return nil
}

func (r *InMemoryTokenRepository) DeleteToken(ctx context.Context) error {
	r.token = nil
	return nil
}
