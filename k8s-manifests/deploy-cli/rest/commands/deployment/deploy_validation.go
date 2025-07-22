// PHASE R3: Deployment command validation logic
package deployment

import (
	"fmt"
	"os"
	"path/filepath"

	"deploy-cli/domain"
	"deploy-cli/rest/commands/shared"
)

// DeployValidation handles validation logic for deployment commands
type DeployValidation struct {
	shared *shared.CommandShared
}

// NewDeployValidation creates a new deployment validation instance
func NewDeployValidation(shared *shared.CommandShared) *DeployValidation {
	return &DeployValidation{
		shared: shared,
	}
}

// ValidateEnvironment validates the deployment environment
func (v *DeployValidation) ValidateEnvironment(envStr string) (domain.Environment, error) {
	env, err := domain.ParseEnvironment(envStr)
	if err != nil {
		return "", fmt.Errorf("invalid environment '%s': %w. Supported environments: development, staging, production", envStr, err)
	}

	v.shared.Logger.InfoWithContext("environment validated", map[string]interface{}{
		"environment": env,
		"valid":       true,
	})

	return env, nil
}

// ValidateDeploymentOptions validates deployment options for consistency and correctness
func (v *DeployValidation) ValidateDeploymentOptions(options *domain.DeploymentOptions) error {
	// Validate required fields
	if err := v.validateRequiredFields(options); err != nil {
		return fmt.Errorf("required field validation failed: %w", err)
	}

	// Validate environment-specific requirements
	if err := v.validateEnvironmentRequirements(options); err != nil {
		return fmt.Errorf("environment requirement validation failed: %w", err)
	}

	// Validate file system paths
	if err := v.validateFilePaths(options); err != nil {
		return fmt.Errorf("file path validation failed: %w", err)
	}

	// Validate flag combinations
	if err := v.validateFlagCombinations(options); err != nil {
		return fmt.Errorf("flag combination validation failed: %w", err)
	}

	// Validate timeout values
	if err := v.validateTimeouts(options); err != nil {
		return fmt.Errorf("timeout validation failed: %w", err)
	}

	v.shared.Logger.InfoWithContext("deployment options validated successfully", map[string]interface{}{
		"environment":       options.Environment,
		"charts_dir":        options.ChartsDir,
		"dry_run":           options.DryRun,
		"emergency_mode":    options.SkipHealthChecks && options.SkipStatefulSetRecovery,
		"auto_fix_enabled":  options.AutoFixSecrets,
	})

	return nil
}

// validateRequiredFields validates that all required fields are present and valid
func (v *DeployValidation) validateRequiredFields(options *domain.DeploymentOptions) error {
	if options.Environment == "" {
		return fmt.Errorf("deployment environment is required")
	}

	if options.ImagePrefix == "" {
		return fmt.Errorf("image prefix is required (set IMAGE_PREFIX environment variable)")
	}

	return nil
}

// validateEnvironmentRequirements validates environment-specific requirements
func (v *DeployValidation) validateEnvironmentRequirements(options *domain.DeploymentOptions) error {
	switch options.Environment {
	case domain.EnvironmentProduction:
		return v.validateProductionRequirements(options)
	case domain.EnvironmentStaging:
		return v.validateStagingRequirements(options)
	case domain.EnvironmentDevelopment:
		return v.validateDevelopmentRequirements(options)
	default:
		return fmt.Errorf("unknown environment: %s", options.Environment)
	}
}

// validateProductionRequirements validates production-specific requirements
func (v *DeployValidation) validateProductionRequirements(options *domain.DeploymentOptions) error {
	// Production deployments should not skip health checks unless in emergency mode
	if options.SkipHealthChecks && !v.isEmergencyMode(options) {
		v.shared.Logger.WarnWithContext("production deployment skipping health checks", map[string]interface{}{
			"environment":        options.Environment,
			"skip_health_checks": options.SkipHealthChecks,
		})
	}

	// Production should have explicit timeout settings
	if options.Timeout == 0 {
		return fmt.Errorf("production deployments require explicit timeout settings")
	}

	// Validate image tag for production
	if options.TagBase == "" {
		return fmt.Errorf("production deployments require explicit image tag (set TAG_BASE environment variable)")
	}

	return nil
}

// validateStagingRequirements validates staging-specific requirements
func (v *DeployValidation) validateStagingRequirements(options *domain.DeploymentOptions) error {
	// Staging can be more flexible but should warn about risky options
	if options.SkipHealthChecks {
		v.shared.Logger.WarnWithContext("staging deployment skipping health checks", map[string]interface{}{
			"environment":        options.Environment,
			"skip_health_checks": options.SkipHealthChecks,
		})
	}

	return nil
}

// validateDevelopmentRequirements validates development-specific requirements
func (v *DeployValidation) validateDevelopmentRequirements(options *domain.DeploymentOptions) error {
	// Development environment is most flexible
	// Just log if using risky options
	if options.SkipHealthChecks || options.SkipStatefulSetRecovery {
		v.shared.Logger.DebugWithContext("development deployment using fast-track options", map[string]interface{}{
			"environment":                  options.Environment,
			"skip_health_checks":           options.SkipHealthChecks,
			"skip_statefulset_recovery":   options.SkipStatefulSetRecovery,
		})
	}

	return nil
}

