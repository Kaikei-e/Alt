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
	"os"
	"path/filepath"
	"time"

	"deploy-cli/domain"
	"deploy-cli/port"
	"deploy-cli/port/logger_port"
	"deploy-cli/usecase/secret_usecase"
	"gopkg.in/yaml.v2"
)

// SSLManagementUsecase handles SSL certificate lifecycle management
type SSLManagementUsecase struct {
	secretUsecase         *secret_usecase.SecretUsecase
	sslUsecase            *secret_usecase.SSLCertificateUsecase
	logger                logger_port.LoggerPort
	generatedCertificates *port.GeneratedCertificates
}

// NewSSLManagementUsecase creates a new SSL management usecase
func NewSSLManagementUsecase(
	secretUsecase *secret_usecase.SecretUsecase,
	sslUsecase *secret_usecase.SSLCertificateUsecase,
	logger logger_port.LoggerPort,
) *SSLManagementUsecase {
	return &SSLManagementUsecase{
		secretUsecase: secretUsecase,
		sslUsecase:    sslUsecase,
		logger:        logger,
	}
}

// ManageCertificateLifecycle manages SSL certificate lifecycle
func (s *SSLManagementUsecase) ManageCertificateLifecycle(ctx context.Context, environment domain.Environment, chartsDir string) error {
	s.logger.InfoWithContext("starting SSL certificate lifecycle management", map[string]interface{}{
		"environment": environment.String(),
		"charts_dir":  chartsDir,
	})

	certificatesAvailable := false

	// Step 1: Try to load existing certificates from Kubernetes secrets
	if err := s.LoadExistingCertificates(ctx, environment); err != nil {
		s.logger.WarnWithContext("failed to load existing certificates, will generate new ones", map[string]interface{}{
			"environment": environment.String(),
			"error":       err.Error(),
		})

		// Fall back to generating new certificates
		if err := s.GenerateSSLCertificates(ctx); err != nil {
			s.logger.ErrorWithContext("failed to generate SSL certificates", map[string]interface{}{
				"environment": environment.String(),
				"error":       err.Error(),
			})
			// Continue without certificates for emergency deployment
			s.logger.WarnWithContext("continuing deployment without SSL certificates", map[string]interface{}{
				"environment": environment.String(),
				"reason":      "both certificate loading and generation failed",
			})
			return nil // Don't fail deployment
		} else {
			certificatesAvailable = true
		}
	} else {
		certificatesAvailable = true
	}

	// Step 2: Validate certificates only if they are available
	if certificatesAvailable {
		if err := s.ValidateGeneratedCertificates(ctx); err != nil {
			s.logger.WarnWithContext("certificate validation failed, continuing without SSL", map[string]interface{}{
				"environment": environment.String(),
				"error":       err.Error(),
			})
			// Reset certificates to nil to indicate they are not available
			s.generatedCertificates = nil
			certificatesAvailable = false
		}
	}

	// Step 3: Generate SSL certificate secrets only if certificates are available
	if certificatesAvailable {
		if err := s.GenerateSSLCertificateSecrets(ctx, environment); err != nil {
			s.logger.WarnWithContext("failed to generate SSL certificate secrets", map[string]interface{}{
				"environment": environment.String(),
				"error":       err.Error(),
			})
			// Continue with deployment even if SSL generation fails
		}

		// Step 4: Distribute certificates to all SSL-requiring charts
		sslCharts := []string{"common-ssl", "alt-backend", "alt-frontend", "auth-service", "nginx-external", "kratos"}
		for _, chart := range sslCharts {
			chartPath := filepath.Join(chartsDir, chart)
			if err := s.InjectCertificateData(ctx, chartPath); err != nil {
				s.logger.WarnWithContext("failed to inject certificate data for chart", map[string]interface{}{
					"chart": chart,
					"error": err.Error(),
				})
				// Continue with other charts even if one fails
			}
		}
	} else {
		s.logger.InfoWithContext("skipping SSL certificate distribution - no certificates available", map[string]interface{}{
			"environment": environment.String(),
			"reason":      "certificates not loaded or generated",
		})
	}

	s.logger.InfoWithContext("certificate lifecycle management completed", map[string]interface{}{
		"environment":            environment.String(),
		"charts_dir":             chartsDir,
		"certificates_available": certificatesAvailable,
	})

	return nil
}

