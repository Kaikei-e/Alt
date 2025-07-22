// Phase R4: 環境別設定 - development/staging/production環境固有の設定管理
package config

import (
	"fmt"
	"strings"
	"time"
)

// Environment represents the deployment environment
type Environment string

const (
	// Development environment
	Development Environment = "development"
	// Staging environment
	Staging Environment = "staging"
	// Production environment
	Production Environment = "production"
)

// EnvironmentConfig manages environment-specific configuration
type EnvironmentConfig struct {
	manager     *ConfigManager
	environment Environment
}

// NewEnvironmentConfig creates a new environment-specific configuration manager
func NewEnvironmentConfig(environment Environment) (*EnvironmentConfig, error) {
	if !isValidEnvironment(environment) {
		return nil, fmt.Errorf("invalid environment: %s", environment)
	}

	manager := NewConfigManager(fmt.Sprintf("DEPLOY_CLI_%s", strings.ToUpper(string(environment))))
	
	ec := &EnvironmentConfig{
		manager:     manager,
		environment: environment,
	}

	// Set environment-specific defaults
	ec.setEnvironmentDefaults()
	
	// Add environment-specific config paths
	ec.addEnvironmentConfigPaths()

	return ec, nil
}

// isValidEnvironment checks if the environment is valid
func isValidEnvironment(env Environment) bool {
	switch env {
	case Development, Staging, Production:
		return true
	default:
		return false
	}
}

// setEnvironmentDefaults sets default values based on environment
func (ec *EnvironmentConfig) setEnvironmentDefaults() {
	defaults := ec.getEnvironmentDefaults()
	ec.manager.SetDefaults(defaults)
}

// getEnvironmentDefaults returns environment-specific default values
func (ec *EnvironmentConfig) getEnvironmentDefaults() map[string]interface{} {
	commonDefaults := map[string]interface{}{
		"helm.timeout":                    time.Duration(10 * time.Minute),
		"kubectl.timeout":                 time.Duration(5 * time.Minute),
		"deployment.parallel.enabled":     true,
		"deployment.parallel.max_workers": 3,
		"logging.level":                   "info",
		"logging.format":                  "json",
		"metrics.enabled":                 true,
		"health_check.timeout":            time.Duration(30 * time.Second),
		"health_check.interval":           time.Duration(10 * time.Second),
		"health_check.retries":            3,
		"ssl.verify_certificates":         true,
		"ssl.certificate_path":            "/etc/ssl/certs",
		"cleanup.auto_cleanup":            false,
		"cleanup.retention_days":          30,
	}

	switch ec.environment {
	case Development:
		return ec.getDevelopmentDefaults(commonDefaults)
	case Staging:
		return ec.getStagingDefaults(commonDefaults)
	case Production:
		return ec.getProductionDefaults(commonDefaults)
	default:
		return commonDefaults
	}
}

// getDevelopmentDefaults returns development-specific defaults
func (ec *EnvironmentConfig) getDevelopmentDefaults(common map[string]interface{}) map[string]interface{} {
	// Development environment optimized for speed and debugging
	common["helm.timeout"] = time.Duration(5 * time.Minute)
	common["kubectl.timeout"] = time.Duration(2 * time.Minute)
	common["deployment.parallel.max_workers"] = 2
	common["logging.level"] = "debug"
	common["logging.format"] = "text"
	common["health_check.timeout"] = time.Duration(15 * time.Second)
	common["health_check.retries"] = 2
	common["ssl.verify_certificates"] = false
	common["cleanup.auto_cleanup"] = true
	common["cleanup.retention_days"] = 7
	common["metrics.detailed"] = true
	common["debug.enabled"] = true
	common["debug.verbose"] = true
	common["emergency.mode_timeout"] = time.Duration(2 * time.Minute)
	
	return common
}

// getStagingDefaults returns staging-specific defaults
func (ec *EnvironmentConfig) getStagingDefaults(common map[string]interface{}) map[string]interface{} {
	// Staging environment balanced for testing and performance
	common["helm.timeout"] = time.Duration(8 * time.Minute)
	common["kubectl.timeout"] = time.Duration(4 * time.Minute)
	common["deployment.parallel.max_workers"] = 3
	common["logging.level"] = "info"
	common["health_check.timeout"] = time.Duration(25 * time.Second)
	common["health_check.retries"] = 3
	common["ssl.verify_certificates"] = true
	common["cleanup.auto_cleanup"] = false
	common["cleanup.retention_days"] = 14
	common["metrics.detailed"] = false
	common["debug.enabled"] = false
	common["emergency.mode_timeout"] = time.Duration(5 * time.Minute)
	
	return common
}

