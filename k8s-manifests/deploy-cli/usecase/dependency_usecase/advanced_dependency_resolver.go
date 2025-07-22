package dependency_usecase

import (
	"context"
	"fmt"
	"log/slog"

	"deploy-cli/domain"
)

// AdvancedDependencyResolver provides sophisticated dependency resolution
// 高度な依存関係解決エンジン
type AdvancedDependencyResolver struct {
	graphBuilder DependencyGraphBuilder
	topoSorter   TopologicalSorter
	cycleDetect  CycleDependencyDetector
	optimizer    DeploymentOptimizer
	validator    DependencyValidator
	logger       *slog.Logger
}

// DependencyGraphBuilder builds dependency graphs from charts
type DependencyGraphBuilder interface {
	BuildGraph(ctx context.Context, charts []domain.Chart, environment domain.Environment) (*DependencyGraph, error)
	AddCustomDependency(source, target string, dependencyType DependencyType) error
	RemoveDependency(source, target string) error
	AnalyzeChartDependencies(ctx context.Context, chart domain.Chart) ([]ChartDependency, error)
}

// TopologicalSorter sorts charts based on dependencies
type TopologicalSorter interface {
	Sort(ctx context.Context, graph *DependencyGraph) ([]DeploymentStage, error)
	OptimizeParallelism(ctx context.Context, stages []DeploymentStage) ([]DeploymentStage, error)
	CalculateDeploymentOrder(ctx context.Context, charts []domain.Chart, dependencies map[string][]string) ([]string, error)
}

// CycleDependencyDetector detects and reports circular dependencies
type CycleDependencyDetector interface {
	DetectCycles(ctx context.Context, graph *DependencyGraph) ([]DependencyCycle, error)
	FindStronglyConnectedComponents(ctx context.Context, graph *DependencyGraph) ([][]string, error)
	SuggestCycleResolution(ctx context.Context, cycle DependencyCycle) ([]ResolutionSuggestion, error)
}

// DeploymentOptimizer optimizes deployment strategies
type DeploymentOptimizer interface {
	OptimizeDeploymentStages(ctx context.Context, stages []DeploymentStage) ([]DeploymentStage, error)
	CalculateOptimalBatchSize(ctx context.Context, charts []domain.Chart) (int, error)
	EstimateDeploymentTime(ctx context.Context, stages []DeploymentStage) (DeploymentTimeEstimate, error)
}

// DependencyValidator validates dependency configurations
type DependencyValidator interface {
	ValidateDependencies(ctx context.Context, graph *DependencyGraph) error
	CheckCompatibility(ctx context.Context, source, target domain.Chart) error
	ValidateVersionConstraints(ctx context.Context, dependencies []ChartDependency) error
}

// DependencyGraph represents chart dependency relationships
type DependencyGraph struct {
	Nodes       map[string]*ChartNode
	Edges       map[string][]string
	Metadata    GraphMetadata
	DeployOrder [][]string
}

// ChartNode represents a chart in the dependency graph
type ChartNode struct {
	Chart        domain.Chart
	Dependencies []string
	Dependents   []string
	Depth        int
	Priority     int
	Metadata     NodeMetadata
}

// GraphMetadata contains graph-level information
type GraphMetadata struct {
	Environment       domain.Environment
	TotalCharts       int
	TotalDependencies int
	MaxDepth          int
	HasCycles         bool
	Cycles            []DependencyCycle
	CriticalPath      []string
	EstimatedTime     DeploymentTimeEstimate
}

// NodeMetadata contains node-level information
type NodeMetadata struct {
	DeploymentTime   DeploymentTimeEstimate
	ResourceRequirements ResourceRequirements
	HealthCheckConfig    HealthCheckConfig
	Dependencies        []ChartDependency
}

// DeploymentStage represents a stage in deployment
type DeploymentStage struct {
	StageNumber    int
	Charts         []domain.Chart
	CanParallelize bool
	Dependencies   []string
	EstimatedTime  DeploymentTimeEstimate
	StageType      StageType
}

// ChartDependency represents a dependency relationship
type ChartDependency struct {
	Source         string
	Target         string
	Type           DependencyType
	VersionConstraint string
	Optional       bool
	Condition      string
}

// DependencyCycle represents a circular dependency
type DependencyCycle struct {
	Charts      []string
	CycleType   CycleType
	Severity    CycleSeverity
	Description string
}

