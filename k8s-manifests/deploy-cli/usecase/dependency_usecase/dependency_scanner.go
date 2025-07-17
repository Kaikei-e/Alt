package dependency_usecase

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"deploy-cli/port/filesystem_port"
	"deploy-cli/port/logger_port"
)

// DependencyType represents the type of dependency
type DependencyType string

const (
	// HelmDependency represents a Helm chart dependency
	HelmDependency DependencyType = "helm"
	// ServiceDependency represents a service-to-service dependency
	ServiceDependency DependencyType = "service"
	// DatabaseDependency represents a database dependency
	DatabaseDependency DependencyType = "database"
	// ConfigDependency represents a configuration dependency
	ConfigDependency DependencyType = "config"
	// SecretDependency represents a secret dependency
	SecretDependency DependencyType = "secret"
)

// ChartDependency represents a dependency between charts
type ChartDependency struct {
	FromChart    string         `json:"from_chart"`
	ToChart      string         `json:"to_chart"`
	Type         DependencyType `json:"type"`
	Required     bool           `json:"required"`
	Description  string         `json:"description"`
	DetectedFrom string         `json:"detected_from"`
}

// DependencyGraph represents the complete dependency graph
type DependencyGraph struct {
	Charts       []string                    `json:"charts"`
	Dependencies []ChartDependency           `json:"dependencies"`
	Groups       map[string][]string         `json:"groups"`
	DeployOrder  [][]string                  `json:"deploy_order"`
	Metadata     DependencyGraphMetadata     `json:"metadata"`
}

// DependencyGraphMetadata contains metadata about the dependency graph
type DependencyGraphMetadata struct {
	GeneratedAt       time.Time `json:"generated_at"`
	TotalCharts       int       `json:"total_charts"`
	TotalDependencies int       `json:"total_dependencies"`
	MaxDepth          int       `json:"max_depth"`
	HasCycles         bool      `json:"has_cycles"`
	Cycles            [][]string `json:"cycles,omitempty"`
}

// DependencyScanner analyzes chart dependencies
type DependencyScanner struct {
	filesystem filesystem_port.FileSystemPort
	logger     logger_port.LoggerPort
	patterns   *DependencyPatterns
}

// DependencyPatterns holds regex patterns for dependency detection
type DependencyPatterns struct {
	ServiceReferences   []*regexp.Regexp
	DatabaseReferences  []*regexp.Regexp
	SecretReferences    []*regexp.Regexp
	ConfigReferences    []*regexp.Regexp
	HelmDependencies    []*regexp.Regexp
}

// NewDependencyScanner creates a new dependency scanner
func NewDependencyScanner(filesystem filesystem_port.FileSystemPort, logger logger_port.LoggerPort) *DependencyScanner {
	return &DependencyScanner{
		filesystem: filesystem,
		logger:     logger,
		patterns:   createDependencyPatterns(),
	}
}

// createDependencyPatterns creates regex patterns for dependency detection
func createDependencyPatterns() *DependencyPatterns {
	return &DependencyPatterns{
		ServiceReferences: []*regexp.Regexp{
			// Service DNS references: service.namespace.svc.cluster.local
			regexp.MustCompile(`([a-z0-9-]+)\.([a-z0-9-]+)\.svc\.cluster\.local`),
			// Simple service references
			regexp.MustCompile(`([a-z0-9-]+)-service`),
			// Database service patterns
			regexp.MustCompile(`(postgres|clickhouse|meilisearch)\.([a-z0-9-]+)`),
		},
		DatabaseReferences: []*regexp.Regexp{
			// Database connection strings
			regexp.MustCompile(`postgresql://.*@([a-z0-9-]+)`),
			// Database host references
			regexp.MustCompile(`DB_HOST.*[:=]\s*([a-z0-9.-]+)`),
			// ClickHouse references
			regexp.MustCompile(`CLICKHOUSE_HOST.*[:=]\s*([a-z0-9.-]+)`),
		},
		SecretReferences: []*regexp.Regexp{
			// Secret references in values
			regexp.MustCompile(`secretName:\s*([a-z0-9-]+)`),
			// Secret key references
			regexp.MustCompile(`secretKeyRef:\s*name:\s*([a-z0-9-]+)`),
			// Environment from secret
			regexp.MustCompile(`envFromSecret:\s*name:\s*([a-z0-9-]+)`),
		},
		ConfigReferences: []*regexp.Regexp{
			// ConfigMap references
			regexp.MustCompile(`configMapRef:\s*name:\s*([a-z0-9-]+)`),
			// ConfigMap volume references
			regexp.MustCompile(`configMap:\s*name:\s*([a-z0-9-]+)`),
		},
		HelmDependencies: []*regexp.Regexp{
			// Helm Chart.yaml dependencies
			regexp.MustCompile(`name:\s*([a-z0-9-]+)`),
		},
	}
}

