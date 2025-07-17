package secret_usecase

import (
	"context"
	"fmt"
	"strings"
	
	"deploy-cli/domain"
	"deploy-cli/port/logger_port"
	"deploy-cli/gateway/kubectl_gateway"
)

// SecretUsecase handles secret validation and conflict detection
type SecretUsecase struct {
	kubectlGateway *kubectl_gateway.KubectlGateway
	logger         logger_port.LoggerPort
}

// NewSecretUsecase creates a new secret usecase
func NewSecretUsecase(
	kubectlGateway *kubectl_gateway.KubectlGateway,
	logger logger_port.LoggerPort,
) *SecretUsecase {
	return &SecretUsecase{
		kubectlGateway: kubectlGateway,
		logger:         logger,
	}
}

// ValidateSecretState performs comprehensive secret validation
func (u *SecretUsecase) ValidateSecretState(ctx context.Context, environment domain.Environment) (*domain.SecretValidationResult, error) {
	u.logger.InfoWithContext("starting secret state validation", map[string]interface{}{
		"environment": environment.String(),
	})
	
	result := &domain.SecretValidationResult{
		Environment: environment,
		Conflicts:   []domain.SecretConflict{},
		Warnings:    []string{},
		Valid:       true,
	}
	
	// Check for ownership conflicts
	conflicts, err := u.detectOwnershipConflicts(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to detect ownership conflicts: %w", err)
	}
	result.Conflicts = conflicts
	
	// Check namespace distribution
	warnings, err := u.validateNamespaceDistribution(ctx, environment)
	if err != nil {
		return nil, fmt.Errorf("failed to validate namespace distribution: %w", err)
	}
	result.Warnings = warnings
	
	// Determine overall validity
	result.Valid = len(result.Conflicts) == 0
	
	u.logger.InfoWithContext("secret state validation completed", map[string]interface{}{
		"environment":    environment.String(),
		"conflicts":      len(result.Conflicts),
		"warnings":       len(result.Warnings),
		"valid":          result.Valid,
	})
	
	return result, nil
}

// detectOwnershipConflicts identifies secrets with cross-namespace ownership issues
func (u *SecretUsecase) detectOwnershipConflicts(ctx context.Context) ([]domain.SecretConflict, error) {
	var conflicts []domain.SecretConflict
	
	// Get all secrets with Helm annotations
	cmd := []string{
		"get", "secrets", "--all-namespaces", "-o", 
		"jsonpath={range .items[*]}{.metadata.namespace}{\"\\t\"}{.metadata.name}{\"\\t\"}{.metadata.annotations.meta\\.helm\\.sh/release-name}{\"\\t\"}{.metadata.annotations.meta\\.helm\\.sh/release-namespace}{\"\\n\"}{end}",
	}
	
	output, err := u.kubectlGateway.ExecuteCommand(ctx, cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to get secrets: %w", err)
	}
	
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		
		parts := strings.Split(line, "\t")
		if len(parts) < 4 {
			continue
		}
		
		secretNamespace := parts[0]
		secretName := parts[1]
		releaseName := parts[2]
		releaseNamespace := parts[3]
		
		// Skip if no Helm annotations
		if releaseName == "" || releaseNamespace == "" {
			continue
		}
		
		// Check for cross-namespace ownership
		if secretNamespace != releaseNamespace {
			conflict := domain.SecretConflict{
				SecretName:       secretName,
				SecretNamespace:  secretNamespace,
				ReleaseName:      releaseName,
				ReleaseNamespace: releaseNamespace,
				ConflictType:     domain.ConflictTypeCrossNamespace,
				Description: fmt.Sprintf("Secret %s/%s is owned by Helm release %s in namespace %s", 
					secretNamespace, secretName, releaseName, releaseNamespace),
			}
			conflicts = append(conflicts, conflict)
		}
	}
	
	return conflicts, nil
}

// validateNamespaceDistribution checks if secrets are properly distributed
func (u *SecretUsecase) validateNamespaceDistribution(ctx context.Context, environment domain.Environment) ([]string, error) {
	var warnings []string
	
	// Define expected secret distribution based on environment
	expectedDistribution := u.getExpectedSecretDistribution(environment)
	
	for secretName, expectedNamespaces := range expectedDistribution {
		for _, namespace := range expectedNamespaces {
			// Check if secret exists in expected namespace
			cmd := []string{"get", "secret", secretName, "-n", namespace}
			_, err := u.kubectlGateway.ExecuteCommand(ctx, cmd)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("Secret %s not found in expected namespace %s", secretName, namespace))
			}
		}
	}
	
	return warnings, nil
}

