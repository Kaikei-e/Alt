package secret_usecase

import (
	"context"
	"fmt"

	"deploy-cli/domain"
	"deploy-cli/port/kubectl_port"
	"deploy-cli/port/logger_port"
)

// SecretDistributionStrategy handles centralized secret distribution
type SecretDistributionStrategy struct {
	kubectlGateway kubectl_port.KubectlPort
	logger         logger_port.LoggerPort
}

// NewSecretDistributionStrategy creates a new secret distribution strategy
func NewSecretDistributionStrategy(kubectlGateway kubectl_port.KubectlPort, logger logger_port.LoggerPort) *SecretDistributionStrategy {
	return &SecretDistributionStrategy{
		kubectlGateway: kubectlGateway,
		logger:         logger,
	}
}

// DistributeSecrets distributes secrets according to the centralized strategy
func (s *SecretDistributionStrategy) DistributeSecrets(ctx context.Context, environment domain.Environment) error {
	s.logger.InfoWithContext("starting centralized secret distribution", map[string]interface{}{
		"environment": environment,
	})

	// Get the secret distribution plan for the environment
	plan := s.getDistributionPlan(environment)

	// Execute the distribution plan
	for _, secretDist := range plan {
		if err := s.distributeSecret(ctx, secretDist); err != nil {
			return fmt.Errorf("failed to distribute secret %s: %w", secretDist.SecretName, err)
		}
	}

	s.logger.InfoWithContext("centralized secret distribution completed", map[string]interface{}{
		"environment": environment,
		"secrets_distributed": len(plan),
	})
	return nil
}

// SecretDistribution represents a secret to be distributed
type SecretDistribution struct {
	SecretName      string
	SourceNamespace string
	TargetNamespaces []string
	SecretType      string
	Data            map[string]string
}

// getDistributionPlan returns the secret distribution plan for an environment
func (s *SecretDistributionStrategy) getDistributionPlan(environment domain.Environment) []SecretDistribution {
	switch environment {
	case domain.Production:
		return s.getProductionDistributionPlan()
	case domain.Staging:
		return s.getStagingDistributionPlan()
	case domain.Development:
		return s.getDevelopmentDistributionPlan()
	default:
		return s.getDefaultDistributionPlan()
	}
}

// getProductionDistributionPlan returns the production secret distribution plan
func (s *SecretDistributionStrategy) getProductionDistributionPlan() []SecretDistribution {
	return []SecretDistribution{
		{
			SecretName:       "postgres-secrets",
			SourceNamespace:  "default",
			TargetNamespaces: []string{"alt-database", "alt-auth", "alt-apps"},
			SecretType:       "Opaque",
		},
		{
			SecretName:       "auth-postgres-secrets",
			SourceNamespace:  "default",
			TargetNamespaces: []string{"alt-auth"},
			SecretType:       "Opaque",
		},
		{
			SecretName:       "kratos-postgres-secrets",
			SourceNamespace:  "default",
			TargetNamespaces: []string{"alt-auth"},
			SecretType:       "Opaque",
		},
		{
			SecretName:       "clickhouse-secrets",
			SourceNamespace:  "default",
			TargetNamespaces: []string{"alt-database", "alt-observability"},
			SecretType:       "Opaque",
		},
		{
			SecretName:       "huggingface-secret",
			SourceNamespace:  "default",
			TargetNamespaces: []string{"alt-auth", "alt-apps"},
			SecretType:       "Opaque",
		},
		{
			SecretName:       "meilisearch-secrets",
			SourceNamespace:  "default",
			TargetNamespaces: []string{"alt-search", "alt-apps"},
			SecretType:       "Opaque",
		},
		{
			SecretName:       "backend-secrets",
			SourceNamespace:  "default",
			TargetNamespaces: []string{"alt-apps"},
			SecretType:       "Opaque",
		},
		{
			SecretName:       "pre-processor-secrets",
			SourceNamespace:  "default",
			TargetNamespaces: []string{"alt-apps"},
			SecretType:       "Opaque",
		},
		{
			SecretName:       "tag-generator-secrets",
			SourceNamespace:  "default",
			TargetNamespaces: []string{"alt-apps"},
			SecretType:       "Opaque",
		},
		{
			SecretName:       "search-indexer-secrets",
			SourceNamespace:  "default",
			TargetNamespaces: []string{"alt-search"},
			SecretType:       "Opaque",
		},
		{
			SecretName:       "auth-service-secrets",
			SourceNamespace:  "default",
			TargetNamespaces: []string{"alt-auth"},
			SecretType:       "Opaque",
		},
	}
}

// getStagingDistributionPlan returns the staging secret distribution plan
func (s *SecretDistributionStrategy) getStagingDistributionPlan() []SecretDistribution {
	return []SecretDistribution{
		{
			SecretName:       "postgres-secrets",
			SourceNamespace:  "alt-staging",
			TargetNamespaces: []string{"alt-staging"},
			SecretType:       "Opaque",
		},
		{
			SecretName:       "huggingface-secret",
			SourceNamespace:  "alt-staging",
			TargetNamespaces: []string{"alt-staging"},
			SecretType:       "Opaque",
		},
		{
			SecretName:       "meilisearch-secrets",
			SourceNamespace:  "alt-staging",
			TargetNamespaces: []string{"alt-staging"},
			SecretType:       "Opaque",
		},
	}
}

