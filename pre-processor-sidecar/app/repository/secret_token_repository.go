// ABOUTME: This file implements OAuth2TokenRepository using Kubernetes Secrets
// ABOUTME: Provides secure token storage with base64 encoding and Kubernetes RBAC

package repository

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"pre-processor-sidecar/models"
	
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/apimachinery/pkg/api/errors"
)

// Repository error definitions (additional to base errors)
var (
	ErrStorageError = fmt.Errorf("storage operation failed")
)

// SecretBasedTokenRepository implements OAuth2TokenRepository using Kubernetes Secrets
type SecretBasedTokenRepository struct {
	kubeClient kubernetes.Interface
	namespace  string
	secretName string
}

// TokenSecretData represents the structure stored in Kubernetes Secret
type TokenSecretData struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	TokenType    string    `json:"token_type"`
	ExpiresIn    int       `json:"expires_in"`
	ExpiresAt    time.Time `json:"expires_at"`
	Scope        string    `json:"scope"`
	IssuedAt     time.Time `json:"issued_at"`
}

// NewSecretBasedTokenRepository creates a new Kubernetes Secret-based token repository
func NewSecretBasedTokenRepository(kubeClient kubernetes.Interface, namespace, secretName string) OAuth2TokenRepository {
	return &SecretBasedTokenRepository{
		kubeClient: kubeClient,
		namespace:  namespace,
		secretName: secretName,
	}
}

// GetCurrentToken retrieves the current OAuth2 token from Kubernetes Secret
func (r *SecretBasedTokenRepository) GetCurrentToken(ctx context.Context) (*models.OAuth2Token, error) {
	secret, err := r.kubeClient.CoreV1().Secrets(r.namespace).Get(ctx, r.secretName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, ErrTokenNotFound
		}
		return nil, fmt.Errorf("%w: failed to get secret: %v", ErrStorageError, err)
	}

	// Decode token data from secret
	tokenData, exists := secret.Data["token_data"]
	if !exists {
		return nil, ErrTokenNotFound
	}

	var secretData TokenSecretData
	if err := json.Unmarshal(tokenData, &secretData); err != nil {
		return nil, fmt.Errorf("%w: failed to unmarshal token data: %v", ErrStorageError, err)
	}

	// Convert to OAuth2Token model
	token := &models.OAuth2Token{
		AccessToken:  secretData.AccessToken,
		RefreshToken: secretData.RefreshToken,
		TokenType:    secretData.TokenType,
		ExpiresIn:    secretData.ExpiresIn,
		ExpiresAt:    secretData.ExpiresAt,
		Scope:        secretData.Scope,
		IssuedAt:     secretData.IssuedAt,
	}

	return token, nil
}

// SaveToken stores a new OAuth2 token in Kubernetes Secret
func (r *SecretBasedTokenRepository) SaveToken(ctx context.Context, token *models.OAuth2Token) error {
	if err := r.validateToken(token); err != nil {
		return err
	}

	secretData := TokenSecretData{
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		TokenType:    token.TokenType,
		ExpiresIn:    token.ExpiresIn,
		ExpiresAt:    token.ExpiresAt,
		Scope:        token.Scope,
		IssuedAt:     token.IssuedAt,
	}

	tokenDataBytes, err := json.Marshal(secretData)
	if err != nil {
		return fmt.Errorf("%w: failed to marshal token data: %v", ErrStorageError, err)
	}

	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.secretName,
			Namespace: r.namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":      "pre-processor-sidecar",
				"app.kubernetes.io/component": "oauth2-token",
				"app.kubernetes.io/part-of":   "alt-processing",
			},
		},
		Type: v1.SecretTypeOpaque,
		Data: map[string][]byte{
			"token_data":     tokenDataBytes,
			"access_token":   []byte(base64.StdEncoding.EncodeToString([]byte(token.AccessToken))),
			"refresh_token":  []byte(base64.StdEncoding.EncodeToString([]byte(token.RefreshToken))),
			"expires_at":     []byte(base64.StdEncoding.EncodeToString([]byte(token.ExpiresAt.Format(time.RFC3339)))),
		},
	}

	_, err = r.kubeClient.CoreV1().Secrets(r.namespace).Create(ctx, secret, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("%w: failed to create secret: %v", ErrStorageError, err)
	}

	return nil
}

// UpdateToken updates an existing OAuth2 token in Kubernetes Secret
func (r *SecretBasedTokenRepository) UpdateToken(ctx context.Context, token *models.OAuth2Token) error {
	if err := r.validateToken(token); err != nil {
		return err
	}

	secretData := TokenSecretData{
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		TokenType:    token.TokenType,
		ExpiresIn:    token.ExpiresIn,
		ExpiresAt:    token.ExpiresAt,
		Scope:        token.Scope,
		IssuedAt:     token.IssuedAt,
	}

	tokenDataBytes, err := json.Marshal(secretData)
	if err != nil {
		return fmt.Errorf("%w: failed to marshal token data: %v", ErrStorageError, err)
	}

	// Get existing secret
	secret, err := r.kubeClient.CoreV1().Secrets(r.namespace).Get(ctx, r.secretName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			// If secret doesn't exist, create it
			return r.SaveToken(ctx, token)
		}
		return fmt.Errorf("%w: failed to get existing secret: %v", ErrStorageError, err)
	}

	// Update secret data
	secret.Data = map[string][]byte{
		"token_data":     tokenDataBytes,
		"access_token":   []byte(base64.StdEncoding.EncodeToString([]byte(token.AccessToken))),
		"refresh_token":  []byte(base64.StdEncoding.EncodeToString([]byte(token.RefreshToken))),
		"expires_at":     []byte(base64.StdEncoding.EncodeToString([]byte(token.ExpiresAt.Format(time.RFC3339)))),
	}

	_, err = r.kubeClient.CoreV1().Secrets(r.namespace).Update(ctx, secret, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("%w: failed to update secret: %v", ErrStorageError, err)
	}

	return nil
}

// DeleteToken removes the OAuth2 token from Kubernetes Secret
func (r *SecretBasedTokenRepository) DeleteToken(ctx context.Context) error {
	err := r.kubeClient.CoreV1().Secrets(r.namespace).Delete(ctx, r.secretName, metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("%w: failed to delete secret: %v", ErrStorageError, err)
	}
	return nil
}

// validateToken validates the OAuth2 token before storage operations
func (r *SecretBasedTokenRepository) validateToken(token *models.OAuth2Token) error {
	if token == nil {
		return ErrInvalidToken
	}
	
	if token.AccessToken == "" {
		return fmt.Errorf("%w: access_token is required", ErrInvalidToken)
	}
	
	if token.RefreshToken == "" {
		return fmt.Errorf("%w: refresh_token is required", ErrInvalidToken)
	}
	
	if token.ExpiresAt.IsZero() {
		return fmt.Errorf("%w: expires_at is required", ErrInvalidToken)
	}
	
	return nil
}