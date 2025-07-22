// PHASE R1: Dependency resolution for deployment orchestration
package orchestration

import (
	"fmt"
	"sort"

	"deploy-cli/domain"
	"deploy-cli/port/logger_port"
)

// DependencyResolver resolves chart dependencies for deployment ordering
type DependencyResolver struct {
	logger logger_port.LoggerPort
}

// DependencyResolverPort defines the interface for dependency resolution
type DependencyResolverPort interface {
	ResolveDependencies(charts []domain.Chart) ([]domain.Chart, error)
	BuildDependencyGraph(charts []domain.Chart) (*DependencyGraph, error)
	ValidateDependencies(charts []domain.Chart) error
	GetDeploymentOrder(charts []domain.Chart) ([][]domain.Chart, error) // Returns layers of charts that can be deployed in parallel
}

// DependencyGraph represents the dependency relationships between charts
type DependencyGraph struct {
	Nodes map[string]*DependencyNode
	Edges map[string][]string // chart name -> dependent chart names
}

// DependencyNode represents a chart in the dependency graph
type DependencyNode struct {
	Chart        domain.Chart
	Dependencies []string // chart names this chart depends on
	Dependents   []string // chart names that depend on this chart
}

// NewDependencyResolver creates a new dependency resolver
func NewDependencyResolver(logger logger_port.LoggerPort) *DependencyResolver {
	return &DependencyResolver{
		logger: logger,
	}
}

// ResolveDependencies resolves dependencies and returns charts in deployment order
func (d *DependencyResolver) ResolveDependencies(charts []domain.Chart) ([]domain.Chart, error) {
	d.logger.InfoWithContext("resolving chart dependencies", map[string]interface{}{
		"chart_count": len(charts),
	})

	// Build dependency graph
	graph, err := d.BuildDependencyGraph(charts)
	if err != nil {
		return nil, fmt.Errorf("failed to build dependency graph: %w", err)
	}

	// Perform topological sort
	orderedCharts, err := d.topologicalSort(graph, charts)
	if err != nil {
		return nil, fmt.Errorf("dependency resolution failed: %w", err)
	}

	d.logger.InfoWithContext("dependency resolution completed", map[string]interface{}{
		"chart_count":      len(charts),
		"ordered_count":    len(orderedCharts),
		"dependency_edges": len(graph.Edges),
	})

	return orderedCharts, nil
}

// BuildDependencyGraph builds a dependency graph from charts
func (d *DependencyResolver) BuildDependencyGraph(charts []domain.Chart) (*DependencyGraph, error) {
	d.logger.DebugWithContext("building dependency graph", map[string]interface{}{
		"chart_count": len(charts),
	})

	graph := &DependencyGraph{
		Nodes: make(map[string]*DependencyNode),
		Edges: make(map[string][]string),
	}

	// Create nodes for all charts
	chartMap := make(map[string]domain.Chart)
	for _, chart := range charts {
		chartMap[chart.Name] = chart
		graph.Nodes[chart.Name] = &DependencyNode{
			Chart:        chart,
			Dependencies: make([]string, 0),
			Dependents:   make([]string, 0),
		}
	}

	// Build edges based on chart type and implicit dependencies
	for _, chart := range charts {
		dependencies := d.inferDependencies(chart, chartMap)
		
		for _, depName := range dependencies {
			if _, exists := chartMap[depName]; exists {
				// Add dependency edge
				graph.Nodes[chart.Name].Dependencies = append(graph.Nodes[chart.Name].Dependencies, depName)
				graph.Nodes[depName].Dependents = append(graph.Nodes[depName].Dependents, chart.Name)
				
				// Add to edges map
				if _, exists := graph.Edges[depName]; !exists {
					graph.Edges[depName] = make([]string, 0)
				}
				graph.Edges[depName] = append(graph.Edges[depName], chart.Name)
			}
		}

		d.logger.DebugWithContext("chart dependencies identified", map[string]interface{}{
			"chart":            chart.Name,
			"dependency_count": len(dependencies),
			"dependencies":     dependencies,
		})
	}

	return graph, nil
}