// ResolutionSuggestion provides suggestions for resolving issues
type ResolutionSuggestion struct {
	Type        SuggestionType
	Description string
	Actions     []string
	Impact      string
}

// DeploymentTimeEstimate estimates deployment timing
type DeploymentTimeEstimate struct {
	MinTime      int // seconds
	MaxTime      int // seconds
	AverageTime  int // seconds
	Confidence   float64
}

// ResourceRequirements defines resource needs
type ResourceRequirements struct {
	CPU     string
	Memory  string
	Storage string
}

// HealthCheckConfig defines health check parameters
type HealthCheckConfig struct {
	Enabled     bool
	Timeout     int
	Retries     int
	Interval    int
}

// Enums
type DependencyType string
const (
	HardDependency DependencyType = "hard"
	SoftDependency DependencyType = "soft"
	ConditionalDependency DependencyType = "conditional"
	ServiceDependency DependencyType = "service"
	DataDependency DependencyType = "data"
)

type StageType string
const (
	InfrastructureStage StageType = "infrastructure"
	ApplicationStage    StageType = "application"
	OperationalStage    StageType = "operational"
)

type CycleType string
const (
	DirectCycle   CycleType = "direct"
	IndirectCycle CycleType = "indirect"
	ConditionalCycle CycleType = "conditional"
)

type CycleSeverity string
const (
	CriticalSeverity CycleSeverity = "critical"
	WarningSeverity  CycleSeverity = "warning"
	InfoSeverity     CycleSeverity = "info"
)

type SuggestionType string
const (
	RemoveDependency  SuggestionType = "remove_dependency"
	ReorderCharts     SuggestionType = "reorder_charts"
	ConditionalDeploy SuggestionType = "conditional_deploy"
	SplitChart        SuggestionType = "split_chart"
)

// NewAdvancedDependencyResolver creates new instance
func NewAdvancedDependencyResolver(logger *slog.Logger) *AdvancedDependencyResolver {
	return &AdvancedDependencyResolver{
		graphBuilder: NewDependencyGraphBuilder(logger),
		topoSorter:   NewTopologicalSorter(logger),
		cycleDetect:  NewCycleDependencyDetector(logger),
		optimizer:    NewDeploymentOptimizer(logger),
		validator:    NewDependencyValidator(logger),
		logger:       logger,
	}
}

// ResolveDeploymentOrder resolves optimal deployment order
func (adr *AdvancedDependencyResolver) ResolveDeploymentOrder(
	ctx context.Context,
	charts []domain.Chart,
	environment domain.Environment,
) ([]DeploymentStage, error) {
	adr.logger.Info("Resolving deployment order",
		"charts", len(charts),
		"environment", environment)

	// Phase 1: Build dependency graph
	graph, err := adr.graphBuilder.BuildGraph(ctx, charts, environment)
	if err != nil {
		return nil, fmt.Errorf("failed to build dependency graph: %w", err)
	}

	// Phase 2: Detect and handle cycles
	cycles, err := adr.cycleDetect.DetectCycles(ctx, graph)
	if err != nil {
		return nil, fmt.Errorf("cycle detection failed: %w", err)
	}

	if len(cycles) > 0 {
		adr.logger.Warn("Circular dependencies detected", "count", len(cycles))
		for _, cycle := range cycles {
			adr.logger.Warn("Cycle detected",
				"charts", cycle.Charts,
				"type", cycle.CycleType,
				"severity", cycle.Severity)
			
			// Get resolution suggestions
			suggestions, err := adr.cycleDetect.SuggestCycleResolution(ctx, cycle)
			if err != nil {
				adr.logger.Warn("Failed to get cycle resolution suggestions", "error", err)
				continue
			}

			for _, suggestion := range suggestions {
				adr.logger.Info("Cycle resolution suggestion",
					"type", suggestion.Type,
					"description", suggestion.Description,
					"impact", suggestion.Impact)
			}
		}

		if adr.hasCriticalCycles(cycles) {
			return nil, fmt.Errorf("critical circular dependencies detected: %d cycles", len(cycles))
		}
	}

	// Phase 3: Validate dependencies
	if err := adr.validator.ValidateDependencies(ctx, graph); err != nil {
		return nil, fmt.Errorf("dependency validation failed: %w", err)
	}

	// Phase 4: Perform topological sort
	stages, err := adr.topoSorter.Sort(ctx, graph)
	if err != nil {
		return nil, fmt.Errorf("topological sort failed: %w", err)
	}

	// Phase 5: Optimize for parallelism
	optimizedStages, err := adr.topoSorter.OptimizeParallelism(ctx, stages)
	if err != nil {
		adr.logger.Warn("Parallelism optimization failed", "error", err)
		optimizedStages = stages // Use original stages if optimization fails
	}

	// Phase 6: Final optimization
	finalStages, err := adr.optimizer.OptimizeDeploymentStages(ctx, optimizedStages)
	if err != nil {
		adr.logger.Warn("Final optimization failed", "error", err)
		finalStages = optimizedStages
	}

	adr.logger.Info("Deployment order resolved",
		"total_stages", len(finalStages),
		"parallel_stages", adr.countParallelStages(finalStages),
		"estimated_time", adr.calculateTotalTime(finalStages))

	return finalStages, nil
}

