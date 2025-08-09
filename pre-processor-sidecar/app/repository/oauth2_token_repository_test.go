// TDD TEST FILE: OAuth2TokenRepository のテスト
// RED-GREEN-REFACTOR アプローチでインターフェースの動作を定義

package repository

import (
	"context"
	"testing"
	"time"

	"pre-processor-sidecar/models"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOAuth2TokenRepository_GetCurrentToken tests token retrieval
func TestOAuth2TokenRepository_GetCurrentToken(t *testing.T) {
	tests := map[string]struct {
		setupFunc     func() OAuth2TokenRepository
		expectedError bool
		validateFunc  func(*testing.T, *models.OAuth2Token, error)
	}{
		"valid_token_exists": {
			setupFunc: func() OAuth2TokenRepository {
				// Will implement SecretBasedTokenRepository here
				return NewInMemoryTokenRepository() // Test implementation
			},
			expectedError: false,
			validateFunc: func(t *testing.T, token *models.OAuth2Token, err error) {
				require.NoError(t, err)
				assert.NotNil(t, token)
				assert.NotEmpty(t, token.AccessToken)
				assert.NotEmpty(t, token.RefreshToken)
			},
		},
		"no_token_exists": {
			setupFunc: func() OAuth2TokenRepository {
				return NewEmptyTokenRepository() // Test implementation
			},
			expectedError: true,
			validateFunc: func(t *testing.T, token *models.OAuth2Token, err error) {
				require.Error(t, err)
				assert.Nil(t, token)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			repo := tc.setupFunc()
			ctx := context.Background()
			
			token, err := repo.GetCurrentToken(ctx)
			tc.validateFunc(t, token, err)
		})
	}
}

// TestOAuth2TokenRepository_SaveToken tests token saving
func TestOAuth2TokenRepository_SaveToken(t *testing.T) {
	tests := map[string]struct {
		token         *models.OAuth2Token
		expectedError bool
	}{
		"valid_token": {
			token: &models.OAuth2Token{
				AccessToken:  "test_access_token",
				RefreshToken: "test_refresh_token",
				TokenType:    "Bearer",
				ExpiresIn:    3600,
				ExpiresAt:    time.Now().Add(1 * time.Hour),
				Scope:        "read",
				IssuedAt:     time.Now(),
			},
			expectedError: false,
		},
		"nil_token": {
			token:         nil,
			expectedError: true,
		},
		"empty_access_token": {
			token: &models.OAuth2Token{
				AccessToken:  "",
				RefreshToken: "test_refresh_token",
				TokenType:    "Bearer",
			},
			expectedError: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			repo := NewInMemoryTokenRepository()
			ctx := context.Background()
			
			err := repo.SaveToken(ctx, tc.token)
			
			if tc.expectedError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				
				// Verify token was saved by retrieving it
				savedToken, err := repo.GetCurrentToken(ctx)
				require.NoError(t, err)
				assert.Equal(t, tc.token.AccessToken, savedToken.AccessToken)
				assert.Equal(t, tc.token.RefreshToken, savedToken.RefreshToken)
			}
		})
	}
}

// TestOAuth2TokenRepository_UpdateToken tests token updating
func TestOAuth2TokenRepository_UpdateToken(t *testing.T) {
	repo := NewInMemoryTokenRepository()
	ctx := context.Background()
	
	// Save initial token
	initialToken := &models.OAuth2Token{
		AccessToken:  "initial_access_token",
		RefreshToken: "initial_refresh_token",
		TokenType:    "Bearer",
		ExpiresIn:    3600,
		ExpiresAt:    time.Now().Add(1 * time.Hour),
		Scope:        "read",
		IssuedAt:     time.Now(),
	}
	
	err := repo.SaveToken(ctx, initialToken)
	require.NoError(t, err)
	
	// Update token
	updatedToken := &models.OAuth2Token{
		AccessToken:  "updated_access_token",
		RefreshToken: "initial_refresh_token", // Refresh token usually stays the same
		TokenType:    "Bearer",
		ExpiresIn:    3600,
		ExpiresAt:    time.Now().Add(1 * time.Hour),
		Scope:        "read",
		IssuedAt:     time.Now(),
	}
	
	err = repo.UpdateToken(ctx, updatedToken)
	require.NoError(t, err)
	
	// Verify token was updated
	retrievedToken, err := repo.GetCurrentToken(ctx)
	require.NoError(t, err)
	assert.Equal(t, "updated_access_token", retrievedToken.AccessToken)
	assert.Equal(t, "initial_refresh_token", retrievedToken.RefreshToken)
}

// TestOAuth2TokenRepository_DeleteToken tests token deletion
func TestOAuth2TokenRepository_DeleteToken(t *testing.T) {
	repo := NewInMemoryTokenRepository()
	ctx := context.Background()
	
	// Save a token first
	token := &models.OAuth2Token{
		AccessToken:  "test_access_token",
		RefreshToken: "test_refresh_token",
		TokenType:    "Bearer",
		ExpiresIn:    3600,
		ExpiresAt:    time.Now().Add(1 * time.Hour),
		Scope:        "read",
		IssuedAt:     time.Now(),
	}
	
	err := repo.SaveToken(ctx, token)
	require.NoError(t, err)
	
	// Delete the token
	err = repo.DeleteToken(ctx)
	require.NoError(t, err)
	
	// Verify token was deleted
	_, err = repo.GetCurrentToken(ctx)
	require.Error(t, err)
}

// --- TEST IMPLEMENTATIONS ---

// InMemoryTokenRepository is a test implementation for OAuth2TokenRepository
type InMemoryTokenRepository struct {
	token *models.OAuth2Token
}

// NewInMemoryTokenRepository creates a new in-memory token repository with a test token
func NewInMemoryTokenRepository() OAuth2TokenRepository {
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

// NewEmptyTokenRepository creates a new in-memory token repository without a token
func NewEmptyTokenRepository() OAuth2TokenRepository {
	return &InMemoryTokenRepository{
		token: nil,
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

// Repository errors
var (
	ErrTokenNotFound = assert.AnError // Will define proper errors later
	ErrInvalidToken  = assert.AnError
)