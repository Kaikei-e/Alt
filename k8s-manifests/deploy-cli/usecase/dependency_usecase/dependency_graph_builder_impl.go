package dependency_usecase

import (
	"context"
	"log/slog"
	"strings"

	"deploy-cli/domain"
)

// dependencyGraphBuilderImpl implements DependencyGraphBuilder interface
type dependencyGraphBuilderImpl struct {
	logger *slog.Logger
}

// NewDependencyGraphBuilder creates new DependencyGraphBuilder instance
func NewDependencyGraphBuilder(logger *slog.Logger) DependencyGraphBuilder {
	return &dependencyGraphBuilderImpl{
		logger: logger,
	}
}

// BuildGraph builds dependency graph from charts
func (dgb *dependencyGraphBuilderImpl) BuildGraph(
	ctx context.Context,
	charts []domain.Chart,
	environment domain.Environment,
) (*DependencyGraph, error) {
	dgb.logger.Info("Building dependency graph",
		"charts", len(charts),
		"environment", environment)

	graph := &DependencyGraph{
		Nodes: make(map[string]*ChartNode),
		Edges: make(map[string][]string),
		Metadata: GraphMetadata{
			Environment: environment,
			TotalCharts: len(charts),
		},
	}

	// Phase 1: Create nodes for all charts
	for _, chart := range charts {
		node := &ChartNode{
			Chart:        chart,
			Dependencies: []string{},
			Dependents:   []string{},
			Depth:        0,
			Priority:     dgb.calculateChartPriority(chart),
			Metadata: NodeMetadata{
				DeploymentTime:       dgb.estimateChartDeploymentTime(chart),
				ResourceRequirements: dgb.extractResourceRequirements(chart),
				HealthCheckConfig:    dgb.extractHealthCheckConfig(chart),
			},
		}
		graph.Nodes[chart.Name] = node
	}

	// Phase 2: Analyze and build dependencies
	for _, chart := range charts {
		dependencies, err := dgb.AnalyzeChartDependencies(ctx, chart)
		if err != nil {
			dgb.logger.Warn("Failed to analyze chart dependencies",
				"chart", chart.Name,
				"error", err)
			continue
		}

		// Add dependencies to graph
		for _, dep := range dependencies {
			if dgb.isValidDependency(graph, dep.Source, dep.Target) {
				dgb.addDependencyToGraph(graph, dep)
			}
		}
	}

	// Phase 3: Add environment-specific dependencies
	dgb.addEnvironmentDependencies(graph, environment)

	// Phase 4: Calculate graph metadata
	dgb.calculateGraphMetadata(graph)

	dgb.logger.Info("Dependency graph built successfully",
		"nodes", len(graph.Nodes),
		"edges", len(graph.Edges),
		"max_depth", graph.Metadata.MaxDepth,
		"has_cycles", graph.Metadata.HasCycles)

	return graph, nil
}

// AnalyzeChartDependencies analyzes dependencies for a single chart
func (dgb *dependencyGraphBuilderImpl) AnalyzeChartDependencies(
	ctx context.Context,
	chart domain.Chart,
) ([]ChartDependency, error) {
	dgb.logger.Debug("Analyzing chart dependencies", "chart", chart.Name)

	var dependencies []ChartDependency

	// Analyze based on chart type and known patterns
	switch chart.Type {
	case domain.InfrastructureChart:
		dependencies = append(dependencies, dgb.getInfrastructureDependencies(chart)...)
	case domain.ApplicationChart:
		dependencies = append(dependencies, dgb.getApplicationDependencies(chart)...)
	case domain.OperationalChart:
		dependencies = append(dependencies, dgb.getOperationalDependencies(chart)...)
	}

	// Add common dependencies based on chart name patterns
	dependencies = append(dependencies, dgb.getPatternBasedDependencies(chart)...)

	dgb.logger.Debug("Chart dependencies analyzed",
		"chart", chart.Name,
		"dependencies", len(dependencies))

	return dependencies, nil
}

