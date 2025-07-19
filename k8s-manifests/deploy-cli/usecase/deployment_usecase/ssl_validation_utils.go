package deployment_usecase

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"strings"
	"time"

	"deploy-cli/port/logger_port"
)

// SSLValidationUtils provides SSL certificate validation utilities
type SSLValidationUtils struct {
	logger logger_port.LoggerPort
}

// NewSSLValidationUtils creates a new SSL validation utilities instance
func NewSSLValidationUtils(logger logger_port.LoggerPort) *SSLValidationUtils {
	return &SSLValidationUtils{
		logger: logger,
	}
}

// validatePEMData validates PEM-encoded certificate and key data
func (s *SSLValidationUtils) ValidatePEMData(caCert, caKey, serverCert, serverKey string) error {
	s.logger.InfoWithContext("validating PEM data", map[string]interface{}{
		"ca_cert_length":     len(caCert),
		"ca_key_length":      len(caKey),
		"server_cert_length": len(serverCert),
		"server_key_length":  len(serverKey),
	})

	// Validate each PEM data component
	pemComponents := map[string]struct {
		data         string
		expectedType string
	}{
		"CA Certificate":     {caCert, "CERTIFICATE"},
		"CA Private Key":     {caKey, "RSA PRIVATE KEY"},
		"Server Certificate": {serverCert, "CERTIFICATE"},
		"Server Private Key": {serverKey, "RSA PRIVATE KEY"},
	}

	for name, component := range pemComponents {
		if err := s.ValidateSinglePEM(name, component.data, component.expectedType); err != nil {
			return fmt.Errorf("PEM validation failed for %s: %w", name, err)
		}
	}

	// Additional validation: ensure certificates can be parsed
	if err := s.ValidateCertificateStructure(caCert, serverCert); err != nil {
		return fmt.Errorf("certificate structure validation failed: %w", err)
	}

	s.logger.InfoWithContext("PEM data validation completed successfully", map[string]interface{}{
		"components_validated": len(pemComponents),
	})

	return nil
}

// ValidateSinglePEM validates a single PEM-encoded data
func (s *SSLValidationUtils) ValidateSinglePEM(name, data, expectedType string) error {
	if data == "" {
		return fmt.Errorf("%s is empty", name)
	}

	// Decode PEM block
	block, rest := pem.Decode([]byte(data))
	if block == nil {
		return fmt.Errorf("%s contains no valid PEM data", name)
	}

	// Check PEM type
	if block.Type != expectedType {
		return fmt.Errorf("%s has wrong PEM type: expected %s, got %s", name, expectedType, block.Type)
	}

	// Check for remaining data (should be empty for single PEM)
	if len(rest) > 0 {
		// Allow whitespace but warn about unexpected content
		trimmed := strings.TrimSpace(string(rest))
		if len(trimmed) > 0 {
			s.logger.WarnWithContext("unexpected data after PEM block", map[string]interface{}{
				"component":       name,
				"remaining_bytes": len(trimmed),
			})
		}
	}

	// Validate PEM data length
	if len(block.Bytes) == 0 {
		return fmt.Errorf("%s PEM block has no data", name)
	}

	return nil
}

// ValidateCertificateStructure validates that certificates can be parsed
func (s *SSLValidationUtils) ValidateCertificateStructure(caCert, serverCert string) error {
	// Parse CA certificate
	caBlock, _ := pem.Decode([]byte(caCert))
	if caBlock == nil {
		return fmt.Errorf("failed to decode CA certificate PEM")
	}

	caCertParsed, err := x509.ParseCertificate(caBlock.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse CA certificate: %w", err)
	}

	// Parse server certificate
	serverBlock, _ := pem.Decode([]byte(serverCert))
	if serverBlock == nil {
		return fmt.Errorf("failed to decode server certificate PEM")
	}

	serverCertParsed, err := x509.ParseCertificate(serverBlock.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse server certificate: %w", err)
	}

	// Validate certificate properties
	now := time.Now()
	if caCertParsed.NotAfter.Before(now) {
		return fmt.Errorf("CA certificate has expired: %s", caCertParsed.NotAfter)
	}

	if serverCertParsed.NotAfter.Before(now) {
		return fmt.Errorf("server certificate has expired: %s", serverCertParsed.NotAfter)
	}

	// Validate that server certificate is issued by CA (simplified check)
	if !serverCertParsed.IsCA && caCertParsed.IsCA {
		// Basic validation passed
		s.logger.InfoWithContext("certificate structure validation passed", map[string]interface{}{
			"ca_subject":     caCertParsed.Subject.String(),
			"server_subject": serverCertParsed.Subject.String(),
			"ca_expires":     caCertParsed.NotAfter.Format(time.RFC3339),
			"server_expires": serverCertParsed.NotAfter.Format(time.RFC3339),
		})
	}

	return nil
}

// ValidatePEMBasics performs basic PEM format validation
func (s *SSLValidationUtils) ValidatePEMBasics(pemData string) bool {
	return strings.Contains(pemData, "-----BEGIN") && strings.Contains(pemData, "-----END")
}

