package deployment_usecase

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"time"

	"deploy-cli/domain"
	"deploy-cli/port"
	"deploy-cli/port/logger_port"
	"deploy-cli/usecase/secret_usecase"
)

// SSLCertificateManager handles SSL certificate generation and validation
type SSLCertificateManager struct {
	secretUsecase         *secret_usecase.SecretUsecase
	sslUsecase            *secret_usecase.SSLCertificateUsecase
	logger                logger_port.LoggerPort
	generatedCertificates *port.GeneratedCertificates
}

// NewSSLCertificateManager creates a new SSL certificate manager
func NewSSLCertificateManager(
	secretUsecase *secret_usecase.SecretUsecase,
	sslUsecase *secret_usecase.SSLCertificateUsecase,
	logger logger_port.LoggerPort,
) *SSLCertificateManager {
	return &SSLCertificateManager{
		secretUsecase: secretUsecase,
		sslUsecase:    sslUsecase,
		logger:        logger,
	}
}

// GenerateSSLCertificates generates SSL certificates for the application
func (s *SSLCertificateManager) GenerateSSLCertificates(ctx context.Context) error {
	s.logger.InfoWithContext("generating SSL certificates", map[string]interface{}{
		"system": "ssl-certificate-manager",
	})

	// Generate CA private key
	caPrivateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return fmt.Errorf("failed to generate CA private key: %w", err)
	}

	// Create CA certificate template
	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Alt RSS Reader"},
			Country:      []string{"JP"},
			Province:     []string{"Tokyo"},
			Locality:     []string{"Tokyo"},
			CommonName:   "Alt RSS Reader CA",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(5, 0, 0), // 5年間有効
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	// Generate CA certificate
	caCertDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caPrivateKey.PublicKey, caPrivateKey)
	if err != nil {
		return fmt.Errorf("failed to create CA certificate: %w", err)
	}

	// Encode CA certificate to PEM format
	caCertPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: caCertDER,
	})

	// Encode CA private key to PEM format
	caPrivateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(caPrivateKey),
	})

	// Generate server private key
	serverPrivateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return fmt.Errorf("failed to generate server private key: %w", err)
	}

	// Create server certificate template
	serverTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject: pkix.Name{
			Organization: []string{"Alt RSS Reader"},
			Country:      []string{"JP"},
			Province:     []string{"Tokyo"},
			Locality:     []string{"Tokyo"},
			CommonName:   "*.alt-app.local",
		},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().AddDate(1, 0, 0), // 1年間有効
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		KeyUsage:    x509.KeyUsageDigitalSignature,
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:    []string{"localhost", "*.alt-app.local", "alt-backend", "alt-frontend", "auth-service"},
	}

	// Generate server certificate
	serverCertDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, caTemplate, &serverPrivateKey.PublicKey, caPrivateKey)
	if err != nil {
		return fmt.Errorf("failed to create server certificate: %w", err)
	}

	// Encode server certificate to PEM format
	serverCertPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: serverCertDER,
	})

	// Encode server private key to PEM format
	serverPrivateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(serverPrivateKey),
	})

	// Store generated certificates
	s.generatedCertificates = &port.GeneratedCertificates{
		CACert:           string(caCertPEM),
		CAPrivateKey:     string(caPrivateKeyPEM),
		ServerCert:       string(serverCertPEM),
		ServerPrivateKey: string(serverPrivateKeyPEM),
		Generated:        time.Now(),
	}

	s.logger.InfoWithContext("SSL certificates generated successfully", map[string]interface{}{
		"ca_cert_length":     len(caCertPEM),
		"server_cert_length": len(serverCertPEM),
		"generated_at":       s.generatedCertificates.Generated,
	})

	return nil
}

