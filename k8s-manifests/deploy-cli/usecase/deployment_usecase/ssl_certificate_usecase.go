package deployment_usecase

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
	"os"
	"path/filepath"
	"time"

	"deploy-cli/domain"
	"deploy-cli/port"
	"deploy-cli/port/logger_port"
	"deploy-cli/usecase/secret_usecase"
	"gopkg.in/yaml.v2"
)

// SSLCertificateUsecase handles SSL certificate lifecycle management
type SSLCertificateUsecase struct {
	logger              logger_port.LoggerPort
	secretUsecase       *secret_usecase.SecretUsecase
	sslUsecase          *secret_usecase.SSLCertificateUsecase
	generatedCertificates *port.GeneratedCertificates
}

// NewSSLCertificateUsecase creates a new SSL certificate usecase
func NewSSLCertificateUsecase(
	logger logger_port.LoggerPort,
	secretUsecase *secret_usecase.SecretUsecase,
	sslUsecase *secret_usecase.SSLCertificateUsecase,
) *SSLCertificateUsecase {
	return &SSLCertificateUsecase{
		logger:        logger,
		secretUsecase: secretUsecase,
		sslUsecase:    sslUsecase,
	}
}

// PreDeploymentSSLCheck performs comprehensive SSL certificate validation before deployment
func (u *SSLCertificateUsecase) PreDeploymentSSLCheck(ctx context.Context, options *domain.DeploymentOptions) error {
	u.logger.InfoWithContext("starting SSL certificate validation", map[string]interface{}{
		"environment": options.Environment.String(),
	})

	// Identify SSL certificate requirements based on environment
	requiredCertificates := u.identifySSLRequirements(options.Environment)
	
	// Validate existing certificates
	for _, certName := range requiredCertificates {
		exists, err := u.sslUsecase.ValidateCertificateExists(ctx, certName, options.Environment)
		if err != nil {
			u.logger.ErrorWithContext("failed to validate SSL certificate", map[string]interface{}{
				"certificate": certName,
				"environment": options.Environment.String(),
				"error": err.Error(),
			})
			return fmt.Errorf("SSL certificate validation failed for %s: %w", certName, err)
		}

		if !exists {
			u.logger.InfoWithContext("SSL certificate missing, attempting auto-generation", map[string]interface{}{
				"certificate": certName,
				"environment": options.Environment.String(),
			})

			// Auto-generate missing SSL certificates
			if err := u.sslUsecase.GenerateCertificate(ctx, certName, options.Environment); err != nil {
				u.logger.ErrorWithContext("failed to auto-generate SSL certificate", map[string]interface{}{
					"certificate": certName,
					"environment": options.Environment.String(),
					"error": err.Error(),
				})
				return fmt.Errorf("failed to auto-generate SSL certificate for %s: %w", certName, err)
			}

			u.logger.InfoWithContext("SSL certificate auto-generated successfully", map[string]interface{}{
				"certificate": certName,
				"environment": options.Environment.String(),
			})
		}
	}

	u.logger.InfoWithContext("SSL certificate validation completed", map[string]interface{}{
		"environment": options.Environment.String(),
		"certificates_checked": len(requiredCertificates),
	})

	return nil
}

// identifySSLRequirements returns the list of required SSL certificates based on environment
func (u *SSLCertificateUsecase) identifySSLRequirements(env domain.Environment) []string {
	switch env {
	case domain.Production:
		return []string{
			"alt-backend-tls",
			"alt-frontend-tls", 
			"auth-service-tls",
			"nginx-external-tls",
			"kratos-tls",
		}
	case domain.Staging:
		return []string{
			"alt-backend-tls",
			"alt-frontend-tls",
			"auth-service-tls", 
			"nginx-external-tls",
			"kratos-tls",
		}
	case domain.Development:
		return []string{
			"alt-backend-tls",
			"alt-frontend-tls",
		}
	default:
		return []string{}
	}
}

