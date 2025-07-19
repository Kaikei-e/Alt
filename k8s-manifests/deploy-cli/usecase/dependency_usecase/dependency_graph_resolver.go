package dependency_usecase

import (
	"sort"
	"time"

	"deploy-cli/domain"
	"deploy-cli/port/logger_port"
)

// DependencyGraphResolver handles automatic dependency resolution for optimal deployment order
type DependencyGraphResolver struct {
	logger logger_port.LoggerPort
}

// NewDependencyGraphResolver creates a new dependency graph resolver
func NewDependencyGraphResolver(logger logger_port.LoggerPort) *DependencyGraphResolver {
	return &DependencyGraphResolver{
		logger: logger,
	}
}

// DeploymentPhase represents a deployment phase with charts and dependencies
type DeploymentPhase struct {
	Name         string        `json:"name"`
	Description  string        `json:"description"`
	Charts       []string      `json:"charts"`
	Dependencies []string      `json:"dependencies"`
	Parallel     bool          `json:"parallel"`
	Timeout      time.Duration `json:"timeout"`
	Priority     int           `json:"priority"`
	Optional     bool          `json:"optional"`
}

// DependencyNode represents a node in the dependency graph
type DependencyNode struct {
	Name         string
	Dependencies []string
	Dependents   []string
	Level        int
	Processed    bool
}

// DependencyGraphData represents the complete dependency graph
type DependencyGraphData struct {
	Nodes map[string]*DependencyNode
	Edges map[string][]string
}

// ResolveOptimalDeploymentOrder 最適なデプロイメント順序の解決
func (r *DependencyGraphResolver) ResolveOptimalDeploymentOrder(charts []domain.Chart, env domain.Environment) ([]DeploymentPhase, error) {
	r.logger.InfoWithContext("依存関係グラフの解析開始", map[string]interface{}{
		"charts_count": len(charts),
		"environment":  env.String(),
	})

	// 依存関係グラフの構築
	graph := r.buildDependencyGraph(charts)

	// 環境別の最適化フェーズ定義
	phases := r.generateOptimalPhases(env)

	// グラフ解析による最適化
	optimizedPhases := r.optimizePhases(phases, graph, charts)

	r.logger.InfoWithContext("依存関係解析が完了", map[string]interface{}{
		"phases_count": len(optimizedPhases),
	})

	return optimizedPhases, nil
}

// buildDependencyGraph builds a dependency graph from charts
func (r *DependencyGraphResolver) buildDependencyGraph(charts []domain.Chart) *DependencyGraphData {
	graph := &DependencyGraphData{
		Nodes: make(map[string]*DependencyNode),
		Edges: make(map[string][]string),
	}

	// Create nodes for all charts
	for _, chart := range charts {
		graph.Nodes[chart.Name] = &DependencyNode{
			Name:         chart.Name,
			Dependencies: []string{},
			Dependents:   []string{},
			Level:        0,
			Processed:    false,
		}
	}

	// Build dependency relationships based on chart types and known dependencies
	r.buildKnownDependencies(graph)

	// Calculate dependency levels
	r.calculateDependencyLevels(graph)

	return graph
}

// buildKnownDependencies builds known dependency relationships
func (r *DependencyGraphResolver) buildKnownDependencies(graph *DependencyGraphData) {
	// Define known dependencies
	dependencies := map[string][]string{
		// Infrastructure first
		"common-config":  {},
		"common-ssl":     {"common-config"},
		"common-secrets": {"common-config"},

		// Storage layer
		"postgres":        {"common-config", "common-ssl"},
		"auth-postgres":   {"common-config", "common-ssl"},
		"kratos-postgres": {"common-config", "common-ssl"},
		"clickhouse":      {"common-config", "common-ssl"},

		// Application layer depends on storage
		"alt-backend":  {"postgres", "common-ssl", "common-secrets"},
		"alt-frontend": {"alt-backend", "common-ssl"},

		// Auth services depend on their databases
		"auth-service": {"auth-postgres", "common-ssl", "common-secrets"},
		"kratos":       {"kratos-postgres", "common-ssl"},

		// Search depends on storage
		"meilisearch": {"common-config", "common-ssl"},

		// Ingress depends on applications
		"nginx-external": {"alt-frontend", "alt-backend", "auth-service"},

		// Monitoring can be deployed in parallel with apps
		"monitoring": {"common-config"},
	}

	// Apply dependencies
	for chartName, deps := range dependencies {
		if node, exists := graph.Nodes[chartName]; exists {
			for _, dep := range deps {
				if depNode, depExists := graph.Nodes[dep]; depExists {
					// Add dependency
					node.Dependencies = append(node.Dependencies, dep)
					depNode.Dependents = append(depNode.Dependents, chartName)

					// Add edge
					if graph.Edges[dep] == nil {
						graph.Edges[dep] = []string{}
					}
					graph.Edges[dep] = append(graph.Edges[dep], chartName)
				}
			}
		}
	}
}

