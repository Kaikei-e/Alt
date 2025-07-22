package ssl_usecase

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log/slog"
	"time"

	"deploy-cli/port/kubectl_port"
)

// certificateValidatorImpl implements CertificateValidator interface
type certificateValidatorImpl struct {
	kubectl kubectl_port.KubectlPort
	logger  *slog.Logger
}

// NewCertificateValidator creates new CertificateValidator instance
func NewCertificateValidator(kubectl kubectl_port.KubectlPort, logger *slog.Logger) CertificateValidator {
	return &certificateValidatorImpl{
		kubectl: kubectl,
		logger:  logger,
	}
}

// ValidateCertificate validates certificate integrity and format
func (cv *certificateValidatorImpl) ValidateCertificate(ctx context.Context, cert *Certificate) error {
	cv.logger.Debug("Validating certificate",
		"name", cert.Name,
		"namespace", cert.Namespace)

	// Validate certificate PEM format
	if err := cv.validatePEMFormat(cert.Certificate); err != nil {
		return fmt.Errorf("certificate PEM validation failed: %w", err)
	}

	// Validate private key PEM format
	if err := cv.validatePrivateKeyPEMFormat(cert.PrivateKey); err != nil {
		return fmt.Errorf("private key PEM validation failed: %w", err)
	}

	// Parse and validate certificate
	x509Cert, err := cv.parseCertificate(cert.Certificate)
	if err != nil {
		return fmt.Errorf("certificate parsing failed: %w", err)
	}

	// Validate certificate fields
	if err := cv.validateCertificateFields(x509Cert, cert); err != nil {
		return fmt.Errorf("certificate field validation failed: %w", err)
	}

	// Validate certificate chain if CA cert is present
	if len(cert.CACert) > 0 {
		if err := cv.validateCertificateChain(x509Cert, cert.CACert); err != nil {
			return fmt.Errorf("certificate chain validation failed: %w", err)
		}
	}

	cv.logger.Info("Certificate validation passed",
		"name", cert.Name,
		"namespace", cert.Namespace,
		"common_name", cert.CommonName,
		"expires_at", cert.ExpiresAt)

	return nil
}

// CheckExpiration checks certificate expiration status
func (cv *certificateValidatorImpl) CheckExpiration(ctx context.Context, cert *Certificate) (*ExpirationStatus, error) {
	cv.logger.Debug("Checking certificate expiration",
		"name", cert.Name,
		"expires_at", cert.ExpiresAt)

	now := time.Now()
	timeUntilExpiry := cert.ExpiresAt.Sub(now)
	daysUntilExpiry := int(timeUntilExpiry.Hours() / 24)

	status := &ExpirationStatus{
		IsExpired:       now.After(cert.ExpiresAt),
		DaysUntilExpiry: daysUntilExpiry,
		ExpiresAt:       cert.ExpiresAt,
		RiskLevel:       cv.calculateRiskLevel(daysUntilExpiry, now.After(cert.ExpiresAt)),
	}

	cv.logger.Debug("Certificate expiration status",
		"name", cert.Name,
		"days_until_expiry", daysUntilExpiry,
		"risk_level", status.RiskLevel,
		"is_expired", status.IsExpired)

	return status, nil
}

// PerformHealthCheck performs comprehensive health check on certificate
func (cv *certificateValidatorImpl) PerformHealthCheck(ctx context.Context, namespace, certName string) error {
	cv.logger.Debug("Performing certificate health check",
		"namespace", namespace,
		"certificate", certName)

	// Get certificate from Kubernetes
	secrets, err := cv.kubectl.GetSecrets(ctx, namespace)
	if err != nil {
		return fmt.Errorf("failed to get secrets from namespace %s: %w", namespace, err)
	}

	var certSecret *kubectl_port.KubernetesSecret
	for _, secret := range secrets {
		if secret.Name == certName {
			certSecret = &secret
			break
		}
	}

	if certSecret == nil {
		return fmt.Errorf("certificate secret %s not found in namespace %s", certName, namespace)
	}

	// Validate secret type
	if certSecret.Type != "kubernetes.io/tls" {
		return fmt.Errorf("secret %s is not a TLS secret (type: %s)", certName, certSecret.Type)
	}

	// Parse certificate from secret
	cert, err := cv.parseCertificateFromSecret(certSecret, namespace)
	if err != nil {
		return fmt.Errorf("failed to parse certificate from secret: %w", err)
	}

	// Perform full validation
	if err := cv.ValidateCertificate(ctx, cert); err != nil {
		return fmt.Errorf("certificate validation failed: %w", err)
	}

	// Check expiration
	expiration, err := cv.CheckExpiration(ctx, cert)
	if err != nil {
		return fmt.Errorf("expiration check failed: %w", err)
	}

	// Log health status
	cv.logger.Info("Certificate health check completed",
		"namespace", namespace,
		"certificate", certName,
		"risk_level", expiration.RiskLevel,
		"days_until_expiry", expiration.DaysUntilExpiry,
		"health_status", cv.determineHealthStatus(expiration))

	return nil
}