// ScanDependencies scans all charts for dependencies
func (s *DependencyScanner) ScanDependencies(ctx context.Context, chartsDir string) (*DependencyGraph, error) {
	s.logger.InfoWithContext("starting dependency scan", map[string]interface{}{
		"charts_dir": chartsDir,
	})

	// Discover all charts
	charts, err := s.discoverCharts(chartsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to discover charts: %w", err)
	}

	s.logger.InfoWithContext("discovered charts", map[string]interface{}{
		"count": len(charts),
		"charts": charts,
	})

	// Scan each chart for dependencies
	var allDependencies []ChartDependency
	for _, chart := range charts {
		deps, err := s.scanChartDependencies(ctx, chartsDir, chart)
		if err != nil {
			s.logger.WarnWithContext("failed to scan chart dependencies", map[string]interface{}{
				"chart": chart,
				"error": err.Error(),
			})
			continue
		}
		allDependencies = append(allDependencies, deps...)
	}

	// Build dependency graph
	graph := &DependencyGraph{
		Charts:       charts,
		Dependencies: allDependencies,
		Metadata: DependencyGraphMetadata{
			GeneratedAt:       time.Now(),
			TotalCharts:       len(charts),
			TotalDependencies: len(allDependencies),
		},
	}

	// Analyze graph structure
	if err := s.analyzeGraph(graph); err != nil {
		s.logger.WarnWithContext("failed to analyze dependency graph", map[string]interface{}{
			"error": err.Error(),
		})
	}

	s.logger.InfoWithContext("dependency scan completed", map[string]interface{}{
		"total_charts":       graph.Metadata.TotalCharts,
		"total_dependencies": graph.Metadata.TotalDependencies,
		"has_cycles":         graph.Metadata.HasCycles,
		"max_depth":          graph.Metadata.MaxDepth,
	})

	return graph, nil
}

// discoverCharts discovers all charts in the charts directory
func (s *DependencyScanner) discoverCharts(chartsDir string) ([]string, error) {
	var charts []string

	entries, err := s.filesystem.ListDirectory(chartsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to list charts directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			// Check if this directory contains a Chart.yaml
			chartYamlPath := filepath.Join(chartsDir, entry.Name(), "Chart.yaml")
			if s.filesystem.FileExists(chartYamlPath) {
				charts = append(charts, entry.Name())
			}
		}
	}

	return charts, nil
}

// scanChartDependencies scans a single chart for dependencies
func (s *DependencyScanner) scanChartDependencies(ctx context.Context, chartsDir, chartName string) ([]ChartDependency, error) {
	var dependencies []ChartDependency

	chartPath := filepath.Join(chartsDir, chartName)

	// Scan Chart.yaml for Helm dependencies
	helmDeps, err := s.scanHelmDependencies(chartPath, chartName)
	if err != nil {
		s.logger.WarnWithContext("failed to scan helm dependencies", map[string]interface{}{
			"chart": chartName,
			"error": err.Error(),
		})
	} else {
		dependencies = append(dependencies, helmDeps...)
	}

	// Scan values files for service dependencies
	valueDeps, err := s.scanValuesDependencies(chartPath, chartName)
	if err != nil {
		s.logger.WarnWithContext("failed to scan values dependencies", map[string]interface{}{
			"chart": chartName,
			"error": err.Error(),
		})
	} else {
		dependencies = append(dependencies, valueDeps...)
	}

	// Scan templates for references
	templateDeps, err := s.scanTemplateDependencies(chartPath, chartName)
	if err != nil {
		s.logger.WarnWithContext("failed to scan template dependencies", map[string]interface{}{
			"chart": chartName,
			"error": err.Error(),
		})
	} else {
		dependencies = append(dependencies, templateDeps...)
	}

	return dependencies, nil
}

