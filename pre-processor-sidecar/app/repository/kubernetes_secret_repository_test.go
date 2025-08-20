// ABOUTME: Tests for KubernetesSecretTokenRepository implementation
// ABOUTME: Covers token storage, retrieval, rotation, and error scenarios

package repository

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"pre-processor-sidecar/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestKubernetesSecretRepository_GetCurrentToken_Success(t *testing.T) {
	// Setup
	namespace := "test-namespace"
	secretName := "test-secret"
	
	token := &models.OAuth2Token{
		AccessToken:  "test-access-token",
		RefreshToken: "test-refresh-token",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(1 * time.Hour),
		IssuedAt:     time.Now(),
	}

	// Create fake Kubernetes client with pre-existing secret
	fakeClient := fake.NewSimpleClientset(createTestSecret(namespace, secretName, token))
	
	repo := NewKubernetesSecretRepositoryWithClientset(fakeClient, namespace, secretName, nil)

	// Test
	retrievedToken, err := repo.GetCurrentToken(context.Background())

	// Assertions
	require.NoError(t, err)
	assert.Equal(t, token.AccessToken, retrievedToken.AccessToken)
	assert.Equal(t, token.RefreshToken, retrievedToken.RefreshToken)
	assert.Equal(t, token.TokenType, retrievedToken.TokenType)
}

func TestKubernetesSecretRepository_GetCurrentToken_NotFound(t *testing.T) {
	// Setup
	namespace := "test-namespace"
	secretName := "test-secret"
	
	// Create fake Kubernetes client with no secrets
	fakeClient := fake.NewSimpleClientset()
	
	repo := NewKubernetesSecretRepositoryWithClientset(fakeClient, namespace, secretName, nil)

	// Test
	_, err := repo.GetCurrentToken(context.Background())

	// Assertions
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to retrieve token secret")
}

func TestKubernetesSecretRepository_SaveToken_NewSecret(t *testing.T) {
	// Setup
	namespace := "test-namespace"
	secretName := "test-secret"
	
	token := &models.OAuth2Token{
		AccessToken:  "test-access-token",
		RefreshToken: "test-refresh-token",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(1 * time.Hour),
		IssuedAt:     time.Now(),
	}

	// Create fake Kubernetes client with no secrets
	fakeClient := fake.NewSimpleClientset()
	
	repo := NewKubernetesSecretRepositoryWithClientset(fakeClient, namespace, secretName, nil)

	// Test
	err := repo.SaveToken(context.Background(), token)

	// Assertions
	require.NoError(t, err)
	
	// Verify secret was created
	secret, err := fakeClient.CoreV1().Secrets(namespace).Get(
		context.Background(), secretName, metav1.GetOptions{})
	require.NoError(t, err)
	assert.NotNil(t, secret.Data["token_data"])
	assert.NotNil(t, secret.Data["access_token"])
	assert.NotNil(t, secret.Data["refresh_token"])
}

func TestKubernetesSecretRepository_UpdateToken_ExistingSecret(t *testing.T) {
	// Setup
	namespace := "test-namespace"
	secretName := "test-secret"
	
	oldToken := &models.OAuth2Token{
		AccessToken:  "old-access-token",
		RefreshToken: "old-refresh-token",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(30 * time.Minute),
		IssuedAt:     time.Now().Add(-30 * time.Minute),
	}

	newToken := &models.OAuth2Token{
		AccessToken:  "new-access-token",
		RefreshToken: "new-refresh-token",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(2 * time.Hour),
		IssuedAt:     time.Now(),
	}

	// Create fake Kubernetes client with existing secret
	fakeClient := fake.NewSimpleClientset(createTestSecret(namespace, secretName, oldToken))
	
	repo := NewKubernetesSecretRepositoryWithClientset(fakeClient, namespace, secretName, nil)

	// Test
	err := repo.UpdateToken(context.Background(), newToken)

	// Assertions
	require.NoError(t, err)
	
	// Verify secret was updated
	_, err = fakeClient.CoreV1().Secrets(namespace).Get(
		context.Background(), secretName, metav1.GetOptions{})
	require.NoError(t, err)
	
	// Verify updated token data
	retrievedToken, err := repo.GetCurrentToken(context.Background())
	require.NoError(t, err)
	assert.Equal(t, newToken.AccessToken, retrievedToken.AccessToken)
	assert.Equal(t, newToken.RefreshToken, retrievedToken.RefreshToken)
}

