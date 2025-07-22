// PHASE R2: Helm template processing functionality
package core

import (
	"context"
	"fmt"
	"strings"

	"deploy-cli/domain"
	"deploy-cli/port/helm_port"
	"deploy-cli/port/logger_port"
)

// HelmTemplateGateway handles Helm template operations
type HelmTemplateGateway struct {
	helmPort helm_port.HelmPort
	logger   logger_port.LoggerPort
}

// HelmTemplateGatewayPort defines the interface for Helm template operations
type HelmTemplateGatewayPort interface {
	RenderTemplate(ctx context.Context, chart domain.Chart, options *domain.TemplateOptions) (*domain.TemplateResult, error)
	ValidateTemplate(ctx context.Context, chart domain.Chart, options *domain.TemplateOptions) (*domain.ValidationResult, error)
	LintChart(ctx context.Context, chart domain.Chart, options *domain.DeploymentOptions) (*domain.LintResult, error)
	DryRunChart(ctx context.Context, chart domain.Chart, options *domain.DeploymentOptions) (*domain.DryRunResult, error)
}

// NewHelmTemplateGateway creates a new Helm template gateway
func NewHelmTemplateGateway(
	helmPort helm_port.HelmPort,
	logger logger_port.LoggerPort,
) *HelmTemplateGateway {
	return &HelmTemplateGateway{
		helmPort: helmPort,
		logger:   logger,
	}
}

// RenderTemplate renders Helm templates without deploying
func (h *HelmTemplateGateway) RenderTemplate(ctx context.Context, chart domain.Chart, options *domain.TemplateOptions) (*domain.TemplateResult, error) {
	h.logger.InfoWithContext("rendering Helm template", map[string]interface{}{
		"chart":     chart.Name,
		"namespace": options.Namespace,
		"path":      chart.Path,
	})

	// Use the Template method with basic options
	templateOptions := helm_port.HelmTemplateOptions{
		Namespace: options.Namespace,
		SetValues: make(map[string]string),
	}
	
	// Convert interface{} values to string values  
	for k, v := range options.Values {
		if s, ok := v.(string); ok {
			templateOptions.SetValues[k] = s
		}
	}
	
	renderedManifests, err := h.helmPort.Template(ctx, options.ReleaseName, chart.Path, templateOptions)
	if err != nil {
		h.logger.ErrorWithContext("Helm template rendering failed", map[string]interface{}{
			"chart":     chart.Name,
			"namespace": options.Namespace,
			"error":     err.Error(),
		})
		return nil, fmt.Errorf("Helm template rendering failed for chart %s: %w", chart.Name, err)
	}

	// Create result from rendered manifests
	result := &domain.TemplateResult{
		Success:  true,
		Manifest: renderedManifests,
		Values:   options.Values,
	}

	h.logger.InfoWithContext("Helm template rendering completed", map[string]interface{}{
		"chart":         chart.Name,
		"namespace":     options.Namespace,
		"manifest_size": len(result.Manifest),
	})

	return result, nil
}

// ValidateTemplate validates Helm templates for syntax and content
func (h *HelmTemplateGateway) ValidateTemplate(ctx context.Context, chart domain.Chart, options *domain.TemplateOptions) (*domain.ValidationResult, error) {
	h.logger.InfoWithContext("validating Helm template", map[string]interface{}{
		"chart":     chart.Name,
		"namespace": options.Namespace,
	})

	// First, render the template to get the manifests
	renderResult, err := h.RenderTemplate(ctx, chart, options)
	if err != nil {
		return &domain.ValidationResult{
			Valid:  false,
			Errors: []domain.ValidationError{{
				Type:        domain.ValidationErrorTypeConfiguration,
				Severity:    domain.ValidationSeverityError,
				Message:     "Template rendering failed",
				Details:     err.Error(),
				File:        chart.Path,
			}},
		}, nil
	}

	validationResult := &domain.ValidationResult{
		Valid:    true,
		Errors:   make([]domain.ValidationError, 0),
		Warnings: make([]domain.ValidationWarning, 0),
	}

	// Validate the manifest
	if renderResult.Manifest != "" {
		if err := h.validateManifest(renderResult.Manifest, chart.Path); err != nil {
			validationResult.Valid = false
			validationResult.Errors = append(validationResult.Errors, domain.ValidationError{
				Type:        domain.ValidationErrorTypeResource,
				Severity:    domain.ValidationSeverityError,
				Message:     "Invalid manifest",
				Details:     err.Error(),
				File:        chart.Path,
			})
		}

		// Check for common issues
		warnings := h.checkManifestWarnings(renderResult.Manifest, chart.Path)
		validationResult.Warnings = append(validationResult.Warnings, warnings...)
	}

	h.logger.InfoWithContext("Helm template validation completed", map[string]interface{}{
		"chart":    chart.Name,
		"valid":    validationResult.Valid,
		"errors":   len(validationResult.Errors),
		"warnings": len(validationResult.Warnings),
	})

	return validationResult, nil
}