// scanHelmDependencies scans Chart.yaml for Helm dependencies
func (s *DependencyScanner) scanHelmDependencies(chartPath, chartName string) ([]ChartDependency, error) {
	var dependencies []ChartDependency

	chartYamlPath := filepath.Join(chartPath, "Chart.yaml")
	content, err := s.filesystem.ReadFile(chartYamlPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read Chart.yaml: %w", err)
	}

	// Look for dependencies section
	lines := strings.Split(string(content), "\n")
	inDependencies := false

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if line == "dependencies:" {
			inDependencies = true
			continue
		}

		if inDependencies {
			// Stop if we reach another top-level section
			if strings.HasSuffix(line, ":") && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "-") {
				break
			}

			// Look for dependency names
			if strings.Contains(line, "name:") {
				parts := strings.Split(line, ":")
				if len(parts) >= 2 {
					depName := strings.TrimSpace(parts[1])
					depName = strings.Trim(depName, "\"'")

					if depName != "" {
						dependencies = append(dependencies, ChartDependency{
							FromChart:    chartName,
							ToChart:      depName,
							Type:         HelmDependency,
							Required:     true,
							Description:  "Helm chart dependency",
							DetectedFrom: "Chart.yaml",
						})
					}
				}
			}
		}
	}

	return dependencies, nil
}

// scanValuesDependencies scans values files for dependencies
func (s *DependencyScanner) scanValuesDependencies(chartPath, chartName string) ([]ChartDependency, error) {
	var dependencies []ChartDependency

	// Scan all values*.yaml files
	valuesFiles := []string{"values.yaml", "values-production.yaml", "values-staging.yaml", "values-development.yaml"}

	for _, valuesFile := range valuesFiles {
		valuesPath := filepath.Join(chartPath, valuesFile)
		if !s.filesystem.FileExists(valuesPath) {
			continue
		}

		content, err := s.filesystem.ReadFile(valuesPath)
		if err != nil {
			s.logger.WarnWithContext("failed to read values file", map[string]interface{}{
				"file":  valuesPath,
				"error": err.Error(),
			})
			continue
		}

		deps := s.extractDependenciesFromContent(string(content), chartName, valuesFile)
		dependencies = append(dependencies, deps...)
	}

	return dependencies, nil
}

// scanTemplateDependencies scans template files for dependencies
func (s *DependencyScanner) scanTemplateDependencies(chartPath, chartName string) ([]ChartDependency, error) {
	var dependencies []ChartDependency

	templatesPath := filepath.Join(chartPath, "templates")
	if !s.filesystem.DirectoryExists(templatesPath) {
		return dependencies, nil
	}

	entries, err := s.filesystem.ListDirectory(templatesPath)
	if err != nil {
		return nil, fmt.Errorf("failed to list templates directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() && (strings.HasSuffix(entry.Name(), ".yaml") || strings.HasSuffix(entry.Name(), ".yml")) {
			templatePath := filepath.Join(templatesPath, entry.Name())
			content, err := s.filesystem.ReadFile(templatePath)
			if err != nil {
				s.logger.WarnWithContext("failed to read template file", map[string]interface{}{
					"file":  templatePath,
					"error": err.Error(),
				})
				continue
			}

			deps := s.extractDependenciesFromContent(string(content), chartName, fmt.Sprintf("templates/%s", entry.Name()))
			dependencies = append(dependencies, deps...)
		}
	}

	return dependencies, nil
}

