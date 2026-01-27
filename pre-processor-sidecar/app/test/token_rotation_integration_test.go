// ABOUTME: Integration tests for token rotation functionality
// ABOUTME: Tests end-to-end token refresh, rotation detection, and storage updates

package test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"pre-processor-sidecar/driver"
	"pre-processor-sidecar/models"
	"pre-processor-sidecar/repository"
	"pre-processor-sidecar/service"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestTokenRotation_WithRefreshTokenRotation(t *testing.T) {
	// Setup mock OAuth2 server that rotates refresh tokens
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth2/token" {
			// Return new tokens with rotated refresh token
			response := map[string]interface{}{
				"access_token":  "new-access-token",
				"token_type":    "Bearer",
				"expires_in":    3600,
				"refresh_token": "new-refresh-token", // Simulated rotation
				"scope":         "read",
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
			return
		}

		if r.URL.Path == "/user-info" {
			// Mock token validation endpoint
			auth := r.Header.Get("Authorization")
			if strings.Contains(auth, "new-access-token") {
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(map[string]string{"status": "valid"})
			} else {
				w.WriteHeader(http.StatusUnauthorized)
			}
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	// Setup components
	logger := slog.Default()
	namespace := "test-namespace"
	secretName := "test-secret"

	// Create initial token
	initialToken := &models.OAuth2Token{
		AccessToken:  "initial-access-token",
		RefreshToken: "initial-refresh-token",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(-1 * time.Minute), // Expired to force refresh
		IssuedAt:     time.Now().Add(-2 * time.Hour),
	}

	// Create fake Kubernetes client with initial secret
	fakeClient := fake.NewSimpleClientset(createTestSecret(namespace, secretName, initialToken))

	// Create repository
	repo := createTestKubernetesSecretRepository(fakeClient, namespace, secretName, logger)

	// Create OAuth2 client pointing to mock server
	oauth2Client := driver.NewOAuth2Client("test-client", "test-secret", server.URL, logger)

	// Create token management service
	tokenService := service.NewTokenManagementService(repo, oauth2Client, logger)

	// Test: Ensure valid token (should trigger refresh with rotation)
	validToken, err := tokenService.EnsureValidToken(context.Background())

	// Assertions
	require.NoError(t, err)
	assert.Equal(t, "new-access-token", validToken.AccessToken)
	assert.Equal(t, "new-refresh-token", validToken.RefreshToken)

	// Verify token was persisted with rotation
	persistedToken, err := repo.GetCurrentToken(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "new-access-token", persistedToken.AccessToken)
	assert.Equal(t, "new-refresh-token", persistedToken.RefreshToken)

	// Verify rotation metadata was stored
	secret, err := fakeClient.CoreV1().Secrets(namespace).Get(
		context.Background(), secretName, metav1.GetOptions{})
	require.NoError(t, err)
	assert.NotNil(t, secret.Data["rotation_metadata"])
}

func TestTokenRotation_WithoutRefreshTokenRotation(t *testing.T) {
	// Setup mock OAuth2 server that does NOT rotate refresh tokens
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth2/token" {
			// Return new access token but no new refresh token
			response := map[string]interface{}{
				"access_token": "new-access-token",
				"token_type":   "Bearer",
				"expires_in":   3600,
				// No refresh_token in response - simulates no rotation
				"scope": "read",
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	// Setup components
	logger := slog.Default()
	namespace := "test-namespace"
	secretName := "test-secret"

	// Create initial token
	initialToken := &models.OAuth2Token{
		AccessToken:  "initial-access-token",
		RefreshToken: "persistent-refresh-token",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(-1 * time.Minute), // Expired to force refresh
		IssuedAt:     time.Now().Add(-2 * time.Hour),
	}

	// Create fake Kubernetes client with initial secret
	fakeClient := fake.NewSimpleClientset(createTestSecret(namespace, secretName, initialToken))

	// Create repository
	repo := createTestKubernetesSecretRepository(fakeClient, namespace, secretName, logger)

	// Create OAuth2 client pointing to mock server
	oauth2Client := driver.NewOAuth2Client("test-client", "test-secret", server.URL, logger)

	// Create token management service
	tokenService := service.NewTokenManagementService(repo, oauth2Client, logger)

	// Test: Ensure valid token (should refresh without rotation)
	validToken, err := tokenService.EnsureValidToken(context.Background())

	// Assertions
	require.NoError(t, err)
	assert.Equal(t, "new-access-token", validToken.AccessToken)
	assert.Equal(t, "persistent-refresh-token", validToken.RefreshToken) // Should remain unchanged

	// Verify token was persisted without rotation
	persistedToken, err := repo.GetCurrentToken(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "new-access-token", persistedToken.AccessToken)
	assert.Equal(t, "persistent-refresh-token", persistedToken.RefreshToken)
}

func TestTokenRotation_ErrorRecovery(t *testing.T) {
	// Setup mock OAuth2 server that returns errors
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth2/token" {
			// Return error response
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{
				"error":             "invalid_grant",
				"error_description": "The provided authorization grant is invalid, expired, revoked, does not match the redirection URI used in the authorization request, or was issued to another client.",
			})
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	// Setup components
	logger := slog.Default()
	namespace := "test-namespace"
	secretName := "test-secret"

	// Create expired token
	expiredToken := &models.OAuth2Token{
		AccessToken:  "expired-access-token",
		RefreshToken: "invalid-refresh-token",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(-1 * time.Hour), // Expired
		IssuedAt:     time.Now().Add(-2 * time.Hour),
	}

	// Create fake Kubernetes client with expired token
	fakeClient := fake.NewSimpleClientset(createTestSecret(namespace, secretName, expiredToken))

	// Create repository
	repo := createTestKubernetesSecretRepository(fakeClient, namespace, secretName, logger)

	// Create OAuth2 client pointing to mock server
	oauth2Client := driver.NewOAuth2Client("test-client", "test-secret", server.URL, logger)

	// Create token management service
	tokenService := service.NewTokenManagementService(repo, oauth2Client, logger)

	// Test: Ensure valid token (should fail due to invalid refresh token)
	_, err := tokenService.EnsureValidToken(context.Background())

	// Assertions
	require.Error(t, err)
	assert.Contains(t, err.Error(), "token refresh failed")
}

// Helper function to create test Kubernetes Secret repository
func createTestKubernetesSecretRepository(
	fakeClient *fake.Clientset,
	namespace, secretName string,
	logger *slog.Logger,
) *repository.KubernetesSecretRepository {
	// Use the test constructor with fake clientset
	return repository.NewKubernetesSecretRepositoryWithClientset(fakeClient, namespace, secretName, logger)
}

// Helper function to create test secret (reuse from main test)
func createTestSecret(namespace, secretName string, token *models.OAuth2Token) *corev1.Secret {
	tokenBytes, _ := json.Marshal(token)

	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":      "pre-processor-sidecar",
				"app.kubernetes.io/component": "oauth2-token",
			},
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
