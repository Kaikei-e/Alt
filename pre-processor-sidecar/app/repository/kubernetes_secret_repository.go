// ABOUTME: Kubernetes Secret-based OAuth2TokenRepository implementation
// ABOUTME: Provides persistent storage for OAuth2 tokens with rotation support

package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"pre-processor-sidecar/models"
	
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// KubernetesSecretRepository implements OAuth2TokenRepository using Kubernetes Secrets
type KubernetesSecretRepository struct {
	clientset  kubernetes.Interface
	namespace  string
	secretName string
	logger     *slog.Logger
}

// NewKubernetesSecretRepository creates a new Kubernetes Secret-based token repository
func NewKubernetesSecretRepository(
	namespace, secretName string,
	logger *slog.Logger,
) (*KubernetesSecretRepository, error) {
	if logger == nil {
		logger = slog.Default()
	}

	// Create in-cluster config (for Pod environment)
	config, err := rest.InClusterConfig()
	if err != nil {
		logger.Error("Failed to create in-cluster config", "error", err)
		return nil, fmt.Errorf("failed to create Kubernetes config: %w", err)
	}

	// Create clientset from config
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		logger.Error("Failed to create Kubernetes clientset", "error", err)
		return nil, fmt.Errorf("failed to create Kubernetes clientset: %w", err)
	}

	return &KubernetesSecretRepository{
		clientset:  clientset,
		namespace:  namespace,
		secretName: secretName,
		logger:     logger,
	}, nil
}

// NewKubernetesSecretRepositoryWithClientset creates a repository with custom clientset (for testing)
func NewKubernetesSecretRepositoryWithClientset(
	clientset kubernetes.Interface,
	namespace, secretName string,
	logger *slog.Logger,
) *KubernetesSecretRepository {
	if logger == nil {
		logger = slog.Default()
	}

	return &KubernetesSecretRepository{
		clientset:  clientset,
		namespace:  namespace,
		secretName: secretName,
		logger:     logger,
	}
}

// GetCurrentToken retrieves the current OAuth2 token from Kubernetes Secret
func (r *KubernetesSecretRepository) GetCurrentToken(ctx context.Context) (*models.OAuth2Token, error) {
	r.logger.Debug("Retrieving OAuth2 token from Kubernetes Secret",
		"namespace", r.namespace,
		"secret_name", r.secretName)

	secret, err := r.clientset.CoreV1().Secrets(r.namespace).Get(
		ctx, r.secretName, metav1.GetOptions{})
	if err != nil {
		r.logger.Error("Failed to retrieve secret from Kubernetes",
			"error", err,
			"namespace", r.namespace,
			"secret_name", r.secretName)
		return nil, fmt.Errorf("failed to retrieve token secret: %w", err)
	}

	// Extract token data from secret
	tokenDataBytes, exists := secret.Data["token_data"]
	if !exists {
		r.logger.Error("Token data not found in secret", "secret_name", r.secretName)
		return nil, ErrTokenNotFound
	}

	// Parse JSON token data
	var token models.OAuth2Token
	if err := json.Unmarshal(tokenDataBytes, &token); err != nil {
		r.logger.Error("Failed to parse token data from secret", "error", err)
		return nil, fmt.Errorf("invalid token data in secret: %w", err)
	}

	r.logger.Info("Successfully retrieved OAuth2 token from Kubernetes Secret",
		"expires_at", token.ExpiresAt,
		"time_until_expiry", token.TimeUntilExpiry(),
		"is_expired", token.IsExpired())

	return &token, nil
}

