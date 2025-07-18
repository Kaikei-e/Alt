package deployment_usecase

import (
	"context"
	"fmt"
	"strings"

	"deploy-cli/domain"
	"deploy-cli/gateway/kubectl_gateway"
	"deploy-cli/port/logger_port"
	"deploy-cli/usecase/secret_usecase"
)

// SecretRequirement represents a missing secret that needs to be provisioned
type SecretRequirement struct {
	Name      string
	Namespace string
	Chart     string
}

// SecretManagementUsecase handles secret creation, validation, and Helm ownership management
type SecretManagementUsecase struct {
	kubectlGateway    *kubectl_gateway.KubectlGateway
	secretUsecase     *secret_usecase.SecretUsecase
	logger            logger_port.LoggerPort
}

// NewSecretManagementUsecase creates a new secret management usecase
func NewSecretManagementUsecase(
	kubectlGateway *kubectl_gateway.KubectlGateway,
	secretUsecase *secret_usecase.SecretUsecase,
	logger logger_port.LoggerPort,
) *SecretManagementUsecase {
	return &SecretManagementUsecase{
		kubectlGateway: kubectlGateway,
		secretUsecase:  secretUsecase,
		logger:         logger,
	}
}

// ValidateSecretsBeforeDeployment performs comprehensive secret validation before deployment
func (u *SecretManagementUsecase) ValidateSecretsBeforeDeployment(ctx context.Context, charts []domain.Chart) error {
	u.logger.InfoWithContext("starting pre-deployment secret validation", map[string]interface{}{
		"charts_count": len(charts),
	})

	for _, chart := range charts {
		if err := u.validateChartSecrets(ctx, chart); err != nil {
			if u.isOwnershipError(err) {
				u.logger.WarnWithContext("Secret ownership conflict detected, attempting resolution", map[string]interface{}{
					"chart": chart.Name,
					"error": err.Error(),
				})
				if err := u.resolveOwnershipConflict(ctx, chart); err != nil {
					return fmt.Errorf("failed to resolve ownership conflict for chart %s: %w", chart.Name, err)
				}
				continue
			}
			return fmt.Errorf("secret validation failed for chart %s: %w", chart.Name, err)
		}
	}

	u.logger.InfoWithContext("Secret validation completed successfully", map[string]interface{}{
		"charts_count": len(charts),
	})
	return nil
}

// validateChartSecrets validates all secrets for a specific chart
func (u *SecretManagementUsecase) validateChartSecrets(ctx context.Context, chart domain.Chart) error {
	u.logger.InfoWithContext("validating secrets for chart", map[string]interface{}{
		"chart_name": chart.Name,
		"chart_path": chart.Path,
	})

	// Step 0: Validate namespace consistency (CRITICAL FIX)
	if err := u.validateNamespaceConsistency(chart); err != nil {
		u.logger.ErrorWithContext("namespace consistency validation failed", map[string]interface{}{
			"chart_name": chart.Name,
			"error":      err.Error(),
		})
		return err
	}

	// Step 1: Auto-generate missing secrets if possible (MOVED UP - Critical for --auto-fix-secrets)
	if err := u.autoGenerateMissingSecrets(ctx, chart); err != nil {
		u.logger.ErrorWithContext("auto-generation of missing secrets failed", map[string]interface{}{
			"chart_name": chart.Name,
			"error":      err.Error(),
		})
		return err
	}

	// Step 2: Check secret existence (AFTER auto-generation)
	if err := u.validateSecretExistence(ctx, chart); err != nil {
		u.logger.ErrorWithContext("secret existence validation failed", map[string]interface{}{
			"chart_name": chart.Name,
			"error":      err.Error(),
		})
		return err
	}

	// Step 3: Validate secret metadata
	if err := u.validateSecretMetadata(ctx, chart); err != nil {
		u.logger.ErrorWithContext("secret metadata validation failed", map[string]interface{}{
			"chart_name": chart.Name,
			"error":      err.Error(),
		})
		return err
	}

	return nil
}