// AddCustomDependency adds a custom dependency
func (dgb *dependencyGraphBuilderImpl) AddCustomDependency(
	source, target string,
	dependencyType DependencyType,
) error {
	dgb.logger.Info("Adding custom dependency",
		"source", source,
		"target", target,
		"type", dependencyType)

	// In real implementation, store custom dependencies for graph building
	return nil
}

// RemoveDependency removes a dependency
func (dgb *dependencyGraphBuilderImpl) RemoveDependency(source, target string) error {
	dgb.logger.Info("Removing dependency",
		"source", source,
		"target", target)

	// In real implementation, remove from stored dependencies
	return nil
}

// Helper methods

func (dgb *dependencyGraphBuilderImpl) calculateChartPriority(chart domain.Chart) int {
	// Priority based on chart type and name
	switch chart.Type {
	case domain.InfrastructureChart:
		if strings.Contains(chart.Name, "postgres") {
			return 10 // Highest priority for databases
		}
		if strings.Contains(chart.Name, "secret") || strings.Contains(chart.Name, "config") {
			return 9 // High priority for configuration
		}
		return 8 // High priority for infrastructure
	case domain.ApplicationChart:
		if strings.Contains(chart.Name, "backend") {
			return 6 // Medium-high priority for backend services
		}
		return 5 // Medium priority for applications
	case domain.OperationalChart:
		return 3 // Lower priority for operational charts
	default:
		return 1
	}
}

func (dgb *dependencyGraphBuilderImpl) estimateChartDeploymentTime(chart domain.Chart) DeploymentTimeEstimate {
	// Estimate based on chart type and complexity
	switch chart.Type {
	case domain.InfrastructureChart:
		if strings.Contains(chart.Name, "postgres") {
			return DeploymentTimeEstimate{MinTime: 120, MaxTime: 300, AverageTime: 180, Confidence: 0.8}
		}
		return DeploymentTimeEstimate{MinTime: 30, MaxTime: 120, AverageTime: 60, Confidence: 0.7}
	case domain.ApplicationChart:
		return DeploymentTimeEstimate{MinTime: 60, MaxTime: 180, AverageTime: 90, Confidence: 0.75}
	case domain.OperationalChart:
		return DeploymentTimeEstimate{MinTime: 15, MaxTime: 60, AverageTime: 30, Confidence: 0.9}
	default:
		return DeploymentTimeEstimate{MinTime: 30, MaxTime: 90, AverageTime: 45, Confidence: 0.6}
	}
}

func (dgb *dependencyGraphBuilderImpl) extractResourceRequirements(chart domain.Chart) ResourceRequirements {
	// Extract or estimate resource requirements
	switch chart.Type {
	case domain.InfrastructureChart:
		if strings.Contains(chart.Name, "postgres") {
			return ResourceRequirements{CPU: "500m", Memory: "1Gi", Storage: "10Gi"}
		}
		return ResourceRequirements{CPU: "200m", Memory: "512Mi", Storage: "5Gi"}
	case domain.ApplicationChart:
		return ResourceRequirements{CPU: "300m", Memory: "768Mi", Storage: "2Gi"}
	default:
		return ResourceRequirements{CPU: "100m", Memory: "256Mi", Storage: "1Gi"}
	}
}

func (dgb *dependencyGraphBuilderImpl) extractHealthCheckConfig(chart domain.Chart) HealthCheckConfig {
	// Extract or set default health check configuration
	if chart.WaitReady {
		return HealthCheckConfig{Enabled: true, Timeout: 300, Retries: 5, Interval: 30}
	}
	return HealthCheckConfig{Enabled: false, Timeout: 60, Retries: 3, Interval: 10}
}

func (dgb *dependencyGraphBuilderImpl) getInfrastructureDependencies(chart domain.Chart) []ChartDependency {
	var deps []ChartDependency

	switch chart.Name {
	case "auth-postgres":
		deps = append(deps, ChartDependency{
			Source: chart.Name,
			Target: "common-secrets",
			Type:   HardDependency,
		})
	case "kratos-postgres":
		deps = append(deps, ChartDependency{
			Source: chart.Name,
			Target: "common-secrets",
			Type:   HardDependency,
		})
	case "postgres":
		deps = append(deps, ChartDependency{
			Source: chart.Name,
			Target: "common-config",
			Type:   SoftDependency,
		})
	}

	return deps
}

