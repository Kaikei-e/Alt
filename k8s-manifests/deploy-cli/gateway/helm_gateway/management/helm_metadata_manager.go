// PHASE R2: Helm metadata and configuration management
package management

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"deploy-cli/domain"
	"deploy-cli/port/helm_port"
	"deploy-cli/port/logger_port"
)

// HelmMetadataManager handles Helm chart metadata and configuration management
type HelmMetadataManager struct {
	helmPort helm_port.HelmPort
	logger   logger_port.LoggerPort
}

// HelmMetadataManagerPort defines the interface for Helm metadata management operations
type HelmMetadataManagerPort interface {
	GetChartMetadata(ctx context.Context, chartPath string) (*domain.ChartMetadata, error)
	ValidateChartMetadata(ctx context.Context, metadata *domain.ChartMetadata) (*domain.ValidationResult, error)
	UpdateChartMetadata(ctx context.Context, chartPath string, updates *domain.MetadataUpdates) error
	GetChartDependencies(ctx context.Context, chartPath string) ([]*domain.ChartDependency, error)
	UpdateChartDependencies(ctx context.Context, chartPath string, dependencies []*domain.ChartDependency) error
	GetChartValues(ctx context.Context, chartPath string) (map[string]interface{}, error)
	MergeValues(ctx context.Context, baseValues, overrideValues map[string]interface{}) (map[string]interface{}, error)
	GenerateChartSummary(ctx context.Context, chartPath string) (*domain.ChartSummary, error)
}

// NewHelmMetadataManager creates a new Helm metadata manager
func NewHelmMetadataManager(
	helmPort helm_port.HelmPort,
	logger logger_port.LoggerPort,
) *HelmMetadataManager {
	return &HelmMetadataManager{
		helmPort: helmPort,
		logger:   logger,
	}
}

// GetChartMetadata retrieves chart metadata from Chart.yaml
func (h *HelmMetadataManager) GetChartMetadata(ctx context.Context, chartPath string) (*domain.ChartMetadata, error) {
	h.logger.DebugWithContext("getting chart metadata", map[string]interface{}{
		"chart_path": chartPath,
	})

	request := &domain.HelmMetadataRequest{
		ChartPath: chartPath,
	}

	metadata, err := h.helmPort.GetChartMetadata(ctx, request)
	if err != nil {
		h.logger.ErrorWithContext("failed to get chart metadata", map[string]interface{}{
			"chart_path": chartPath,
			"error":      err.Error(),
		})
		return nil, fmt.Errorf("failed to get chart metadata: %w", err)
	}

	// Enhance metadata with additional information
	h.enhanceMetadata(metadata, chartPath)

	h.logger.DebugWithContext("chart metadata retrieved successfully", map[string]interface{}{
		"chart_path": chartPath,
		"name":       metadata.Name,
		"version":    metadata.Version,
		"type":       metadata.Type,
	})

	return metadata, nil
}