// validateSecretExistence checks if required secrets exist for a chart
func (u *SecretManagementUsecase) validateSecretExistence(ctx context.Context, chart domain.Chart) error {
	requiredSecrets := u.getRequiredSecretsForChart(chart)
	
	for _, secretName := range requiredSecrets {
		u.logger.DebugWithContext("checking secret existence", map[string]interface{}{
			"secret_name": secretName,
			"chart_name":  chart.Name,
		})

		exists, err := u.secretUsecase.SecretExists(ctx, secretName, u.getNamespaceForChart(chart))
		if err != nil {
			return fmt.Errorf("failed to check secret existence for %s: %w", secretName, err)
		}

		if !exists {
			u.logger.WarnWithContext("required secret missing", map[string]interface{}{
				"secret_name": secretName,
				"chart_name":  chart.Name,
				"namespace":   u.getNamespaceForChart(chart),
			})
			return fmt.Errorf("required secret %s does not exist for chart %s", secretName, chart.Name)
		}
	}

	return nil
}

// validateSecretMetadata validates secret labels and data format
func (u *SecretManagementUsecase) validateSecretMetadata(ctx context.Context, chart domain.Chart) error {
	requiredSecrets := u.getRequiredSecretsForChart(chart)
	
	for _, secretName := range requiredSecrets {
		secret, err := u.secretUsecase.GetSecret(ctx, secretName, u.getNamespaceForChart(chart))
		if err != nil {
			return fmt.Errorf("failed to get secret %s for validation: %w", secretName, err)
		}

		// Validate secret labels
		if err := u.validateSecretLabels(secret, chart); err != nil {
			return fmt.Errorf("secret label validation failed for %s: %w", secretName, err)
		}

		// Validate secret data format
		if err := u.validateSecretDataFormat(secret, chart); err != nil {
			return fmt.Errorf("secret data format validation failed for %s: %w", secretName, err)
		}
	}

	return nil
}

// autoGenerateMissingSecrets automatically generates missing secrets where possible
func (u *SecretManagementUsecase) autoGenerateMissingSecrets(ctx context.Context, chart domain.Chart) error {
	autoGeneratableSecrets := u.getAutoGeneratableSecretsForChart(chart)
	
	for _, secretName := range autoGeneratableSecrets {
		exists, err := u.secretUsecase.SecretExists(ctx, secretName, u.getNamespaceForChart(chart))
		if err != nil {
			return fmt.Errorf("failed to check secret existence for %s: %w", secretName, err)
		}

		if !exists {
			u.logger.InfoWithContext("auto-generating missing secret", map[string]interface{}{
				"secret_name": secretName,
				"chart_name":  chart.Name,
				"namespace":   u.getNamespaceForChart(chart),
			})

			if err := u.generateSecret(ctx, secretName, u.getNamespaceForChart(chart), chart); err != nil {
				return fmt.Errorf("failed to auto-generate secret %s: %w", secretName, err)
			}
		}
	}

	return nil
}

// getRequiredSecretsForChart returns the required secrets for a chart
func (u *SecretManagementUsecase) getRequiredSecretsForChart(chart domain.Chart) []string {
	switch chart.Name {
	case "postgres":
		return []string{"postgres-secrets"}
	case "auth-postgres":
		return []string{"auth-postgres-secrets"}
	case "kratos-postgres":
		return []string{"kratos-postgres-secrets"}
	case "clickhouse":
		return []string{"clickhouse-secrets"}
	case "meilisearch":
		return []string{"meilisearch-secrets"}
	case "auth-service":
		return []string{"auth-service-secrets", "auth-postgres-secrets"}
	case "alt-backend":
		return []string{"backend-secrets"}
	case "alt-frontend":
		return []string{"frontend-secrets"}
	case "kratos":
		return []string{"kratos-secrets"}
	case "nginx":
		return []string{"nginx-secrets"}
	case "nginx-external":
		return []string{"nginx-external-secrets"}
	default:
		return []string{}
	}
}