// inferDependencies infers dependencies based on chart type and naming patterns
func (d *DependencyResolver) inferDependencies(chart domain.Chart, chartMap map[string]domain.Chart) []string {
	dependencies := make([]string, 0)

	switch chart.Type {
	case domain.ApplicationChart:
		// Application charts depend on infrastructure and config
		dependencies = append(dependencies, d.findChartsOfType(chartMap, domain.InfrastructureChart)...)
		dependencies = append(dependencies, d.findChartsOfType(chartMap, domain.ConfigChart)...)
		
		// Specific application dependencies
		if chart.Name == "alt-backend" {
			dependencies = append(dependencies, "postgres", "common-secrets", "meilisearch")
		}
		if chart.Name == "pre-processor" {
			dependencies = append(dependencies, "postgres", "common-secrets")
		}
		if chart.Name == "search-indexer" {
			dependencies = append(dependencies, "meilisearch", "common-secrets")
		}
		if chart.Name == "tag-generator" {
			dependencies = append(dependencies, "postgres", "common-secrets")
		}

	case domain.FrontendChart:
		// Frontend depends on backend services
		dependencies = append(dependencies, d.findChartsOfType(chartMap, domain.ApplicationChart)...)
		dependencies = append(dependencies, "common-config")

	case domain.ServiceChart:
		// Service charts depend on infrastructure
		dependencies = append(dependencies, d.findChartsOfType(chartMap, domain.InfrastructureChart)...)
		if chart.Name == "meilisearch" {
			dependencies = append(dependencies, "common-secrets")
		}

	case domain.SecurityChart:
		// Security charts depend on their databases
		if chart.Name == "auth-service" {
			dependencies = append(dependencies, "auth-postgres", "kratos", "common-secrets")
		}
		if chart.Name == "kratos" {
			dependencies = append(dependencies, "kratos-postgres", "common-secrets")
		}

	case domain.IngressChart:
		// Ingress depends on all application services being ready
		dependencies = append(dependencies, d.findChartsOfType(chartMap, domain.ApplicationChart)...)
		dependencies = append(dependencies, d.findChartsOfType(chartMap, domain.FrontendChart)...)

	case domain.MonitoringChart:
		// Monitoring can be deployed independently but after basic infrastructure
		dependencies = append(dependencies, d.findChartsOfType(chartMap, domain.ConfigChart)...)

	case domain.InfrastructureChart:
		// Infrastructure charts have minimal dependencies
		if chart.Name == "clickhouse" {
			dependencies = append(dependencies, "common-secrets")
		}

	case domain.ConfigChart:
		// Config charts are usually deployed early with minimal dependencies
		// No additional dependencies
	}

	// Remove duplicates and the chart itself
	dependencies = d.removeDuplicates(dependencies, chart.Name)

	return dependencies
}

// findChartsOfType finds all charts of a specific type
func (d *DependencyResolver) findChartsOfType(chartMap map[string]domain.Chart, chartType domain.ChartType) []string {
	charts := make([]string, 0)
	for name, chart := range chartMap {
		if chart.Type == chartType {
			charts = append(charts, name)
		}
	}
	return charts
}

// removeDuplicates removes duplicate entries and excludes the chart itself
func (d *DependencyResolver) removeDuplicates(slice []string, exclude string) []string {
	keys := make(map[string]bool)
	result := make([]string, 0)

	for _, item := range slice {
		if item != exclude && !keys[item] {
			keys[item] = true
			result = append(result, item)
		}
	}

	return result
}