// LoadExistingCertificates loads existing SSL certificates from Kubernetes secrets
func (s *SSLManagementUsecase) LoadExistingCertificates(ctx context.Context, environment domain.Environment) error {
	s.logger.InfoWithContext("loading existing SSL certificates", map[string]interface{}{
		"environment": environment.String(),
	})

	namespace := s.getNamespaceForEnvironment(environment)

	// Try to load CA certificate from common-ssl secret
	caSecret, err := s.secretUsecase.GetSecret(ctx, "ca-secret", namespace)
	if err != nil {
		s.logger.DebugWithContext("CA certificate secret not found", map[string]interface{}{
			"secret_name": "ca-secret",
			"namespace":   namespace,
			"error":       err.Error(),
		})
		return fmt.Errorf("failed to load CA certificate secret: %w", err)
	}

	// Try to load server certificate from one of the SSL secrets
	serverSecret, err := s.secretUsecase.GetSecret(ctx, "alt-backend-ssl-certs-prod", namespace)
	if err != nil {
		s.logger.DebugWithContext("server certificate secret not found", map[string]interface{}{
			"secret_name": "alt-backend-ssl-certs-prod",
			"namespace":   namespace,
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
		"environment":             environment.String(),
		"ca_cert_length":          len(s.generatedCertificates.CACert),
		"server_cert_length":      len(s.generatedCertificates.ServerCert),
		"ca_has_private_key":      len(s.generatedCertificates.CAPrivateKey) > 0,
		"server_has_private_key":  len(s.generatedCertificates.ServerPrivateKey) > 0,
	})

	return nil
}

// getNamespaceForEnvironment returns the appropriate namespace for the environment
func (s *SSLManagementUsecase) getNamespaceForEnvironment(env domain.Environment) string {
	switch env {
	case domain.Production:
		return "alt-production"
	case domain.Staging:
		return "alt-staging"
	case domain.Development:
		return "alt-dev"
	default:
		return "alt-production"
	}
}

// GenerateSSLCertificates generates SSL certificates for the application
func (s *SSLManagementUsecase) GenerateSSLCertificates(ctx context.Context) error {
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
			Organization:  []string{"Alt RSS Reader"},
			Country:       []string{"JP"},
			Province:      []string{"Tokyo"},
			Locality:      []string{"Tokyo"},
			CommonName:    "Alt RSS Reader CA",
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
			Organization:  []string{"Alt RSS Reader"},
			Country:       []string{"JP"},
			Province:      []string{"Tokyo"},
			Locality:      []string{"Tokyo"},
			CommonName:    "*.alt-app.local",
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

// ValidateGeneratedCertificates validates the SSL certificates (either loaded or generated)
func (s *SSLManagementUsecase) ValidateGeneratedCertificates(ctx context.Context) error {
	if s.generatedCertificates == nil {
		return fmt.Errorf("no certificates available to validate")
	}

	s.logger.InfoWithContext("validating SSL certificates", map[string]interface{}{
		"loaded_time": s.generatedCertificates.Generated.Format(time.RFC3339),
	})

	// Validate CA certificate (PEM format, not base64)
	if err := s.ValidateCertificatePEM(s.generatedCertificates.CACert, "CA"); err != nil {
		return fmt.Errorf("CA certificate validation failed: %w", err)
	}

	// Validate server certificate (PEM format, not base64)
	if err := s.ValidateCertificatePEM(s.generatedCertificates.ServerCert, "Server"); err != nil {
		return fmt.Errorf("Server certificate validation failed: %w", err)
	}

	s.logger.InfoWithContext("SSL certificate validation completed successfully", map[string]interface{}{
		"validated_at": time.Now().Format(time.RFC3339),
	})

	return nil
}

// ValidateCertificatePEM validates a certificate in PEM format
func (s *SSLManagementUsecase) ValidateCertificatePEM(certPEM, certType string) error {
	if certPEM == "" {
		return fmt.Errorf("certificate PEM is empty")
	}

	// Parse PEM block
	block, _ := pem.Decode([]byte(certPEM))
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
		"cert_type":    certType,
		"subject":      cert.Subject.String(),
		"not_before":   cert.NotBefore.Format(time.RFC3339),
		"not_after":    cert.NotAfter.Format(time.RFC3339),
		"is_ca":        cert.IsCA,
	})

	return nil
}

// ValidateCertificate validates a certificate in base64 format
func (s *SSLManagementUsecase) ValidateCertificate(certBase64, certType string) error {
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
		"cert_type":      certType,
		"subject":        cert.Subject.String(),
		"not_before":     cert.NotBefore.Format(time.RFC3339),
		"not_after":      cert.NotAfter.Format(time.RFC3339),
		"is_ca":          cert.IsCA,
	})

	return nil
}

// InjectCertificateData injects certificate data into chart configurations
func (s *SSLManagementUsecase) InjectCertificateData(ctx context.Context, chartPath string) error {
	if s.generatedCertificates == nil {
		return fmt.Errorf("no certificates available for injection")
	}

	s.logger.InfoWithContext("injecting certificate data", map[string]interface{}{
		"chart_path": chartPath,
	})

	// Extract chart name from path
	chartName := filepath.Base(chartPath)

	// Handle different chart types
	switch chartName {
	case "common-ssl":
		return s.InjectCommonSSLCertificates(ctx, chartPath)
	case "alt-backend", "alt-frontend", "auth-service", "nginx-external", "kratos":
		return s.InjectServiceSSLCertificates(ctx, chartPath, chartName)
	default:
		// Skip injection for charts that don't need SSL
		s.logger.InfoWithContext("skipping SSL injection for chart", map[string]interface{}{
			"chart_name": chartName,
		})
		return nil
	}
}

