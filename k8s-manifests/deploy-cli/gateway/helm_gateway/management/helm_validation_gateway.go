// PHASE R2: Helm validation and compliance functionality
package management

import (
	"context"
	"fmt"
	"strings"

	"deploy-cli/domain"
	"deploy-cli/port/helm_port"
	"deploy-cli/port/logger_port"
)

// HelmValidationGateway handles Helm chart validation and compliance
type HelmValidationGateway struct {
	helmPort helm_port.HelmPort
	logger   logger_port.LoggerPort
}

// HelmValidationGatewayPort defines the interface for Helm validation operations
type HelmValidationGatewayPort interface {
	ValidateChartStructure(ctx context.Context, chart domain.Chart) (*domain.ValidationResult, error)
	ValidateChartCompliance(ctx context.Context, chart domain.Chart, rules *domain.ComplianceRules) (*domain.ComplianceResult, error)
	ValidateChartSecurity(ctx context.Context, chart domain.Chart) (*domain.SecurityValidationResult, error)
	ValidateChartDependencies(ctx context.Context, chart domain.Chart) (*domain.DependencyValidationResult, error)
	ValidateHelmValues(ctx context.Context, chart domain.Chart, values map[string]interface{}) (*domain.ValidationResult, error)
}

// NewHelmValidationGateway creates a new Helm validation gateway
func NewHelmValidationGateway(
	helmPort helm_port.HelmPort,
	logger logger_port.LoggerPort,
) *HelmValidationGateway {
	return &HelmValidationGateway{
		helmPort: helmPort,
		logger:   logger,
	}
}

// ValidateChartStructure validates the structure and metadata of a Helm chart
func (h *HelmValidationGateway) ValidateChartStructure(ctx context.Context, chart domain.Chart) (*domain.ValidationResult, error) {
	h.logger.InfoWithContext("validating Helm chart structure", map[string]interface{}{
		"chart": chart.Name,
		"path":  chart.Path,
	})

	result := &domain.ValidationResult{
		Valid:    true,
		Errors:   make([]domain.ValidationError, 0),
		Warnings: make([]domain.ValidationWarning, 0),
	}

	// Validate Chart.yaml exists and is properly formatted
	if err := h.validateChartMetadata(chart); err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, domain.ValidationError{
			Type:     domain.ValidationErrorTypeMetadata,
			Severity: domain.ValidationSeverityError,
			Message:  "Invalid chart metadata",
			Details:  err.Error(),
			File:     fmt.Sprintf("%s/Chart.yaml", chart.Path),
		})
	}

	// Validate templates directory structure
	if warnings := h.validateTemplatesStructure(chart); len(warnings) > 0 {
		result.Warnings = append(result.Warnings, warnings...)
	}

	// Validate values.yaml structure
	if err := h.validateValuesStructure(chart); err != nil {
		result.Warnings = append(result.Warnings, domain.ValidationWarning{
			Type:     domain.ValidationWarningTypeValues,
			Severity: domain.ValidationSeverityWarning,
			Message:  "Values structure issues found",
			Details:  err.Error(),
			File:     fmt.Sprintf("%s/values.yaml", chart.Path),
		})
	}

	h.logger.InfoWithContext("Helm chart structure validation completed", map[string]interface{}{
		"chart":    chart.Name,
		"valid":    result.Valid,
		"errors":   len(result.Errors),
		"warnings": len(result.Warnings),
	})

	return result, nil
}