// AnalyzeDeploymentComplexity analyzes deployment complexity
func (adr *AdvancedDependencyResolver) AnalyzeDeploymentComplexity(
	ctx context.Context,
	charts []domain.Chart,
	environment domain.Environment,
) (*ComplexityAnalysis, error) {
	adr.logger.Info("Analyzing deployment complexity", "charts", len(charts))

	// Build dependency graph
	graph, err := adr.graphBuilder.BuildGraph(ctx, charts, environment)
	if err != nil {
		return nil, fmt.Errorf("failed to build dependency graph: %w", err)
	}

	// Detect cycles
	cycles, err := adr.cycleDetect.DetectCycles(ctx, graph)
	if err != nil {
		return nil, fmt.Errorf("cycle detection failed: %w", err)
	}

	// Calculate complexity metrics
	analysis := &ComplexityAnalysis{
		TotalCharts:        len(charts),
		DependencyCount:    adr.calculateDependencyCount(graph),
		MaxDepth:          graph.Metadata.MaxDepth,
		CycleCount:        len(cycles),
		CriticalPathLength: len(graph.Metadata.CriticalPath),
		ComplexityScore:   adr.calculateComplexityScore(graph, cycles),
		Environment:       environment,
	}

	// Calculate time estimate
	stages, err := adr.topoSorter.Sort(ctx, graph)
	if err == nil {
		timeEstimate, err := adr.optimizer.EstimateDeploymentTime(ctx, stages)
		if err == nil {
			analysis.EstimatedTime = timeEstimate
		}
	}

	adr.logger.Info("Complexity analysis completed",
		"complexity_score", analysis.ComplexityScore,
		"dependency_count", analysis.DependencyCount,
		"max_depth", analysis.MaxDepth,
		"cycle_count", analysis.CycleCount)

	return analysis, nil
}

// OptimizeDeploymentStrategy optimizes deployment strategy
func (adr *AdvancedDependencyResolver) OptimizeDeploymentStrategy(
	ctx context.Context,
	stages []DeploymentStage,
) (*OptimizedStrategy, error) {
	adr.logger.Info("Optimizing deployment strategy", "stages", len(stages))

	// Optimize batch sizes
	optimizedStages := make([]DeploymentStage, len(stages))
	copy(optimizedStages, stages)

	for i := range optimizedStages {
		if optimizedStages[i].CanParallelize && len(optimizedStages[i].Charts) > 1 {
			optimalBatch, err := adr.optimizer.CalculateOptimalBatchSize(ctx, optimizedStages[i].Charts)
			if err != nil {
				continue
			}

			// Split stage if batch size is smaller than chart count
			if optimalBatch < len(optimizedStages[i].Charts) {
				splitStages := adr.splitStageIntoBatches(optimizedStages[i], optimalBatch)
				// Replace current stage with split stages
				optimizedStages = append(optimizedStages[:i], append(splitStages, optimizedStages[i+1:]...)...)
			}
		}
	}

	// Calculate final time estimate
	timeEstimate, err := adr.optimizer.EstimateDeploymentTime(ctx, optimizedStages)
	if err != nil {
		timeEstimate = DeploymentTimeEstimate{MinTime: 0, MaxTime: 0, AverageTime: 0, Confidence: 0}
	}

	strategy := &OptimizedStrategy{
		Stages:           optimizedStages,
		TotalTime:        timeEstimate,
		ParallelStages:   adr.countParallelStages(optimizedStages),
		OptimizationGain: adr.calculateOptimizationGain(stages, optimizedStages),
		Recommendations:  adr.generateOptimizationRecommendations(optimizedStages),
	}

	adr.logger.Info("Deployment strategy optimized",
		"original_stages", len(stages),
		"optimized_stages", len(optimizedStages),
		"parallel_stages", strategy.ParallelStages,
		"optimization_gain", strategy.OptimizationGain)

	return strategy, nil
}