// generateSSLCertificates generates SSL certificates for the application
func (u *SSLCertificateUsecase) generateSSLCertificates(ctx context.Context) error {
	u.logger.InfoWithContext("generating SSL certificates", map[string]interface{}{
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
		NotBefore:    time.Now(),
		NotAfter:     time.Now().AddDate(1, 0, 0), // 1年間有効
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		KeyUsage:     x509.KeyUsageDigitalSignature,
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:     []string{"localhost", "*.alt-app.local", "alt-backend", "alt-frontend", "auth-service"},
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
	u.generatedCertificates = &port.GeneratedCertificates{
		CACert:           string(caCertPEM),
		CAPrivateKey:     string(caPrivateKeyPEM),
		ServerCert:       string(serverCertPEM),
		ServerPrivateKey: string(serverPrivateKeyPEM),
		Generated:        time.Now(),
	}

	u.logger.InfoWithContext("SSL certificates generated successfully", map[string]interface{}{
		"ca_cert_length":      len(caCertPEM),
		"server_cert_length":  len(serverCertPEM),
		"generated_at":        u.generatedCertificates.Generated,
	})

	return nil
}

// validateGeneratedCertificates validates the generated certificates
func (u *SSLCertificateUsecase) validateGeneratedCertificates(ctx context.Context) error {
	if u.generatedCertificates == nil {
		return fmt.Errorf("no certificates generated")
	}

	u.logger.InfoWithContext("validating generated certificates", map[string]interface{}{
		"generated_at": u.generatedCertificates.Generated,
	})

	// Validate CA certificate
	if err := u.validateCertificate(u.generatedCertificates.CACert, "CA"); err != nil {
		return fmt.Errorf("CA certificate validation failed: %w", err)
	}

	// Validate server certificate
	if err := u.validateCertificate(u.generatedCertificates.ServerCert, "Server"); err != nil {
		return fmt.Errorf("Server certificate validation failed: %w", err)
	}

	u.logger.InfoWithContext("certificate validation completed successfully", map[string]interface{}{
		"validated_at": time.Now(),
	})

	return nil
}

// validateCertificate validates a certificate string
func (u *SSLCertificateUsecase) validateCertificate(certPEM, certType string) error {
	if certPEM == "" {
		return fmt.Errorf("certificate is empty")
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

	u.logger.DebugWithContext("certificate validation passed", map[string]interface{}{
		"cert_type":    certType,
		"subject":      cert.Subject.String(),
		"not_before":   cert.NotBefore,
		"not_after":    cert.NotAfter,
		"serial_number": cert.SerialNumber,
	})

	return nil
}

// manageCertificateLifecycle manages the complete certificate lifecycle
func (u *SSLCertificateUsecase) manageCertificateLifecycle(ctx context.Context, environment domain.Environment, chartsDir string) error {
	u.logger.InfoWithContext("managing certificate lifecycle", map[string]interface{}{
		"environment": environment.String(),
		"charts_dir":  chartsDir,
	})

	// Step 1: Generate certificates if needed
	if err := u.generateSSLCertificates(ctx); err != nil {
		return fmt.Errorf("failed to generate SSL certificates: %w", err)
	}

	// Step 2: Validate generated certificates
	if err := u.validateGeneratedCertificates(ctx); err != nil {
		return fmt.Errorf("failed to validate generated certificates: %w", err)
	}

	// Step 3: Inject certificates into chart configurations
	if err := u.injectCertificateData(ctx, chartsDir); err != nil {
		return fmt.Errorf("failed to inject certificate data: %w", err)
	}

	u.logger.InfoWithContext("certificate lifecycle management completed", map[string]interface{}{
		"environment": environment.String(),
	})

	return nil
}

// injectCertificateData injects certificate data into chart configurations
func (u *SSLCertificateUsecase) injectCertificateData(ctx context.Context, chartPath string) error {
	if u.generatedCertificates == nil {
		return fmt.Errorf("no certificates available for injection")
	}

	u.logger.InfoWithContext("injecting certificate data", map[string]interface{}{
		"chart_path": chartPath,
	})

	// Extract chart name from path
	chartName := filepath.Base(chartPath)
	
	// Create values file for SSL configuration
	valuesFile := filepath.Join(chartPath, "values-ssl.yaml")
	
	// Create SSL configuration based on chart type
	var sslConfig map[string]interface{}
	
	switch chartName {
	case "common-ssl":
		sslConfig = map[string]interface{}{
			"ssl": map[string]interface{}{
				"enabled": true,
				"ca": map[string]interface{}{
					"cert": u.generatedCertificates.CACert,
					"key":  u.generatedCertificates.CAPrivateKey,
				},
				"server": map[string]interface{}{
					"cert": u.generatedCertificates.ServerCert,
					"key":  u.generatedCertificates.ServerPrivateKey,
				},
			},
		}
	case "alt-backend", "alt-frontend", "auth-service", "nginx-external", "kratos":
		sslConfig = map[string]interface{}{
			"ssl": map[string]interface{}{
				"enabled": true,
				"tls": map[string]interface{}{
					"cert": u.generatedCertificates.ServerCert,
					"key":  u.generatedCertificates.ServerPrivateKey,
				},
				"ca": map[string]interface{}{
					"cert": u.generatedCertificates.CACert,
				},
			},
		}
	default:
		// Skip injection for charts that don't need SSL
		u.logger.InfoWithContext("skipping SSL injection for chart", map[string]interface{}{
			"chart_name": chartName,
		})
		return nil
	}

	// Write SSL configuration to values file
	if err := u.writeSSLValuesFile(valuesFile, sslConfig); err != nil {
		return fmt.Errorf("failed to write SSL values file: %w", err)
	}
	
	u.logger.InfoWithContext("certificate data injection completed", map[string]interface{}{
		"chart_path": chartPath,
		"chart_name": chartName,
		"values_file": valuesFile,
	})

	return nil
}

// writeSSLValuesFile writes SSL configuration to a YAML values file
func (u *SSLCertificateUsecase) writeSSLValuesFile(filename string, config map[string]interface{}) error {
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
	if err := encoder.Encode(config); err != nil {
		return fmt.Errorf("failed to encode SSL config: %w", err)
	}

	return nil
}