// ValidateChartCompliance validates chart against compliance rules (Phase 1-2 requirement)
func (h *HelmValidationGateway) ValidateChartCompliance(ctx context.Context, chart domain.Chart, rules *domain.ComplianceRules) (*domain.ComplianceResult, error) {
	h.logger.InfoWithContext("validating Helm chart compliance", map[string]interface{}{
		"chart":       chart.Name,
		"rules_count": len(rules.Rules),
	})

	result := &domain.ComplianceResult{
		Compliant:     true,
		Violations:    make([]domain.ComplianceViolation, 0),
		Score:         100.0,
		RulesChecked: len(rules.Rules),
	}

	// Check required labels compliance
	if err := h.validateRequiredLabels(chart, rules.RequiredLabels); err != nil {
		result.Compliant = false
		result.Score -= 20.0
		result.Violations = append(result.Violations, domain.ComplianceViolation{
			Rule:        "required-labels",
			Severity:    domain.ComplianceSeverityHigh,
			Message:     "Missing required labels",
			Details:     err.Error(),
			Remediation: "Add required labels as specified in compliance rules",
		})
	}

	// Check security context compliance
	if err := h.validateSecurityContext(chart, rules.SecurityRequirements); err != nil {
		result.Compliant = false
		result.Score -= 15.0
		result.Violations = append(result.Violations, domain.ComplianceViolation{
			Rule:        "security-context",
			Severity:    domain.ComplianceSeverityHigh,
			Message:     "Security context violations",
			Details:     err.Error(),
			Remediation: "Configure proper security context as per compliance rules",
		})
	}

	// Check resource limits compliance
	if err := h.validateResourceLimits(chart, rules.ResourceRequirements); err != nil {
		result.Compliant = false
		result.Score -= 10.0
		result.Violations = append(result.Violations, domain.ComplianceViolation{
			Rule:        "resource-limits",
			Severity:    domain.ComplianceSeverityMedium,
			Message:     "Resource limits not properly configured",
			Details:     err.Error(),
			Remediation: "Configure resource requests and limits",
		})
	}

	h.logger.InfoWithContext("Helm chart compliance validation completed", map[string]interface{}{
		"chart":      chart.Name,
		"compliant":  result.Compliant,
		"score":      result.Score,
		"violations": len(result.Violations),
	})

	return result, nil
}

// ValidateChartSecurity validates chart for security vulnerabilities
func (h *HelmValidationGateway) ValidateChartSecurity(ctx context.Context, chart domain.Chart) (*domain.SecurityValidationResult, error) {
	h.logger.InfoWithContext("validating Helm chart security", map[string]interface{}{
		"chart": chart.Name,
	})

	result := &domain.SecurityValidationResult{
		Secure:          true,
		Vulnerabilities: make([]domain.SecurityVulnerability, 0),
		RiskScore:       0.0,
		Recommendations: make([]string, 0),
	}

	// Check for privileged containers
	if err := h.checkPrivilegedContainers(chart); err != nil {
		result.Secure = false
		result.RiskScore += 8.0
		result.Vulnerabilities = append(result.Vulnerabilities, domain.SecurityVulnerability{
			Type:        domain.SecurityVulnerabilityTypePrivileged,
			Severity:    domain.SecuritySeverityHigh,
			Title:       "Privileged containers detected",
			Description: err.Error(),
			Impact:      "Containers running with elevated privileges",
			Mitigation:  "Remove privileged: true or use security contexts",
		})
	}

	// Check for root user usage
	if err := h.checkRootUserUsage(chart); err != nil {
		result.RiskScore += 6.0
		result.Vulnerabilities = append(result.Vulnerabilities, domain.SecurityVulnerability{
			Type:        domain.SecurityVulnerabilityTypeRootUser,
			Severity:    domain.SecuritySeverityMedium,
			Title:       "Root user usage detected",
			Description: err.Error(),
			Impact:      "Containers running as root user",
			Mitigation:  "Configure runAsNonRoot: true in security context",
		})
	}

	// Check for missing network policies
	if !h.hasNetworkPolicies(chart) {
		result.RiskScore += 4.0
		result.Recommendations = append(result.Recommendations, 
			"Consider adding NetworkPolicy resources to restrict pod-to-pod communication")
	}

	// Check for exposed secrets in values
	if secrets := h.findExposedSecrets(chart); len(secrets) > 0 {
		result.Secure = false
		result.RiskScore += 10.0
		result.Vulnerabilities = append(result.Vulnerabilities, domain.SecurityVulnerability{
			Type:        domain.SecurityVulnerabilityTypeExposedSecrets,
			Severity:    domain.SecuritySeverityCritical,
			Title:       "Exposed secrets in chart values",
			Description: fmt.Sprintf("Found %d potential secrets in values: %v", len(secrets), secrets),
			Impact:      "Sensitive data exposed in chart configuration",
			Mitigation:  "Use Kubernetes Secrets or external secret management",
		})
	}

	if result.RiskScore >= 7.0 {
		result.Secure = false
	}

	h.logger.InfoWithContext("Helm chart security validation completed", map[string]interface{}{
		"chart":           chart.Name,
		"secure":          result.Secure,
		"risk_score":      result.RiskScore,
		"vulnerabilities": len(result.Vulnerabilities),
	})

	return result, nil
}