// getAutoGeneratableSecretsForChart returns secrets that can be auto-generated for a chart
func (u *SecretManagementUsecase) getAutoGeneratableSecretsForChart(chart domain.Chart) []string {
	switch chart.Name {
	case "postgres":
		return []string{"postgres-secrets"}
	case "auth-postgres":
		return []string{"auth-postgres-secrets"}
	case "kratos-postgres":
		return []string{"kratos-postgres-secrets"}
	case "clickhouse":
		return []string{"clickhouse-secrets"}
	case "meilisearch":
		return []string{"meilisearch-secrets"}
	case "auth-service":
		return []string{"auth-service-secrets"}
	case "alt-backend":
		return []string{"backend-secrets"}
	case "alt-frontend":
		return []string{"frontend-secrets"}
	case "kratos":
		return []string{"kratos-secrets"}
	case "nginx":
		return []string{"nginx-secrets"}
	case "nginx-external":
		return []string{"nginx-external-secrets"}
	default:
		return []string{}
	}
}

// validateSecretLabels validates secret labels for deploy-cli management
func (u *SecretManagementUsecase) validateSecretLabels(secret *domain.Secret, chart domain.Chart) error {
	if secret.Labels == nil {
		return fmt.Errorf("secret missing labels")
	}
	
	// Validate deploy-cli management label (now using DEBUG level to reduce noise)
	if managed, exists := secret.Labels["deploy-cli/managed"]; !exists || managed != "true" {
		u.logger.DebugWithContext("secret not managed by deploy-cli", map[string]interface{}{
			"secret_name": secret.Name,
			"chart_name":  chart.Name,
		})
	}
	
	return nil
}

// validateSecretDataFormat validates secret data format based on secret type
func (u *SecretManagementUsecase) validateSecretDataFormat(secret *domain.Secret, chart domain.Chart) error {
	// This is a placeholder for secret data format validation
	// Implementation would depend on the specific secret type and required format
	return nil
}

// generateSecret generates a new secret for the specified chart
func (u *SecretManagementUsecase) generateSecret(ctx context.Context, secretName, namespace string, chart domain.Chart) error {
	u.logger.InfoWithContext("generating secret", map[string]interface{}{
		"secret_name": secretName,
		"namespace":   namespace,
		"chart_name":  chart.Name,
	})

	// Route to appropriate secret generation method based on chart type
	var err error
	switch chart.Name {
	case "nginx", "nginx-external":
		// Generate web server credentials for nginx charts
		u.logger.InfoWithContext("generating web server credentials", map[string]interface{}{
			"secret_name": secretName,
			"chart_name":  chart.Name,
			"type":        "webserver",
		})
		err = u.secretUsecase.GenerateWebServerCredentials(ctx, secretName, namespace)
	case "postgres", "auth-postgres", "kratos-postgres", "clickhouse", "meilisearch":
		// Generate database credentials for database charts
		u.logger.InfoWithContext("generating database credentials", map[string]interface{}{
			"secret_name": secretName,
			"chart_name":  chart.Name,
			"type":        "database",
		})
		err = u.secretUsecase.GenerateDatabaseCredentials(ctx, secretName, namespace)
	case "alt-backend", "alt-frontend", "auth-service", "kratos":
		// Generate application credentials for application charts
		u.logger.InfoWithContext("generating application credentials", map[string]interface{}{
			"secret_name": secretName,
			"chart_name":  chart.Name,
			"type":        "application",
		})
		err = u.secretUsecase.GenerateApplicationCredentials(ctx, secretName, namespace)
	default:
		// Default to database credentials for unknown charts
		u.logger.InfoWithContext("generating database credentials (default)", map[string]interface{}{
			"secret_name": secretName,
			"chart_name":  chart.Name,
			"type":        "database",
		})
		err = u.secretUsecase.GenerateDatabaseCredentials(ctx, secretName, namespace)
	}

	if err != nil {
		return fmt.Errorf("failed to generate secret %s: %w", secretName, err)
	}

	u.logger.InfoWithContext("secret generated successfully", map[string]interface{}{
		"secret_name": secretName,
		"namespace":   namespace,
		"chart_name":  chart.Name,
	})

	return nil
}