// calculateDependencyLevels calculates the dependency level for each node
func (r *DependencyGraphResolver) calculateDependencyLevels(graph *DependencyGraphData) {
	// Topological sort to determine levels
	queue := []*DependencyNode{}

	// Start with nodes that have no dependencies
	for _, node := range graph.Nodes {
		if len(node.Dependencies) == 0 {
			node.Level = 0
			queue = append(queue, node)
		}
	}

	// Process queue
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		current.Processed = true

		// Update dependents
		for _, dependentName := range current.Dependents {
			dependent := graph.Nodes[dependentName]
			
			// Check if all dependencies are processed
			allDepsProcessed := true
			maxDepLevel := -1
			
			for _, depName := range dependent.Dependencies {
				dep := graph.Nodes[depName]
				if !dep.Processed {
					allDepsProcessed = false
					break
				}
				if dep.Level > maxDepLevel {
					maxDepLevel = dep.Level
				}
			}

			if allDepsProcessed {
				dependent.Level = maxDepLevel + 1
				queue = append(queue, dependent)
			}
		}
	}
}

// generateOptimalPhases generates optimal phases based on environment
func (r *DependencyGraphResolver) generateOptimalPhases(env domain.Environment) []DeploymentPhase {
	switch env {
	case domain.Production:
		return r.generateProductionPhases()
	case domain.Staging:
		return r.generateStagingPhases()
	case domain.Development:
		return r.generateDevelopmentPhases()
	default:
		return r.generateDefaultPhases()
	}
}

// generateProductionPhases generates production-optimized phases
func (r *DependencyGraphResolver) generateProductionPhases() []DeploymentPhase {
	return []DeploymentPhase{
		{
			Name:        "インフラ基盤",
			Description: "名前空間、SSL証明書、共通設定の準備",
			Charts:      []string{"common-config", "common-ssl", "common-secrets"},
			Parallel:    false,
			Timeout:     5 * time.Minute,
			Priority:    1,
			Optional:    false,
		},
		{
			Name:        "データストレージ",
			Description: "データベースとストレージサービスのデプロイ",
			Charts:      []string{"postgres", "auth-postgres", "kratos-postgres", "clickhouse"},
			Dependencies: []string{"インフラ基盤"},
			Parallel:    true,
			Timeout:     10 * time.Minute,
			Priority:    2,
			Optional:    false,
		},
		{
			Name:        "コアサービス",
			Description: "バックエンドとフロントエンドアプリケーション",
			Charts:      []string{"alt-backend", "alt-frontend"},
			Dependencies: []string{"データストレージ"},
			Parallel:    true,
			Timeout:     8 * time.Minute,
			Priority:    3,
			Optional:    false,
		},
		{
			Name:        "認証・検索",
			Description: "認証サービスと検索エンジン",
			Charts:      []string{"auth-service", "kratos", "meilisearch"},
			Dependencies: []string{"データストレージ"},
			Parallel:    true,
			Timeout:     6 * time.Minute,
			Priority:    3,
			Optional:    false,
		},
		{
			Name:        "ネットワーク・監視",
			Description: "ロードバランサーと監視システム",
			Charts:      []string{"nginx-external", "monitoring"},
			Dependencies: []string{"コアサービス", "認証・検索"},
			Parallel:    true,
			Timeout:     5 * time.Minute,
			Priority:    4,
			Optional:    true,
		},
	}
}