// ValidateChartMetadata validates chart metadata for correctness and compliance
func (h *HelmMetadataManager) ValidateChartMetadata(ctx context.Context, metadata *domain.ChartMetadata) (*domain.ValidationResult, error) {
	h.logger.DebugWithContext("validating chart metadata", map[string]interface{}{
		"name":    metadata.Name,
		"version": metadata.Version,
		"type":    metadata.Type,
	})

	result := &domain.ValidationResult{
		Valid:    true,
		Errors:   make([]domain.ValidationError, 0),
		Warnings: make([]domain.ValidationWarning, 0),
	}

	// Validate required fields
	if err := h.validateRequiredFields(metadata); err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, domain.ValidationError{
			Type:     domain.ValidationErrorTypeMetadata,
			Severity: domain.ValidationSeverityError,
			Message:  "Missing required metadata fields",
			Details:  err.Error(),
		})
	}

	// Validate version format
	if err := h.validateVersionFormat(metadata.Version); err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, domain.ValidationError{
			Type:     domain.ValidationErrorTypeVersion,
			Severity: domain.ValidationSeverityError,
			Message:  "Invalid version format",
			Details:  err.Error(),
		})
	}

	// Validate chart name
	if warnings := h.validateChartName(metadata.Name); len(warnings) > 0 {
		result.Warnings = append(result.Warnings, warnings...)
	}

	// Validate dependencies
	if len(metadata.Dependencies) > 0 {
		// Convert to pointer slice for validation
		depPointers := make([]*domain.ChartDependency, len(metadata.Dependencies))
		for i := range metadata.Dependencies {
			depPointers[i] = &metadata.Dependencies[i]
		}
		if err := h.validateDependencyMetadata(depPointers); err != nil {
			result.Valid = false
			result.Errors = append(result.Errors, domain.ValidationError{
				Type:     domain.ValidationErrorTypeDependency,
				Severity: domain.ValidationSeverityError,
				Message:  "Invalid dependency metadata",
				Details:  err.Error(),
			})
		}
	}

	// Check for best practices
	if warnings := h.checkMetadataBestPractices(metadata); len(warnings) > 0 {
		result.Warnings = append(result.Warnings, warnings...)
	}

	h.logger.DebugWithContext("chart metadata validation completed", map[string]interface{}{
		"name":     metadata.Name,
		"valid":    result.Valid,
		"errors":   len(result.Errors),
		"warnings": len(result.Warnings),
	})

	return result, nil
}

// UpdateChartMetadata updates chart metadata with provided changes
func (h *HelmMetadataManager) UpdateChartMetadata(ctx context.Context, chartPath string, updates *domain.MetadataUpdates) error {
	h.logger.InfoWithContext("updating chart metadata", map[string]interface{}{
		"chart_path": chartPath,
		"updates":    len(updates.Fields),
	})

	request := &domain.HelmMetadataUpdateRequest{
		ChartPath: chartPath,
		Fields:    updates.Fields,
	}

	err := h.helmPort.UpdateChartMetadata(ctx, request)
	if err != nil {
		h.logger.ErrorWithContext("failed to update chart metadata", map[string]interface{}{
			"chart_path": chartPath,
			"error":      err.Error(),
		})
		return fmt.Errorf("failed to update chart metadata: %w", err)
	}

	h.logger.InfoWithContext("chart metadata updated successfully", map[string]interface{}{
		"chart_path": chartPath,
	})

	return nil
}

// GetChartDependencies retrieves chart dependencies
func (h *HelmMetadataManager) GetChartDependencies(ctx context.Context, chartPath string) ([]*domain.ChartDependency, error) {
	h.logger.DebugWithContext("getting chart dependencies", map[string]interface{}{
		"chart_path": chartPath,
	})

	request := &domain.HelmDependencyRequest{
		ChartPath: chartPath,
	}

	dependencies, err := h.helmPort.GetChartDependencies(ctx, request)
	if err != nil {
		h.logger.ErrorWithContext("failed to get chart dependencies", map[string]interface{}{
			"chart_path": chartPath,
			"error":      err.Error(),
		})
		return nil, fmt.Errorf("failed to get chart dependencies: %w", err)
	}

	// Convert to ChartDependency format
	chartDeps := make([]*domain.ChartDependency, 0, len(dependencies))
	for _, dep := range dependencies {
		chartDep := &domain.ChartDependency{
			Name:         dep.Name,
			Version:      dep.Version,
			Repository:   dep.Repository,
			Condition:    dep.Condition,
			Tags:         dep.Tags,
			ImportValues: dep.ImportValues,
			Alias:        dep.Alias,
		}
		chartDeps = append(chartDeps, chartDep)
	}

	h.logger.DebugWithContext("chart dependencies retrieved successfully", map[string]interface{}{
		"chart_path":   chartPath,
		"dependencies": len(chartDeps),
	})

	return chartDeps, nil
}