// isOwnershipError checks if the error is related to Helm ownership conflicts
func (u *SecretManagementUsecase) isOwnershipError(err error) bool {
	if err == nil {
		return false
	}
	
	errorMsg := strings.ToLower(err.Error())
	ownershipPatterns := []string{
		"managed by helm",
		"ownership conflict",
		"cannot patch resource",
		"forbidden",
		"access denied",
	}
	
	for _, pattern := range ownershipPatterns {
		if strings.Contains(errorMsg, pattern) {
			return true
		}
	}
	
	return false
}

// resolveOwnershipConflict attempts to resolve Helm ownership conflicts
func (u *SecretManagementUsecase) resolveOwnershipConflict(ctx context.Context, chart domain.Chart) error {
	u.logger.InfoWithContext("attempting to resolve ownership conflict", map[string]interface{}{
		"chart_name": chart.Name,
	})

	// Try to adopt secrets for the chart
	if err := u.adoptSecretsForChart(chart.Name); err != nil {
		return fmt.Errorf("failed to adopt secrets for chart %s: %w", chart.Name, err)
	}

	u.logger.InfoWithContext("ownership conflict resolved successfully", map[string]interface{}{
		"chart_name": chart.Name,
	})

	return nil
}

// adoptSecretsForChart adopts secrets for Helm management
func (u *SecretManagementUsecase) adoptSecretsForChart(chartName string) error {
	u.logger.InfoWithContext("adopting secrets for chart", map[string]interface{}{
		"chart_name": chartName,
	})

	// Use the secret usecase to adopt secrets
	if err := u.secretUsecase.AdoptSecretsForChart(context.Background(), chartName); err != nil {
		return fmt.Errorf("failed to adopt secrets for chart %s: %w", chartName, err)
	}

	u.logger.InfoWithContext("secrets adopted successfully", map[string]interface{}{
		"chart_name": chartName,
	})

	return nil
}

// handleSecretOwnershipError handles secret ownership errors during deployment
func (u *SecretManagementUsecase) handleSecretOwnershipError(err error, chartName string) error {
	if err == nil {
		return nil
	}

	if !u.isOwnershipError(err) {
		return err
	}

	u.logger.WarnWithContext("detected secret-related error, attempting automatic fix", map[string]interface{}{
		"chart_name": chartName,
		"error":      err.Error(),
	})

	// Attempt to fix the issue by adopting secrets
	if adoptErr := u.adoptSecretsForChart(chartName); adoptErr != nil {
		return fmt.Errorf("failed to automatically fix secret ownership issue: %w", adoptErr)
	}

	u.logger.InfoWithContext("secret ownership issue resolved", map[string]interface{}{
		"chart_name": chartName,
	})

	return nil
}

// provisionAllRequiredSecrets provisions all required secrets for charts
func (u *SecretManagementUsecase) provisionAllRequiredSecrets(ctx context.Context, charts []domain.Chart) error {
	u.logger.InfoWithContext("provisioning all required secrets", map[string]interface{}{
		"charts_count": len(charts),
	})

	for _, chart := range charts {
		if err := u.autoGenerateMissingSecrets(ctx, chart); err != nil {
			return fmt.Errorf("failed to provision secrets for chart %s: %w", chart.Name, err)
		}
	}

	u.logger.InfoWithContext("all required secrets provisioned successfully", map[string]interface{}{
		"charts_count": len(charts),
	})

	return nil
}

// detectMissingSecrets detects missing secrets across all charts
func (u *SecretManagementUsecase) detectMissingSecrets(ctx context.Context, charts []domain.Chart) []SecretRequirement {
	var missingSecrets []SecretRequirement

	for _, chart := range charts {
		requiredSecrets := u.getRequiredSecretsForChart(chart)
		namespace := u.getNamespaceForChart(chart)

		for _, secretName := range requiredSecrets {
			exists, err := u.secretUsecase.SecretExists(ctx, secretName, namespace)
			if err != nil || !exists {
				missingSecrets = append(missingSecrets, SecretRequirement{
					Name:      secretName,
					Namespace: namespace,
					Chart:     chart.Name,
				})
			}
		}
	}

	u.logger.InfoWithContext("missing secrets detected", map[string]interface{}{
		"missing_count": len(missingSecrets),
	})

	return missingSecrets
}