func (dgb *dependencyGraphBuilderImpl) getApplicationDependencies(chart domain.Chart) []ChartDependency {
	var deps []ChartDependency

	switch chart.Name {
	case "alt-backend":
		deps = append(deps, 
			ChartDependency{Source: chart.Name, Target: "postgres", Type: HardDependency},
			ChartDependency{Source: chart.Name, Target: "meilisearch", Type: ServiceDependency},
		)
	case "auth-service":
		deps = append(deps, 
			ChartDependency{Source: chart.Name, Target: "auth-postgres", Type: HardDependency},
			ChartDependency{Source: chart.Name, Target: "common-ssl", Type: SoftDependency},
		)
	case "kratos":
		deps = append(deps, 
			ChartDependency{Source: chart.Name, Target: "kratos-postgres", Type: HardDependency},
		)
	case "alt-frontend":
		deps = append(deps, 
			ChartDependency{Source: chart.Name, Target: "alt-backend", Type: ServiceDependency},
			ChartDependency{Source: chart.Name, Target: "auth-service", Type: ServiceDependency},
		)
	}

	return deps
}

func (dgb *dependencyGraphBuilderImpl) getOperationalDependencies(chart domain.Chart) []ChartDependency {
	var deps []ChartDependency

	switch chart.Name {
	case "migrate":
		deps = append(deps, 
			ChartDependency{Source: chart.Name, Target: "postgres", Type: HardDependency},
			ChartDependency{Source: chart.Name, Target: "auth-postgres", Type: HardDependency},
			ChartDependency{Source: chart.Name, Target: "kratos-postgres", Type: HardDependency},
		)
	case "backup":
		deps = append(deps, 
			ChartDependency{Source: chart.Name, Target: "postgres", Type: DataDependency},
		)
	}

	return deps
}

func (dgb *dependencyGraphBuilderImpl) getPatternBasedDependencies(chart domain.Chart) []ChartDependency {
	var deps []ChartDependency

	// Add SSL dependencies for services that need SSL
	if strings.Contains(chart.Name, "service") || strings.Contains(chart.Name, "backend") {
		deps = append(deps, ChartDependency{
			Source: chart.Name,
			Target: "common-ssl",
			Type:   ConditionalDependency,
			Condition: "ssl.enabled",
		})
	}

	return deps
}

func (dgb *dependencyGraphBuilderImpl) isValidDependency(graph *DependencyGraph, source, target string) bool {
	// Check if both source and target exist in the graph
	_, sourceExists := graph.Nodes[source]
	_, targetExists := graph.Nodes[target]
	
	return sourceExists && targetExists && source != target
}

func (dgb *dependencyGraphBuilderImpl) addDependencyToGraph(graph *DependencyGraph, dep ChartDependency) {
	// Add edge
	graph.Edges[dep.Target] = append(graph.Edges[dep.Target], dep.Source)
	
	// Update node dependencies
	if sourceNode, exists := graph.Nodes[dep.Source]; exists {
		sourceNode.Dependencies = append(sourceNode.Dependencies, dep.Target)
		sourceNode.Metadata.Dependencies = append(sourceNode.Metadata.Dependencies, dep)
	}
	
	// Update node dependents
	if targetNode, exists := graph.Nodes[dep.Target]; exists {
		targetNode.Dependents = append(targetNode.Dependents, dep.Source)
	}
}

func (dgb *dependencyGraphBuilderImpl) addEnvironmentDependencies(graph *DependencyGraph, environment domain.Environment) {
	// Add environment-specific dependencies
	switch environment {
	case domain.Production:
		// Production requires stricter dependencies
		dgb.addProductionDependencies(graph)
	case domain.Staging:
		// Staging has comprehensive dependencies
		dgb.addStagingDependencies(graph)
	case domain.Development:
		// Development has minimal dependencies
		dgb.addDevelopmentDependencies(graph)
	}
}