// UpdateChartDependencies updates chart dependencies
func (h *HelmMetadataManager) UpdateChartDependencies(ctx context.Context, chartPath string, dependencies []*domain.ChartDependency) error {
	h.logger.InfoWithContext("updating chart dependencies", map[string]interface{}{
		"chart_path":   chartPath,
		"dependencies": len(dependencies),
	})

	// Convert ChartDependency to pointer slice
	deps := make([]*domain.ChartDependency, 0, len(dependencies))
	for _, dep := range dependencies {
		deps = append(deps, &domain.ChartDependency{
			Name:         dep.Name,
			Version:      dep.Version,
			Repository:   dep.Repository,
			Condition:    dep.Condition,
			Tags:         dep.Tags,
			ImportValues: dep.ImportValues,
			Alias:        dep.Alias,
		})
	}

	request := &domain.HelmDependencyUpdateRequest{
		ChartPath:    chartPath,
		Dependencies: deps,
	}

	err := h.helmPort.UpdateChartDependencies(ctx, request)
	if err != nil {
		h.logger.ErrorWithContext("failed to update chart dependencies", map[string]interface{}{
			"chart_path": chartPath,
			"error":      err.Error(),
		})
		return fmt.Errorf("failed to update chart dependencies: %w", err)
	}

	h.logger.InfoWithContext("chart dependencies updated successfully", map[string]interface{}{
		"chart_path": chartPath,
	})

	return nil
}

// GetChartValues retrieves chart values from values.yaml
func (h *HelmMetadataManager) GetChartValues(ctx context.Context, chartPath string) (map[string]interface{}, error) {
	h.logger.DebugWithContext("getting chart values", map[string]interface{}{
		"chart_path": chartPath,
	})

	request := &domain.HelmValuesRequest{
		ChartPath: chartPath,
	}

	values, err := h.helmPort.GetChartValues(ctx, request)
	if err != nil {
		h.logger.ErrorWithContext("failed to get chart values", map[string]interface{}{
			"chart_path": chartPath,
			"error":      err.Error(),
		})
		return nil, fmt.Errorf("failed to get chart values: %w", err)
	}

	h.logger.DebugWithContext("chart values retrieved successfully", map[string]interface{}{
		"chart_path":  chartPath,
		"values_keys": len(values),
	})

	return values, nil
}

// MergeValues merges base values with override values using Helm merge strategy
func (h *HelmMetadataManager) MergeValues(ctx context.Context, baseValues, overrideValues map[string]interface{}) (map[string]interface{}, error) {
	h.logger.DebugWithContext("merging Helm values", map[string]interface{}{
		"base_keys":     len(baseValues),
		"override_keys": len(overrideValues),
	})

	merged := h.deepMergeValues(baseValues, overrideValues)

	h.logger.DebugWithContext("Helm values merged successfully", map[string]interface{}{
		"merged_keys": len(merged),
	})

	return merged, nil
}

// GenerateChartSummary generates a comprehensive summary of the chart
func (h *HelmMetadataManager) GenerateChartSummary(ctx context.Context, chartPath string) (*domain.ChartSummary, error) {
	h.logger.InfoWithContext("generating chart summary", map[string]interface{}{
		"chart_path": chartPath,
	})

	summary := &domain.ChartSummary{
		Path:      chartPath,
		Generated: time.Now(),
	}

	// Get metadata
	metadata, err := h.GetChartMetadata(ctx, chartPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get metadata for summary: %w", err)
	}
	summary.Metadata = metadata

	// Get dependencies
	dependencies, err := h.GetChartDependencies(ctx, chartPath)
	if err != nil {
		h.logger.WarnWithContext("failed to get dependencies for summary", map[string]interface{}{
			"chart_path": chartPath,
			"error":      err.Error(),
		})
		// Continue without dependencies
		dependencies = make([]*domain.ChartDependency, 0)
	}
	summary.Dependencies = dependencies

	// Get values
	values, err := h.GetChartValues(ctx, chartPath)
	if err != nil {
		h.logger.WarnWithContext("failed to get values for summary", map[string]interface{}{
			"chart_path": chartPath,
			"error":      err.Error(),
		})
		// Continue without values
		values = make(map[string]interface{})
	}
	summary.Values = values

	// Analyze chart structure
	summary.TemplateCount = h.countTemplates(chartPath)
	summary.ResourceTypes = h.analyzeResourceTypes(chartPath)
	summary.ComplexityScore = int(h.calculateComplexityScore(summary))

	// Generate recommendations
	summary.Recommendations = h.generateRecommendations(summary)

	h.logger.InfoWithContext("chart summary generated successfully", map[string]interface{}{
		"chart_path":      chartPath,
		"name":            summary.Metadata.Name,
		"template_count":  summary.TemplateCount,
		"complexity":      summary.ComplexityScore,
		"recommendations": len(summary.Recommendations),
	})

	return summary, nil
}

