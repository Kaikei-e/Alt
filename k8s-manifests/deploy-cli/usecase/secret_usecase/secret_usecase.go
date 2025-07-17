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
	secrets, err := u.kubectlGateway.GetSecretsWithMetadata(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get secrets: %w", err)
	}
	
	// Track secrets by name to detect potential ownership conflicts
	secretsByName := make(map[string][]string) // secretName -> []namespace
	helmOwnership := make(map[string]string)   // "namespace/secretName" -> "releaseNamespace/releaseName"
	
	for _, secret := range secrets {
		secretNamespace := secret.Namespace
		secretName := secret.Name
		releaseName := secret.ReleaseName
		releaseNamespace := secret.ReleaseNamespace
		
		// Track all instances of each secret name
		secretsByName[secretName] = append(secretsByName[secretName], secretNamespace)
		
		// Skip if no Helm annotations
		if releaseName == "" || releaseNamespace == "" {
			continue
		}
		
		secretKey := fmt.Sprintf("%s/%s", secretNamespace, secretName)
		ownerKey := fmt.Sprintf("%s/%s", releaseNamespace, releaseName)
		helmOwnership[secretKey] = ownerKey
		
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
	
	// Check for potential Helm metadata conflicts (same secret name with different owners)
	for secretName, namespaces := range secretsByName {
		if len(namespaces) > 1 {
			// Check if these secrets have different Helm owners
			owners := make(map[string][]string) // owner -> []namespaces
			
			for _, namespace := range namespaces {
				secretKey := fmt.Sprintf("%s/%s", namespace, secretName)
				if owner, exists := helmOwnership[secretKey]; exists {
					owners[owner] = append(owners[owner], namespace)
				}
			}
			
			// If we have multiple owners for the same secret name, it's a potential conflict
			if len(owners) > 1 {
				for owner, ownedNamespaces := range owners {
					parts := strings.Split(owner, "/")
					if len(parts) == 2 {
						releaseNamespace := parts[0]
						releaseName := parts[1]
						
						for _, namespace := range ownedNamespaces {
							conflict := domain.SecretConflict{
								SecretName:       secretName,
								SecretNamespace:  namespace,
								ReleaseName:      releaseName,
								ReleaseNamespace: releaseNamespace,
								ConflictType:     domain.ConflictTypeMetadataConflict,
								Description: fmt.Sprintf("Secret %s exists in multiple namespaces with different Helm owners - potential metadata conflict when deploying %s", 
									secretName, releaseName),
							}
							conflicts = append(conflicts, conflict)
						}
					}
				}
			}
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
			// Check if secret exists in expected namespace by getting all secrets in that namespace
			secrets, err := u.kubectlGateway.GetSecrets(ctx, namespace)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("Failed to check secrets in namespace %s: %v", namespace, err))
				continue
			}
			
			// Check if the expected secret exists
			found := false
			for _, secret := range secrets {
				if secret.Name == secretName {
					found = true
					break
				}
			}
			
			if !found {
				warnings = append(warnings, fmt.Sprintf("Secret %s not found in expected namespace %s", secretName, namespace))
			}
		}
	}
	
	return warnings, nil
}

