package deployment_usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	"deploy-cli/domain"
	"deploy-cli/port/logger_port"
	"deploy-cli/gateway/helm_gateway"
	"deploy-cli/gateway/kubectl_gateway"
)

// DeploymentStateValidator handles validation of deployment state
type DeploymentStateValidator struct {
	helmGateway    *helm_gateway.HelmGateway
	kubectlGateway *kubectl_gateway.KubectlGateway
	logger         logger_port.LoggerPort
}

// NewDeploymentStateValidator creates a new deployment state validator
func NewDeploymentStateValidator(
	helmGateway *helm_gateway.HelmGateway,
	kubectlGateway *kubectl_gateway.KubectlGateway,
	logger logger_port.LoggerPort,
) *DeploymentStateValidator {
	return &DeploymentStateValidator{
		helmGateway:    helmGateway,
		kubectlGateway: kubectlGateway,
		logger:         logger,
	}
}

// DeploymentStateReport represents the current state of deployment
type DeploymentStateReport struct {
	IsHealthy                bool
	StuckOperations         []StuckOperation
	CorruptedReleases       []CorruptedRelease
	ResourceInconsistencies []ResourceInconsistency
	ValidationErrors        []ValidationError
	Summary                 string
}

// StuckOperation represents a stuck Helm operation
type StuckOperation struct {
	ReleaseName string
	Namespace   string
	Operation   string
	Duration    time.Duration
	Error       string
}

// CorruptedRelease represents a corrupted Helm release
type CorruptedRelease struct {
	ReleaseName string
	Namespace   string
	Status      string
	Issue       string
	Recoverable bool
}

// ResourceInconsistency represents inconsistency between Helm and Kubernetes resources
type ResourceInconsistency struct {
	ResourceType string
	ResourceName string
	Namespace    string
	Issue        string
	Severity     string
}

// ValidationError represents a validation error
type ValidationError struct {
	Type        string
	Description string
	Severity    string
	Suggestion  string
}

// ValidateDeploymentState performs comprehensive validation of deployment state
func (v *DeploymentStateValidator) ValidateDeploymentState(ctx context.Context, environment domain.Environment) (*DeploymentStateReport, error) {
	v.logger.InfoWithContext("starting deployment state validation", map[string]interface{}{
		"environment": environment.String(),
	})

	report := &DeploymentStateReport{
		IsHealthy:                true,
		StuckOperations:         []StuckOperation{},
		CorruptedReleases:       []CorruptedRelease{},
		ResourceInconsistencies: []ResourceInconsistency{},
		ValidationErrors:        []ValidationError{},
	}

	// Check for stuck Helm operations
	if err := v.checkStuckOperations(ctx, report); err != nil {
		v.logger.ErrorWithContext("failed to check stuck operations", map[string]interface{}{
			"error": err.Error(),
		})
		report.ValidationErrors = append(report.ValidationErrors, ValidationError{
			Type:        "stuck_operations_check",
			Description: fmt.Sprintf("Failed to check stuck operations: %v", err),
			Severity:    "high",
			Suggestion:  "Check Helm installation and cluster connectivity",
		})
	}

	// Check for corrupted releases
	if err := v.checkCorruptedReleases(ctx, report); err != nil {
		v.logger.ErrorWithContext("failed to check corrupted releases", map[string]interface{}{
			"error": err.Error(),
		})
		report.ValidationErrors = append(report.ValidationErrors, ValidationError{
			Type:        "corrupted_releases_check",
			Description: fmt.Sprintf("Failed to check corrupted releases: %v", err),
			Severity:    "high",
			Suggestion:  "Check Helm installation and cluster connectivity",
		})
	}

	// Check resource inconsistencies
	if err := v.checkResourceInconsistencies(ctx, report); err != nil {
		v.logger.ErrorWithContext("failed to check resource inconsistencies", map[string]interface{}{
			"error": err.Error(),
		})
		report.ValidationErrors = append(report.ValidationErrors, ValidationError{
			Type:        "resource_inconsistencies_check",
			Description: fmt.Sprintf("Failed to check resource inconsistencies: %v", err),
			Severity:    "medium",
			Suggestion:  "Check Kubernetes cluster health",
		})
	}

	// Determine overall health
	report.IsHealthy = len(report.StuckOperations) == 0 && 
		len(report.CorruptedReleases) == 0 && 
		len(report.ResourceInconsistencies) == 0 && 
		len(report.ValidationErrors) == 0

	// Generate summary
	report.Summary = v.generateSummary(report)

	v.logger.InfoWithContext("deployment state validation completed", map[string]interface{}{
		"is_healthy":                report.IsHealthy,
		"stuck_operations":          len(report.StuckOperations),
		"corrupted_releases":        len(report.CorruptedReleases),
		"resource_inconsistencies":  len(report.ResourceInconsistencies),
		"validation_errors":         len(report.ValidationErrors),
	})

	return report, nil
}

