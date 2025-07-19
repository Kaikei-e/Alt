package deployment_usecase

import (
	"context"
	"time"

	"deploy-cli/domain"
	"deploy-cli/port"
	"deploy-cli/port/logger_port"
	"deploy-cli/usecase/secret_usecase"
)

// SSLManagementUsecase handles SSL certificate lifecycle management using composed components
type SSLManagementUsecaseRefactored struct {
	certificateManager *SSLCertificateManager
	lifecycleManager   *SSLLifecycleManager
	validationUtils    *SSLValidationUtils
	logger             logger_port.LoggerPort
}

// NewSSLManagementUsecaseRefactored creates a new SSL management usecase with composed components
func NewSSLManagementUsecaseRefactored(
	secretUsecase *secret_usecase.SecretUsecase,
	sslUsecase *secret_usecase.SSLCertificateUsecase,
	logger logger_port.LoggerPort,
) *SSLManagementUsecaseRefactored {
	// Create certificate manager
	certificateManager := NewSSLCertificateManager(secretUsecase, sslUsecase, logger)

	// Create validation utils
	validationUtils := NewSSLValidationUtils(logger)

	// Create lifecycle manager
	lifecycleManager := NewSSLLifecycleManager(certificateManager, validationUtils, secretUsecase, logger)

	return &SSLManagementUsecaseRefactored{
		certificateManager: certificateManager,
		lifecycleManager:   lifecycleManager,
		validationUtils:    validationUtils,
		logger:             logger,
	}
}

// ManageCertificateLifecycle manages SSL certificate lifecycle - delegates to lifecycle manager
func (s *SSLManagementUsecaseRefactored) ManageCertificateLifecycle(ctx context.Context, environment domain.Environment, chartsDir string) error {
	return s.lifecycleManager.ManageCertificateLifecycle(ctx, environment, chartsDir)
}

// LoadExistingCertificates loads existing SSL certificates - delegates to certificate manager
func (s *SSLManagementUsecaseRefactored) LoadExistingCertificates(ctx context.Context, environment domain.Environment) error {
	return s.certificateManager.LoadExistingCertificates(ctx, environment)
}

// GenerateSSLCertificates generates SSL certificates - delegates to certificate manager
func (s *SSLManagementUsecaseRefactored) GenerateSSLCertificates(ctx context.Context) error {
	return s.certificateManager.GenerateSSLCertificates(ctx)
}

// ValidateGeneratedCertificates validates certificates - delegates to lifecycle manager
func (s *SSLManagementUsecaseRefactored) ValidateGeneratedCertificates(ctx context.Context) error {
	return s.lifecycleManager.ValidateGeneratedCertificates(ctx)
}

// InjectCertificateData injects certificate data - delegates to lifecycle manager
func (s *SSLManagementUsecaseRefactored) InjectCertificateData(ctx context.Context, chartPath string) error {
	return s.lifecycleManager.InjectCertificateData(ctx, chartPath)
}

// CreateSSLCertificateSecret creates SSL certificate secrets - delegates to certificate manager
func (s *SSLManagementUsecaseRefactored) CreateSSLCertificateSecret(ctx context.Context, serviceName, secretName, namespace string) error {
	return s.certificateManager.CreateSSLCertificateSecret(ctx, serviceName, secretName, namespace)
}

// GenerateSSLCertificateSecrets generates SSL certificate secrets for all services - delegates to lifecycle manager
func (s *SSLManagementUsecaseRefactored) GenerateSSLCertificateSecrets(ctx context.Context, environment domain.Environment) error {
	return s.lifecycleManager.GenerateSSLCertificateSecrets(ctx, environment)
}

// ValidateCertificatePEM validates a certificate in PEM format - delegates to certificate manager
func (s *SSLManagementUsecaseRefactored) ValidateCertificatePEM(certPEM, certType string) error {
	return s.certificateManager.ValidateCertificatePEM(certPEM, certType)
}

// ValidateCertificate validates a certificate in base64 format - delegates to certificate manager
func (s *SSLManagementUsecaseRefactored) ValidateCertificate(certBase64, certType string) error {
	return s.certificateManager.ValidateCertificate(certBase64, certType)
}

// GetGeneratedCertificates returns the generated certificates - delegates to certificate manager
func (s *SSLManagementUsecaseRefactored) GetGeneratedCertificates() *port.GeneratedCertificates {
	return s.certificateManager.GetGeneratedCertificates()
}

// HasCertificates returns true if certificates are available - delegates to certificate manager
func (s *SSLManagementUsecaseRefactored) HasCertificates() bool {
	return s.certificateManager.HasCertificates()
}

// GetCertificateGenerationTime returns the time when certificates were generated/loaded - delegates to certificate manager
func (s *SSLManagementUsecaseRefactored) GetCertificateGenerationTime() time.Time {
	return s.certificateManager.GetCertificateGenerationTime()
}

// ValidatePEMData validates PEM-encoded certificate and key data - delegates to validation utils
func (s *SSLManagementUsecaseRefactored) ValidatePEMData(caCert, caKey, serverCert, serverKey string) error {
	return s.validationUtils.ValidatePEMData(caCert, caKey, serverCert, serverKey)
}

// ValidateSinglePEM validates a single PEM-encoded data - delegates to validation utils
func (s *SSLManagementUsecaseRefactored) ValidateSinglePEM(name, data, expectedType string) error {
	return s.validationUtils.ValidateSinglePEM(name, data, expectedType)
}

// ValidateCertificateStructure validates that certificates can be parsed - delegates to validation utils
func (s *SSLManagementUsecaseRefactored) ValidateCertificateStructure(caCert, serverCert string) error {
	return s.validationUtils.ValidateCertificateStructure(caCert, serverCert)
}

// GenerateCSRForService generates CSR for a specific service - delegates to lifecycle manager
func (s *SSLManagementUsecaseRefactored) GenerateCSRForService(ctx context.Context, config CSRGenerationConfig) error {
	return s.lifecycleManager.GenerateCSRForService(ctx, config)
}

// DistributeCertificates distributes certificates across namespaces - delegates to lifecycle manager
func (s *SSLManagementUsecaseRefactored) DistributeCertificates(ctx context.Context, config DistributeCertificatesConfig) error {
	return s.lifecycleManager.DistributeCertificates(ctx, config)
}