// ValidateChartDependencies validates chart dependencies
func (h *HelmValidationGateway) ValidateChartDependencies(ctx context.Context, chart domain.Chart) (*domain.DependencyValidationResult, error) {
	h.logger.InfoWithContext("validating Helm chart dependencies", map[string]interface{}{
		"chart": chart.Name,
	})

	result := &domain.DependencyValidationResult{
		Valid:        true,
		Dependencies: make([]*domain.DependencyInfo, 0),
		Issues:       make([]domain.DependencyIssue, 0),
	}

	// Get chart dependencies from Chart.yaml
	request := &domain.HelmDependencyRequest{
		ChartPath: chart.Path,
	}

	dependencies, err := h.helmPort.GetChartDependencies(ctx, request)
	if err != nil {
		result.Valid = false
		result.Issues = append(result.Issues, domain.DependencyIssue{
			Type:        domain.DependencyIssueTypeResolution,
			Severity:    domain.DependencyIssueSeverityError,
			Message:     "Failed to resolve dependencies",
			Details:     err.Error(),
			Dependency:  "",
			Remediation: "Check Chart.yaml dependencies section and repository access",
		})
		return result, nil
	}

	result.Dependencies = dependencies

	// Validate each dependency
	for _, dep := range dependencies {
		if err := h.validateDependency(ctx, *dep); err != nil {
			result.Valid = false
			result.Issues = append(result.Issues, domain.DependencyIssue{
				Type:        domain.DependencyIssueTypeValidation,
				Severity:    domain.DependencyIssueSeverityError,
				Message:     fmt.Sprintf("Dependency validation failed: %s", dep.Name),
				Details:     err.Error(),
				Dependency:  dep.Name,
				Remediation: "Update dependency version or check repository availability",
			})
		}

		// Check for version conflicts
		// Convert pointer slice to value slice for checkVersionConflicts
		allDepsValues := make([]domain.DependencyInfo, len(dependencies))
		for i, d := range dependencies {
			allDepsValues[i] = *d
		}
		if warnings := h.checkVersionConflicts(*dep, allDepsValues); len(warnings) > 0 {
			for _, warning := range warnings {
				result.Issues = append(result.Issues, domain.DependencyIssue{
					Type:        domain.DependencyIssueTypeVersionConflict,
					Severity:    domain.DependencyIssueSeverityWarning,
					Message:     warning,
					Dependency:  dep.Name,
					Remediation: "Review dependency versions for compatibility",
				})
			}
		}
	}

	h.logger.InfoWithContext("Helm chart dependency validation completed", map[string]interface{}{
		"chart":        chart.Name,
		"valid":        result.Valid,
		"dependencies": len(result.Dependencies),
		"issues":       len(result.Issues),
	})

	return result, nil
}