func TestKubernetesSecretRepository_UpdateWithRefreshRotation(t *testing.T) {
	// Setup
	namespace := "test-namespace"
	secretName := "test-secret"
	
	oldRefreshToken := "old-refresh-token"
	newToken := &models.OAuth2Token{
		AccessToken:  "new-access-token",
		RefreshToken: "new-refresh-token",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(2 * time.Hour),
		IssuedAt:     time.Now(),
	}

	// Create fake Kubernetes client with existing secret
	initialToken := &models.OAuth2Token{
		AccessToken:  "initial-access-token",
		RefreshToken: oldRefreshToken,
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(30 * time.Minute),
		IssuedAt:     time.Now().Add(-30 * time.Minute),
	}
	
	fakeClient := fake.NewSimpleClientset(createTestSecret(namespace, secretName, initialToken))
	
	repo := NewKubernetesSecretRepositoryWithClientset(fakeClient, namespace, secretName, nil)

	// Test
	err := repo.UpdateWithRefreshRotation(context.Background(), newToken, oldRefreshToken)

	// Assertions
	require.NoError(t, err)
	
	// Verify secret was updated with rotation metadata
	secret, err := fakeClient.CoreV1().Secrets(namespace).Get(
		context.Background(), secretName, metav1.GetOptions{})
	require.NoError(t, err)
	
	assert.NotNil(t, secret.Data["rotation_metadata"])
	
	// Verify updated token data
	retrievedToken, err := repo.GetCurrentToken(context.Background())
	require.NoError(t, err)
	assert.Equal(t, newToken.AccessToken, retrievedToken.AccessToken)
	assert.Equal(t, newToken.RefreshToken, retrievedToken.RefreshToken)
}

func TestKubernetesSecretRepository_DeleteToken(t *testing.T) {
	// Setup
	namespace := "test-namespace"
	secretName := "test-secret"
	
	token := &models.OAuth2Token{
		AccessToken:  "test-access-token",
		RefreshToken: "test-refresh-token",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(1 * time.Hour),
		IssuedAt:     time.Now(),
	}

	// Create fake Kubernetes client with existing secret
	fakeClient := fake.NewSimpleClientset(createTestSecret(namespace, secretName, token))
	
	repo := NewKubernetesSecretRepositoryWithClientset(fakeClient, namespace, secretName, nil)

	// Test
	err := repo.DeleteToken(context.Background())

	// Assertions
	require.NoError(t, err)
	
	// Verify secret was deleted
	_, err = fakeClient.CoreV1().Secrets(namespace).Get(
		context.Background(), secretName, metav1.GetOptions{})
	assert.Error(t, err) // Should be not found error
}

func TestKubernetesSecretRepository_SaveToken_InvalidToken(t *testing.T) {
	// Setup
	namespace := "test-namespace"
	secretName := "test-secret"
	
	fakeClient := fake.NewSimpleClientset()
	
	repo := NewKubernetesSecretRepositoryWithClientset(fakeClient, namespace, secretName, nil)

	// Test cases
	testCases := []struct {
		name  string
		token *models.OAuth2Token
	}{
		{
			name:  "nil token",
			token: nil,
		},
		{
			name: "empty access token",
			token: &models.OAuth2Token{
				AccessToken:  "",
				RefreshToken: "test-refresh-token",
				TokenType:    "Bearer",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := repo.SaveToken(context.Background(), tc.token)
			assert.Equal(t, ErrInvalidToken, err)
		})
	}
}

// Helper function to create test secret
func createTestSecret(namespace, secretName string, token *models.OAuth2Token) *corev1.Secret {
	tokenBytes, _ := json.Marshal(token)
	
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"token_data":    tokenBytes,
			"access_token":  []byte(token.AccessToken),
			"refresh_token": []byte(token.RefreshToken),
			"expires_at":    []byte(token.ExpiresAt.Format(time.RFC3339)),
		},
	}
}