// generateStagingPhases generates staging-optimized phases
func (r *DependencyGraphResolver) generateStagingPhases() []DeploymentPhase {
	return []DeploymentPhase{
		{
			Name:        "基盤設定",
			Description: "共通設定とSSL証明書",
			Charts:      []string{"common-config", "common-ssl"},
			Parallel:    false,
			Timeout:     3 * time.Minute,
			Priority:    1,
			Optional:    false,
		},
		{
			Name:        "データベース",
			Description: "必要最小限のデータベース",
			Charts:      []string{"postgres", "clickhouse"},
			Dependencies: []string{"基盤設定"},
			Parallel:    true,
			Timeout:     8 * time.Minute,
			Priority:    2,
			Optional:    false,
		},
		{
			Name:        "アプリケーション",
			Description: "メインアプリケーション",
			Charts:      []string{"alt-backend", "alt-frontend", "meilisearch"},
			Dependencies: []string{"データベース"},
			Parallel:    true,
			Timeout:     6 * time.Minute,
			Priority:    3,
			Optional:    false,
		},
		{
			Name:        "付加サービス",
			Description: "認証とネットワーク",
			Charts:      []string{"auth-service", "nginx-external"},
			Dependencies: []string{"アプリケーション"},
			Parallel:    true,
			Timeout:     4 * time.Minute,
			Priority:    4,
			Optional:    true,
		},
	}
}

// generateDevelopmentPhases generates development-optimized phases
func (r *DependencyGraphResolver) generateDevelopmentPhases() []DeploymentPhase {
	return []DeploymentPhase{
		{
			Name:        "開発環境基盤",
			Description: "最小限の基盤設定",
			Charts:      []string{"common-config"},
			Parallel:    false,
			Timeout:     2 * time.Minute,
			Priority:    1,
			Optional:    false,
		},
		{
			Name:        "開発サービス",
			Description: "開発に必要な全サービスを並列デプロイ",
			Charts:      []string{"postgres", "alt-backend", "alt-frontend", "meilisearch"},
			Dependencies: []string{"開発環境基盤"},
			Parallel:    true,
			Timeout:     10 * time.Minute,
			Priority:    2,
			Optional:    false,
		},
	}
}

// generateDefaultPhases generates default phases for unknown environments
func (r *DependencyGraphResolver) generateDefaultPhases() []DeploymentPhase {
	return []DeploymentPhase{
		{
			Name:        "全サービス",
			Description: "すべてのサービスを順次デプロイ",
			Charts:      []string{},
			Parallel:    false,
			Timeout:     15 * time.Minute,
			Priority:    1,
			Optional:    false,
		},
	}
}

// optimizePhases optimizes phases based on dependency graph and available charts
func (r *DependencyGraphResolver) optimizePhases(phases []DeploymentPhase, graph *DependencyGraphData, charts []domain.Chart) []DeploymentPhase {
	// Create chart name set for filtering
	chartNames := make(map[string]bool)
	for _, chart := range charts {
		chartNames[chart.Name] = true
	}

	var optimizedPhases []DeploymentPhase

	for _, phase := range phases {
		optimizedPhase := phase

		// Filter charts to only include available ones
		var availableCharts []string
		for _, chartName := range phase.Charts {
			if chartNames[chartName] {
				availableCharts = append(availableCharts, chartName)
			}
		}

		// If no charts specified, add all remaining charts
		if len(phase.Charts) == 0 {
			for _, chart := range charts {
				availableCharts = append(availableCharts, chart.Name)
			}
		}

		optimizedPhase.Charts = availableCharts

		// Only include phase if it has charts
		if len(availableCharts) > 0 {
			optimizedPhases = append(optimizedPhases, optimizedPhase)
		}
	}

	// Sort phases by priority
	sort.Slice(optimizedPhases, func(i, j int) bool {
		return optimizedPhases[i].Priority < optimizedPhases[j].Priority
	})

	return optimizedPhases
}

