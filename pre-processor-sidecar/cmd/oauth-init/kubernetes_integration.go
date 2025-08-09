// ABOUTME: This file adds Kubernetes Secret integration to oauth-init tool
// ABOUTME: Automatically creates/updates OAuth2 token secrets in Kubernetes

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

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

// KubernetesSecretManager manages OAuth2 tokens in Kubernetes Secrets
type KubernetesSecretManager struct {
	client     kubernetes.Interface
	namespace  string
	secretName string
}

// NewKubernetesSecretManager creates a new Kubernetes secret manager
func NewKubernetesSecretManager(namespace, secretName string) (*KubernetesSecretManager, error) {
	// Try in-cluster config first, fallback to kubeconfig
	config, err := rest.InClusterConfig()
	if err != nil {
		// Fallback to kubeconfig
		config, err = clientcmd.BuildConfigFromFlags("", "")
		if err != nil {
			return nil, fmt.Errorf("failed to create kubernetes config: %w", err)
		}
	}

	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	return &KubernetesSecretManager{
		client:     client,
		namespace:  namespace,
		secretName: secretName,
	}, nil
}

// CreateOrUpdateTokenSecret creates or updates the OAuth2 token secret
func (k *KubernetesSecretManager) CreateOrUpdateTokenSecret(ctx context.Context, tokens *TokenResponse, clientID, clientSecret string) error {
	// Prepare secret data
	now := time.Now()
	expiresAt := now.Add(time.Duration(tokens.ExpiresIn) * time.Second)
	
	tokenData := TokenSecretData{
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		TokenType:    tokens.TokenType,
		ExpiresIn:    tokens.ExpiresIn,
		ExpiresAt:    expiresAt,
		Scope:        tokens.Scope,
		IssuedAt:     now,
	}

	tokenDataBytes, err := json.Marshal(tokenData)
	if err != nil {
		return fmt.Errorf("failed to marshal token data: %w", err)
	}

	secretData := map[string][]byte{
		"token_data":     tokenDataBytes,
		"access_token":   []byte(base64Encode(tokens.AccessToken)),
		"refresh_token":  []byte(base64Encode(tokens.RefreshToken)),
		"expires_at":     []byte(base64Encode(expiresAt.Format(time.RFC3339))),
	}

	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      k.secretName,
			Namespace: k.namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":      "pre-processor-sidecar",
				"app.kubernetes.io/component": "oauth2-token",
				"app.kubernetes.io/part-of":   "alt-processing",
				"app.kubernetes.io/managed-by": "oauth-init-tool",
			},
			Annotations: map[string]string{
				"oauth2.pre-processor-sidecar/created-at": now.Format(time.RFC3339),
				"oauth2.pre-processor-sidecar/expires-at": expiresAt.Format(time.RFC3339),
				"oauth2.pre-processor-sidecar/token-type": tokens.TokenType,
				"oauth2.pre-processor-sidecar/scope":      tokens.Scope,
			},
		},
		Type: v1.SecretTypeOpaque,
		Data: secretData,
	}

	// Try to update existing secret first
	_, err = k.client.CoreV1().Secrets(k.namespace).Update(ctx, secret, metav1.UpdateOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			// Secret doesn't exist, create it
			_, err = k.client.CoreV1().Secrets(k.namespace).Create(ctx, secret, metav1.CreateOptions{})
			if err != nil {
				return fmt.Errorf("failed to create OAuth2 token secret: %w", err)
			}
			fmt.Println("✅ Created OAuth2 token secret in Kubernetes")
		} else {
			return fmt.Errorf("failed to update OAuth2 token secret: %w", err)
		}
	} else {
		fmt.Println("✅ Updated OAuth2 token secret in Kubernetes")
	}

	return nil
}

// VerifySecretAccess verifies that the oauth-init tool can access the secret
func (k *KubernetesSecretManager) VerifySecretAccess(ctx context.Context) error {
	_, err := k.client.CoreV1().Secrets(k.namespace).Get(ctx, k.secretName, metav1.GetOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to verify secret access: %w", err)
	}
	return nil
}

// DisplayKubernetesInstructions shows Kubernetes-specific instructions
func DisplayKubernetesInstructions(namespace, secretName string) {
	fmt.Println("📋 Kubernetes Integration Complete!")
	fmt.Println()
	fmt.Println("🔍 Verify token secret:")
	fmt.Printf("kubectl get secret %s -n %s -o yaml\n", secretName, namespace)
	fmt.Println()
	fmt.Println("🔍 Check token expiration:")
	fmt.Printf("kubectl get secret %s -n %s -o jsonpath='{.metadata.annotations.oauth2\\.pre-processor-sidecar/expires-at}'\n", secretName, namespace)
	fmt.Println()
	fmt.Println("🚀 Deploy pre-processor-sidecar:")
	fmt.Println("The CronJob will now automatically use the stored OAuth2 token!")
}