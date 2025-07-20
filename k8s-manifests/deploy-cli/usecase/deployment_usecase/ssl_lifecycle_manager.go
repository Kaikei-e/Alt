package deployment_usecase

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"deploy-cli/domain"
	"deploy-cli/port/logger_port"
	"deploy-cli/usecase/secret_usecase"
	"gopkg.in/yaml.v2"
)

// SSLLifecycleManager handles SSL certificate lifecycle management
type SSLLifecycleManager struct {
	certificateManager *SSLCertificateManager
	validationUtils    *SSLValidationUtils
	secretUsecase      *secret_usecase.SecretUsecase
	logger             logger_port.LoggerPort
}

// NewSSLLifecycleManager creates a new SSL lifecycle manager
func NewSSLLifecycleManager(
	certificateManager *SSLCertificateManager,
	validationUtils *SSLValidationUtils,
	secretUsecase *secret_usecase.SecretUsecase,
	logger logger_port.LoggerPort,
) *SSLLifecycleManager {
	return &SSLLifecycleManager{
		certificateManager: certificateManager,
		validationUtils:    validationUtils,
		secretUsecase:      secretUsecase,
		logger:             logger,
	}
}

// ManageCertificateLifecycle manages SSL certificate lifecycle
func (s *SSLLifecycleManager) ManageCertificateLifecycle(ctx context.Context, environment domain.Environment, chartsDir string) error {
	s.logger.InfoWithContext("starting SSL certificate lifecycle management", map[string]interface{}{
		"environment": environment.String(),
		"charts_dir":  chartsDir,
	})

	certificatesAvailable := false

	// Step 1: Try to load existing certificates from Kubernetes secrets
	if err := s.certificateManager.LoadExistingCertificates(ctx, environment); err != nil {
		s.logger.WarnWithContext("failed to load existing certificates, will generate new ones", map[string]interface{}{
			"environment": environment.String(),
			"error":       err.Error(),
		})

		// Fall back to generating new certificates
		if err := s.certificateManager.GenerateSSLCertificates(ctx); err != nil {
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

// ValidateGeneratedCertificates validates the SSL certificates (either loaded or generated)
func (s *SSLLifecycleManager) ValidateGeneratedCertificates(ctx context.Context) error {
	certificates := s.certificateManager.GetGeneratedCertificates()
	if certificates == nil {
		return fmt.Errorf("no certificates available to validate")
	}

	s.logger.InfoWithContext("validating SSL certificates", map[string]interface{}{
		"loaded_time": certificates.Generated.Format(time.RFC3339),
	})

	// Validate CA certificate (PEM format, not base64)
	if err := s.certificateManager.ValidateCertificatePEM(certificates.CACert, "CA"); err != nil {
		return fmt.Errorf("CA certificate validation failed: %w", err)
	}

	// Validate server certificate (PEM format, not base64)
	if err := s.certificateManager.ValidateCertificatePEM(certificates.ServerCert, "Server"); err != nil {
		return fmt.Errorf("Server certificate validation failed: %w", err)
	}

	s.logger.InfoWithContext("SSL certificate validation completed successfully", map[string]interface{}{
		"validated_at": time.Now().Format(time.RFC3339),
	})

	return nil
}

// InjectCertificateData injects certificate data into chart configurations
func (s *SSLLifecycleManager) InjectCertificateData(ctx context.Context, chartPath string) error {
	certificates := s.certificateManager.GetGeneratedCertificates()
	if certificates == nil {
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
func (s *SSLLifecycleManager) InjectCommonSSLCertificates(ctx context.Context, chartPath string) error {
	certificates := s.certificateManager.GetGeneratedCertificates()
	valuesFile := filepath.Join(chartPath, "values-ssl.yaml")

	// Create SSL configuration for common-ssl chart
	sslConfig := map[string]interface{}{
		"ssl": map[string]interface{}{
			"enabled": true,
			"ca": map[string]interface{}{
				"cert": certificates.CACert,
				"key":  certificates.CAPrivateKey,
			},
			"server": map[string]interface{}{
				"cert": certificates.ServerCert,
				"key":  certificates.ServerPrivateKey,
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
func (s *SSLLifecycleManager) InjectServiceSSLCertificates(ctx context.Context, chartPath string, chartName string) error {
	certificates := s.certificateManager.GetGeneratedCertificates()
	valuesFile := filepath.Join(chartPath, "values-ssl.yaml")

	// Create SSL configuration for service charts
	sslConfig := map[string]interface{}{
		"ssl": map[string]interface{}{
			"enabled": true,
			"tls": map[string]interface{}{
				"cert": certificates.ServerCert,
				"key":  certificates.ServerPrivateKey,
			},
			"ca": map[string]interface{}{
				"cert": certificates.CACert,
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
func (s *SSLLifecycleManager) writeSSLValuesFile(filename string, config map[string]interface{}) error {
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
func (s *SSLLifecycleManager) GenerateSSLCertificateSecrets(ctx context.Context, environment domain.Environment) error {
	s.logger.InfoWithContext("generating SSL certificate secrets", map[string]interface{}{
		"environment": environment.String(),
	})

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
		// Get the appropriate namespace for each service
		serviceNamespace := s.getNamespaceForService(service, environment)
		secretName := s.getChartCompatibleSecretName(service)

		if err := s.certificateManager.CreateSSLCertificateSecret(ctx, service, secretName, serviceNamespace); err != nil {
			s.logger.WarnWithContext("failed to create SSL certificate secret", map[string]interface{}{
				"service":     service,
				"secret_name": secretName,
				"namespace":   serviceNamespace,
				"error":       err.Error(),
			})
			// Continue with other services
		}
	}

	s.logger.InfoWithContext("SSL certificate secrets generation completed", map[string]interface{}{
		"environment": environment.String(),
		"services":    len(services),
	})

	return nil
}

// CSRGenerationConfig represents CSR generation configuration
type CSRGenerationConfig struct {
	ServiceName  string
	Namespace    string
	DNSNames     []string
	IPAddresses  []net.IP
	SignerName   string
	KeySize      int
	Organization []string
}

// GenerateCSRForService generates CSR for a specific service
func (s *SSLLifecycleManager) GenerateCSRForService(ctx context.Context, config CSRGenerationConfig) error {
	s.logger.InfoWithContext("generating CSR for service", map[string]interface{}{
		"service_name": config.ServiceName,
		"namespace":    config.Namespace,
		"dns_names":    config.DNSNames,
		"ip_addresses": config.IPAddresses,
		"signer_name":  config.SignerName,
	})

	// 1. Generate private key
	privateKey, err := s.generatePrivateKey(config.KeySize)
	if err != nil {
		return fmt.Errorf("failed to generate private key: %w", err)
	}

	// 2. Create CSR template
	template := &x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName:   config.ServiceName,
			Organization: config.Organization,
		},
		DNSNames:    config.DNSNames,
		IPAddresses: config.IPAddresses,
	}

	// 3. Generate CSR
	csrDER, err := x509.CreateCertificateRequest(rand.Reader, template, privateKey)
	if err != nil {
		return fmt.Errorf("failed to create CSR: %w", err)
	}

	// 4. Create CertificateSigningRequest resource
	csrName := fmt.Sprintf("%s-%s", config.ServiceName, config.Namespace)
	csrPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER})

	s.logger.InfoWithContext("CSR generated successfully", map[string]interface{}{
		"csr_name":       csrName,
		"csr_pem_length": len(csrPEM),
	})

	// 5. Submit CSR to Kubernetes API
	return s.submitCSR(ctx, csrName, csrPEM, config.SignerName, privateKey)
}

// generatePrivateKey generates a new RSA private key
func (s *SSLLifecycleManager) generatePrivateKey(keySize int) (*rsa.PrivateKey, error) {
	if keySize == 0 {
		keySize = 2048 // Default key size
	}

	privateKey, err := rsa.GenerateKey(rand.Reader, keySize)
	if err != nil {
		return nil, fmt.Errorf("failed to generate RSA private key: %w", err)
	}

	return privateKey, nil
}

// submitCSR submits a CertificateSigningRequest to Kubernetes API
func (s *SSLLifecycleManager) submitCSR(ctx context.Context, csrName string, csrPEM []byte, signerName string, privateKey *rsa.PrivateKey) error {
	s.logger.InfoWithContext("submitting CSR to Kubernetes API", map[string]interface{}{
		"csr_name":    csrName,
		"signer_name": signerName,
	})

	// TODO: Implement actual Kubernetes API submission
	// This will be implemented when we have the Kubernetes client integration

	// For now, we'll log the CSR details and store the private key
	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})

	s.logger.InfoWithContext("CSR submission prepared", map[string]interface{}{
		"csr_name":           csrName,
		"csr_pem_length":     len(csrPEM),
		"private_key_length": len(privateKeyPEM),
	})

	// Store for later use
	s.storePendingCSR(csrName, csrPEM, privateKeyPEM, signerName)

	return nil
}

// storePendingCSR stores a pending CSR for later processing
func (s *SSLLifecycleManager) storePendingCSR(csrName string, csrPEM, privateKeyPEM []byte, signerName string) {
	s.logger.InfoWithContext("storing pending CSR", map[string]interface{}{
		"csr_name":    csrName,
		"signer_name": signerName,
	})

	// TODO: Implement persistent storage for pending CSRs
	// This could be stored in a ConfigMap or Secret for later retrieval
}

// DistributeCertificatesConfig represents certificate distribution configuration
type DistributeCertificatesConfig struct {
	Environment   domain.Environment
	Namespaces    []string
	Services      []string
	UseProjection bool
}

// DistributeCertificates distributes certificates across namespaces
func (s *SSLLifecycleManager) DistributeCertificates(ctx context.Context, config DistributeCertificatesConfig) error {
	s.logger.InfoWithContext("starting certificate distribution", map[string]interface{}{
		"environment":    config.Environment.String(),
		"namespaces":     config.Namespaces,
		"services":       config.Services,
		"use_projection": config.UseProjection,
	})

	if config.UseProjection {
		return s.distributeViaProjectedVolumes(ctx, config)
	}

	return s.distributeViaSecrets(ctx, config)
}

// distributeViaProjectedVolumes uses ServiceAccount token volume projection
func (s *SSLLifecycleManager) distributeViaProjectedVolumes(ctx context.Context, config DistributeCertificatesConfig) error {
	s.logger.InfoWithContext("distributing certificates via projected volumes", map[string]interface{}{
		"environment": config.Environment.String(),
	})

	for _, namespace := range config.Namespaces {
		for _, service := range config.Services {
			s.logger.InfoWithContext("processing service for projected volume distribution", map[string]interface{}{
				"service":   service,
				"namespace": namespace,
			})

			// 1. Create ServiceAccount
			if err := s.createServiceAccount(ctx, service, namespace); err != nil {
				s.logger.ErrorWithContext("failed to create ServiceAccount", map[string]interface{}{
					"service":   service,
					"namespace": namespace,
					"error":     err.Error(),
				})
				return fmt.Errorf("failed to create ServiceAccount: %w", err)
			}

			// 2. Create projected volume configuration
			if err := s.createProjectedVolumeConfig(ctx, service, namespace); err != nil {
				s.logger.ErrorWithContext("failed to create projected volume config", map[string]interface{}{
					"service":   service,
					"namespace": namespace,
					"error":     err.Error(),
				})
				return fmt.Errorf("failed to create projected volume config: %w", err)
			}

			// 3. Update deployment to use projected volumes
			if err := s.updateDeploymentWithProjectedVolumes(ctx, service, namespace); err != nil {
				s.logger.ErrorWithContext("failed to update deployment", map[string]interface{}{
					"service":   service,
					"namespace": namespace,
					"error":     err.Error(),
				})
				return fmt.Errorf("failed to update deployment: %w", err)
			}
		}
	}

	s.logger.InfoWithContext("certificate distribution via projected volumes completed", map[string]interface{}{
		"environment": config.Environment.String(),
	})

	return nil
}

// distributeViaSecrets distributes certificates via traditional secrets
func (s *SSLLifecycleManager) distributeViaSecrets(ctx context.Context, config DistributeCertificatesConfig) error {
	s.logger.InfoWithContext("distributing certificates via secrets", map[string]interface{}{
		"environment": config.Environment.String(),
	})

	// Use existing secret distribution logic
	return s.GenerateSSLCertificateSecrets(ctx, config.Environment)
}

// createServiceAccount creates a ServiceAccount for certificate distribution
func (s *SSLLifecycleManager) createServiceAccount(ctx context.Context, service, namespace string) error {
	s.logger.InfoWithContext("creating ServiceAccount", map[string]interface{}{
		"service":   service,
		"namespace": namespace,
	})

	// TODO: Implement ServiceAccount creation via Kubernetes API
	// This will be implemented when we have the Kubernetes client integration

	return nil
}

// createProjectedVolumeConfig creates projected volume configuration
func (s *SSLLifecycleManager) createProjectedVolumeConfig(ctx context.Context, service, namespace string) error {
	s.logger.InfoWithContext("creating projected volume config", map[string]interface{}{
		"service":   service,
		"namespace": namespace,
	})

	// TODO: Implement projected volume configuration
	// This will create the necessary ConfigMaps and Secrets for projected volumes

	return nil
}

// updateDeploymentWithProjectedVolumes updates deployment to use projected volumes
func (s *SSLLifecycleManager) updateDeploymentWithProjectedVolumes(ctx context.Context, service, namespace string) error {
	s.logger.InfoWithContext("updating deployment with projected volumes", map[string]interface{}{
		"service":   service,
		"namespace": namespace,
	})

	// TODO: Implement deployment update logic
	// This will modify the deployment to use projected volumes for certificates

	return nil
}

// getNamespaceForService returns the appropriate namespace for a service in the given environment
func (s *SSLLifecycleManager) getNamespaceForService(serviceName string, env domain.Environment) string {
	// Use the same logic as domain.DetermineNamespace to ensure consistency
	return domain.DetermineNamespace(serviceName, env)
}

// getChartCompatibleSecretName returns the secret name expected by Helm charts
func (s *SSLLifecycleManager) getChartCompatibleSecretName(serviceName string) string {
	// Map service names to their chart-expected secret names
	secretNameMappings := map[string]string{
		"postgres":         "postgres-ssl-secret",
		"auth-postgres":    "auth-postgres-ssl-certs",
		"kratos-postgres":  "kratos-postgres-ssl-certs",
		"clickhouse":       "clickhouse-ssl-certs",
		"meilisearch":      "meilisearch-ssl-certs",
		"nginx-external":   "nginx-external-ssl-certs",
		"nginx":            "nginx-ssl-certs",
		"kratos":           "kratos-ssl-certs",
		"alt-backend":      "alt-backend-ssl-certs-prod",
		"alt-frontend":     "alt-frontend-ssl-certs-prod",
		"auth-service":     "auth-service-ssl-certs-prod",
	}

	// Return mapped name if exists, otherwise use the old pattern as fallback
	if mappedName, exists := secretNameMappings[serviceName]; exists {
		return mappedName
	}

	// Fallback to old pattern for unmapped services
	return fmt.Sprintf("%s-ssl-certs-prod", serviceName)
}