// LintChart runs Helm lint on the chart
func (h *HelmTemplateGateway) LintChart(ctx context.Context, chart domain.Chart, options *domain.DeploymentOptions) (*domain.LintResult, error) {
	h.logger.InfoWithContext("linting Helm chart", map[string]interface{}{
		"chart": chart.Name,
		"path":  chart.Path,
	})

	// Use the Lint method with basic options
	lintOptions := helm_port.HelmLintOptions{
		Strict: true, // Default to strict mode
	}

	helmResult, err := h.helmPort.Lint(ctx, chart.Path, lintOptions)
	if err != nil {
		h.logger.ErrorWithContext("Helm chart linting failed", map[string]interface{}{
			"chart": chart.Name,
			"error": err.Error(),
		})
		return nil, fmt.Errorf("Helm chart linting failed for %s: %w", chart.Name, err)
	}

	// Convert helm_port.HelmLintResult to domain.LintResult
	messages := make([]domain.LintMessage, 0, len(helmResult.Errors)+len(helmResult.Warnings))
	errorCount := len(helmResult.Errors)
	warningCount := len(helmResult.Warnings)

	// Add error messages
	for _, msg := range helmResult.Errors {
		messages = append(messages, domain.LintMessage{
			Severity: domain.LintSeverityError,
			Message:  msg.Message,
			File:     msg.Path,
		})
	}

	// Add warning messages
	for _, msg := range helmResult.Warnings {
		messages = append(messages, domain.LintMessage{
			Severity: domain.LintSeverityWarning,
			Message:  msg.Message,
			File:     msg.Path,
		})
	}

	result := &domain.LintResult{
		Success:      helmResult.Success,
		Messages:     messages,
		ErrorCount:   errorCount,
		WarningCount: warningCount,
		InfoCount:    0,
		Summary:      helmResult.Output,
	}

	h.logger.InfoWithContext("Helm chart linting completed", map[string]interface{}{
		"chart":    chart.Name,
		"success":  result.Success,
		"errors":   result.ErrorCount,
		"warnings": result.WarningCount,
	})

	return result, nil
}

// DryRunChart performs a dry-run deployment
func (h *HelmTemplateGateway) DryRunChart(ctx context.Context, chart domain.Chart, options *domain.DeploymentOptions) (*domain.DryRunResult, error) {
	h.logger.InfoWithContext("performing Helm dry-run", map[string]interface{}{
		"chart":     chart.Name,
		"namespace": options.GetNamespace(chart.Name),
	})

	deploymentGateway := NewHelmDeploymentGateway(h.helmPort, h.logger)
	
	// Create dry-run options
	dryRunOptions := *options
	dryRunOptions.DryRun = true

	// Attempt the deployment as a dry-run
	err := deploymentGateway.DeployChart(ctx, chart, &dryRunOptions)
	
	result := &domain.DryRunResult{
		Success:   err == nil,
		Resources: make([]domain.ResourceInfo, 0),
		Hooks:     make([]domain.HookInfo, 0),
	}

	if err != nil {
		result.Errors = []string{err.Error()}
		h.logger.WarnWithContext("Helm dry-run revealed issues", map[string]interface{}{
			"chart": chart.Name,
			"error": err.Error(),
		})
	} else {
		h.logger.InfoWithContext("Helm dry-run completed successfully", map[string]interface{}{
			"chart":     chart.Name,
			"namespace": options.GetNamespace(chart.Name),
		})
	}

	return result, nil
}

