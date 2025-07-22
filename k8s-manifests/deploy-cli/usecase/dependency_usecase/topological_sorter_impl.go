package dependency_usecase

import (
	"context"
	"fmt"
	"log/slog"
	"sort"

	"deploy-cli/domain"
)

// topologicalSorterImpl implements TopologicalSorter interface
type topologicalSorterImpl struct {
	logger *slog.Logger
}

// NewTopologicalSorter creates new TopologicalSorter instance
func NewTopologicalSorter(logger *slog.Logger) TopologicalSorter {
	return &topologicalSorterImpl{
		logger: logger,
	}
}

// Sort performs topological sort on dependency graph
func (ts *topologicalSorterImpl) Sort(
	ctx context.Context,
	graph *DependencyGraph,
) ([]DeploymentStage, error) {
	ts.logger.Info("Performing topological sort", "nodes", len(graph.Nodes))

	// Kahn's algorithm for topological sorting
	inDegree := make(map[string]int)
	adjList := make(map[string][]string)

	// Initialize in-degree and adjacency list
	for nodeName := range graph.Nodes {
		inDegree[nodeName] = 0
		adjList[nodeName] = []string{}
	}

	// Build in-degree count and adjacency list
	for target, sources := range graph.Edges {
		for _, source := range sources {
			adjList[target] = append(adjList[target], source)
			inDegree[source]++
		}
	}

	var stages []DeploymentStage
	stageNumber := 1

	for len(inDegree) > 0 {
		// Find all nodes with in-degree 0
		var currentStageNodes []string
		for nodeName, degree := range inDegree {
			if degree == 0 {
				currentStageNodes = append(currentStageNodes, nodeName)
			}
		}

		if len(currentStageNodes) == 0 {
			// Cycle detected - no nodes with in-degree 0
			remaining := make([]string, 0, len(inDegree))
			for nodeName := range inDegree {
				remaining = append(remaining, nodeName)
			}
			return nil, fmt.Errorf("circular dependency detected among charts: %v", remaining)
		}

		// Sort nodes by priority for deterministic ordering
		sort.Slice(currentStageNodes, func(i, j int) bool {
			return graph.Nodes[currentStageNodes[i]].Priority > graph.Nodes[currentStageNodes[j]].Priority
		})

		// Create deployment stage
		var stageCharts []domain.Chart
		var stageDependencies []string
		totalTime := 0
		stageType := ts.determineStageType(graph, currentStageNodes)

		for _, nodeName := range currentStageNodes {
			node := graph.Nodes[nodeName]
			stageCharts = append(stageCharts, node.Chart)
			stageDependencies = append(stageDependencies, node.Dependencies...)
			totalTime += node.Metadata.DeploymentTime.AverageTime
		}

		stage := DeploymentStage{
			StageNumber:    stageNumber,
			Charts:         stageCharts,
			CanParallelize: len(currentStageNodes) > 1 && ts.canParallelizeStage(graph, currentStageNodes),
			Dependencies:   ts.removeDuplicates(stageDependencies),
			EstimatedTime: DeploymentTimeEstimate{
				AverageTime: totalTime,
				MinTime:     int(float64(totalTime) * 0.8),
				MaxTime:     int(float64(totalTime) * 1.3),
				Confidence:  0.8,
			},
			StageType: stageType,
		}

		stages = append(stages, stage)

		// Remove processed nodes and update in-degrees
		for _, nodeName := range currentStageNodes {
			delete(inDegree, nodeName)
			
			// Decrease in-degree for dependent nodes
			for _, dependent := range adjList[nodeName] {
				if _, exists := inDegree[dependent]; exists {
					inDegree[dependent]--
				}
			}
		}

		stageNumber++
	}

	ts.logger.Info("Topological sort completed",
		"total_stages", len(stages),
		"parallel_stages", ts.countParallelizableStages(stages))

	return stages, nil
}

// OptimizeParallelism optimizes stages for maximum parallelism
func (ts *topologicalSorterImpl) OptimizeParallelism(
	ctx context.Context,
	stages []DeploymentStage,
) ([]DeploymentStage, error) {
	ts.logger.Info("Optimizing deployment parallelism", "stages", len(stages))

	optimizedStages := make([]DeploymentStage, 0, len(stages))

	for _, stage := range stages {
		if stage.CanParallelize && len(stage.Charts) > 1 {
			// Try to split stage further based on sub-dependencies
			subStages := ts.splitStageBySubDependencies(stage)
			optimizedStages = append(optimizedStages, subStages...)
		} else {
			optimizedStages = append(optimizedStages, stage)
		}
	}

	// Renumber stages
	for i := range optimizedStages {
		optimizedStages[i].StageNumber = i + 1
	}

	ts.logger.Info("Parallelism optimization completed",
		"original_stages", len(stages),
		"optimized_stages", len(optimizedStages))

	return optimizedStages, nil
}

