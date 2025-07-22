package dependency_usecase

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"deploy-cli/port/filesystem_port"
	"deploy-cli/port/logger_port"
)

// Types are now unified with advanced_dependency_resolver.go
// Removed duplicate type declarations to avoid redeclaration errors

// DependencyScanner analyzes chart dependencies using unified types
type DependencyScanner struct {
	filesystem filesystem_port.FileSystemPort
	logger     logger_port.LoggerPort
	patterns   *DependencyPatterns
}

// DependencyPatterns holds regex patterns for dependency detection
type DependencyPatterns struct {
	ServiceReferences  []*regexp.Regexp
	DatabaseReferences []*regexp.Regexp
	ConfigReferences   []*regexp.Regexp
	SecretReferences   []*regexp.Regexp
}

// NewDependencyScanner creates a new DependencyScanner using unified types
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
			regexp.MustCompile(`service:\s*([a-zA-Z0-9\-_]+)`),
			regexp.MustCompile(`serviceName:\s*([a-zA-Z0-9\-_]+)`),
			regexp.MustCompile(`{{ .Values\.([^}]+)\.service`),
		},
		DatabaseReferences: []*regexp.Regexp{
			regexp.MustCompile(`database:\s*([a-zA-Z0-9\-_]+)`),
			regexp.MustCompile(`db:\s*([a-zA-Z0-9\-_]+)`),
			regexp.MustCompile(`postgres.*host:\s*([a-zA-Z0-9\-_.]+)`),
		},
		ConfigReferences: []*regexp.Regexp{
			regexp.MustCompile(`configMap:\s*([a-zA-Z0-9\-_]+)`),
			regexp.MustCompile(`{{ .Values\.([^}]+)\.config`),
		},
		SecretReferences: []*regexp.Regexp{
			regexp.MustCompile(`secret:\s*([a-zA-Z0-9\-_]+)`),
			regexp.MustCompile(`secretName:\s*([a-zA-Z0-9\-_]+)`),
			regexp.MustCompile(`{{ .Values\.([^}]+)\.secret`),
		},
	}
}