// getProductionDefaults returns production-specific defaults
func (ec *EnvironmentConfig) getProductionDefaults(common map[string]interface{}) map[string]interface{} {
	// Production environment optimized for reliability and security
	common["helm.timeout"] = time.Duration(20 * time.Minute)
	common["kubectl.timeout"] = time.Duration(10 * time.Minute)
	common["deployment.parallel.max_workers"] = 5
	common["logging.level"] = "warn"
	common["health_check.timeout"] = time.Duration(45 * time.Second)
	common["health_check.retries"] = 5
	common["ssl.verify_certificates"] = true
	common["cleanup.auto_cleanup"] = false
	common["cleanup.retention_days"] = 90
	common["metrics.detailed"] = false
	common["debug.enabled"] = false
	common["emergency.mode_timeout"] = time.Duration(10 * time.Minute)
	common["security.strict_mode"] = true
	common["security.audit_logs"] = true
	
	return common
}

// addEnvironmentConfigPaths adds environment-specific configuration file paths
func (ec *EnvironmentConfig) addEnvironmentConfigPaths() {
	// Add paths in order of priority (lower index = higher priority)
	paths := []string{
		fmt.Sprintf("/etc/deploy-cli/%s.json", ec.environment),
		fmt.Sprintf("./config/%s.json", ec.environment),
		fmt.Sprintf("./config/%s.env", ec.environment),
		fmt.Sprintf("./%s.config.json", ec.environment),
		fmt.Sprintf("./%s.env", ec.environment),
		"./config.json",
		"./.env",
	}

	for _, path := range paths {
		ec.manager.AddConfigPath(path)
	}
}

// LoadConfig loads configuration from all sources
func (ec *EnvironmentConfig) LoadConfig() error {
	return ec.manager.LoadConfig()
}

// GetEnvironment returns the current environment
func (ec *EnvironmentConfig) GetEnvironment() Environment {
	return ec.environment
}

// GetManager returns the underlying config manager
func (ec *EnvironmentConfig) GetManager() *ConfigManager {
	return ec.manager
}

// GetHelmConfig returns Helm-specific configuration
func (ec *EnvironmentConfig) GetHelmConfig() *HelmConfig {
	return &HelmConfig{
		Timeout:               ec.manager.GetDuration("helm.timeout"),
		MaxRetries:           ec.manager.GetInt("helm.max_retries"),
		RetryDelay:           ec.manager.GetDuration("helm.retry_delay"),
		DryRun:               ec.manager.GetBool("helm.dry_run"),
		SkipCRDs:             ec.manager.GetBool("helm.skip_crds"),
		DisableHooks:         ec.manager.GetBool("helm.disable_hooks"),
		Force:                ec.manager.GetBool("helm.force"),
		Wait:                 ec.manager.GetBool("helm.wait"),
		WaitForJobs:          ec.manager.GetBool("helm.wait_for_jobs"),
		Debug:                ec.manager.GetBool("helm.debug"),
		RepositoryConfig:     ec.manager.GetString("helm.repository_config"),
		RepositoryCache:      ec.manager.GetString("helm.repository_cache"),
	}
}

// GetKubectlConfig returns kubectl-specific configuration
func (ec *EnvironmentConfig) GetKubectlConfig() *KubectlConfig {
	return &KubectlConfig{
		Timeout:              ec.manager.GetDuration("kubectl.timeout"),
		MaxRetries:          ec.manager.GetInt("kubectl.max_retries"),
		RetryDelay:          ec.manager.GetDuration("kubectl.retry_delay"),
		DryRun:              ec.manager.GetBool("kubectl.dry_run"),
		ValidateYAML:        ec.manager.GetBool("kubectl.validate_yaml"),
		ServerSideApply:     ec.manager.GetBool("kubectl.server_side_apply"),
		ForceConflicts:      ec.manager.GetBool("kubectl.force_conflicts"),
		KubeconfigPath:      ec.manager.GetString("kubectl.kubeconfig_path"),
		Context:             ec.manager.GetString("kubectl.context"),
		Namespace:           ec.manager.GetString("kubectl.namespace"),
	}
}