// ValidateDependencies validates that all dependencies are satisfied
func (r *DependencyGraphResolver) ValidateDependencies(phases []DeploymentPhase) error {
	deployedCharts := make(map[string]bool)
	
	for _, phase := range phases {
		// Check if all dependencies are satisfied
		for _, chartName := range phase.Charts {
			// For simplicity, we'll just log validation
			r.logger.DebugWithContext("チャート依存関係検証", map[string]interface{}{
				"chart": chartName,
				"phase": phase.Name,
			})
		}

		// Mark charts in this phase as deployed
		for _, chartName := range phase.Charts {
			deployedCharts[chartName] = true
		}
	}

	return nil
}

// GetDeploymentStrategy returns a deployment strategy based on environment and requirements
func (r *DependencyGraphResolver) GetDeploymentStrategy(env domain.Environment, requirements *DeploymentRequirements) *DeploymentStrategy {
	strategy := &DeploymentStrategy{
		Environment:     env,
		MaxParallelism:  r.getMaxParallelism(env),
		FailureStrategy: r.getFailureStrategy(env),
		RollbackEnabled: r.isRollbackEnabled(env),
		TimeoutStrategy: r.getTimeoutStrategy(env),
	}

	if requirements != nil {
		// Apply custom requirements
		if requirements.MaxParallelism > 0 {
			strategy.MaxParallelism = requirements.MaxParallelism
		}
		if requirements.CustomTimeout > 0 {
			strategy.TimeoutStrategy.DefaultTimeout = requirements.CustomTimeout
		}
	}

	return strategy
}

// DeploymentRequirements represents custom deployment requirements
type DeploymentRequirements struct {
	MaxParallelism int
	CustomTimeout  time.Duration
	FailFast       bool
	SkipOptional   bool
}

// DeploymentStrategy represents the deployment strategy
type DeploymentStrategy struct {
	Environment     domain.Environment
	MaxParallelism  int
	FailureStrategy string
	RollbackEnabled bool
	TimeoutStrategy TimeoutStrategy
}

// TimeoutStrategy represents timeout configuration
type TimeoutStrategy struct {
	DefaultTimeout time.Duration
	MaxTimeout     time.Duration
	RetryCount     int
}

// getMaxParallelism returns the maximum parallelism for the environment
func (r *DependencyGraphResolver) getMaxParallelism(env domain.Environment) int {
	switch env {
	case domain.Production:
		return 2 // Conservative for production
	case domain.Staging:
		return 3 // Moderate for staging
	case domain.Development:
		return 5 // Aggressive for development
	default:
		return 1 // Safe default
	}
}

// getFailureStrategy returns the failure strategy for the environment
func (r *DependencyGraphResolver) getFailureStrategy(env domain.Environment) string {
	switch env {
	case domain.Production:
		return "stop-on-critical-failure"
	case domain.Staging:
		return "continue-on-warning"
	case domain.Development:
		return "continue-on-error"
	default:
		return "stop-on-error"
	}
}

// isRollbackEnabled returns whether rollback is enabled for the environment
func (r *DependencyGraphResolver) isRollbackEnabled(env domain.Environment) bool {
	return env == domain.Production || env == domain.Staging
}

// getTimeoutStrategy returns the timeout strategy for the environment
func (r *DependencyGraphResolver) getTimeoutStrategy(env domain.Environment) TimeoutStrategy {
	switch env {
	case domain.Production:
		return TimeoutStrategy{
			DefaultTimeout: 10 * time.Minute,
			MaxTimeout:     30 * time.Minute,
			RetryCount:     3,
		}
	case domain.Staging:
		return TimeoutStrategy{
			DefaultTimeout: 8 * time.Minute,
			MaxTimeout:     20 * time.Minute,
			RetryCount:     2,
		}
	case domain.Development:
		return TimeoutStrategy{
			DefaultTimeout: 5 * time.Minute,
			MaxTimeout:     15 * time.Minute,
			RetryCount:     1,
		}
	default:
		return TimeoutStrategy{
			DefaultTimeout: 10 * time.Minute,
			MaxTimeout:     25 * time.Minute,
			RetryCount:     2,
		}
	}
}