// LoadExistingCertificates loads existing SSL certificates from Kubernetes secrets
func (s *SSLCertificateManager) LoadExistingCertificates(ctx context.Context, environment domain.Environment) error {
	s.logger.InfoWithContext("loading existing SSL certificates", map[string]interface{}{
		"environment": environment.String(),
	})

	// Use common-ssl namespace for CA certificate
	caNamespace := s.getNamespaceForService("common-ssl", environment)
	// Use alt-backend namespace for server certificate
	serverNamespace := s.getNamespaceForService("alt-backend", environment)

	// Try to load CA certificate from common-ssl secret
	caSecret, err := s.secretUsecase.GetSecret(ctx, "ca-secret", caNamespace)
	if err != nil {
		s.logger.DebugWithContext("CA certificate secret not found", map[string]interface{}{
			"secret_name": "ca-secret",
			"namespace":   caNamespace,
			"error":       err.Error(),
		})
		return fmt.Errorf("failed to load CA certificate secret: %w", err)
	}

	// Try to load server certificate from one of the SSL secrets
	serverSecret, err := s.secretUsecase.GetSecret(ctx, "alt-backend-ssl-certs-prod", serverNamespace)
	if err != nil {
		s.logger.DebugWithContext("server certificate secret not found", map[string]interface{}{
			"secret_name": "alt-backend-ssl-certs-prod",
			"namespace":   serverNamespace,
			"error":       err.Error(),
		})
		return fmt.Errorf("failed to load server certificate secret: %w", err)
	}

	// Validate that required certificate data exists
	caCert, hasCACert := caSecret.Data["ca.crt"]
	caKey, hasCAKey := caSecret.Data["ca.key"]
	serverCert, hasServerCert := serverSecret.Data["tls.crt"]
	serverKey, hasServerKey := serverSecret.Data["tls.key"]

	if !hasCACert || !hasCAKey || !hasServerCert || !hasServerKey {
		return fmt.Errorf("incomplete certificate data in secrets: ca.crt=%v, ca.key=%v, tls.crt=%v, tls.key=%v",
			hasCACert, hasCAKey, hasServerCert, hasServerKey)
	}

	if len(caCert) == 0 || len(caKey) == 0 || len(serverCert) == 0 || len(serverKey) == 0 {
		return fmt.Errorf("empty certificate data in secrets")
	}

	// Create GeneratedCertificates struct from loaded secrets
	s.generatedCertificates = &port.GeneratedCertificates{
		CACert:           caCert,
		CAPrivateKey:     caKey,
		ServerCert:       serverCert,
		ServerPrivateKey: serverKey,
		Generated:        time.Now(), // Mark as loaded
	}

	s.logger.InfoWithContext("SSL certificates loaded successfully", map[string]interface{}{
		"environment":            environment.String(),
		"ca_cert_length":         len(s.generatedCertificates.CACert),
		"server_cert_length":     len(s.generatedCertificates.ServerCert),
		"ca_has_private_key":     len(s.generatedCertificates.CAPrivateKey) > 0,
		"server_has_private_key": len(s.generatedCertificates.ServerPrivateKey) > 0,
	})

	return nil
}

// ValidateCertificatePEM validates a certificate in PEM format
func (s *SSLCertificateManager) ValidateCertificatePEM(certPEM, certType string) error {
	if certPEM == "" {
		return fmt.Errorf("certificate PEM is empty")
	}

	// Auto-detect and convert certificate format
	normalizedCert, err := s.detectCertificateFormat([]byte(certPEM))
	if err != nil {
		s.logger.ErrorWithContext("certificate format detection failed", map[string]interface{}{
			"cert_type": certType,
			"error":     err.Error(),
		})
		return fmt.Errorf("certificate format detection failed: %w", err)
	}

	// Parse PEM block
	block, _ := pem.Decode([]byte(normalizedCert))
	if block == nil {
		return fmt.Errorf("failed to parse certificate PEM")
	}

	// Parse certificate
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse certificate: %w", err)
	}

	// Check if certificate is expired
	if time.Now().After(cert.NotAfter) {
		return fmt.Errorf("certificate has expired")
	}

	// Check if certificate is not yet valid
	if time.Now().Before(cert.NotBefore) {
		return fmt.Errorf("certificate is not yet valid")
	}

	s.logger.InfoWithContext("certificate validation successful", map[string]interface{}{
		"cert_type":  certType,
		"subject":    cert.Subject.String(),
		"not_before": cert.NotBefore.Format(time.RFC3339),
		"not_after":  cert.NotAfter.Format(time.RFC3339),
		"is_ca":      cert.IsCA,
	})

	return nil
}