// checkStuckOperations checks for stuck Helm operations
func (v *DeploymentStateValidator) checkStuckOperations(ctx context.Context, report *DeploymentStateReport) error {
	// This would typically involve checking Helm secrets for pending operations
	// For now, we'll implement a basic check
	
	// Check for helm list command hanging or failing
	helmListOutput, err := v.helmGateway.ListReleases(ctx, "")
	if err != nil {
		if strings.Contains(err.Error(), "another operation") {
			report.StuckOperations = append(report.StuckOperations, StuckOperation{
				ReleaseName: "unknown",
				Namespace:   "unknown",
				Operation:   "unknown",
				Duration:    time.Duration(0),
				Error:       err.Error(),
			})
			report.IsHealthy = false
		}
		return err
	}

	// Check each release for stuck state
	for _, release := range helmListOutput {
		if release.Status == "pending-install" || release.Status == "pending-upgrade" || release.Status == "pending-rollback" {
			report.StuckOperations = append(report.StuckOperations, StuckOperation{
				ReleaseName: release.Name,
				Namespace:   release.Namespace,
				Operation:   release.Status,
				Duration:    time.Since(release.Updated),
				Error:       "Operation appears to be stuck",
			})
			report.IsHealthy = false
		}
	}

	return nil
}

// checkCorruptedReleases checks for corrupted Helm releases
func (v *DeploymentStateValidator) checkCorruptedReleases(ctx context.Context, report *DeploymentStateReport) error {
	helmListOutput, err := v.helmGateway.ListReleases(ctx, "")
	if err != nil {
		return err
	}

	for _, release := range helmListOutput {
		if release.Status == "failed" || release.Status == "unknown" {
			report.CorruptedReleases = append(report.CorruptedReleases, CorruptedRelease{
				ReleaseName: release.Name,
				Namespace:   release.Namespace,
				Status:      release.Status,
				Issue:       "Release in failed or unknown state",
				Recoverable: release.Status == "failed", // failed releases can often be recovered
			})
			report.IsHealthy = false
		}
	}

	return nil
}

// checkResourceInconsistencies checks for inconsistencies between Helm and Kubernetes
func (v *DeploymentStateValidator) checkResourceInconsistencies(ctx context.Context, report *DeploymentStateReport) error {
	// This would involve comparing Helm-managed resources with actual Kubernetes resources
	// For now, we'll implement a basic check

	namespaces := []string{
		"alt-apps",
		"alt-database", 
		"alt-search",
		"alt-auth",
		"alt-ingress",
		"alt-observability",
		"alt-production",
	}

	// Get all existing namespaces
	existingNamespaces, err := v.kubectlGateway.GetNamespaces(ctx)
	if err != nil {
		report.ResourceInconsistencies = append(report.ResourceInconsistencies, ResourceInconsistency{
			ResourceType: "namespace",
			ResourceName: "all",
			Namespace:    "all",
			Issue:        fmt.Sprintf("Failed to get namespaces: %v", err),
			Severity:     "high",
		})
		report.IsHealthy = false
		return nil
	}

	// Create a map of existing namespaces for quick lookup
	existingNamespaceMap := make(map[string]bool)
	for _, ns := range existingNamespaces {
		existingNamespaceMap[ns.Name] = true
	}

	// Check if expected namespaces exist
	for _, namespace := range namespaces {
		if !existingNamespaceMap[namespace] {
			report.ResourceInconsistencies = append(report.ResourceInconsistencies, ResourceInconsistency{
				ResourceType: "namespace",
				ResourceName: namespace,
				Namespace:    namespace,
				Issue:        "Expected namespace does not exist",
				Severity:     "medium",
			})
		}
	}

	return nil
}

// generateSummary generates a human-readable summary of the deployment state
func (v *DeploymentStateValidator) generateSummary(report *DeploymentStateReport) string {
	if report.IsHealthy {
		return "✅ Deployment state is healthy - no issues detected"
	}

	var issues []string
	
	if len(report.StuckOperations) > 0 {
		issues = append(issues, fmt.Sprintf("❌ %d stuck operations detected", len(report.StuckOperations)))
	}
	
	if len(report.CorruptedReleases) > 0 {
		issues = append(issues, fmt.Sprintf("❌ %d corrupted releases detected", len(report.CorruptedReleases)))
	}
	
	if len(report.ResourceInconsistencies) > 0 {
		issues = append(issues, fmt.Sprintf("⚠️ %d resource inconsistencies detected", len(report.ResourceInconsistencies)))
	}
	
	if len(report.ValidationErrors) > 0 {
		issues = append(issues, fmt.Sprintf("⚠️ %d validation errors detected", len(report.ValidationErrors)))
	}

	summary := "🚨 Deployment state issues detected:\n" + strings.Join(issues, "\n")
	
	// Add recommendation
	if len(report.StuckOperations) > 0 || len(report.CorruptedReleases) > 0 {
		summary += "\n\n💡 Recommendation: Consider running 'deploy-cli emergency-reset' to resolve persistent issues"
	}

	return summary
}

// DetectCorruptionPattern detects specific corruption patterns
func (v *DeploymentStateValidator) DetectCorruptionPattern(ctx context.Context, environment domain.Environment) (string, error) {
	report, err := v.ValidateDeploymentState(ctx, environment)
	if err != nil {
		return "", fmt.Errorf("failed to validate deployment state: %w", err)
	}

	// Check for the specific "another operation in progress" pattern
	for _, op := range report.StuckOperations {
		if strings.Contains(op.Error, "another operation") {
			return "helm_operation_lock", nil
		}
	}

	// Check for multiple failed releases
	if len(report.CorruptedReleases) > 2 {
		return "multiple_failed_releases", nil
	}

	// Check for missing namespaces
	namespaceIssues := 0
	for _, inconsistency := range report.ResourceInconsistencies {
		if inconsistency.ResourceType == "namespace" && strings.Contains(inconsistency.Issue, "does not exist") {
			namespaceIssues++
		}
	}

	if namespaceIssues > 3 {
		return "missing_namespaces", nil
	}

	if !report.IsHealthy {
		return "general_corruption", nil
	}

	return "healthy", nil
}