// CalculateDeploymentOrder calculates deployment order from chart dependencies
func (ts *topologicalSorterImpl) CalculateDeploymentOrder(
	ctx context.Context,
	charts []domain.Chart,
	dependencies map[string][]string,
) ([]string, error) {
	ts.logger.Info("Calculating deployment order",
		"charts", len(charts),
		"dependencies", len(dependencies))

	// Build chart name set for validation
	chartSet := make(map[string]bool)
	for _, chart := range charts {
		chartSet[chart.Name] = true
	}

	// Validate dependencies
	for chart, deps := range dependencies {
		if !chartSet[chart] {
			return nil, fmt.Errorf("chart %s in dependencies not found in charts list", chart)
		}
		for _, dep := range deps {
			if !chartSet[dep] {
				return nil, fmt.Errorf("dependency %s for chart %s not found in charts list", dep, chart)
			}
		}
	}

	// Perform topological sort using DFS
	visited := make(map[string]bool)
	recursionStack := make(map[string]bool)
	var result []string

	var dfs func(string) error
	dfs = func(chart string) error {
		if recursionStack[chart] {
			return fmt.Errorf("circular dependency detected involving chart: %s", chart)
		}
		if visited[chart] {
			return nil
		}

		visited[chart] = true
		recursionStack[chart] = true

		// Visit all dependencies first
		for _, dep := range dependencies[chart] {
			if err := dfs(dep); err != nil {
				return err
			}
		}

		recursionStack[chart] = false
		result = append([]string{chart}, result...) // Prepend to result

		return nil
	}

	// Visit all charts
	for _, chart := range charts {
		if !visited[chart.Name] {
			if err := dfs(chart.Name); err != nil {
				return nil, err
			}
		}
	}

	ts.logger.Info("Deployment order calculated", "order", result)
	return result, nil
}

// Helper methods

func (ts *topologicalSorterImpl) determineStageType(graph *DependencyGraph, nodeNames []string) StageType {
	// Determine stage type based on chart types in the stage
	infrastructureCount := 0
	applicationCount := 0
	operationalCount := 0

	for _, nodeName := range nodeNames {
		node := graph.Nodes[nodeName]
		switch node.Chart.Type {
		case domain.InfrastructureChart:
			infrastructureCount++
		case domain.ApplicationChart:
			applicationCount++
		case domain.OperationalChart:
			operationalCount++
		}
	}

	// Determine majority type
	if infrastructureCount >= applicationCount && infrastructureCount >= operationalCount {
		return InfrastructureStage
	} else if applicationCount >= operationalCount {
		return ApplicationStage
	} else {
		return OperationalStage
	}
}

func (ts *topologicalSorterImpl) canParallelizeStage(graph *DependencyGraph, nodeNames []string) bool {
	// Check if nodes in the stage can be deployed in parallel
	// They can be parallel if they don't depend on each other

	for i, node1 := range nodeNames {
		for j, node2 := range nodeNames {
			if i != j {
				// Check if node1 depends on node2 or vice versa
				if ts.hasDependency(graph, node1, node2) || ts.hasDependency(graph, node2, node1) {
					return false
				}
			}
		}
	}

	return true
}

func (ts *topologicalSorterImpl) hasDependency(graph *DependencyGraph, source, target string) bool {
	// Check if source depends on target (directly or indirectly)
	visited := make(map[string]bool)
	return ts.hasDependencyDFS(graph, source, target, visited)
}

func (ts *topologicalSorterImpl) hasDependencyDFS(graph *DependencyGraph, current, target string, visited map[string]bool) bool {
	if current == target {
		return true
	}
	if visited[current] {
		return false
	}

	visited[current] = true
	node := graph.Nodes[current]

	for _, dep := range node.Dependencies {
		if ts.hasDependencyDFS(graph, dep, target, visited) {
			return true
		}
	}

	return false
}

func (ts *topologicalSorterImpl) countParallelizableStages(stages []DeploymentStage) int {
	count := 0
	for _, stage := range stages {
		if stage.CanParallelize {
			count++
		}
	}
	return count
}

func (ts *topologicalSorterImpl) splitStageBySubDependencies(stage DeploymentStage) []DeploymentStage {
	// Try to split stage into smaller parallel groups
	// This is a simplified implementation - could be more sophisticated

	if len(stage.Charts) <= 2 {
		return []DeploymentStage{stage}
	}

	// Group charts by type for better parallelization
	typeGroups := make(map[domain.ChartType][]domain.Chart)
	for _, chart := range stage.Charts {
		typeGroups[chart.Type] = append(typeGroups[chart.Type], chart)
	}

	var subStages []DeploymentStage
	stageNum := stage.StageNumber

	for chartType, charts := range typeGroups {
		if len(charts) > 0 {
			subStage := DeploymentStage{
				StageNumber:    stageNum,
				Charts:         charts,
				CanParallelize: true,
				Dependencies:   stage.Dependencies,
				EstimatedTime: DeploymentTimeEstimate{
					AverageTime: stage.EstimatedTime.AverageTime / len(typeGroups),
					MinTime:     stage.EstimatedTime.MinTime / len(typeGroups),
					MaxTime:     stage.EstimatedTime.MaxTime / len(typeGroups),
					Confidence:  stage.EstimatedTime.Confidence,
				},
				StageType: ts.mapChartTypeToStageType(chartType),
			}
			subStages = append(subStages, subStage)
			stageNum++
		}
	}

	if len(subStages) <= 1 {
		return []DeploymentStage{stage}
	}

	return subStages
}

func (ts *topologicalSorterImpl) mapChartTypeToStageType(chartType domain.ChartType) StageType {
	switch chartType {
	case domain.InfrastructureChart:
		return InfrastructureStage
	case domain.ApplicationChart:
		return ApplicationStage
	case domain.OperationalChart:
		return OperationalStage
	default:
		return InfrastructureStage
	}
}

func (ts *topologicalSorterImpl) removeDuplicates(slice []string) []string {
	keys := make(map[string]bool)
	var result []string

	for _, item := range slice {
		if !keys[item] {
			keys[item] = true
			result = append(result, item)
		}
	}

	return result
}