// getDevelopmentDistributionPlan returns the development secret distribution plan
func (s *SecretDistributionStrategy) getDevelopmentDistributionPlan() []SecretDistribution {
	return []SecretDistribution{
		{
			SecretName:       "postgres-secrets",
			SourceNamespace:  "alt-dev",
			TargetNamespaces: []string{"alt-dev"},
			SecretType:       "Opaque",
		},
		{
			SecretName:       "huggingface-secret",
			SourceNamespace:  "alt-dev",
			TargetNamespaces: []string{"alt-dev"},
			SecretType:       "Opaque",
		},
		{
			SecretName:       "meilisearch-secrets",
			SourceNamespace:  "alt-dev",
			TargetNamespaces: []string{"alt-dev"},
			SecretType:       "Opaque",
		},
	}
}

// getDefaultDistributionPlan returns the default distribution plan
func (s *SecretDistributionStrategy) getDefaultDistributionPlan() []SecretDistribution {
	return s.getProductionDistributionPlan()
}

// distributeSecret distributes a single secret to target namespaces
func (s *SecretDistributionStrategy) distributeSecret(ctx context.Context, dist SecretDistribution) error {
	s.logger.InfoWithContext("distributing secret", map[string]interface{}{
		"secret": dist.SecretName,
		"source_namespace": dist.SourceNamespace,
		"target_namespaces": dist.TargetNamespaces,
	})

	// Get the source secret if it exists
	sourceSecret, err := s.kubectlGateway.GetSecret(ctx, dist.SecretName, dist.SourceNamespace)
	if err != nil {
		s.logger.WarnWithContext("source secret not found, skipping distribution", map[string]interface{}{
			"secret": dist.SecretName,
			"source_namespace": dist.SourceNamespace,
			"error": err.Error(),
		})
		return nil // Don't fail if source secret doesn't exist
	}

	// Distribute to each target namespace
	for _, targetNamespace := range dist.TargetNamespaces {
		if err := s.copySecretToNamespace(ctx, sourceSecret, targetNamespace); err != nil {
			return fmt.Errorf("failed to copy secret %s to namespace %s: %w", 
				dist.SecretName, targetNamespace, err)
		}
	}

	return nil
}

// copySecretToNamespace copies a secret to a target namespace
func (s *SecretDistributionStrategy) copySecretToNamespace(ctx context.Context, sourceSecret *kubectl_port.KubernetesSecret, targetNamespace string) error {
	// Create a new secret for the target namespace
	targetSecret := &kubectl_port.KubernetesSecret{
		Name:      sourceSecret.Name,
		Namespace: targetNamespace,
		Type:      sourceSecret.Type,
		Data:      sourceSecret.Data,
		Labels: map[string]string{
			"app.kubernetes.io/managed-by": "deploy-cli",
			"app.kubernetes.io/component":  "secrets",
			"app.kubernetes.io/part-of":    "alt",
			"alt.deployment/source":        "centralized-distribution",
		},
		Annotations: map[string]string{
			"alt.deployment/distributed-from": sourceSecret.Namespace,
			"alt.deployment/distribution-time": fmt.Sprintf("%d", ctx.Value("timestamp")),
		},
	}

	// Apply the secret
	if err := s.kubectlGateway.ApplySecret(ctx, targetSecret); err != nil {
		return fmt.Errorf("failed to apply secret to namespace %s: %w", targetNamespace, err)
	}

	s.logger.InfoWithContext("secret copied successfully", map[string]interface{}{
		"secret": sourceSecret.Name,
		"target_namespace": targetNamespace,
	})

	return nil
}

// ValidateDistribution validates that secrets are properly distributed
func (s *SecretDistributionStrategy) ValidateDistribution(ctx context.Context, environment domain.Environment) (*domain.SecretDistributionValidation, error) {
	s.logger.InfoWithContext("validating secret distribution", map[string]interface{}{
		"environment": environment,
	})

	plan := s.getDistributionPlan(environment)
	validation := &domain.SecretDistributionValidation{
		Environment:     environment,
		TotalSecrets:    len(plan),
		ValidSecrets:    0,
		MissingSecrets:  []string{},
		ConflictSecrets: []string{},
		Issues:          []string{},
	}

	for _, dist := range plan {
		for _, targetNamespace := range dist.TargetNamespaces {
			_, err := s.kubectlGateway.GetSecret(ctx, dist.SecretName, targetNamespace)
			if err != nil {
				validation.MissingSecrets = append(validation.MissingSecrets, 
					fmt.Sprintf("%s in %s", dist.SecretName, targetNamespace))
				validation.Issues = append(validation.Issues, 
					fmt.Sprintf("Secret %s missing in namespace %s", dist.SecretName, targetNamespace))
			} else {
				validation.ValidSecrets++
			}
		}
	}

	validation.IsValid = len(validation.MissingSecrets) == 0 && len(validation.ConflictSecrets) == 0

	s.logger.InfoWithContext("secret distribution validation completed", map[string]interface{}{
		"environment": environment,
		"valid": validation.IsValid,
		"missing_count": len(validation.MissingSecrets),
		"conflict_count": len(validation.ConflictSecrets),
	})

	return validation, nil
}