// extractDependenciesFromContent extracts dependencies from file content
func (s *DependencyScanner) extractDependenciesFromContent(content, chartName, fileName string) []ChartDependency {
	var dependencies []ChartDependency

	// Service dependencies
	for _, pattern := range s.patterns.ServiceReferences {
		matches := pattern.FindAllStringSubmatch(content, -1)
		for _, match := range matches {
			if len(match) >= 2 {
				serviceName := match[1]
				if serviceName != chartName && s.isValidChartName(serviceName) {
					dependencies = append(dependencies, ChartDependency{
						FromChart:    chartName,
						ToChart:      serviceName,
						Type:         ServiceDependency,
						Required:     true,
						Description:  fmt.Sprintf("Service reference to %s", serviceName),
						DetectedFrom: fileName,
					})
				}
			}
		}
	}

	// Database dependencies
	for _, pattern := range s.patterns.DatabaseReferences {
		matches := pattern.FindAllStringSubmatch(content, -1)
		for _, match := range matches {
			if len(match) >= 2 {
				dbHost := match[1]
				// Extract chart name from database host
				if strings.Contains(dbHost, ".") {
					parts := strings.Split(dbHost, ".")
					if len(parts) > 0 {
						dbChart := parts[0]
						if dbChart != chartName && s.isValidChartName(dbChart) {
							dependencies = append(dependencies, ChartDependency{
								FromChart:    chartName,
								ToChart:      dbChart,
								Type:         DatabaseDependency,
								Required:     true,
								Description:  fmt.Sprintf("Database dependency on %s", dbChart),
								DetectedFrom: fileName,
							})
						}
					}
				}
			}
		}
	}

	// Secret dependencies
	for _, pattern := range s.patterns.SecretReferences {
		matches := pattern.FindAllStringSubmatch(content, -1)
		for _, match := range matches {
			if len(match) >= 2 {
				secretName := match[1]
				// Try to map secret to chart
				if relatedChart := s.mapSecretToChart(secretName); relatedChart != "" && relatedChart != chartName {
					dependencies = append(dependencies, ChartDependency{
						FromChart:    chartName,
						ToChart:      relatedChart,
						Type:         SecretDependency,
						Required:     false,
						Description:  fmt.Sprintf("Secret dependency on %s", secretName),
						DetectedFrom: fileName,
					})
				}
			}
		}
	}

	return dependencies
}

// isValidChartName checks if a name could be a valid chart name
func (s *DependencyScanner) isValidChartName(name string) bool {
	// Simple validation - chart names are typically lowercase with hyphens
	validName := regexp.MustCompile(`^[a-z0-9-]+$`)
	return validName.MatchString(name) && len(name) > 2
}

// mapSecretToChart maps a secret name to a chart name
func (s *DependencyScanner) mapSecretToChart(secretName string) string {
	// Common patterns for mapping secrets to charts
	secretToChart := map[string]string{
		"postgres-secrets":       "postgres",
		"auth-postgres-secrets":  "auth-postgres",
		"kratos-postgres-secrets": "kratos-postgres",
		"clickhouse-secrets":     "clickhouse",
		"meilisearch-secrets":    "meilisearch",
		"backend-secrets":        "alt-backend",
		"auth-service-secrets":   "auth-service",
		"frontend-secrets":       "alt-frontend",
		"nginx-secrets":          "nginx",
	}

	if chart, exists := secretToChart[secretName]; exists {
		return chart
	}

	// Try to infer from secret name pattern
	if strings.HasSuffix(secretName, "-secrets") {
		return strings.TrimSuffix(secretName, "-secrets")
	}

	return ""
}

// analyzeGraph analyzes the dependency graph structure
func (s *DependencyScanner) analyzeGraph(graph *DependencyGraph) error {
	// Group charts by type
	graph.Groups = s.groupChartsByType(graph.Charts)

	// Calculate deployment order
	order, err := s.calculateDeploymentOrder(graph)
	if err != nil {
		return fmt.Errorf("failed to calculate deployment order: %w", err)
	}
	graph.DeployOrder = order

	// Detect cycles
	cycles := s.detectCycles(graph)
	graph.Metadata.HasCycles = len(cycles) > 0
	graph.Metadata.Cycles = cycles

	// Calculate max depth
	graph.Metadata.MaxDepth = len(graph.DeployOrder)

	return nil
}