// Helper methods

// prepareValuesForLint prepares values for linting operation
func (h *HelmTemplateGateway) prepareValuesForLint(chart domain.Chart, options *domain.DeploymentOptions) map[string]interface{} {
	values := make(map[string]interface{})

	// Add chart-specific values
	if chart.Values != nil {
		for key, value := range chart.Values {
			values[key] = value
		}
	}

	// Add basic deployment values for linting
	values["namespace"] = map[string]interface{}{
		"name": options.GetNamespace(chart.Name),
	}

	// Add minimal image configuration if chart supports it
	if chart.SupportsImageOverride() {
		values["image"] = map[string]interface{}{
			"tag":        "latest",
			"pullPolicy": "IfNotPresent",
		}
	}

	return values
}

// validateManifest validates a single Kubernetes manifest
func (h *HelmTemplateGateway) validateManifest(manifest, filePath string) error {
	if strings.TrimSpace(manifest) == "" {
		return fmt.Errorf("manifest is empty")
	}

	// Basic YAML structure validation
	if !strings.Contains(manifest, "apiVersion:") {
		return fmt.Errorf("manifest missing apiVersion field")
	}

	if !strings.Contains(manifest, "kind:") {
		return fmt.Errorf("manifest missing kind field")
	}

	if !strings.Contains(manifest, "metadata:") {
		return fmt.Errorf("manifest missing metadata field")
	}

	// Check for Helm template syntax issues
	if strings.Contains(manifest, "{{") && !strings.Contains(manifest, "}}") {
		return fmt.Errorf("unclosed Helm template expression")
	}

	return nil
}

// checkManifestWarnings checks for common issues that are warnings, not errors
func (h *HelmTemplateGateway) checkManifestWarnings(manifest, filePath string) []domain.ValidationWarning {
	warnings := make([]domain.ValidationWarning, 0)

	// Check for missing labels
	if !strings.Contains(manifest, "app.kubernetes.io/name") {
		warnings = append(warnings, domain.ValidationWarning{
			Type:     domain.ValidationWarningTypeBestPractices,
			Severity: domain.ValidationSeverityWarning,
			Message:  "Recommended label 'app.kubernetes.io/name' is missing",
			File:     filePath,
		})
	}

	// Check for missing Helm metadata annotations (Phase 1-2 requirement)
	if !strings.Contains(manifest, "meta.helm.sh/release-name") {
		warnings = append(warnings, domain.ValidationWarning{
			Type:     domain.ValidationWarningTypeBestPractices,
			Severity: domain.ValidationSeverityWarning,
			Message:  "Helm metadata annotation 'meta.helm.sh/release-name' is missing",
			File:     filePath,
		})
	}

	if !strings.Contains(manifest, "app.kubernetes.io/managed-by: Helm") {
		warnings = append(warnings, domain.ValidationWarning{
			Type:     domain.ValidationWarningTypeBestPractices,
			Severity: domain.ValidationSeverityWarning,
			Message:  "Helm managed-by label is missing",
			File:     filePath,
		})
	}

	// Check for hardcoded values
	if strings.Contains(manifest, "image: nginx:latest") {
		warnings = append(warnings, domain.ValidationWarning{
			Type:     domain.ValidationWarningTypeBestPractices,
			Severity: domain.ValidationSeverityInfo,
			Message:  "Consider using configurable image tag instead of 'latest'",
			File:     filePath,
		})
	}

	// Check for missing resource limits
	if strings.Contains(manifest, "kind: Deployment") && !strings.Contains(manifest, "resources:") {
		warnings = append(warnings, domain.ValidationWarning{
			Type:     domain.ValidationWarningTypeBestPractices,
			Severity: domain.ValidationSeverityWarning,
			Message:  "Resource limits/requests are not specified",
			File:     filePath,
		})
	}

	return warnings
}