// getExpectedSecretDistribution returns expected secret distribution for environment
func (u *SecretUsecase) getExpectedSecretDistribution(environment domain.Environment) map[string][]string {
	switch environment {
	case domain.EnvironmentProduction:
		return map[string][]string{
			"huggingface-secret":     {"alt-auth", "alt-apps"},
			"meilisearch-secrets":    {"alt-search"},
			"postgres-secrets":       {"alt-database"},
			"auth-postgres-secrets":  {"alt-database"},
			"auth-service-secrets":   {"alt-auth"},
			"backend-secrets":        {"alt-apps"},
		}
	case domain.EnvironmentStaging:
		return map[string][]string{
			"huggingface-secret":     {"alt-staging"},
			"meilisearch-secrets":    {"alt-staging"},
			"postgres-secrets":       {"alt-staging"},
		}
	case domain.EnvironmentDevelopment:
		return map[string][]string{
			"huggingface-secret":     {"alt-dev"},
			"meilisearch-secrets":    {"alt-dev"},
			"postgres-secrets":       {"alt-dev"},
		}
	default:
		return map[string][]string{}
	}
}

// ResolveConflicts attempts to automatically resolve detected conflicts
func (u *SecretUsecase) ResolveConflicts(ctx context.Context, conflicts []domain.SecretConflict, dryRun bool) error {
	if len(conflicts) == 0 {
		u.logger.InfoWithContext("no conflicts to resolve", map[string]interface{}{})
		return nil
	}
	
	u.logger.InfoWithContext("resolving secret conflicts", map[string]interface{}{
		"conflict_count": len(conflicts),
		"dry_run":        dryRun,
	})
	
	for _, conflict := range conflicts {
		if err := u.resolveConflict(ctx, conflict, dryRun); err != nil {
			u.logger.ErrorWithContext("failed to resolve conflict", map[string]interface{}{
				"secret":    conflict.SecretName,
				"namespace": conflict.SecretNamespace,
				"error":     err.Error(),
			})
			return fmt.Errorf("failed to resolve conflict for %s/%s: %w", 
				conflict.SecretNamespace, conflict.SecretName, err)
		}
	}
	
	return nil
}

// resolveConflict resolves a single secret conflict
func (u *SecretUsecase) resolveConflict(ctx context.Context, conflict domain.SecretConflict, dryRun bool) error {
	switch conflict.ConflictType {
	case domain.ConflictTypeCrossNamespace:
		return u.resolveCrossNamespaceConflict(ctx, conflict, dryRun)
	default:
		return fmt.Errorf("unknown conflict type: %s", conflict.ConflictType)
	}
}

// resolveCrossNamespaceConflict resolves cross-namespace ownership conflicts
func (u *SecretUsecase) resolveCrossNamespaceConflict(ctx context.Context, conflict domain.SecretConflict, dryRun bool) error {
	u.logger.InfoWithContext("resolving cross-namespace conflict", map[string]interface{}{
		"secret":           conflict.SecretName,
		"secret_namespace": conflict.SecretNamespace,
		"release_name":     conflict.ReleaseName,
		"release_namespace": conflict.ReleaseNamespace,
		"dry_run":          dryRun,
	})
	
	if dryRun {
		u.logger.InfoWithContext("dry-run: would delete conflicting secret", map[string]interface{}{
			"secret":    conflict.SecretName,
			"namespace": conflict.SecretNamespace,
		})
		return nil
	}
	
	// Delete the conflicting secret
	cmd := []string{"delete", "secret", conflict.SecretName, "-n", conflict.SecretNamespace}
	_, err := u.kubectlGateway.ExecuteCommand(ctx, cmd)
	if err != nil {
		return fmt.Errorf("failed to delete conflicting secret: %w", err)
	}
	
	u.logger.InfoWithContext("deleted conflicting secret", map[string]interface{}{
		"secret":    conflict.SecretName,
		"namespace": conflict.SecretNamespace,
	})
	
	return nil
}