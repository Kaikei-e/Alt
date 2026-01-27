// ABOUTME: This file defines the OAuth2 token repository interface and implementations
// ABOUTME: Handles secure storage and retrieval of OAuth2 tokens for Inoreader API integration

package repository

import (
	"context"
	"fmt"
	"pre-processor-sidecar/models"
)

// OAuth2TokenRepository defines the interface for OAuth2 token storage operations
type OAuth2TokenRepository interface {
	// GetCurrentToken retrieves the current OAuth2 token from storage
	GetCurrentToken(ctx context.Context) (*models.OAuth2Token, error)

	// SaveToken stores a new OAuth2 token
	SaveToken(ctx context.Context, token *models.OAuth2Token) error

	// UpdateToken updates an existing OAuth2 token
	UpdateToken(ctx context.Context, token *models.OAuth2Token) error

	// DeleteToken removes the current OAuth2 token from storage
	DeleteToken(ctx context.Context) error
}

// Repository error definitions
var (
	ErrTokenNotFound = fmt.Errorf("OAuth2 token not found in storage")
	ErrInvalidToken  = fmt.Errorf("invalid OAuth2 token provided")
)