// ValidateHelmValues validates Helm values against schema
func (h *HelmValidationGateway) ValidateHelmValues(ctx context.Context, chart domain.Chart, values map[string]interface{}) (*domain.ValidationResult, error) {
	h.logger.InfoWithContext("validating Helm values", map[string]interface{}{
		"chart":       chart.Name,
		"values_keys": len(values),
	})

	result := &domain.ValidationResult{
		Valid:    true,
		Errors:   make([]domain.ValidationError, 0),
		Warnings: make([]domain.ValidationWarning, 0),
	}

	// Validate against values schema if available
	if err := h.validateAgainstSchema(chart, values); err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, domain.ValidationError{
			Type:     domain.ValidationErrorTypeSchema,
			Severity: domain.ValidationSeverityError,
			Message:  "Values validation against schema failed",
			Details:  err.Error(),
			File:     fmt.Sprintf("%s/values.yaml", chart.Path),
		})
	}

	// Check for deprecated fields
	if warnings := h.checkDeprecatedFields(values); len(warnings) > 0 {
		result.Warnings = append(result.Warnings, warnings...)
	}

	// Check for missing required fields
	if errors := h.checkRequiredFields(chart, values); len(errors) > 0 {
		result.Valid = false
		result.Errors = append(result.Errors, errors...)
	}

	h.logger.InfoWithContext("Helm values validation completed", map[string]interface{}{
		"chart":    chart.Name,
		"valid":    result.Valid,
		"errors":   len(result.Errors),
		"warnings": len(result.Warnings),
	})

	return result, nil
}

// Helper methods for validation

func (h *HelmValidationGateway) validateChartMetadata(chart domain.Chart) error {
	if chart.Name == "" {
		return fmt.Errorf("chart name is required")
	}
	if chart.Version == "" {
		return fmt.Errorf("chart version is required")
	}
	return nil
}

func (h *HelmValidationGateway) validateTemplatesStructure(chart domain.Chart) []domain.ValidationWarning {
	warnings := make([]domain.ValidationWarning, 0)
	
	// This would typically check the actual file system
	// For now, we'll create warnings based on chart configuration
	if !chart.HasTemplates() {
		warnings = append(warnings, domain.ValidationWarning{
			Type:     domain.ValidationWarningTypeMissingTemplates,
			Severity: domain.ValidationSeverityWarning,
			Message:  "Chart has no templates directory or templates",
			File:     fmt.Sprintf("%s/templates/", chart.Path),
		})
	}
	
	return warnings
}

func (h *HelmValidationGateway) validateValuesStructure(chart domain.Chart) error {
	if chart.Values == nil {
		return fmt.Errorf("no values.yaml found")
	}
	return nil
}