func (dgb *dependencyGraphBuilderImpl) addProductionDependencies(graph *DependencyGraph) {
	// Production-specific dependencies
	if _, exists := graph.Nodes["monitoring"]; exists {
		// All services should depend on monitoring in production
		for name, node := range graph.Nodes {
			if node.Chart.Type == domain.ApplicationChart && name != "monitoring" {
				dep := ChartDependency{
					Source: name,
					Target: "monitoring",
					Type:   SoftDependency,
				}
				dgb.addDependencyToGraph(graph, dep)
			}
		}
	}
}

func (dgb *dependencyGraphBuilderImpl) addStagingDependencies(graph *DependencyGraph) {
	// Staging-specific dependencies
	// Similar to production but with some relaxed constraints
}

func (dgb *dependencyGraphBuilderImpl) addDevelopmentDependencies(graph *DependencyGraph) {
	// Development-specific dependencies
	// Minimal dependencies for faster development cycles
}

func (dgb *dependencyGraphBuilderImpl) calculateGraphMetadata(graph *DependencyGraph) {
	// Calculate maximum depth
	maxDepth := 0
	for _, node := range graph.Nodes {
		depth := dgb.calculateNodeDepth(graph, node.Chart.Name, make(map[string]bool))
		node.Depth = depth
		if depth > maxDepth {
			maxDepth = depth
		}
	}
	graph.Metadata.MaxDepth = maxDepth

	// Calculate critical path
	graph.Metadata.CriticalPath = dgb.findCriticalPath(graph)

	// Estimate total deployment time
	graph.Metadata.EstimatedTime = dgb.estimateGraphDeploymentTime(graph)
}

func (dgb *dependencyGraphBuilderImpl) calculateNodeDepth(graph *DependencyGraph, nodeName string, visited map[string]bool) int {
	if visited[nodeName] {
		return 0 // Avoid infinite recursion in cycles
	}
	visited[nodeName] = true

	node := graph.Nodes[nodeName]
	maxDepth := 0
	
	for _, dep := range node.Dependencies {
		depth := dgb.calculateNodeDepth(graph, dep, visited) + 1
		if depth > maxDepth {
			maxDepth = depth
		}
	}

	return maxDepth
}

func (dgb *dependencyGraphBuilderImpl) findCriticalPath(graph *DependencyGraph) []string {
	// Find the longest path in the dependency graph
	var criticalPath []string
	maxTime := 0

	for nodeName := range graph.Nodes {
		path := dgb.findLongestPath(graph, nodeName, make(map[string]bool))
		pathTime := dgb.calculatePathTime(graph, path)
		
		if pathTime > maxTime {
			maxTime = pathTime
			criticalPath = path
		}
	}

	return criticalPath
}

func (dgb *dependencyGraphBuilderImpl) findLongestPath(graph *DependencyGraph, nodeName string, visited map[string]bool) []string {
	if visited[nodeName] {
		return []string{}
	}
	visited[nodeName] = true

	node := graph.Nodes[nodeName]
	longestPath := []string{nodeName}
	maxLength := 0

	for _, dep := range node.Dependencies {
		path := dgb.findLongestPath(graph, dep, visited)
		if len(path) > maxLength {
			maxLength = len(path)
			longestPath = append([]string{nodeName}, path...)
		}
	}

	return longestPath
}

func (dgb *dependencyGraphBuilderImpl) calculatePathTime(graph *DependencyGraph, path []string) int {
	totalTime := 0
	for _, nodeName := range path {
		if node, exists := graph.Nodes[nodeName]; exists {
			totalTime += node.Metadata.DeploymentTime.AverageTime
		}
	}
	return totalTime
}

func (dgb *dependencyGraphBuilderImpl) estimateGraphDeploymentTime(graph *DependencyGraph) DeploymentTimeEstimate {
	// Estimate based on critical path and parallelization opportunities
	criticalPathTime := dgb.calculatePathTime(graph, graph.Metadata.CriticalPath)
	
	return DeploymentTimeEstimate{
		MinTime:     int(float64(criticalPathTime) * 0.8),
		MaxTime:     int(float64(criticalPathTime) * 1.5),
		AverageTime: criticalPathTime,
		Confidence:  0.7,
	}
}