// SaveToken stores a new OAuth2 token to Kubernetes Secret
func (r *KubernetesSecretRepository) SaveToken(ctx context.Context, token *models.OAuth2Token) error {
	if token == nil || token.AccessToken == "" {
		return ErrInvalidToken
	}

	r.logger.Info("Saving OAuth2 token to Kubernetes Secret",
		"secret_name", r.secretName,
		"expires_at", token.ExpiresAt)

	// Serialize token to JSON
	tokenBytes, err := json.Marshal(token)
	if err != nil {
		return fmt.Errorf("failed to serialize token: %w", err)
	}

	// Create or update secret data
	secretData := map[string][]byte{
		"token_data":    tokenBytes,
		"access_token":  []byte(token.AccessToken),
		"refresh_token": []byte(token.RefreshToken),
		"expires_at":    []byte(token.ExpiresAt.Format(time.RFC3339)),
	}

	// Try to get existing secret first
	_, err = r.clientset.CoreV1().Secrets(r.namespace).Get(
		ctx, r.secretName, metav1.GetOptions{})
	
	if err != nil {
		// Secret doesn't exist, create it
		return r.createSecret(ctx, secretData)
	} else {
		// Secret exists, update it
		return r.updateSecret(ctx, secretData)
	}
}

// UpdateToken updates an existing OAuth2 token in Kubernetes Secret
func (r *KubernetesSecretRepository) UpdateToken(ctx context.Context, token *models.OAuth2Token) error {
	if token == nil || token.AccessToken == "" {
		return ErrInvalidToken
	}

	r.logger.Info("Updating OAuth2 token in Kubernetes Secret",
		"secret_name", r.secretName,
		"expires_at", token.ExpiresAt,
		"refresh_token_changed", true) // Log when refresh token might have changed

	// For update, always use the same logic as save to ensure consistency
	return r.SaveToken(ctx, token)
}

// UpdateWithRefreshRotation specifically handles refresh token rotation
func (r *KubernetesSecretRepository) UpdateWithRefreshRotation(
	ctx context.Context, 
	newToken *models.OAuth2Token,
	oldRefreshToken string,
) error {
	r.logger.Info("Updating OAuth2 token with refresh token rotation",
		"secret_name", r.secretName,
		"old_refresh_token_prefix", oldRefreshToken[:min(8, len(oldRefreshToken))],
		"new_refresh_token_prefix", newToken.RefreshToken[:min(8, len(newToken.RefreshToken))],
		"refresh_token_rotated", oldRefreshToken != newToken.RefreshToken)

	// Add rotation metadata for auditing
	rotationData := map[string]interface{}{
		"rotation_timestamp": time.Now().Format(time.RFC3339),
		"old_refresh_token_hash": hashToken(oldRefreshToken),
		"new_refresh_token_hash": hashToken(newToken.RefreshToken),
		"rotation_detected": oldRefreshToken != newToken.RefreshToken,
	}

	// Serialize rotation metadata
	rotationBytes, _ := json.Marshal(rotationData)

	// Update token with rotation tracking
	tokenBytes, err := json.Marshal(newToken)
	if err != nil {
		return fmt.Errorf("failed to serialize token: %w", err)
	}

	secretData := map[string][]byte{
		"token_data":        tokenBytes,
		"access_token":      []byte(newToken.AccessToken),
		"refresh_token":     []byte(newToken.RefreshToken),
		"expires_at":        []byte(newToken.ExpiresAt.Format(time.RFC3339)),
		"rotation_metadata": rotationBytes,
	}

	return r.updateSecret(ctx, secretData)
}

// DeleteToken removes the OAuth2 token from Kubernetes Secret
func (r *KubernetesSecretRepository) DeleteToken(ctx context.Context) error {
	r.logger.Info("Deleting OAuth2 token from Kubernetes Secret", "secret_name", r.secretName)

	err := r.clientset.CoreV1().Secrets(r.namespace).Delete(
		ctx, r.secretName, metav1.DeleteOptions{})
	if err != nil {
		r.logger.Error("Failed to delete secret", "error", err)
		return fmt.Errorf("failed to delete token secret: %w", err)
	}

	r.logger.Info("Successfully deleted OAuth2 token secret")
	return nil
}