// validateNamespaceConsistency validates that secrets are in the correct namespace for their charts
func (u *SecretManagementUsecase) validateNamespaceConsistency(chart domain.Chart) error {
	expectedNamespace := u.getNamespaceForChart(chart)
	
	u.logger.DebugWithContext("validating namespace consistency", map[string]interface{}{
		"chart_name":         chart.Name,
		"expected_namespace": expectedNamespace,
	})

	// CRITICAL FIX: Validate actual namespace mismatches
	requiredSecrets := u.getRequiredSecretsForChart(chart)
	
	for _, secretName := range requiredSecrets {
		// Check if secret exists in the expected namespace
		exists, err := u.secretUsecase.SecretExists(context.Background(), secretName, expectedNamespace)
		if err != nil {
			u.logger.WarnWithContext("failed to check secret existence for namespace validation", map[string]interface{}{
				"secret_name": secretName,
				"namespace":   expectedNamespace,
				"chart_name":  chart.Name,
				"error":       err.Error(),
			})
			continue
		}
		
		if !exists {
			// Check if secret exists in other common namespaces
			alternativeNamespaces := []string{"alt-database", "alt-auth", "alt-apps", "alt-ingress", "alt-search"}
			for _, altNamespace := range alternativeNamespaces {
				if altNamespace == expectedNamespace {
					continue
				}
				
				altExists, altErr := u.secretUsecase.SecretExists(context.Background(), secretName, altNamespace)
				if altErr != nil {
					continue
				}
				
				if altExists {
					u.logger.WarnWithContext("NAMESPACE MISMATCH DETECTED", map[string]interface{}{
						"secret_name":        secretName,
						"chart_name":         chart.Name,
						"expected_namespace": expectedNamespace,
						"actual_namespace":   altNamespace,
						"fix_suggestion":     fmt.Sprintf("Consider moving secret from %s to %s", altNamespace, expectedNamespace),
					})
					
					// This is a warning, not an error - let the deployment continue
					// but log the inconsistency for manual resolution
				}
			}
		}
	}
	
	return nil
}

// getNamespaceForChart returns the appropriate namespace for a chart
func (u *SecretManagementUsecase) getNamespaceForChart(chart domain.Chart) string {
	// For multi-namespace charts, return the primary namespace
	if chart.MultiNamespace && len(chart.TargetNamespaces) > 0 {
		return chart.TargetNamespaces[0]
	}
	
	// CRITICAL FIX: Prioritize chart name mapping over chart type to prevent namespace mismatches
	// Handle auth-postgres specifically to ensure it goes to alt-auth (not alt-database)
	switch chart.Name {
	case "auth-postgres", "kratos-postgres", "auth-service", "kratos":
		return "alt-auth"
	case "postgres", "clickhouse", "meilisearch":
		return "alt-database"
	case "nginx", "nginx-external":
		return "alt-ingress"
	case "alt-backend", "alt-frontend":
		return "alt-apps"
	}
	
	// Fallback to chart type mapping
	switch chart.Type {
	case domain.InfrastructureChart:
		if chart.Name == "postgres" || chart.Name == "clickhouse" || chart.Name == "meilisearch" {
			return "alt-database"
		}
		if chart.Name == "nginx" || chart.Name == "nginx-external" {
			return "alt-ingress"
		}
		if chart.Name == "auth-postgres" || chart.Name == "kratos-postgres" || chart.Name == "kratos" {
			return "alt-auth"
		}
		return "alt-apps"
	case domain.ApplicationChart:
		if chart.Name == "auth-service" || chart.Name == "kratos" {
			return "alt-auth"
		}
		return "alt-apps"
	case domain.OperationalChart:
		return "alt-apps"
	default:
		return "alt-apps"
	}
}