// ValidateCertificate validates a certificate in base64 format
func (s *SSLCertificateManager) ValidateCertificate(certBase64, certType string) error {
	if certBase64 == "" {
		return fmt.Errorf("certificate is empty")
	}

	// Decode base64 certificate
	certData, err := base64.StdEncoding.DecodeString(certBase64)
	if err != nil {
		return fmt.Errorf("failed to decode base64 certificate: %w", err)
	}

	// Parse PEM block
	block, _ := pem.Decode(certData)
	if block == nil {
		return fmt.Errorf("failed to parse certificate PEM")
	}

	// Parse certificate
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse certificate: %w", err)
	}

	// Check if certificate is expired
	if time.Now().After(cert.NotAfter) {
		return fmt.Errorf("certificate has expired")
	}

	// Check if certificate is not yet valid
	if time.Now().Before(cert.NotBefore) {
		return fmt.Errorf("certificate is not yet valid")
	}

	s.logger.InfoWithContext("certificate validation successful", map[string]interface{}{
		"cert_type":  certType,
		"subject":    cert.Subject.String(),
		"not_before": cert.NotBefore.Format(time.RFC3339),
		"not_after":  cert.NotAfter.Format(time.RFC3339),
		"is_ca":      cert.IsCA,
	})

	return nil
}

// detectCertificateFormat detects and converts certificate data format
func (s *SSLCertificateManager) detectCertificateFormat(data []byte) (string, error) {
	dataStr := string(data)

	// Check if it's already PEM format
	if isPEMFormat(dataStr) {
		s.logger.DebugWithContext("certificate data detected as PEM format", map[string]interface{}{
			"data_length": len(data),
		})
		return dataStr, nil
	}

	// Check if it's base64 encoded
	if isBase64Encoded(dataStr) {
		s.logger.DebugWithContext("certificate data detected as base64 format", map[string]interface{}{
			"data_length": len(data),
		})
		decoded, err := base64.StdEncoding.DecodeString(dataStr)
		if err != nil {
			return "", fmt.Errorf("failed to decode base64 data: %w", err)
		}
		return string(decoded), nil
	}

	return dataStr, nil
}

// CreateSSLCertificateSecret creates an SSL certificate secret for a specific service
func (s *SSLCertificateManager) CreateSSLCertificateSecret(ctx context.Context, serviceName, secretName, namespace string) error {
	if s.generatedCertificates == nil {
		return fmt.Errorf("no certificates available for secret creation")
	}

	s.logger.InfoWithContext("creating SSL certificate secret", map[string]interface{}{
		"service":     serviceName,
		"secret_name": secretName,
		"namespace":   namespace,
	})

	// Create secret with SSL certificate data
	secret := domain.NewSecret(secretName, namespace, domain.SSLSecret)
	secret.AddData("tls.crt", s.generatedCertificates.ServerCert)
	secret.AddData("tls.key", s.generatedCertificates.ServerPrivateKey)
	secret.AddData("ca.crt", s.generatedCertificates.CACert)

	// Add labels for management
	secret.Labels["app.kubernetes.io/name"] = serviceName
	secret.Labels["app.kubernetes.io/component"] = "ssl-certificate"
	secret.Labels["deploy-cli/managed"] = "true"

	// Create the secret
	if err := s.secretUsecase.CreateSecret(ctx, secret); err != nil {
		return fmt.Errorf("failed to create SSL certificate secret: %w", err)
	}

	s.logger.InfoWithContext("SSL certificate secret created successfully", map[string]interface{}{
		"service":     serviceName,
		"secret_name": secretName,
		"namespace":   namespace,
	})

	return nil
}

// GetGeneratedCertificates returns the generated certificates
func (s *SSLCertificateManager) GetGeneratedCertificates() *port.GeneratedCertificates {
	return s.generatedCertificates
}

// HasCertificates returns true if certificates are available
func (s *SSLCertificateManager) HasCertificates() bool {
	return s.generatedCertificates != nil
}

// GetCertificateGenerationTime returns the time when certificates were generated/loaded
func (s *SSLCertificateManager) GetCertificateGenerationTime() time.Time {
	if s.generatedCertificates == nil {
		return time.Time{}
	}
	return s.generatedCertificates.Generated
}

// getNamespaceForService returns the appropriate namespace for a service in the given environment
func (s *SSLCertificateManager) getNamespaceForService(serviceName string, env domain.Environment) string {
	// Use the same logic as domain.DetermineNamespace to ensure consistency
	return domain.DetermineNamespace(serviceName, env)
}

// Note: Utility functions moved to shared_utils.go to avoid duplication