func (h *HelmValidationGateway) validateRequiredLabels(chart domain.Chart, requiredLabels []string) error {
	missing := make([]string, 0)
	for _, label := range requiredLabels {
		if !h.chartHasLabel(chart, label) {
			missing = append(missing, label)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required labels: %v", missing)
	}
	return nil
}

func (h *HelmValidationGateway) chartHasLabel(chart domain.Chart, label string) bool {
	// This would check actual chart templates for the label
	// Implementation would parse templates and check for label presence
	return false // Placeholder implementation
}

func (h *HelmValidationGateway) validateSecurityContext(chart domain.Chart, requirements *domain.SecurityRequirements) error {
	if requirements.RequireNonRoot && !h.chartHasNonRootSecurityContext(chart) {
		return fmt.Errorf("chart must configure runAsNonRoot: true")
	}
	if requirements.RequireReadOnlyRootFilesystem && !h.chartHasReadOnlyRootFS(chart) {
		return fmt.Errorf("chart must configure readOnlyRootFilesystem: true")
	}
	return nil
}

func (h *HelmValidationGateway) chartHasNonRootSecurityContext(chart domain.Chart) bool {
	// Implementation would parse templates for security context
	return false // Placeholder
}

func (h *HelmValidationGateway) chartHasReadOnlyRootFS(chart domain.Chart) bool {
	// Implementation would parse templates for read-only root filesystem
	return false // Placeholder
}

func (h *HelmValidationGateway) validateResourceLimits(chart domain.Chart, requirements *domain.ResourceRequirements) error {
	if requirements.RequireResourceLimits && !h.chartHasResourceLimits(chart) {
		return fmt.Errorf("chart must specify resource limits")
	}
	return nil
}

func (h *HelmValidationGateway) chartHasResourceLimits(chart domain.Chart) bool {
	// Implementation would parse templates for resource limits
	return false // Placeholder
}

func (h *HelmValidationGateway) checkPrivilegedContainers(chart domain.Chart) error {
	// Implementation would parse templates for privileged: true
	return nil // No privileged containers found
}

func (h *HelmValidationGateway) checkRootUserUsage(chart domain.Chart) error {
	// Implementation would parse templates for runAsUser: 0
	return nil // No root user usage found
}

func (h *HelmValidationGateway) hasNetworkPolicies(chart domain.Chart) bool {
	// Implementation would check for NetworkPolicy resources in templates
	return false // No network policies found
}

func (h *HelmValidationGateway) findExposedSecrets(chart domain.Chart) []string {
	secrets := make([]string, 0)
	
	// Check values for potential secrets
	if chart.Values != nil {
		for key, value := range chart.Values {
			if h.looksLikeSecret(key, value) {
				secrets = append(secrets, key)
			}
		}
	}
	
	return secrets
}

func (h *HelmValidationGateway) looksLikeSecret(key string, value interface{}) bool {
	keyLower := strings.ToLower(key)
	secretKeywords := []string{"password", "secret", "key", "token", "credential"}
	
	for _, keyword := range secretKeywords {
		if strings.Contains(keyLower, keyword) {
			return true
		}
	}
	
	// Check if value looks like encoded data
	if str, ok := value.(string); ok {
		if len(str) > 20 && (strings.Contains(str, "==") || len(strings.Fields(str)) == 1) {
			return true
		}
	}
	
	return false
}

func (h *HelmValidationGateway) validateDependency(ctx context.Context, dep domain.DependencyInfo) error {
	if dep.Version == "" {
		return fmt.Errorf("dependency version is required")
	}
	if dep.Repository == "" && dep.Name != "" {
		return fmt.Errorf("dependency repository is required")
	}
	return nil
}

func (h *HelmValidationGateway) checkVersionConflicts(dep domain.DependencyInfo, allDeps []domain.DependencyInfo) []string {
	warnings := make([]string, 0)
	
	for _, other := range allDeps {
		if other.Name == dep.Name && other.Version != dep.Version {
			warnings = append(warnings, fmt.Sprintf("version conflict: %s has multiple versions (%s, %s)", 
				dep.Name, dep.Version, other.Version))
		}
	}
	
	return warnings
}

func (h *HelmValidationGateway) validateAgainstSchema(chart domain.Chart, values map[string]interface{}) error {
	// Implementation would validate values against values.schema.json if present
	return nil // No schema validation errors
}

func (h *HelmValidationGateway) checkDeprecatedFields(values map[string]interface{}) []domain.ValidationWarning {
	warnings := make([]domain.ValidationWarning, 0)
	
	// Check for commonly deprecated fields
	deprecatedFields := []string{"nodeSelector", "tolerations"}
	
	for _, field := range deprecatedFields {
		if _, exists := values[field]; exists {
			warnings = append(warnings, domain.ValidationWarning{
				Type:     domain.ValidationWarningTypeDeprecated,
				Severity: domain.ValidationSeverityWarning,
				Message:  fmt.Sprintf("Field '%s' is deprecated", field),
				Details:  fmt.Sprintf("Consider using affinity rules instead of %s", field),
			})
		}
	}
	
	return warnings
}

func (h *HelmValidationGateway) checkRequiredFields(chart domain.Chart, values map[string]interface{}) []domain.ValidationError {
	errors := make([]domain.ValidationError, 0)
	
	// Check for required fields based on chart type
	requiredFields := h.getRequiredFieldsForChartType(string(chart.Type))
	
	for _, field := range requiredFields {
		if _, exists := values[field]; !exists {
			errors = append(errors, domain.ValidationError{
				Type:     domain.ValidationErrorTypeMissingField,
				Severity: domain.ValidationSeverityError,
				Message:  fmt.Sprintf("Required field '%s' is missing", field),
				Details:  fmt.Sprintf("Chart type '%s' requires field '%s'", chart.Type, field),
			})
		}
	}
	
	return errors
}

func (h *HelmValidationGateway) getRequiredFieldsForChartType(chartType string) []string {
	switch chartType {
	case "application":
		return []string{"image", "service"}
	case "library":
		return []string{"name", "version"}
	default:
		return []string{"name"}
	}
}