// Helper methods

func (adr *AdvancedDependencyResolver) hasCriticalCycles(cycles []DependencyCycle) bool {
	for _, cycle := range cycles {
		if cycle.Severity == CriticalSeverity {
			return true
		}
	}
	return false
}

func (adr *AdvancedDependencyResolver) countParallelStages(stages []DeploymentStage) int {
	count := 0
	for _, stage := range stages {
		if stage.CanParallelize && len(stage.Charts) > 1 {
			count++
		}
	}
	return count
}

func (adr *AdvancedDependencyResolver) calculateTotalTime(stages []DeploymentStage) int {
	total := 0
	for _, stage := range stages {
		total += stage.EstimatedTime.AverageTime
	}
	return total
}

func (adr *AdvancedDependencyResolver) calculateDependencyCount(graph *DependencyGraph) int {
	count := 0
	for _, edges := range graph.Edges {
		count += len(edges)
	}
	return count
}

func (adr *AdvancedDependencyResolver) calculateComplexityScore(graph *DependencyGraph, cycles []DependencyCycle) float64 {
	// Simple complexity scoring algorithm
	baseScore := float64(len(graph.Nodes))
	dependencyFactor := float64(adr.calculateDependencyCount(graph)) * 0.5
	depthFactor := float64(graph.Metadata.MaxDepth) * 0.3
	cyclePenalty := float64(len(cycles)) * 2.0

	return baseScore + dependencyFactor + depthFactor + cyclePenalty
}

func (adr *AdvancedDependencyResolver) splitStageIntoBatches(stage DeploymentStage, batchSize int) []DeploymentStage {
	var batches []DeploymentStage
	charts := stage.Charts

	for i := 0; i < len(charts); i += batchSize {
		end := i + batchSize
		if end > len(charts) {
			end = len(charts)
		}

		batch := DeploymentStage{
			StageNumber:   stage.StageNumber,
			Charts:        charts[i:end],
			CanParallelize: true,
			Dependencies:  stage.Dependencies,
			EstimatedTime: DeploymentTimeEstimate{
				AverageTime: stage.EstimatedTime.AverageTime / len(charts) * len(charts[i:end]),
			},
			StageType: stage.StageType,
		}

		batches = append(batches, batch)
	}

	return batches
}

func (adr *AdvancedDependencyResolver) calculateOptimizationGain(original, optimized []DeploymentStage) float64 {
	originalTime := adr.calculateTotalTime(original)
	optimizedTime := adr.calculateTotalTime(optimized)

	if originalTime == 0 {
		return 0
	}

	return float64(originalTime-optimizedTime) / float64(originalTime) * 100.0
}

func (adr *AdvancedDependencyResolver) generateOptimizationRecommendations(stages []DeploymentStage) []string {
	var recommendations []string

	parallelCount := adr.countParallelStages(stages)
	if parallelCount < len(stages)/2 {
		recommendations = append(recommendations, "Consider increasing parallelism in deployment stages")
	}

	for i, stage := range stages {
		if len(stage.Charts) > 5 && stage.CanParallelize {
			recommendations = append(recommendations, 
				fmt.Sprintf("Stage %d has %d charts - consider splitting into smaller batches", i+1, len(stage.Charts)))
		}
	}

	return recommendations
}

// Additional types for complex analysis
type ComplexityAnalysis struct {
	TotalCharts        int
	DependencyCount    int
	MaxDepth          int
	CycleCount        int
	CriticalPathLength int
	ComplexityScore   float64
	EstimatedTime     DeploymentTimeEstimate
	Environment       domain.Environment
}

type OptimizedStrategy struct {
	Stages           []DeploymentStage
	TotalTime        DeploymentTimeEstimate
	ParallelStages   int
	OptimizationGain float64
	Recommendations  []string
}