// InjectCommonSSLCertificates injects certificates for common-ssl chart
func (s *SSLManagementUsecase) InjectCommonSSLCertificates(ctx context.Context, chartPath string) error {
	valuesFile := filepath.Join(chartPath, "values-ssl.yaml")

	// Create SSL configuration for common-ssl chart
	sslConfig := map[string]interface{}{
		"ssl": map[string]interface{}{
			"enabled": true,
			"ca": map[string]interface{}{
				"cert": s.generatedCertificates.CACert,
				"key":  s.generatedCertificates.CAPrivateKey,
			},
			"server": map[string]interface{}{
				"cert": s.generatedCertificates.ServerCert,
				"key":  s.generatedCertificates.ServerPrivateKey,
			},
		},
	}

	// Write SSL configuration to values file
	if err := s.writeSSLValuesFile(valuesFile, sslConfig); err != nil {
		return fmt.Errorf("failed to write SSL values file: %w", err)
	}

	s.logger.InfoWithContext("common SSL certificate data injected successfully", map[string]interface{}{
		"chart_path":  chartPath,
		"values_file": valuesFile,
	})

	return nil
}

// InjectServiceSSLCertificates injects certificates for service charts
func (s *SSLManagementUsecase) InjectServiceSSLCertificates(ctx context.Context, chartPath string, chartName string) error {
	valuesFile := filepath.Join(chartPath, "values-ssl.yaml")

	// Create SSL configuration for service charts
	sslConfig := map[string]interface{}{
		"ssl": map[string]interface{}{
			"enabled": true,
			"tls": map[string]interface{}{
				"cert": s.generatedCertificates.ServerCert,
				"key":  s.generatedCertificates.ServerPrivateKey,
			},
			"ca": map[string]interface{}{
				"cert": s.generatedCertificates.CACert,
			},
		},
	}

	// Write SSL configuration to values file
	if err := s.writeSSLValuesFile(valuesFile, sslConfig); err != nil {
		return fmt.Errorf("failed to write SSL values file: %w", err)
	}

	s.logger.InfoWithContext("service SSL certificate data injected successfully", map[string]interface{}{
		"chart_path":  chartPath,
		"chart_name":  chartName,
		"values_file": valuesFile,
	})

	return nil
}

// writeSSLValuesFile writes SSL configuration to a YAML values file
func (s *SSLManagementUsecase) writeSSLValuesFile(filename string, config map[string]interface{}) error {
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to create values file: %w", err)
	}
	defer file.Close()

	// Write YAML header
	if _, err := file.WriteString("# SSL Configuration - Auto-generated by deploy-cli\n"); err != nil {
		return fmt.Errorf("failed to write header: %w", err)
	}

	// Write SSL configuration
	encoder := yaml.NewEncoder(file)
	defer encoder.Close()
	if err := encoder.Encode(config); err != nil {
		return fmt.Errorf("failed to encode SSL config: %w", err)
	}

	return nil
}

// GenerateSSLCertificateSecrets generates SSL certificate secrets for all services that need them
func (s *SSLManagementUsecase) GenerateSSLCertificateSecrets(ctx context.Context, environment domain.Environment) error {
	s.logger.InfoWithContext("generating SSL certificate secrets", map[string]interface{}{
		"environment": environment.String(),
	})

	namespace := s.getNamespaceForEnvironment(environment)

	// List of services that need SSL certificates
	services := []string{
		"alt-backend",
		"alt-frontend",
		"auth-service",
		"nginx-external",
		"kratos",
		"meilisearch",
		"postgres",
		"clickhouse",
	}

	for _, service := range services {
		secretName := fmt.Sprintf("%s-ssl-certs-prod", service)
		if err := s.CreateSSLCertificateSecret(ctx, service, secretName, namespace); err != nil {
			s.logger.WarnWithContext("failed to create SSL certificate secret", map[string]interface{}{
				"service":     service,
				"secret_name": secretName,
				"namespace":   namespace,
				"error":       err.Error(),
			})
			// Continue with other services
		}
	}

	s.logger.InfoWithContext("SSL certificate secrets generation completed", map[string]interface{}{
		"environment": environment.String(),
		"namespace":   namespace,
		"services":    len(services),
	})

	return nil
}

// CreateSSLCertificateSecret creates an SSL certificate secret for a specific service
func (s *SSLManagementUsecase) CreateSSLCertificateSecret(ctx context.Context, serviceName, secretName, namespace string) error {
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
func (s *SSLManagementUsecase) GetGeneratedCertificates() *port.GeneratedCertificates {
	return s.generatedCertificates
}

// HasCertificates returns true if certificates are available
func (s *SSLManagementUsecase) HasCertificates() bool {
	return s.generatedCertificates != nil
}

// GetCertificateGenerationTime returns the time when certificates were generated/loaded
func (s *SSLManagementUsecase) GetCertificateGenerationTime() time.Time {
	if s.generatedCertificates == nil {
		return time.Time{}
	}
	return s.generatedCertificates.Generated
}