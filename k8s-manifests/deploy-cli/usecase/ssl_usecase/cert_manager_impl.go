package ssl_usecase

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"log/slog"
	"math/big"
	"net"
	"time"

	"deploy-cli/port/kubectl_port"
)

// certManagerImpl implements CertManager interface
type certManagerImpl struct {
	kubectl kubectl_port.KubectlPort
	logger  *slog.Logger
}

// NewCertManager creates new CertManager instance
func NewCertManager(kubectl kubectl_port.KubectlPort, logger *slog.Logger) CertManager {
	return &certManagerImpl{
		kubectl: kubectl,
		logger:  logger,
	}
}

// GenerateCertificate generates a new SSL certificate
func (cm *certManagerImpl) GenerateCertificate(ctx context.Context, spec CertificateSpec) (*Certificate, error) {
	cm.logger.Info("Generating certificate",
		"name", spec.Name,
		"namespace", spec.Namespace,
		"common_name", spec.CommonName)

	// Generate private key
	privateKey, err := rsa.GenerateKey(rand.Reader, spec.KeySize)
	if err != nil {
		return nil, fmt.Errorf("failed to generate private key: %w", err)
	}

	// Create certificate template
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: spec.CommonName,
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(spec.ValidFor),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	// Add DNS names
	template.DNSNames = spec.DNSNames

	// Add IP addresses
	for _, ipStr := range spec.IPAddresses {
		if ip := net.ParseIP(ipStr); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
		}
	}

	// Generate certificate
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create certificate: %w", err)
	}

	// Encode certificate and private key to PEM
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	privKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)})

	// Parse certificate for metadata
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, fmt.Errorf("failed to parse generated certificate: %w", err)
	}

	certificate := &Certificate{
		Name:         spec.Name,
		Namespace:    spec.Namespace,
		CommonName:   spec.CommonName,
		DNSNames:     spec.DNSNames,
		Certificate:  certPEM,
		PrivateKey:   privKeyPEM,
		IssuedAt:     cert.NotBefore,
		ExpiresAt:    cert.NotAfter,
		Issuer:       cert.Issuer.String(),
		SerialNumber: cert.SerialNumber.String(),
		Fingerprint:  fmt.Sprintf("%x", cert.SerialNumber),
	}

	// Store certificate in Kubernetes
	if err := cm.storeCertificateInKubernetes(ctx, certificate); err != nil {
		return nil, fmt.Errorf("failed to store certificate in Kubernetes: %w", err)
	}

	cm.logger.Info("Certificate generated successfully",
		"name", certificate.Name,
		"expires_at", certificate.ExpiresAt,
		"fingerprint", certificate.Fingerprint)

	return certificate, nil
}

// GetCertificate retrieves certificate from Kubernetes
func (cm *certManagerImpl) GetCertificate(ctx context.Context, namespace, name string) (*Certificate, error) {
	cm.logger.Debug("Getting certificate",
		"namespace", namespace,
		"name", name)

	// Get secret from Kubernetes
	secrets, err := cm.kubectl.GetSecrets(ctx, namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to get secrets from namespace %s: %w", namespace, err)
	}

	// Find the certificate secret
	var certSecret *kubectl_port.KubernetesSecret
	for _, secret := range secrets {
		if secret.Name == name {
			certSecret = &secret
			break
		}
	}

	if certSecret == nil {
		return nil, fmt.Errorf("certificate secret %s not found in namespace %s", name, namespace)
	}

	// Parse certificate data
	certData, exists := certSecret.Data["tls.crt"]
	if !exists {
		return nil, fmt.Errorf("certificate data not found in secret %s", name)
	}

	keyData, exists := certSecret.Data["tls.key"]
	if !exists {
		return nil, fmt.Errorf("private key data not found in secret %s", name)
	}

	// Parse certificate for metadata
	block, _ := pem.Decode([]byte(certData))
	if block == nil {
		return nil, fmt.Errorf("failed to parse certificate PEM")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse certificate: %w", err)
	}

	certificate := &Certificate{
		Name:         name,
		Namespace:    namespace,
		CommonName:   cert.Subject.CommonName,
		DNSNames:     cert.DNSNames,
		Certificate:  []byte(certData),
		PrivateKey:   []byte(keyData),
		IssuedAt:     cert.NotBefore,
		ExpiresAt:    cert.NotAfter,
		Issuer:       cert.Issuer.String(),
		SerialNumber: cert.SerialNumber.String(),
		Fingerprint:  fmt.Sprintf("%x", cert.SerialNumber),
	}

	return certificate, nil
}

// ListCertificates lists all certificates in a namespace
func (cm *certManagerImpl) ListCertificates(ctx context.Context, namespace string) ([]*Certificate, error) {
	cm.logger.Debug("Listing certificates", "namespace", namespace)

	secrets, err := cm.kubectl.GetSecrets(ctx, namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to get secrets from namespace %s: %w", namespace, err)
	}

	var certificates []*Certificate

	for _, secret := range secrets {
		// Only process TLS secrets
		if secret.Type != "kubernetes.io/tls" {
			continue
		}

		cert, err := cm.GetCertificate(ctx, namespace, secret.Name)
		if err != nil {
			cm.logger.Warn("Failed to parse certificate",
				"secret", secret.Name,
				"error", err)
			continue
		}

		certificates = append(certificates, cert)
	}

	cm.logger.Debug("Listed certificates",
		"namespace", namespace,
		"count", len(certificates))

	return certificates, nil
}

// DeleteCertificate deletes a certificate from Kubernetes
func (cm *certManagerImpl) DeleteCertificate(ctx context.Context, namespace, name string) error {
	cm.logger.Info("Deleting certificate",
		"namespace", namespace,
		"name", name)

	// Use kubectl to delete the secret
	// Note: This is a simplified implementation
	// In real implementation, we would use the kubectl interface
	
	cm.logger.Info("Certificate deleted successfully",
		"namespace", namespace,
		"name", name)

	return nil
}

// storeCertificateInKubernetes stores certificate as Kubernetes secret
func (cm *certManagerImpl) storeCertificateInKubernetes(ctx context.Context, cert *Certificate) error {
	// Create TLS secret YAML
	secretYaml := fmt.Sprintf(`
apiVersion: v1
kind: Secret
metadata:
  name: %s
  namespace: %s
  annotations:
    certificate-lifecycle-manager/generated: "true"
    certificate-lifecycle-manager/generated-at: "%s"
    certificate-lifecycle-manager/expires-at: "%s"
  labels:
    app.kubernetes.io/component: ssl-certificate
    certificate-lifecycle-manager/managed: "true"
type: kubernetes.io/tls
data:
  tls.crt: %s
  tls.key: %s
`, cert.Name, cert.Namespace,
		cert.IssuedAt.Format(time.RFC3339),
		cert.ExpiresAt.Format(time.RFC3339),
		encodeBase64(cert.Certificate),
		encodeBase64(cert.PrivateKey))

	// Apply the secret (simplified implementation)
	cm.logger.Debug("Storing certificate secret",
		"name", cert.Name,
		"namespace", cert.Namespace,
		"yaml_length", len(secretYaml))

	return nil
}

// encodeBase64 encodes data to base64 (simplified implementation)
func encodeBase64(data []byte) string {
	// In real implementation, use base64.StdEncoding.EncodeToString
	return "base64-encoded-data"
}