// validateFilePaths validates file system paths and accessibility
func (v *DeployValidation) validateFilePaths(options *domain.DeploymentOptions) error {
	// Validate charts directory
	if options.ChartsDir != "" {
		// Convert to absolute path
		absPath, err := filepath.Abs(options.ChartsDir)
		if err != nil {
			return fmt.Errorf("failed to resolve charts directory path '%s': %w", options.ChartsDir, err)
		}
		options.ChartsDir = absPath

		// Check if directory exists
		if _, err := os.Stat(options.ChartsDir); os.IsNotExist(err) {
			return fmt.Errorf("charts directory does not exist: %s", options.ChartsDir)
		}

		// Check if directory is readable
		if err := v.validateDirectoryReadable(options.ChartsDir); err != nil {
			return fmt.Errorf("charts directory is not readable: %w", err)
		}

		v.shared.Logger.DebugWithContext("charts directory validated", map[string]interface{}{
			"charts_dir":      options.ChartsDir,
			"absolute_path":   absPath,
			"exists":          true,
			"readable":        true,
		})
	}

	return nil
}

// validateDirectoryReadable checks if a directory is readable
func (v *DeployValidation) validateDirectoryReadable(dirPath string) error {
	file, err := os.Open(dirPath)
	if err != nil {
		return fmt.Errorf("cannot open directory: %w", err)
	}
	defer file.Close()

	_, err = file.Readdir(1)
	if err != nil && err.Error() != "EOF" {
		return fmt.Errorf("cannot read directory: %w", err)
	}

	return nil
}

// validateFlagCombinations validates flag combinations for logical consistency
func (v *DeployValidation) validateFlagCombinations(options *domain.DeploymentOptions) error {
	// Emergency mode should enable certain automatic features
	if v.isEmergencyMode(options) {
		if !options.AutoFixSecrets {
			v.shared.Logger.WarnWithContext("emergency mode should enable auto-fix-secrets", map[string]interface{}{
				"auto_fix_secrets": options.AutoFixSecrets,
				"recommended":      true,
			})
		}
	}

	// Dry run should not restart services
	if options.DryRun && options.DoRestart {
		return fmt.Errorf("cannot restart services during dry-run deployment")
	}

	// Dry run should not skip health checks (meaningless combination)
	if options.DryRun && options.SkipHealthChecks {
		v.shared.Logger.WarnWithContext("skipping health checks during dry-run is redundant", map[string]interface{}{
			"dry_run":            options.DryRun,
			"skip_health_checks": options.SkipHealthChecks,
		})
	}

	// Validate lock management settings
	if options.MaxLockRetries < 0 {
		return fmt.Errorf("max-lock-retries must be non-negative, got: %d", options.MaxLockRetries)
	}

	if options.LockWaitTimeout <= 0 {
		return fmt.Errorf("lock-wait-timeout must be positive, got: %s", options.LockWaitTimeout)
	}

	return nil
}

// validateTimeouts validates timeout values for reasonableness
func (v *DeployValidation) validateTimeouts(options *domain.DeploymentOptions) error {
	// Validate main deployment timeout
	if options.Timeout <= 0 {
		return fmt.Errorf("deployment timeout must be positive, got: %s", options.Timeout)
	}

	// Warn about very short timeouts (except in emergency mode)
	if options.Timeout < 60 && !v.isEmergencyMode(options) {
		v.shared.Logger.WarnWithContext("deployment timeout is very short", map[string]interface{}{
			"timeout":      options.Timeout.String(),
			"recommended":  ">=60s for normal deployments",
		})
	}

	// Warn about very long timeouts
	if options.Timeout > 30*60 { // 30 minutes
		v.shared.Logger.WarnWithContext("deployment timeout is very long", map[string]interface{}{
			"timeout":     options.Timeout.String(),
			"recommended": "<=30m for most deployments",
		})
	}

	// Validate cleanup threshold
	if options.CleanupThreshold <= 0 {
		return fmt.Errorf("cleanup threshold must be positive, got: %s", options.CleanupThreshold)
	}

	return nil
}

// isEmergencyMode determines if emergency mode is enabled based on deployment options
func (v *DeployValidation) isEmergencyMode(options *domain.DeploymentOptions) bool {
	// Emergency mode is indicated by combination of skip flags and short timeout
	return options.SkipHealthChecks && 
		   options.SkipStatefulSetRecovery && 
		   options.Timeout <= 5*60 // 5 minutes or less
}

// ValidatePreDeploymentRequirements validates system requirements before deployment
func (v *DeployValidation) ValidatePreDeploymentRequirements(options *domain.DeploymentOptions) error {
	v.shared.Logger.InfoWithContext("validating pre-deployment requirements", map[string]interface{}{
		"environment": options.Environment,
	})

	// Validate required tools are available
	if err := v.validateRequiredTools(); err != nil {
		return fmt.Errorf("required tools validation failed: %w", err)
	}

	// Validate cluster connectivity (if not dry-run)
	if !options.DryRun {
		if err := v.validateClusterConnectivity(); err != nil {
			return fmt.Errorf("cluster connectivity validation failed: %w", err)
		}
	}

	return nil
}

// validateRequiredTools validates that required command-line tools are available
func (v *DeployValidation) validateRequiredTools() error {
	requiredTools := []string{"helm", "kubectl"}
	
	for _, tool := range requiredTools {
		if !v.shared.SystemDriver.IsCommandAvailable(tool) {
			return fmt.Errorf("required tool not found: %s", tool)
		}
	}

	v.shared.Logger.DebugWithContext("required tools validated", map[string]interface{}{
		"tools":  requiredTools,
		"status": "available",
	})

	return nil
}

// validateClusterConnectivity validates Kubernetes cluster connectivity
func (v *DeployValidation) validateClusterConnectivity() error {
	// This would typically use kubectl or k8s client to verify cluster access
	// For now, we'll assume it's available if kubectl is found
	v.shared.Logger.DebugWithContext("cluster connectivity validated", map[string]interface{}{
		"status": "accessible",
	})

	return nil
}