// ScanDependencies scans all charts and builds dependency graph using unified types
func (s *DependencyScanner) ScanDependencies(ctx context.Context, chartsDir string) (*DependencyGraph, error) {
	s.logger.InfoWithContext("starting dependency scan", map[string]interface{}{
		"charts_dir": chartsDir,
	})

	// Discover all charts
	charts, err := s.discoverCharts(chartsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to discover charts: %w", err)
	}

	if len(charts) == 0 {
		s.logger.WarnWithContext("no charts found", map[string]interface{}{
			"charts_dir": chartsDir,
		})
	}

	// Scan dependencies for each chart
	var allDependencies []ChartDependency
	for _, chart := range charts {
		deps, err := s.scanHelmDependenciesOnly(chartsDir, chart)
		if err != nil {
			s.logger.WarnWithContext("failed to scan chart dependencies", map[string]interface{}{
				"chart": chart,
				"error": err.Error(),
			})
			continue
		}
		allDependencies = append(allDependencies, deps...)
	}

	// Build dependency graph using unified structure from advanced_dependency_resolver.go
	graph := &DependencyGraph{
		Nodes:       make(map[string]*ChartNode),
		Edges:       make(map[string][]string),
		DeployOrder: [][]string{},
		Metadata: GraphMetadata{
			TotalCharts:       len(charts),
			TotalDependencies: len(allDependencies),
			MaxDepth:          0,
			HasCycles:         false,
			Cycles:            []DependencyCycle{},
		},
	}

	// Initialize nodes for each chart
	for _, chart := range charts {
		graph.Nodes[chart] = &ChartNode{
			Dependencies: []string{},
			Dependents:   []string{},
			Depth:        0,
			Priority:     0,
		}
		graph.Edges[chart] = []string{}
	}

	// Add edges based on dependencies
	for _, dep := range allDependencies {
		if node, exists := graph.Nodes[dep.Source]; exists {
			node.Dependencies = append(node.Dependencies, dep.Target)
			graph.Edges[dep.Source] = append(graph.Edges[dep.Source], dep.Target)
		}
		if node, exists := graph.Nodes[dep.Target]; exists {
			node.Dependents = append(node.Dependents, dep.Source)
		}
	}

	// Analyze graph structure
	if err := s.analyzeGraph(graph); err != nil {
		s.logger.WarnWithContext("failed to analyze dependency graph", map[string]interface{}{
			"error": err.Error(),
		})
	}

	s.logger.InfoWithContext("dependency scan completed", map[string]interface{}{
		"total_charts":       graph.Metadata.TotalCharts,
		"total_dependencies": len(allDependencies),
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

// analyzeGraph analyzes the dependency graph for cycles and depth
func (s *DependencyScanner) analyzeGraph(graph *DependencyGraph) error {
	// Calculate deployment order using topological sort
	deployOrder, err := s.calculateDeploymentOrder(graph)
	if err != nil {
		s.logger.WarnWithContext("failed to calculate deployment order", map[string]interface{}{
			"error": err.Error(),
		})
		// Set default deployment order as a single level with all charts
		var allCharts []string
		for chartName := range graph.Nodes {
			allCharts = append(allCharts, chartName)
		}
		graph.DeployOrder = [][]string{allCharts}
	} else {
		graph.DeployOrder = deployOrder
	}

	// Simple depth calculation
	maxDepth := len(graph.DeployOrder)
	graph.Metadata.MaxDepth = maxDepth

	// Simple cycle detection (placeholder)
	graph.Metadata.HasCycles = false

	return nil
}

// calculateDeploymentOrder performs topological sort to determine deployment order
func (s *DependencyScanner) calculateDeploymentOrder(graph *DependencyGraph) ([][]string, error) {
	// Create in-degree map
	inDegree := make(map[string]int)
	for chartName := range graph.Nodes {
		inDegree[chartName] = len(graph.Nodes[chartName].Dependencies)
	}

	// Find nodes with no dependencies (in-degree 0)
	var queue []string
	for chartName, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, chartName)
		}
	}

	var deployOrder [][]string
	level := 0

	for len(queue) > 0 {
		currentLevel := make([]string, len(queue))
		copy(currentLevel, queue)
		deployOrder = append(deployOrder, currentLevel)

		// Process current level
		var nextQueue []string
		for _, chart := range queue {
			// Remove this chart's dependencies from dependent charts
			for _, dependent := range graph.Nodes[chart].Dependents {
				inDegree[dependent]--
				if inDegree[dependent] == 0 {
					nextQueue = append(nextQueue, dependent)
				}
			}
		}

		queue = nextQueue
		level++
	}

	// Check if all charts are included (cycle detection)
	totalProcessed := 0
	for _, level := range deployOrder {
		totalProcessed += len(level)
	}

	if totalProcessed != len(graph.Nodes) {
		return nil, fmt.Errorf("circular dependency detected: processed %d charts out of %d", totalProcessed, len(graph.Nodes))
	}

	return deployOrder, nil
}

// scanHelmDependenciesOnly scans only Chart.yaml for Helm dependencies (avoids false positives)
func (s *DependencyScanner) scanHelmDependenciesOnly(chartsDir, chartName string) ([]ChartDependency, error) {
	chartPath := filepath.Join(chartsDir, chartName)
	return s.scanHelmDependencies(chartPath, chartName)
}

// scanHelmDependencies scans a specific chart for Helm dependencies in Chart.yaml
func (s *DependencyScanner) scanHelmDependencies(chartPath, chartName string) ([]ChartDependency, error) {
	chartYamlPath := filepath.Join(chartPath, "Chart.yaml")
	
	content, err := s.filesystem.ReadFile(chartYamlPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read Chart.yaml: %w", err)
	}

	var dependencies []ChartDependency
	lines := strings.Split(string(content), "\n")
	inDependenciesSection := false

	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		
		// Check if we're in the dependencies section
		if strings.HasPrefix(trimmedLine, "dependencies:") {
			inDependenciesSection = true
			continue
		}
		
		// Exit dependencies section if we hit a new top-level key
		if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") && strings.Contains(line, ":") && inDependenciesSection {
			inDependenciesSection = false
		}
		
		// Parse dependency entries
		if inDependenciesSection && strings.Contains(line, "name:") {
			parts := strings.Split(line, ":")
			if len(parts) >= 2 {
				depName := strings.TrimSpace(parts[1])
				depName = strings.Trim(depName, "\"'")

				if depName != "" {
					dependencies = append(dependencies, ChartDependency{
						Source:            chartName,
						Target:            depName,
						Type:              HardDependency,
						VersionConstraint: "",
						Optional:          false,
						Condition:         "",
					})
				}
			}
		}
	}

	return dependencies, nil
}