// createSecret creates a new secret with the given data
func (r *KubernetesSecretRepository) createSecret(ctx context.Context, data map[string][]byte) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.secretName,
			Namespace: r.namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":       "pre-processor-sidecar",
				"app.kubernetes.io/component":  "oauth2-token",
				"app.kubernetes.io/part-of":    "alt-processing",
				"app.kubernetes.io/managed-by": "pre-processor-sidecar",
			},
			Annotations: map[string]string{
				"pre-processor-sidecar/last-updated": time.Now().Format(time.RFC3339),
				"pre-processor-sidecar/token-version": "1",
			},
		},
		Type: corev1.SecretTypeOpaque,
		Data: data,
	}

	_, err := r.clientset.CoreV1().Secrets(r.namespace).Create(ctx, secret, metav1.CreateOptions{})
	if err != nil {
		r.logger.Error("Failed to create secret", "error", err)
		return fmt.Errorf("failed to create token secret: %w", err)
	}

	r.logger.Info("Successfully created OAuth2 token secret")
	return nil
}

// updateSecret updates an existing secret with the given data
func (r *KubernetesSecretRepository) updateSecret(ctx context.Context, data map[string][]byte) error {
	// Get current secret to preserve metadata and handle optimistic locking
	currentSecret, err := r.clientset.CoreV1().Secrets(r.namespace).Get(
		ctx, r.secretName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get current secret for update: %w", err)
	}

	// Update data and metadata
	currentSecret.Data = data
	if currentSecret.Annotations == nil {
		currentSecret.Annotations = make(map[string]string)
	}
	currentSecret.Annotations["pre-processor-sidecar/last-updated"] = time.Now().Format(time.RFC3339)
	
	// Increment token version for tracking
	currentVersion := currentSecret.Annotations["pre-processor-sidecar/token-version"]
	if currentVersion == "" {
		currentSecret.Annotations["pre-processor-sidecar/token-version"] = "1"
	} else {
		// Simple version increment - in production might want semantic versioning
		currentSecret.Annotations["pre-processor-sidecar/token-version"] = fmt.Sprintf("%d", time.Now().Unix())
	}

	_, err = r.clientset.CoreV1().Secrets(r.namespace).Update(ctx, currentSecret, metav1.UpdateOptions{})
	if err != nil {
		r.logger.Error("Failed to update secret", "error", err)
		return fmt.Errorf("failed to update token secret: %w", err)
	}

	r.logger.Info("Successfully updated OAuth2 token secret",
		"version", currentSecret.Annotations["pre-processor-sidecar/token-version"])
	return nil
}

// IsHealthy checks if the repository can access Kubernetes API
func (r *KubernetesSecretRepository) IsHealthy(ctx context.Context) error {
	// Test connectivity by trying to get the secret (or check if it doesn't exist)
	_, err := r.clientset.CoreV1().Secrets(r.namespace).Get(
		ctx, r.secretName, metav1.GetOptions{})
	
	if err != nil {
		// If secret doesn't exist, that's fine - we can create it
		// If it's an auth or connectivity error, that's a problem
		if !isNotFoundError(err) {
			return fmt.Errorf("Kubernetes API connectivity check failed: %w", err)
		}
	}

	return nil
}

// GetStoragePath returns description of storage location
func (r *KubernetesSecretRepository) GetStoragePath() string {
	return fmt.Sprintf("Kubernetes Secret %s/%s", r.namespace, r.secretName)
}

// Helper function to hash tokens for audit logs (first 8 chars of hash)
func hashToken(token string) string {
	if len(token) < 8 {
		return "****"
	}
	// Simple hash for audit - not cryptographic security
	return fmt.Sprintf("%x", token[:4])[:8]
}

// Helper function to check if error is NotFound
func isNotFoundError(err error) bool {
	// Simple check - in production might want more sophisticated error type checking
	return err != nil && (
		err.Error() == "not found" || 
		strings.Contains(err.Error(), "not found") ||
		strings.Contains(err.Error(), "NotFound"))
}

// Helper function for min
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}