// validatePEMFormat validates PEM format
func (cv *certificateValidatorImpl) validatePEMFormat(pemData []byte) error {
	block, _ := pem.Decode(pemData)
	if block == nil {
		return fmt.Errorf("failed to decode PEM data")
	}

	if block.Type != "CERTIFICATE" {
		return fmt.Errorf("expected CERTIFICATE block, got %s", block.Type)
	}

	return nil
}

// validatePrivateKeyPEMFormat validates private key PEM format
func (cv *certificateValidatorImpl) validatePrivateKeyPEMFormat(pemData []byte) error {
	block, _ := pem.Decode(pemData)
	if block == nil {
		return fmt.Errorf("failed to decode private key PEM data")
	}

	validKeyTypes := []string{"RSA PRIVATE KEY", "PRIVATE KEY", "EC PRIVATE KEY"}
	for _, validType := range validKeyTypes {
		if block.Type == validType {
			return nil
		}
	}

	return fmt.Errorf("invalid private key type: %s", block.Type)
}

// parseCertificate parses X.509 certificate from PEM data
func (cv *certificateValidatorImpl) parseCertificate(pemData []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(pemData)
	if block == nil {
		return nil, fmt.Errorf("failed to decode certificate PEM")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse certificate: %w", err)
	}

	return cert, nil
}

// validateCertificateFields validates certificate fields
func (cv *certificateValidatorImpl) validateCertificateFields(x509Cert *x509.Certificate, cert *Certificate) error {
	// Validate common name
	if x509Cert.Subject.CommonName != cert.CommonName {
		return fmt.Errorf("common name mismatch: expected %s, got %s",
			cert.CommonName, x509Cert.Subject.CommonName)
	}

	// Validate expiration time
	if !x509Cert.NotAfter.Equal(cert.ExpiresAt) {
		return fmt.Errorf("expiration time mismatch: expected %s, got %s",
			cert.ExpiresAt, x509Cert.NotAfter)
	}

	// Validate DNS names
	if len(x509Cert.DNSNames) != len(cert.DNSNames) {
		return fmt.Errorf("DNS names count mismatch: expected %d, got %d",
			len(cert.DNSNames), len(x509Cert.DNSNames))
	}

	return nil
}

// validateCertificateChain validates certificate chain
func (cv *certificateValidatorImpl) validateCertificateChain(cert *x509.Certificate, caCertPEM []byte) error {
	// Parse CA certificate
	caBlock, _ := pem.Decode(caCertPEM)
	if caBlock == nil {
		return fmt.Errorf("failed to decode CA certificate PEM")
	}

	caCert, err := x509.ParseCertificate(caBlock.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse CA certificate: %w", err)
	}

	// Create certificate pool with CA
	roots := x509.NewCertPool()
	roots.AddCert(caCert)

	// Verify certificate chain
	opts := x509.VerifyOptions{
		Roots: roots,
	}

	_, err = cert.Verify(opts)
	if err != nil {
		return fmt.Errorf("certificate chain verification failed: %w", err)
	}

	return nil
}

// calculateRiskLevel calculates risk level based on expiration time
func (cv *certificateValidatorImpl) calculateRiskLevel(daysUntilExpiry int, isExpired bool) string {
	if isExpired {
		return "critical"
	}

	switch {
	case daysUntilExpiry <= 7:
		return "critical"
	case daysUntilExpiry <= 30:
		return "high"
	case daysUntilExpiry <= 90:
		return "medium"
	default:
		return "low"
	}
}

// determineHealthStatus determines overall health status
func (cv *certificateValidatorImpl) determineHealthStatus(expiration *ExpirationStatus) string {
	if expiration.IsExpired {
		return "unhealthy"
	}

	switch expiration.RiskLevel {
	case "critical", "high":
		return "warning"
	case "medium":
		return "attention"
	default:
		return "healthy"
	}
}

// parseCertificateFromSecret parses certificate from Kubernetes secret
func (cv *certificateValidatorImpl) parseCertificateFromSecret(
	secret *kubectl_port.KubernetesSecret,
	namespace string,
) (*Certificate, error) {
	certData, exists := secret.Data["tls.crt"]
	if !exists {
		return nil, fmt.Errorf("tls.crt not found in secret %s", secret.Name)
	}

	keyData, exists := secret.Data["tls.key"]
	if !exists {
		return nil, fmt.Errorf("tls.key not found in secret %s", secret.Name)
	}

	// Parse certificate to get metadata
	x509Cert, err := cv.parseCertificate([]byte(certData))
	if err != nil {
		return nil, fmt.Errorf("failed to parse certificate: %w", err)
	}

	certificate := &Certificate{
		Name:         secret.Name,
		Namespace:    namespace,
		CommonName:   x509Cert.Subject.CommonName,
		DNSNames:     x509Cert.DNSNames,
		Certificate:  []byte(certData),
		PrivateKey:   []byte(keyData),
		IssuedAt:     x509Cert.NotBefore,
		ExpiresAt:    x509Cert.NotAfter,
		Issuer:       x509Cert.Issuer.String(),
		SerialNumber: x509Cert.SerialNumber.String(),
		Fingerprint:  fmt.Sprintf("%x", x509Cert.SerialNumber),
	}

	return certificate, nil
}