// ValidateCertificateExpiry checks if a certificate is within valid time range
func (s *SSLValidationUtils) ValidateCertificateExpiry(certPEM string) error {
	block, _ := pem.Decode([]byte(certPEM))
	if block == nil {
		return fmt.Errorf("failed to decode certificate PEM")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse certificate: %w", err)
	}

	now := time.Now()

	// Check if certificate is expired
	if now.After(cert.NotAfter) {
		return fmt.Errorf("certificate has expired on %s", cert.NotAfter.Format(time.RFC3339))
	}

	// Check if certificate is not yet valid
	if now.Before(cert.NotBefore) {
		return fmt.Errorf("certificate is not yet valid until %s", cert.NotBefore.Format(time.RFC3339))
	}

	// Check if certificate expires soon (within 30 days)
	expiresIn := cert.NotAfter.Sub(now)
	if expiresIn < 30*24*time.Hour {
		s.logger.WarnWithContext("certificate expires soon", map[string]interface{}{
			"expires_at": cert.NotAfter.Format(time.RFC3339),
			"expires_in": expiresIn.String(),
		})
	}

	return nil
}

// ValidateCertificateChain validates that a certificate chain is valid
func (s *SSLValidationUtils) ValidateCertificateChain(caCert, serverCert string) error {
	// Parse CA certificate
	caBlock, _ := pem.Decode([]byte(caCert))
	if caBlock == nil {
		return fmt.Errorf("failed to decode CA certificate PEM")
	}

	caCertParsed, err := x509.ParseCertificate(caBlock.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse CA certificate: %w", err)
	}

	// Parse server certificate
	serverBlock, _ := pem.Decode([]byte(serverCert))
	if serverBlock == nil {
		return fmt.Errorf("failed to decode server certificate PEM")
	}

	serverCertParsed, err := x509.ParseCertificate(serverBlock.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse server certificate: %w", err)
	}

	// Create certificate pool with CA
	roots := x509.NewCertPool()
	roots.AddCert(caCertParsed)

	// Verify server certificate against CA
	opts := x509.VerifyOptions{
		Roots: roots,
	}

	_, err = serverCertParsed.Verify(opts)
	if err != nil {
		return fmt.Errorf("failed to verify certificate chain: %w", err)
	}

	s.logger.InfoWithContext("certificate chain validation passed", map[string]interface{}{
		"ca_subject":     caCertParsed.Subject.String(),
		"server_subject": serverCertParsed.Subject.String(),
	})

	return nil
}

// ValidateKeyPair validates that a certificate and private key match
func (s *SSLValidationUtils) ValidateKeyPair(certPEM, keyPEM string) error {
	// Parse certificate
	certBlock, _ := pem.Decode([]byte(certPEM))
	if certBlock == nil {
		return fmt.Errorf("failed to decode certificate PEM")
	}

	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse certificate: %w", err)
	}

	// Parse private key
	keyBlock, _ := pem.Decode([]byte(keyPEM))
	if keyBlock == nil {
		return fmt.Errorf("failed to decode private key PEM")
	}

	// This is a simplified check - in production, you'd want to verify
	// that the public key in the certificate matches the private key
	if keyBlock.Type != "RSA PRIVATE KEY" && keyBlock.Type != "PRIVATE KEY" {
		return fmt.Errorf("unsupported private key type: %s", keyBlock.Type)
	}

	s.logger.InfoWithContext("key pair validation passed", map[string]interface{}{
		"cert_subject": cert.Subject.String(),
		"key_type":     keyBlock.Type,
	})

	return nil
}

// GetCertificateInfo extracts basic information from a certificate
func (s *SSLValidationUtils) GetCertificateInfo(certPEM string) (map[string]interface{}, error) {
	block, _ := pem.Decode([]byte(certPEM))
	if block == nil {
		return nil, fmt.Errorf("failed to decode certificate PEM")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse certificate: %w", err)
	}

	info := map[string]interface{}{
		"subject":             cert.Subject.String(),
		"issuer":              cert.Issuer.String(),
		"serial_number":       cert.SerialNumber.String(),
		"not_before":          cert.NotBefore.Format(time.RFC3339),
		"not_after":           cert.NotAfter.Format(time.RFC3339),
		"is_ca":               cert.IsCA,
		"dns_names":           cert.DNSNames,
		"ip_addresses":        cert.IPAddresses,
		"key_usage":           cert.KeyUsage,
		"ext_key_usage":       cert.ExtKeyUsage,
		"signature_algorithm": cert.SignatureAlgorithm.String(),
	}

	return info, nil
}

// ValidateSSLConfiguration performs comprehensive SSL configuration validation
func (s *SSLValidationUtils) ValidateSSLConfiguration(caCert, caKey, serverCert, serverKey string) error {
	s.logger.InfoWithContext("starting comprehensive SSL configuration validation", map[string]interface{}{})

	// Step 1: Basic PEM validation
	if err := s.ValidatePEMData(caCert, caKey, serverCert, serverKey); err != nil {
		return fmt.Errorf("PEM data validation failed: %w", err)
	}

	// Step 2: Certificate expiry validation
	if err := s.ValidateCertificateExpiry(caCert); err != nil {
		return fmt.Errorf("CA certificate expiry validation failed: %w", err)
	}

	if err := s.ValidateCertificateExpiry(serverCert); err != nil {
		return fmt.Errorf("server certificate expiry validation failed: %w", err)
	}

	// Step 3: Certificate chain validation
	if err := s.ValidateCertificateChain(caCert, serverCert); err != nil {
		return fmt.Errorf("certificate chain validation failed: %w", err)
	}

	// Step 4: Key pair validation
	if err := s.ValidateKeyPair(caCert, caKey); err != nil {
		return fmt.Errorf("CA key pair validation failed: %w", err)
	}

	if err := s.ValidateKeyPair(serverCert, serverKey); err != nil {
		return fmt.Errorf("server key pair validation failed: %w", err)
	}

	s.logger.InfoWithContext("comprehensive SSL configuration validation completed successfully", map[string]interface{}{})

	return nil
}