// topologicalSort performs topological sort on the dependency graph
func (d *DependencyResolver) topologicalSort(graph *DependencyGraph, charts []domain.Chart) ([]domain.Chart, error) {
	// Create a map for quick chart lookup
	chartMap := make(map[string]domain.Chart)
	for _, chart := range charts {
		chartMap[chart.Name] = chart
	}

	// Calculate in-degrees
	inDegree := make(map[string]int)
	for chartName := range graph.Nodes {
		inDegree[chartName] = len(graph.Nodes[chartName].Dependencies)
	}

	// Initialize queue with nodes that have no dependencies
	queue := make([]string, 0)
	for chartName, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, chartName)
		}
	}

	// Process nodes
	result := make([]domain.Chart, 0, len(charts))
	processed := make(map[string]bool)

	for len(queue) > 0 {
		// Sort queue for deterministic results
		sort.Strings(queue)
		
		// Process next chart
		currentChart := queue[0]
		queue = queue[1:]
		
		if chart, exists := chartMap[currentChart]; exists {
			result = append(result, chart)
			processed[currentChart] = true

			d.logger.DebugWithContext("chart processed in topological sort", map[string]interface{}{
				"chart":     currentChart,
				"position":  len(result),
				"remaining": len(charts) - len(result),
			})

			// Update in-degrees of dependent charts
			for _, dependent := range graph.Nodes[currentChart].Dependents {
				if !processed[dependent] {
					inDegree[dependent]--
					if inDegree[dependent] == 0 {
						queue = append(queue, dependent)
					}
				}
			}
		}
	}

	// Check for circular dependencies
	if len(result) != len(charts) {
		remaining := make([]string, 0)
		for _, chart := range charts {
			if !processed[chart.Name] {
				remaining = append(remaining, chart.Name)
			}
		}
		return nil, fmt.Errorf("circular dependencies detected among charts: %v", remaining)
	}

	return result, nil
}

// ValidateDependencies validates that all dependencies can be resolved
func (d *DependencyResolver) ValidateDependencies(charts []domain.Chart) error {
	d.logger.DebugWithContext("validating chart dependencies", map[string]interface{}{
		"chart_count": len(charts),
	})

	graph, err := d.BuildDependencyGraph(charts)
	if err != nil {
		return fmt.Errorf("dependency graph construction failed: %w", err)
	}

	// Check for circular dependencies by attempting topological sort
	_, err = d.topologicalSort(graph, charts)
	if err != nil {
		return fmt.Errorf("dependency validation failed: %w", err)
	}

	d.logger.InfoWithContext("dependency validation completed successfully", map[string]interface{}{
		"chart_count": len(charts),
	})

	return nil
}

// GetDeploymentOrder returns charts grouped by deployment layers (charts that can be deployed in parallel)
func (d *DependencyResolver) GetDeploymentOrder(charts []domain.Chart) ([][]domain.Chart, error) {
	d.logger.InfoWithContext("determining deployment order layers", map[string]interface{}{
		"chart_count": len(charts),
	})

	// Get ordered charts
	orderedCharts, err := d.ResolveDependencies(charts)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve dependencies: %w", err)
	}

	// Build dependency graph for layer calculation
	graph, err := d.BuildDependencyGraph(charts)
	if err != nil {
		return nil, fmt.Errorf("failed to build dependency graph: %w", err)
	}

	// Group charts into layers based on dependency depth
	layers := make([][]domain.Chart, 0)
	processed := make(map[string]bool)

	for len(processed) < len(orderedCharts) {
		currentLayer := make([]domain.Chart, 0)
		
		// Find charts that can be deployed in this layer
		for _, chart := range orderedCharts {
			if processed[chart.Name] {
				continue
			}

			// Check if all dependencies are satisfied
			canDeploy := true
			for _, depName := range graph.Nodes[chart.Name].Dependencies {
				if !processed[depName] {
					canDeploy = false
					break
				}
			}

			if canDeploy {
				currentLayer = append(currentLayer, chart)
				processed[chart.Name] = true
			}
		}

		if len(currentLayer) > 0 {
			layers = append(layers, currentLayer)
			d.logger.DebugWithContext("deployment layer created", map[string]interface{}{
				"layer":       len(layers),
				"chart_count": len(currentLayer),
				"charts":      d.getChartNames(currentLayer),
			})
		} else {
			// Safety check - should not happen if dependencies are valid
			break
		}
	}

	d.logger.InfoWithContext("deployment order layers determined", map[string]interface{}{
		"total_layers": len(layers),
		"total_charts": len(orderedCharts),
	})

	return layers, nil
}

// getChartNames extracts chart names from a slice of charts
func (d *DependencyResolver) getChartNames(charts []domain.Chart) []string {
	names := make([]string, len(charts))
	for i, chart := range charts {
		names[i] = chart.Name
	}
	return names
}