// groupChartsByType groups charts by their inferred type
func (s *DependencyScanner) groupChartsByType(charts []string) map[string][]string {
	groups := map[string][]string{
		"infrastructure": {},
		"application":    {},
		"operational":    {},
	}

	for _, chart := range charts {
		group := s.inferChartType(chart)
		groups[group] = append(groups[group], chart)
	}

	return groups
}

// inferChartType infers the type of a chart from its name
func (s *DependencyScanner) inferChartType(chartName string) string {
	infrastructureCharts := []string{"postgres", "auth-postgres", "kratos-postgres", "clickhouse", "meilisearch", "nginx", "nginx-external", "common-ssl", "common-secrets", "common-config", "monitoring"}
	operationalCharts := []string{"migrate", "backup"}

	for _, infra := range infrastructureCharts {
		if chartName == infra {
			return "infrastructure"
		}
	}

	for _, ops := range operationalCharts {
		if chartName == ops {
			return "operational"
		}
	}

	return "application"
}

// calculateDeploymentOrder calculates the deployment order based on dependencies
func (s *DependencyScanner) calculateDeploymentOrder(graph *DependencyGraph) ([][]string, error) {
	// Simple topological sort
	inDegree := make(map[string]int)
	adjList := make(map[string][]string)

	// Initialize
	for _, chart := range graph.Charts {
		inDegree[chart] = 0
		adjList[chart] = []string{}
	}

	// Build adjacency list and in-degree count
	for _, dep := range graph.Dependencies {
		if dep.Required {
			adjList[dep.ToChart] = append(adjList[dep.ToChart], dep.FromChart)
			inDegree[dep.FromChart]++
		}
	}

	var order [][]string
	remaining := make(map[string]bool)
	for _, chart := range graph.Charts {
		remaining[chart] = true
	}

	for len(remaining) > 0 {
		var currentLevel []string

		// Find charts with no dependencies
		for chart := range remaining {
			if inDegree[chart] == 0 {
				currentLevel = append(currentLevel, chart)
			}
		}

		if len(currentLevel) == 0 {
			// Cycle detected or error
			var remainingCharts []string
			for chart := range remaining {
				remainingCharts = append(remainingCharts, chart)
			}
			return order, fmt.Errorf("circular dependency detected among charts: %v", remainingCharts)
		}

		order = append(order, currentLevel)

		// Remove current level charts and update in-degrees
		for _, chart := range currentLevel {
			delete(remaining, chart)
			for _, dependent := range adjList[chart] {
				if remaining[dependent] {
					inDegree[dependent]--
				}
			}
		}
	}

	return order, nil
}

// detectCycles detects cycles in the dependency graph
func (s *DependencyScanner) detectCycles(graph *DependencyGraph) [][]string {
	var cycles [][]string
	visited := make(map[string]bool)
	recStack := make(map[string]bool)
	adjList := make(map[string][]string)

	// Build adjacency list
	for _, chart := range graph.Charts {
		adjList[chart] = []string{}
	}

	for _, dep := range graph.Dependencies {
		if dep.Required {
			adjList[dep.FromChart] = append(adjList[dep.FromChart], dep.ToChart)
		}
	}

	// DFS to detect cycles
	var dfs func(string, []string) bool
	dfs = func(node string, path []string) bool {
		visited[node] = true
		recStack[node] = true
		path = append(path, node)

		for _, neighbor := range adjList[node] {
			if !visited[neighbor] {
				if dfs(neighbor, path) {
					return true
				}
			} else if recStack[neighbor] {
				// Found cycle
				cycleStart := -1
				for i, n := range path {
					if n == neighbor {
						cycleStart = i
						break
					}
				}
				if cycleStart >= 0 {
					cycle := append(path[cycleStart:], neighbor)
					cycles = append(cycles, cycle)
				}
				return true
			}
		}

		recStack[node] = false
		return false
	}

	for _, chart := range graph.Charts {
		if !visited[chart] {
			dfs(chart, []string{})
		}
	}

	return cycles
}