// Helper methods

// enhanceMetadata adds additional information to chart metadata
func (h *HelmMetadataManager) enhanceMetadata(metadata *domain.ChartMetadata, chartPath string) {
	metadata.Path = chartPath
	metadata.Directory = filepath.Base(chartPath)
	
	// Set defaults if missing
	if metadata.Type == "" {
		metadata.Type = "application"
	}
	if metadata.APIVersion == "" {
		metadata.APIVersion = "v2"
	}
}

// validateRequiredFields validates that all required metadata fields are present
func (h *HelmMetadataManager) validateRequiredFields(metadata *domain.ChartMetadata) error {
	if metadata.Name == "" {
		return fmt.Errorf("chart name is required")
	}
	if metadata.Version == "" {
		return fmt.Errorf("chart version is required")
	}
	if metadata.APIVersion == "" {
		return fmt.Errorf("chart API version is required")
	}
	return nil
}

// validateVersionFormat validates that the version follows semantic versioning
func (h *HelmMetadataManager) validateVersionFormat(version string) error {
	if version == "" {
		return fmt.Errorf("version cannot be empty")
	}

	// Basic semantic version validation (simplified)
	parts := strings.Split(version, ".")
	if len(parts) < 2 || len(parts) > 4 {
		return fmt.Errorf("version must follow semantic versioning format (e.g., 1.0.0)")
	}

	// Check for pre-release identifiers
	if strings.Contains(version, "+") && !strings.Contains(version, "-") {
		return fmt.Errorf("build metadata requires pre-release identifier")
	}

	return nil
}

// validateChartName validates chart name and returns warnings
func (h *HelmMetadataManager) validateChartName(name string) []domain.ValidationWarning {
	warnings := make([]domain.ValidationWarning, 0)

	if len(name) > 63 {
		warnings = append(warnings, domain.ValidationWarning{
			Type:     domain.ValidationWarningTypeNaming,
			Severity: domain.ValidationSeverityWarning,
			Message:  "Chart name exceeds recommended maximum length of 63 characters",
		})
	}

	if strings.Contains(name, "_") {
		warnings = append(warnings, domain.ValidationWarning{
			Type:     domain.ValidationWarningTypeNaming,
			Severity: domain.ValidationSeverityWarning,
			Message:  "Chart name should use hyphens instead of underscores",
		})
	}

	if strings.ToUpper(name) == name {
		warnings = append(warnings, domain.ValidationWarning{
			Type:     domain.ValidationWarningTypeNaming,
			Severity: domain.ValidationSeverityInfo,
			Message:  "Chart name should be lowercase",
		})
	}

	return warnings
}

// validateDependencyMetadata validates dependency metadata
func (h *HelmMetadataManager) validateDependencyMetadata(dependencies []*domain.ChartDependency) error {
	for _, dep := range dependencies {
		if dep.Name == "" {
			return fmt.Errorf("dependency name is required")
		}
		if dep.Version == "" {
			return fmt.Errorf("dependency version is required for %s", dep.Name)
		}
		if dep.Repository == "" && dep.Name != "" {
			return fmt.Errorf("dependency repository is required for %s", dep.Name)
		}
	}
	return nil
}