// getExpectedSecretDistribution returns expected secret distribution for environment
func (u *SecretUsecase) getExpectedSecretDistribution(environment domain.Environment) map[string][]string {
	switch environment {
	case domain.Production:
		return map[string][]string{
			"huggingface-secret":     {"alt-auth", "alt-apps"},
			"meilisearch-secrets":    {"alt-search"},
			"postgres-secrets":       {"alt-database"},
			"auth-postgres-secrets":  {"alt-database"},
			"auth-service-secrets":   {"alt-auth"},
			"backend-secrets":        {"alt-apps"},
		}
	case domain.Staging:
		return map[string][]string{
			"huggingface-secret":     {"alt-staging"},
			"meilisearch-secrets":    {"alt-staging"},
			"postgres-secrets":       {"alt-staging"},
		}
	case domain.Development:
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
	case domain.ConflictTypeMetadataConflict:
		return u.resolveMetadataConflict(ctx, conflict, dryRun)
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
	err := u.kubectlGateway.DeleteSecret(ctx, conflict.SecretName, conflict.SecretNamespace)
	if err != nil {
		return fmt.Errorf("failed to delete conflicting secret: %w", err)
	}
	
	u.logger.InfoWithContext("deleted conflicting secret", map[string]interface{}{
		"secret":    conflict.SecretName,
		"namespace": conflict.SecretNamespace,
	})
	
	return nil
}

// resolveMetadataConflict resolves Helm metadata annotation conflicts
func (u *SecretUsecase) resolveMetadataConflict(ctx context.Context, conflict domain.SecretConflict, dryRun bool) error {
	u.logger.InfoWithContext("resolving Helm metadata conflict", map[string]interface{}{
		"secret":           conflict.SecretName,
		"secret_namespace": conflict.SecretNamespace,
		"release_name":     conflict.ReleaseName,
		"release_namespace": conflict.ReleaseNamespace,
		"dry_run":          dryRun,
	})
	
	if dryRun {
		u.logger.InfoWithContext("dry-run: would delete secret with metadata conflict", map[string]interface{}{
			"secret":    conflict.SecretName,
			"namespace": conflict.SecretNamespace,
		})
		return nil
	}
	
	// For metadata conflicts, we only delete the secret if it's in the "default" namespace
	// or if it's clearly orphaned (cross-namespace ownership)
	if conflict.SecretNamespace == "default" || conflict.SecretNamespace != conflict.ReleaseNamespace {
		u.logger.InfoWithContext("deleting secret with Helm metadata conflict", map[string]interface{}{
			"secret":    conflict.SecretName,
			"namespace": conflict.SecretNamespace,
			"reason":    "metadata_conflict_safe_to_delete",
		})
		
		err := u.kubectlGateway.DeleteSecret(ctx, conflict.SecretName, conflict.SecretNamespace)
		if err != nil {
			return fmt.Errorf("failed to delete conflicting secret: %w", err)
		}
		
		u.logger.InfoWithContext("deleted secret with metadata conflict", map[string]interface{}{
			"secret":    conflict.SecretName,
			"namespace": conflict.SecretNamespace,
		})
	} else {
		u.logger.WarnWithContext("skipping metadata conflict resolution - not safe to delete", map[string]interface{}{
			"secret":    conflict.SecretName,
			"namespace": conflict.SecretNamespace,
			"reason":    "same_namespace_as_release",
		})
	}
	
	return nil
}

// ListSecrets lists all secrets for an environment
func (u *SecretUsecase) ListSecrets(ctx context.Context, environment domain.Environment) ([]domain.SecretInfo, error) {
	u.logger.InfoWithContext("listing secrets", map[string]interface{}{
		"environment": environment.String(),
	})
	
	var secretInfos []domain.SecretInfo
	
	// Get all secrets with Helm annotations
	secrets, err := u.kubectlGateway.GetSecretsWithMetadata(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get secrets: %w", err)
	}
	
	for _, secret := range secrets {
		owner := ""
		if secret.ReleaseName != "" {
			if secret.ReleaseNamespace != "" {
				owner = fmt.Sprintf("%s/%s", secret.ReleaseNamespace, secret.ReleaseName)
			} else {
				owner = secret.ReleaseName
			}
		}
		
		secretInfos = append(secretInfos, domain.SecretInfo{
			Name:      secret.Name,
			Namespace: secret.Namespace,
			Owner:     owner,
			Type:      secret.Type,
			Age:       secret.Age,
		})
	}
	
	u.logger.InfoWithContext("listed secrets", map[string]interface{}{
		"environment": environment.String(),
		"count":       len(secretInfos),
	})
	
	return secretInfos, nil
}

// FindOrphanedSecrets finds secrets that are orphaned or have invalid ownership
func (u *SecretUsecase) FindOrphanedSecrets(ctx context.Context, environment domain.Environment) ([]domain.SecretInfo, error) {
	u.logger.InfoWithContext("finding orphaned secrets", map[string]interface{}{
		"environment": environment.String(),
	})
	
	var orphaned []domain.SecretInfo
	
	// Get secrets with invalid ownership (cross-namespace ownership)
	conflicts, err := u.detectOwnershipConflicts(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to detect conflicts: %w", err)
	}
	
	for _, conflict := range conflicts {
		if conflict.ConflictType == domain.ConflictTypeCrossNamespace {
			orphaned = append(orphaned, domain.SecretInfo{
				Name:      conflict.SecretName,
				Namespace: conflict.SecretNamespace,
				Owner:     fmt.Sprintf("%s/%s", conflict.ReleaseNamespace, conflict.ReleaseName),
			})
		}
	}
	
	u.logger.InfoWithContext("found orphaned secrets", map[string]interface{}{
		"environment": environment.String(),
		"count":       len(orphaned),
	})
	
	return orphaned, nil
}

// DeleteOrphanedSecrets deletes orphaned secrets
func (u *SecretUsecase) DeleteOrphanedSecrets(ctx context.Context, orphaned []domain.SecretInfo, dryRun bool) error {
	u.logger.InfoWithContext("deleting orphaned secrets", map[string]interface{}{
		"count":   len(orphaned),
		"dry_run": dryRun,
	})
	
	for _, secret := range orphaned {
		if dryRun {
			u.logger.InfoWithContext("dry-run: would delete orphaned secret", map[string]interface{}{
				"secret":    secret.Name,
				"namespace": secret.Namespace,
			})
			continue
		}
		
		err := u.kubectlGateway.DeleteSecret(ctx, secret.Name, secret.Namespace)
		if err != nil {
			u.logger.WarnWithContext("failed to delete orphaned secret", map[string]interface{}{
				"secret":    secret.Name,
				"namespace": secret.Namespace,
				"error":     err.Error(),
			})
			continue
		}
		
		u.logger.InfoWithContext("deleted orphaned secret", map[string]interface{}{
			"secret":    secret.Name,
			"namespace": secret.Namespace,
		})
	}
	
	return nil
}