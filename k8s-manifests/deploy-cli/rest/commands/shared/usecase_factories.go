// PHASE R3: Usecase factories for dependency injection
package shared

import (
	"deploy-cli/gateway/filesystem_gateway"
	"deploy-cli/gateway/helm_gateway"
	"deploy-cli/gateway/kubectl_gateway"
	"deploy-cli/gateway/system_gateway"
	"deploy-cli/usecase/deployment_usecase"
	"deploy-cli/usecase/secret_usecase"
)

// DeploymentUsecaseFactory creates deployment usecases with proper dependencies
type DeploymentUsecaseFactory struct {
	shared *CommandShared
}

// NewDeploymentUsecaseFactory creates a new deployment usecase factory
func NewDeploymentUsecaseFactory(shared *CommandShared) *DeploymentUsecaseFactory {
	return &DeploymentUsecaseFactory{
		shared: shared,
	}
}

// CreateDeploymentUsecase creates a fully configured deployment usecase
func (f *DeploymentUsecaseFactory) CreateDeploymentUsecase() *deployment_usecase.DeploymentUsecase {
	// Create gateways
	systemGateway := system_gateway.NewSystemGateway(f.shared.SystemDriver, f.shared.LoggerPort)
	helmGateway := helm_gateway.NewHelmGateway(f.shared.HelmDriver, f.shared.LoggerPort)
	kubectlGateway := kubectl_gateway.NewKubectlGateway(f.shared.KubectlDriver, f.shared.LoggerPort)
	filesystemGateway := filesystem_gateway.NewFileSystemGateway(f.shared.FilesystemDriver, f.shared.LoggerPort)

	// Create secret usecase
	secretUsecase := f.shared.SecretUsecaseFactory.CreateSecretUsecase()

	// Create SSL certificate usecase
	sslUsecase := secret_usecase.NewSSLCertificateUsecase(secretUsecase, f.shared.LoggerPort)

	// Create deployment usecase with all dependencies
	return deployment_usecase.NewDeploymentUsecase(
		helmGateway,
		kubectlGateway,
		filesystemGateway,
		systemGateway,
		secretUsecase,
		sslUsecase,
		f.shared.LoggerPort,
		f.shared.FilesystemDriver,
	)
}

// CreateRefactoredDeploymentUsecase creates the new refactored deployment usecase
func (f *DeploymentUsecaseFactory) CreateRefactoredDeploymentUsecase() interface{} {
	// This would create the refactored deployment usecase when Phase R1 integration is ready
	// For now, return the standard usecase
	return f.CreateDeploymentUsecase()
}

// SecretUsecaseFactory creates secret usecases with proper dependencies
type SecretUsecaseFactory struct {
	shared *CommandShared
}

// NewSecretUsecaseFactory creates a new secret usecase factory
func NewSecretUsecaseFactory(shared *CommandShared) *SecretUsecaseFactory {
	return &SecretUsecaseFactory{
		shared: shared,
	}
}

// CreateSecretUsecase creates a fully configured secret usecase
func (f *SecretUsecaseFactory) CreateSecretUsecase() *secret_usecase.SecretUsecase {
	kubectlGateway := kubectl_gateway.NewKubectlGateway(f.shared.KubectlDriver, f.shared.LoggerPort)
	
	return secret_usecase.NewSecretUsecase(kubectlGateway, f.shared.LoggerPort)
}

// CreateSSLCertificateUsecase creates an SSL certificate usecase
func (f *SecretUsecaseFactory) CreateSSLCertificateUsecase() *secret_usecase.SSLCertificateUsecase {
	secretUsecase := f.CreateSecretUsecase()
	
	return secret_usecase.NewSSLCertificateUsecase(secretUsecase, f.shared.LoggerPort)
}

// MonitoringUsecaseFactory creates monitoring usecases (placeholder for future implementation)
type MonitoringUsecaseFactory struct {
	shared *CommandShared
}

// NewMonitoringUsecaseFactory creates a new monitoring usecase factory
func NewMonitoringUsecaseFactory(shared *CommandShared) *MonitoringUsecaseFactory {
	return &MonitoringUsecaseFactory{
		shared: shared,
	}
}

// CreateHealthCheckUsecase creates a health check usecase (placeholder)
func (f *MonitoringUsecaseFactory) CreateHealthCheckUsecase() interface{} {
	// This would create health check usecase when monitoring commands are implemented
	// For now, return nil as placeholder
	return nil
}

// CreateMetricsCollectionUsecase creates a metrics collection usecase (placeholder)
func (f *MonitoringUsecaseFactory) CreateMetricsCollectionUsecase() interface{} {
	// This would create metrics collection usecase when monitoring commands are implemented
	// For now, return nil as placeholder
	return nil
}

// MaintenanceUsecaseFactory creates maintenance usecases (placeholder for future implementation)
type MaintenanceUsecaseFactory struct {
	shared *CommandShared
}

// NewMaintenanceUsecaseFactory creates a new maintenance usecase factory
func NewMaintenanceUsecaseFactory(shared *CommandShared) *MaintenanceUsecaseFactory {
	return &MaintenanceUsecaseFactory{
		shared: shared,
	}
}

// CreateCleanupUsecase creates a cleanup usecase (placeholder)
func (f *MaintenanceUsecaseFactory) CreateCleanupUsecase() interface{} {
	// This would create cleanup usecase when maintenance commands are implemented
	// For now, return nil as placeholder
	return nil
}

// CreateTroubleshootUsecase creates a troubleshoot usecase (placeholder)
func (f *MaintenanceUsecaseFactory) CreateTroubleshootUsecase() interface{} {
	// This would create troubleshoot usecase when maintenance commands are implemented  
	// For now, return nil as placeholder
	return nil
}