// checkMetadataBestPractices checks metadata against best practices
func (h *HelmMetadataManager) checkMetadataBestPractices(metadata *domain.ChartMetadata) []domain.ValidationWarning {
	warnings := make([]domain.ValidationWarning, 0)

	if metadata.Description == "" {
		warnings = append(warnings, domain.ValidationWarning{
			Type:     domain.ValidationWarningTypeBestPractices,
			Severity: domain.ValidationSeverityWarning,
			Message:  "Chart description is recommended for better documentation",
		})
	}

	if len(metadata.Keywords) == 0 {
		warnings = append(warnings, domain.ValidationWarning{
			Type:     domain.ValidationWarningTypeBestPractices,
			Severity: domain.ValidationSeverityInfo,
			Message:  "Chart keywords help with discoverability",
		})
	}

	if metadata.Home == "" && metadata.Sources == nil {
		warnings = append(warnings, domain.ValidationWarning{
			Type:     domain.ValidationWarningTypeBestPractices,
			Severity: domain.ValidationSeverityInfo,
			Message:  "Consider adding home URL or source URLs for reference",
		})
	}

	if len(metadata.Maintainers) == 0 {
		warnings = append(warnings, domain.ValidationWarning{
			Type:     domain.ValidationWarningTypeBestPractices,
			Severity: domain.ValidationSeverityWarning,
			Message:  "Chart maintainers information is recommended",
		})
	}

	return warnings
}

// deepMergeValues performs deep merge of values maps
func (h *HelmMetadataManager) deepMergeValues(base, override map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	// Copy base values
	for key, value := range base {
		result[key] = value
	}

	// Merge override values
	for key, overrideValue := range override {
		if baseValue, exists := result[key]; exists {
			// If both are maps, recursively merge
			if baseMap, baseIsMap := baseValue.(map[string]interface{}); baseIsMap {
				if overrideMap, overrideIsMap := overrideValue.(map[string]interface{}); overrideIsMap {
					result[key] = h.deepMergeValues(baseMap, overrideMap)
					continue
				}
			}
		}
		// Override or add new value
		result[key] = overrideValue
	}

	return result
}

// countTemplates counts the number of template files in the chart
func (h *HelmMetadataManager) countTemplates(chartPath string) int {
	// This would typically scan the templates directory
	// For now, return a placeholder count
	return 5 // Placeholder implementation
}

// analyzeResourceTypes analyzes the types of Kubernetes resources in the chart
func (h *HelmMetadataManager) analyzeResourceTypes(chartPath string) []string {
	// This would typically parse template files and identify resource kinds
	// For now, return common resource types
	return []string{"Deployment", "Service", "ConfigMap", "Secret"} // Placeholder
}

// calculateComplexityScore calculates a complexity score for the chart
func (h *HelmMetadataManager) calculateComplexityScore(summary *domain.ChartSummary) float64 {
	score := 0.0

	// Base score from template count
	score += float64(summary.TemplateCount) * 1.5

	// Add score for dependencies
	score += float64(len(summary.Dependencies)) * 2.0

	// Add score for values complexity
	score += h.calculateValuesComplexity(summary.Values) * 0.5

	// Add score for resource variety
	score += float64(len(summary.ResourceTypes)) * 1.0

	return score
}

// calculateValuesComplexity calculates complexity based on values structure
func (h *HelmMetadataManager) calculateValuesComplexity(values map[string]interface{}) float64 {
	return float64(h.countNestedKeys(values))
}

// countNestedKeys counts nested keys in a values map
func (h *HelmMetadataManager) countNestedKeys(values map[string]interface{}) int {
	count := 0
	for _, value := range values {
		count++
		if nestedMap, ok := value.(map[string]interface{}); ok {
			count += h.countNestedKeys(nestedMap)
		}
	}
	return count
}

// generateRecommendations generates recommendations based on chart analysis
func (h *HelmMetadataManager) generateRecommendations(summary *domain.ChartSummary) []string {
	recommendations := make([]string, 0)

	if summary.ComplexityScore > 20 {
		recommendations = append(recommendations, 
			"Consider breaking down this complex chart into smaller, more focused charts")
	}

	if len(summary.Dependencies) > 5 {
		recommendations = append(recommendations, 
			"High number of dependencies may increase maintenance overhead")
	}

	if summary.TemplateCount > 10 {
		recommendations = append(recommendations, 
			"Consider using library charts or helpers to reduce template duplication")
	}

	if summary.Metadata.Description == "" {
		recommendations = append(recommendations, 
			"Add a comprehensive description to improve chart documentation")
	}

	return recommendations
}