// GetDeploymentConfig returns deployment-specific configuration
func (ec *EnvironmentConfig) GetDeploymentConfig() *DeploymentConfig {
	return &DeploymentConfig{
		ParallelEnabled:          ec.manager.GetBool("deployment.parallel.enabled"),
		MaxParallelWorkers:       ec.manager.GetInt("deployment.parallel.max_workers"),
		LayerTimeout:             ec.manager.GetDuration("deployment.layer_timeout"),
		ChartDeploymentTimeout:   ec.manager.GetDuration("deployment.chart_timeout"),
		HealthCheckEnabled:       ec.manager.GetBool("deployment.health_check.enabled"),
		HealthCheckTimeout:       ec.manager.GetDuration("health_check.timeout"),
		HealthCheckInterval:      ec.manager.GetDuration("health_check.interval"),
		HealthCheckRetries:       ec.manager.GetInt("health_check.retries"),
		RollbackOnFailure:        ec.manager.GetBool("deployment.rollback_on_failure"),
		AutoCleanupOnSuccess:     ec.manager.GetBool("deployment.auto_cleanup"),
		EmergencyModeTimeout:     ec.manager.GetDuration("emergency.mode_timeout"),
		ValidateResourcesBeforeDeploy: ec.manager.GetBool("deployment.validate_resources"),
	}
}

// GetLoggingConfig returns logging-specific configuration
func (ec *EnvironmentConfig) GetLoggingConfig() *LoggingConfig {
	return &LoggingConfig{
		Level:               ec.manager.GetString("logging.level"),
		Format:              ec.manager.GetString("logging.format"),
		Output:              ec.manager.GetString("logging.output"),
		FileRotation:        ec.manager.GetBool("logging.file_rotation"),
		MaxFileSize:         ec.manager.GetString("logging.max_file_size"),
		MaxBackups:          ec.manager.GetInt("logging.max_backups"),
		MaxAge:              ec.manager.GetInt("logging.max_age"),
		EnableColors:        ec.manager.GetBool("logging.colors"),
		EnableTimestamp:     ec.manager.GetBool("logging.timestamp"),
		EnableStackTrace:    ec.manager.GetBool("logging.stack_trace"),
		StructuredMetadata:  ec.manager.GetBool("logging.structured_metadata"),
	}
}

// GetSecurityConfig returns security-specific configuration
func (ec *EnvironmentConfig) GetSecurityConfig() *SecurityConfig {
	return &SecurityConfig{
		StrictMode:              ec.manager.GetBool("security.strict_mode"),
		VerifySSLCertificates:   ec.manager.GetBool("ssl.verify_certificates"),
		CertificatePath:         ec.manager.GetString("ssl.certificate_path"),
		EnableAuditLogs:         ec.manager.GetBool("security.audit_logs"),
		RequireNamespaceValidation: ec.manager.GetBool("security.require_namespace_validation"),
		AllowedNamespaces:       strings.Split(ec.manager.GetString("security.allowed_namespaces"), ","),
		ForbiddenResources:      strings.Split(ec.manager.GetString("security.forbidden_resources"), ","),
	}
}

// Configuration structs for different components

// HelmConfig holds Helm-specific configuration
type HelmConfig struct {
	Timeout          time.Duration
	MaxRetries       int
	RetryDelay       time.Duration
	DryRun           bool
	SkipCRDs         bool
	DisableHooks     bool
	Force            bool
	Wait             bool
	WaitForJobs      bool
	Debug            bool
	RepositoryConfig string
	RepositoryCache  string
}

// KubectlConfig holds kubectl-specific configuration
type KubectlConfig struct {
	Timeout         time.Duration
	MaxRetries      int
	RetryDelay      time.Duration
	DryRun          bool
	ValidateYAML    bool
	ServerSideApply bool
	ForceConflicts  bool
	KubeconfigPath  string
	Context         string
	Namespace       string
}

// DeploymentConfig holds deployment-specific configuration
type DeploymentConfig struct {
	ParallelEnabled               bool
	MaxParallelWorkers           int
	LayerTimeout                 time.Duration
	ChartDeploymentTimeout       time.Duration
	HealthCheckEnabled           bool
	HealthCheckTimeout           time.Duration
	HealthCheckInterval          time.Duration
	HealthCheckRetries           int
	RollbackOnFailure            bool
	AutoCleanupOnSuccess         bool
	EmergencyModeTimeout         time.Duration
	ValidateResourcesBeforeDeploy bool
}

// LoggingConfig holds logging-specific configuration
type LoggingConfig struct {
	Level              string
	Format             string
	Output             string
	FileRotation       bool
	MaxFileSize        string
	MaxBackups         int
	MaxAge             int
	EnableColors       bool
	EnableTimestamp    bool
	EnableStackTrace   bool
	StructuredMetadata bool
}

// SecurityConfig holds security-specific configuration
type SecurityConfig struct {
	StrictMode                     bool
	VerifySSLCertificates          bool
	CertificatePath                string
	EnableAuditLogs                bool
	RequireNamespaceValidation     bool
	AllowedNamespaces              []string
	ForbiddenResources             []string
}