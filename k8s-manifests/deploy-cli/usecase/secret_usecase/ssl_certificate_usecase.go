package secret_usecase

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"time"

	"deploy-cli/domain"
	"deploy-cli/port/logger_port"
)

// SSLCertificateUsecase handles SSL certificate generation and management
type SSLCertificateUsecase struct {
	secretUsecase *SecretUsecase
	logger        logger_port.LoggerPort
}

// NewSSLCertificateUsecase creates a new SSL certificate usecase
func NewSSLCertificateUsecase(secretUsecase *SecretUsecase, logger logger_port.LoggerPort) *SSLCertificateUsecase {
	return &SSLCertificateUsecase{
		secretUsecase: secretUsecase,
		logger:        logger,
	}
}

// SSLCertificateConfig represents SSL certificate configuration
type SSLCertificateConfig struct {
	ServiceName  string
	Namespace    string
	Environment  domain.Environment
	DNSNames     []string
	IPAddresses  []net.IP
	ValidityDays int
}

// CreateMeiliSearchSSLCertificate creates SSL certificate for MeiliSearch
func (u *SSLCertificateUsecase) CreateMeiliSearchSSLCertificate(ctx context.Context, namespace string, env domain.Environment) error {
	u.logger.InfoWithContext("creating MeiliSearch SSL certificate", map[string]interface{}{
		"namespace":   namespace,
		"environment": env.String(),
	})

	config := &SSLCertificateConfig{
		ServiceName:  "meilisearch",
		Namespace:    namespace,
		Environment:  env,
		DNSNames: []string{
			"meilisearch",
			fmt.Sprintf("meilisearch.%s", namespace),
			fmt.Sprintf("meilisearch.%s.svc", namespace),
			fmt.Sprintf("meilisearch.%s.svc.cluster.local", namespace),
			"localhost",
		},
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1)},
		ValidityDays: 365,
	}

	return u.createSSLCertificate(ctx, config)
}

// createSSLCertificate generates and creates SSL certificate secret
func (u *SSLCertificateUsecase) createSSLCertificate(ctx context.Context, config *SSLCertificateConfig) error {
	// Generate private key
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return fmt.Errorf("failed to generate private key: %w", err)
	}

	// Create certificate template
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization:  []string{"Alt RSS Reader"},
			Country:       []string{"JP"},
			Province:      []string{""},
			Locality:      []string{""},
			StreetAddress: []string{""},
			PostalCode:    []string{""},
			CommonName:    fmt.Sprintf("%s.%s.svc.cluster.local", config.ServiceName, config.Namespace),
		},
		DNSNames:     config.DNSNames,
		IPAddresses:  config.IPAddresses,
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Duration(config.ValidityDays) * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	// Generate certificate
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return fmt.Errorf("failed to create certificate: %w", err)
	}

	// Encode certificate to PEM
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})

	// Encode private key to PEM
	privateKeyDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return fmt.Errorf("failed to marshal private key: %w", err)
	}

	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: privateKeyDER,
	})

	// Create CA certificate (self-signed for now)
	caPEM := certPEM

	// Create secret
	secretName := fmt.Sprintf("%s-ssl-certs-prod", config.ServiceName)
	secret := domain.NewSecret(secretName, config.Namespace, domain.SSLSecret)
	secret.AddData("server.crt", string(certPEM))
	secret.AddData("server.key", string(privateKeyPEM))
	secret.AddData("ca.crt", string(caPEM))

	// Add labels for management
	secret.Labels["app.kubernetes.io/name"] = config.ServiceName
	secret.Labels["app.kubernetes.io/component"] = "ssl-certificate"
	secret.Labels["app.kubernetes.io/environment"] = config.Environment.String()
	secret.Labels["deploy-cli/managed"] = "true"

	u.logger.InfoWithContext("SSL certificate generated successfully", map[string]interface{}{
		"secret_name":    secretName,
		"namespace":      config.Namespace,
		"dns_names":      config.DNSNames,
		"validity_days":  config.ValidityDays,
	})

	// Use secret usecase to create the secret
	return u.secretUsecase.CreateSecret(ctx, secret)
}

// ValidateSSLCertificate validates an existing SSL certificate
func (u *SSLCertificateUsecase) ValidateSSLCertificate(ctx context.Context, secretName, namespace string) error {
	u.logger.InfoWithContext("validating SSL certificate", map[string]interface{}{
		"secret_name": secretName,
		"namespace":   namespace,
	})

	// Get the secret
	secret, err := u.secretUsecase.GetSecret(ctx, secretName, namespace)
	if err != nil {
		return fmt.Errorf("failed to get SSL secret: %w", err)
	}

	// Validate certificate data
	certData, exists := secret.GetData("server.crt")
	if !exists {
		return fmt.Errorf("certificate data not found in secret")
	}

	keyData, exists := secret.GetData("server.key")
	if !exists {
		return fmt.Errorf("private key data not found in secret")
	}

	// Parse certificate
	certBlock, _ := pem.Decode([]byte(certData))
	if certBlock == nil {
		return fmt.Errorf("failed to decode certificate PEM")
	}

	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse certificate: %w", err)
	}

	// Parse private key
	keyBlock, _ := pem.Decode([]byte(keyData))
	if keyBlock == nil {
		return fmt.Errorf("failed to decode private key PEM")
	}

	_, err = x509.ParsePKCS8PrivateKey(keyBlock.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse private key: %w", err)
	}

	// Check certificate validity
	now := time.Now()
	if now.Before(cert.NotBefore) {
		return fmt.Errorf("certificate is not yet valid")
	}
	if now.After(cert.NotAfter) {
		return fmt.Errorf("certificate has expired")
	}

	// Check if certificate expires soon (within 30 days)
	if now.Add(30 * 24 * time.Hour).After(cert.NotAfter) {
		u.logger.WarnWithContext("certificate expires soon", map[string]interface{}{
			"secret_name": secretName,
			"namespace":   namespace,
			"expires_at":  cert.NotAfter,
		})
	}

	u.logger.InfoWithContext("SSL certificate validation successful", map[string]interface{}{
		"secret_name": secretName,
		"namespace":   namespace,
		"subject":     cert.Subject.CommonName,
		"expires_at":  cert.NotAfter,
		"dns_names":   cert.DNSNames,
	})

	return nil
}

// ListSSLCertificates lists all SSL certificates managed by deploy-cli
func (u *SSLCertificateUsecase) ListSSLCertificates(ctx context.Context, namespace string) ([]domain.SecretInfo, error) {
	u.logger.InfoWithContext("listing SSL certificates", map[string]interface{}{
		"namespace": namespace,
	})

	// Get all secrets in namespace
	secrets, err := u.secretUsecase.ListSecretsInNamespace(ctx, namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to list secrets: %w", err)
	}

	var sslSecrets []domain.SecretInfo
	for _, secret := range secrets {
		// Check if it's an SSL certificate secret
		if secret.Type == string(domain.SSLSecret) || 
		   (secret.Labels != nil && secret.Labels["deploy-cli/managed"] == "true" && 
		    secret.Labels["app.kubernetes.io/component"] == "ssl-certificate") {
			owner := ""
			if secret.Labels != nil {
				owner = secret.Labels["app.kubernetes.io/name"]
			}
			sslSecrets = append(sslSecrets, domain.SecretInfo{
				Name:      secret.Name,
				Namespace: secret.Namespace,
				Type:      secret.Type,
				Owner:     owner,
			})
		}
	}

	u.logger.InfoWithContext("SSL certificates listed", map[string]interface{}{
		"namespace": namespace,
		"count":     len(sslSecrets),
	})

	return sslSecrets, nil
}