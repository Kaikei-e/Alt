package deployment_usecase

import (
	"fmt"
	"time"

	"deploy-cli/domain"
	"deploy-cli/port/logger_port"
)

// DeploymentStrategyUsecase handles deployment strategy management, configuration, and Helm control
type DeploymentStrategyUsecase struct {
	strategyFactory *StrategyFactory
	logger          logger_port.LoggerPort
}

// NewDeploymentStrategyUsecase creates a new deployment strategy usecase
func NewDeploymentStrategyUsecase(
	strategyFactory *StrategyFactory,
	logger logger_port.LoggerPort,
) *DeploymentStrategyUsecase {
	return &DeploymentStrategyUsecase{
		strategyFactory: strategyFactory,
		logger:          logger,
	}
}

// setupDeploymentStrategy sets up the deployment strategy for the given options
func (u *DeploymentStrategyUsecase) setupDeploymentStrategy(options *domain.DeploymentOptions) error {
	// Skip if strategy is already set
	if options.HasDeploymentStrategy() {
		u.logger.InfoWithContext("deployment strategy already set", map[string]interface{}{
			"strategy": options.GetDeploymentStrategy().GetName(),
		})
		return nil
	}

	var strategy domain.DeploymentStrategy
	var err error

	// Use explicit strategy name if provided
	if options.StrategyName != "" {
		strategy, err = u.strategyFactory.CreateStrategyByName(options.StrategyName)
		if err != nil {
			return fmt.Errorf("failed to create strategy by name '%s': %w", options.StrategyName, err)
		}

		// Validate strategy compatibility with environment
		if err := u.strategyFactory.ValidateStrategyForEnvironment(strategy, options.Environment); err != nil {
			return fmt.Errorf("strategy validation failed: %w", err)
		}
	} else {
		// Use environment-based strategy selection
		strategy, err = u.strategyFactory.CreateStrategy(options.Environment)
		if err != nil {
			return fmt.Errorf("failed to create strategy for environment '%s': %w", options.Environment, err)
		}
	}

	// Set the strategy
	options.SetDeploymentStrategy(strategy)

	u.logger.InfoWithContext("deployment strategy configured", map[string]interface{}{
		"strategy":             strategy.GetName(),
		"environment":          options.Environment.String(),
		"global_timeout":       strategy.GetGlobalTimeout(),
		"allows_parallel":      strategy.AllowsParallelDeployment(),
		"health_check_retries": strategy.GetHealthCheckRetries(),
		"zero_downtime":        strategy.RequiresZeroDowntime(),
	})

	return nil
}

// getLayerConfigurations returns layer configurations based on deployment strategy
func (u *DeploymentStrategyUsecase) getLayerConfigurations(options *domain.DeploymentOptions) []domain.LayerConfiguration {
	if options.HasDeploymentStrategy() {
		u.logger.InfoWithContext("using strategy-based layer configurations", map[string]interface{}{
			"strategy": options.GetStrategyName(),
		})
		return options.GetLayerConfigurations()
	}

	// Fallback to default layer configurations
	u.logger.InfoWithContext("using default layer configurations", map[string]interface{}{
		"environment": options.Environment.String(),
	})
	return u.getDefaultLayerConfigurations(nil, options.ChartsDir)
}

// getDefaultLayerConfigurations returns the default layer configurations for backwards compatibility
func (u *DeploymentStrategyUsecase) getDefaultLayerConfigurations(chartConfig *domain.ChartConfig, chartsDir string) []domain.LayerConfiguration {
	return []domain.LayerConfiguration{
		{
			Name: "Storage & Persistent Infrastructure",
			Charts: []domain.Chart{
				{Name: "postgres", Type: domain.InfrastructureChart, Path: chartsDir + "/postgres", WaitReady: true},
				{Name: "auth-postgres", Type: domain.InfrastructureChart, Path: chartsDir + "/auth-postgres", WaitReady: true},
				{Name: "kratos-postgres", Type: domain.InfrastructureChart, Path: chartsDir + "/kratos-postgres", WaitReady: true},
				{Name: "clickhouse", Type: domain.InfrastructureChart, Path: chartsDir + "/clickhouse", WaitReady: true},
				{Name: "meilisearch", Type: domain.InfrastructureChart, Path: chartsDir + "/meilisearch", WaitReady: true},
			},
			RequiresHealthCheck:     true,
			HealthCheckTimeout:      15 * time.Minute,
			WaitBetweenCharts:       30 * time.Second,
			LayerCompletionTimeout:  20 * time.Minute,
			AllowParallelDeployment: false,
			CriticalLayer:           true,
		},
		{
			Name: "Core Services",
			Charts: []domain.Chart{
				{Name: "kratos", Type: domain.ApplicationChart, Path: chartsDir + "/kratos", WaitReady: true},
				{Name: "auth-service", Type: domain.ApplicationChart, Path: chartsDir + "/auth-service", WaitReady: true},
				{Name: "alt-backend", Type: domain.ApplicationChart, Path: chartsDir + "/alt-backend", WaitReady: true},
			},
			RequiresHealthCheck:     true,
			HealthCheckTimeout:      10 * time.Minute,
			WaitBetweenCharts:       15 * time.Second,
			LayerCompletionTimeout:  15 * time.Minute,
			AllowParallelDeployment: true,
			CriticalLayer:           true,
		},
		{
			Name: "Applications & Processing",
			Charts: []domain.Chart{
				{Name: "alt-frontend", Type: domain.ApplicationChart, Path: chartsDir + "/alt-frontend", WaitReady: true},
				{Name: "pre-processor", Type: domain.ApplicationChart, Path: chartsDir + "/pre-processor", WaitReady: true},
				{Name: "search-indexer", Type: domain.ApplicationChart, Path: chartsDir + "/search-indexer", WaitReady: true},
				{Name: "tag-generator", Type: domain.ApplicationChart, Path: chartsDir + "/tag-generator", WaitReady: true},
				{Name: "news-creator", Type: domain.ApplicationChart, Path: chartsDir + "/news-creator", WaitReady: true},
			},
			RequiresHealthCheck:     true,
			HealthCheckTimeout:      8 * time.Minute,
			WaitBetweenCharts:       10 * time.Second,
			LayerCompletionTimeout:  12 * time.Minute,
			AllowParallelDeployment: true,
			CriticalLayer:           false,
		},
		{
			Name: "Ingress & Networking",
			Charts: []domain.Chart{
				{Name: "nginx", Type: domain.InfrastructureChart, Path: chartsDir + "/nginx", WaitReady: true},
				{Name: "nginx-external", Type: domain.InfrastructureChart, Path: chartsDir + "/nginx-external", WaitReady: true},
			},
			RequiresHealthCheck:     true,
			HealthCheckTimeout:      5 * time.Minute,
			WaitBetweenCharts:       5 * time.Second,
			LayerCompletionTimeout:  8 * time.Minute,
			AllowParallelDeployment: true,
			CriticalLayer:           false,
		},
		{
			Name: "Operational & Monitoring",
			Charts: []domain.Chart{
				{Name: "rask-log-forwarder", Type: domain.OperationalChart, Path: chartsDir + "/rask-log-forwarder", WaitReady: false},
				{Name: "rask-log-aggregator", Type: domain.OperationalChart, Path: chartsDir + "/rask-log-aggregator", WaitReady: false},
			},
			RequiresHealthCheck:     false,
			HealthCheckTimeout:      3 * time.Minute,
			WaitBetweenCharts:       5 * time.Second,
			LayerCompletionTimeout:  5 * time.Minute,
			AllowParallelDeployment: true,
			CriticalLayer:           false,
		},
	}
}

// getAllCharts returns all charts from the deployment strategy
func (u *DeploymentStrategyUsecase) getAllCharts(options *domain.DeploymentOptions) []domain.Chart {
	layerConfigs := u.getLayerConfigurations(options)

	var allCharts []domain.Chart
	for _, layerConfig := range layerConfigs {
		allCharts = append(allCharts, layerConfig.Charts...)
	}

	return allCharts
}

// validateStrategyCompatibility validates that the strategy is compatible with the environment
func (u *DeploymentStrategyUsecase) validateStrategyCompatibility(strategy domain.DeploymentStrategy, env domain.Environment) error {
	return u.strategyFactory.ValidateStrategyForEnvironment(strategy, env)
}

// getStrategyRecommendations returns strategy recommendations for the environment
func (u *DeploymentStrategyUsecase) getStrategyRecommendations(env domain.Environment) (string, string, error) {
	recommendedStrategy := u.strategyFactory.GetRecommendedStrategy(env)
	description, err := u.strategyFactory.GetStrategyDescription(recommendedStrategy)
	if err != nil {
		return "", "", fmt.Errorf("failed to get strategy description: %w", err)
	}

	return recommendedStrategy, description, nil
}

// getAvailableStrategies returns all available deployment strategies
func (u *DeploymentStrategyUsecase) getAvailableStrategies() []string {
	return u.strategyFactory.GetAvailableStrategies()
}

// analyzeStrategyPerformance analyzes the performance characteristics of a strategy
func (u *DeploymentStrategyUsecase) analyzeStrategyPerformance(strategy domain.DeploymentStrategy) map[string]interface{} {
	return map[string]interface{}{
		"strategy_name":          strategy.GetName(),
		"environment":            strategy.GetEnvironment().String(),
		"global_timeout":         strategy.GetGlobalTimeout(),
		"allows_parallel":        strategy.AllowsParallelDeployment(),
		"health_check_retries":   strategy.GetHealthCheckRetries(),
		"zero_downtime_required": strategy.RequiresZeroDowntime(),
		"estimated_duration":     u.estimateDeploymentDuration(strategy),
		"risk_level":             u.assessRiskLevel(strategy),
		"recovery_time":          u.estimateRecoveryTime(strategy),
	}
}

// estimateDeploymentDuration estimates how long a deployment will take with this strategy
func (u *DeploymentStrategyUsecase) estimateDeploymentDuration(strategy domain.DeploymentStrategy) time.Duration {
	baseTime := 10 * time.Minute // Base deployment time

	// Adjust based on strategy characteristics
	if strategy.RequiresZeroDowntime() {
		baseTime += 5 * time.Minute // Extra time for zero-downtime procedures
	}

	if strategy.AllowsParallelDeployment() {
		baseTime = baseTime * 2 / 3 // Reduce time for parallel deployment
	}

	// Add timeout buffer
	baseTime += strategy.GetGlobalTimeout() / 4

	return baseTime
}

// assessRiskLevel assesses the risk level of a deployment strategy
func (u *DeploymentStrategyUsecase) assessRiskLevel(strategy domain.DeploymentStrategy) string {
	switch strategy.GetEnvironment() {
	case domain.Development:
		return "low"
	case domain.Staging:
		return "medium"
	case domain.Production:
		if strategy.RequiresZeroDowntime() {
			return "low"
		}
		return "high"
	default:
		return "unknown"
	}
}

// estimateRecoveryTime estimates recovery time in case of deployment failure
func (u *DeploymentStrategyUsecase) estimateRecoveryTime(strategy domain.DeploymentStrategy) time.Duration {
	baseRecovery := 5 * time.Minute

	// Production environments have longer recovery times due to careful procedures
	if strategy.GetEnvironment() == domain.Production {
		baseRecovery = 15 * time.Minute
	}

	// Zero-downtime strategies have faster recovery
	if strategy.RequiresZeroDowntime() {
		baseRecovery = baseRecovery * 2 / 3
	}

	return baseRecovery
}

// optimizeStrategyForEnvironment optimizes strategy selection for specific environment needs
func (u *DeploymentStrategyUsecase) optimizeStrategyForEnvironment(env domain.Environment, requirements map[string]interface{}) (string, error) {
	u.logger.InfoWithContext("optimizing strategy for environment", map[string]interface{}{
		"environment":  env.String(),
		"requirements": requirements,
	})

	// Check if specific requirements are provided
	if speedRequired, exists := requirements["speed_required"]; exists && speedRequired.(bool) {
		if env == domain.Development {
			return "development", nil
		} else if env == domain.Staging {
			return "staging", nil
		}
	}

	if reliabilityRequired, exists := requirements["reliability_required"]; exists && reliabilityRequired.(bool) {
		if env == domain.Production {
			return "production", nil
		}
	}

	if emergencyMode, exists := requirements["emergency_mode"]; exists && emergencyMode.(bool) {
		if env == domain.Production {
			return "disaster-recovery", nil
		}
	}

	// Fall back to recommended strategy
	return u.strategyFactory.GetRecommendedStrategy(env), nil
}

// validateStrategyConfiguration validates the current strategy configuration
func (u *DeploymentStrategyUsecase) validateStrategyConfiguration(options *domain.DeploymentOptions) error {
	if !options.HasDeploymentStrategy() {
		return fmt.Errorf("no deployment strategy configured")
	}

	strategy := options.GetDeploymentStrategy()

	// Validate strategy compatibility with environment
	if err := u.validateStrategyCompatibility(strategy, options.Environment); err != nil {
		return fmt.Errorf("strategy compatibility validation failed: %w", err)
	}

	// Validate strategy-specific requirements
	if err := u.validateStrategyRequirements(strategy, options); err != nil {
		return fmt.Errorf("strategy requirements validation failed: %w", err)
	}

	u.logger.InfoWithContext("strategy configuration validated successfully", map[string]interface{}{
		"strategy":    strategy.GetName(),
		"environment": options.Environment.String(),
	})

	return nil
}

// validateStrategyRequirements validates strategy-specific requirements
func (u *DeploymentStrategyUsecase) validateStrategyRequirements(strategy domain.DeploymentStrategy, options *domain.DeploymentOptions) error {
	// Check if required resources are available for the strategy
	if strategy.RequiresZeroDowntime() && options.ForceUpdate {
		u.logger.WarnWithContext("zero-downtime strategy with force update may cause brief downtime", map[string]interface{}{
			"strategy": strategy.GetName(),
		})
	}

	// Validate timeout configurations
	if strategy.GetGlobalTimeout() < 1*time.Minute {
		return fmt.Errorf("global timeout too short for strategy %s", strategy.GetName())
	}

	// Validate health check retries
	if strategy.GetHealthCheckRetries() < 1 {
		return fmt.Errorf("health check retries must be at least 1 for strategy %s", strategy.GetName())
	}

	return nil
}

// manageHelmControl manages Helm deployment control based on strategy
func (u *DeploymentStrategyUsecase) manageHelmControl(strategy domain.DeploymentStrategy, chartName string) (map[string]interface{}, error) {
	u.logger.InfoWithContext("managing Helm control for strategy", map[string]interface{}{
		"strategy":   strategy.GetName(),
		"chart_name": chartName,
	})

	helmSettings := map[string]interface{}{
		"timeout":                    strategy.GetGlobalTimeout(),
		"wait":                       true,
		"wait_for_jobs":              true,
		"disable_hooks":              false,
		"force":                      false,
		"recreate_pods":              false,
		"reset_values":               false,
		"reuse_values":               false,
		"cleanup_on_fail":            true,
		"atomic":                     strategy.RequiresZeroDowntime(),
		"skip_crds":                  false,
		"render_subchart_notes":      false,
		"disable_openapi_validation": false,
		"include_crds":               true,
		"create_namespace":           true,
		"dependency_update":          true,
	}

	// Adjust settings based on strategy
	switch strategy.GetName() {
	case "development":
		helmSettings["timeout"] = 5 * time.Minute
		helmSettings["wait"] = false
		helmSettings["atomic"] = false
		helmSettings["cleanup_on_fail"] = false

	case "staging":
		helmSettings["timeout"] = 10 * time.Minute
		helmSettings["wait"] = true
		helmSettings["atomic"] = false
		helmSettings["cleanup_on_fail"] = true

	case "production":
		helmSettings["timeout"] = 20 * time.Minute
		helmSettings["wait"] = true
		helmSettings["atomic"] = true
		helmSettings["cleanup_on_fail"] = true
		helmSettings["recreate_pods"] = false

	case "disaster-recovery":
		helmSettings["timeout"] = 3 * time.Minute
		helmSettings["wait"] = false
		helmSettings["atomic"] = false
		helmSettings["cleanup_on_fail"] = false
		helmSettings["force"] = true
	}

	u.logger.InfoWithContext("Helm control settings configured", map[string]interface{}{
		"strategy":      strategy.GetName(),
		"chart_name":    chartName,
		"helm_settings": helmSettings,
	})

	return helmSettings, nil
}

// getStrategyMetrics returns metrics about strategy performance
func (u *DeploymentStrategyUsecase) getStrategyMetrics(strategy domain.DeploymentStrategy) map[string]interface{} {
	return map[string]interface{}{
		"strategy_name":       strategy.GetName(),
		"environment":         strategy.GetEnvironment().String(),
		"performance_score":   u.calculatePerformanceScore(strategy),
		"reliability_score":   u.calculateReliabilityScore(strategy),
		"speed_score":         u.calculateSpeedScore(strategy),
		"resource_efficiency": u.calculateResourceEfficiency(strategy),
		"estimated_duration":  u.estimateDeploymentDuration(strategy),
		"risk_assessment":     u.assessRiskLevel(strategy),
	}
}

// calculatePerformanceScore calculates overall performance score for a strategy
func (u *DeploymentStrategyUsecase) calculatePerformanceScore(strategy domain.DeploymentStrategy) float64 {
	score := 50.0 // Base score

	if strategy.AllowsParallelDeployment() {
		score += 20.0
	}

	if strategy.RequiresZeroDowntime() {
		score += 15.0
	}

	// Adjust based on environment
	switch strategy.GetEnvironment() {
	case domain.Development:
		score += 10.0 // Development optimized
	case domain.Production:
		score += 15.0 // Production optimized
	}

	return score
}

// calculateReliabilityScore calculates reliability score for a strategy
func (u *DeploymentStrategyUsecase) calculateReliabilityScore(strategy domain.DeploymentStrategy) float64 {
	score := 50.0 // Base score

	if strategy.RequiresZeroDowntime() {
		score += 25.0
	}

	score += float64(strategy.GetHealthCheckRetries()) * 5.0

	// Production strategies are more reliable
	if strategy.GetEnvironment() == domain.Production {
		score += 20.0
	}

	return score
}

// calculateSpeedScore calculates speed score for a strategy
func (u *DeploymentStrategyUsecase) calculateSpeedScore(strategy domain.DeploymentStrategy) float64 {
	score := 50.0 // Base score

	if strategy.AllowsParallelDeployment() {
		score += 30.0
	}

	// Shorter timeouts = higher speed score
	timeoutMinutes := float64(strategy.GetGlobalTimeout().Minutes())
	if timeoutMinutes < 10 {
		score += 20.0
	} else if timeoutMinutes < 20 {
		score += 10.0
	}

	// Development strategies are faster
	if strategy.GetEnvironment() == domain.Development {
		score += 15.0
	}

	return score
}

// calculateResourceEfficiency calculates resource efficiency score for a strategy
func (u *DeploymentStrategyUsecase) calculateResourceEfficiency(strategy domain.DeploymentStrategy) float64 {
	score := 50.0 // Base score

	if strategy.AllowsParallelDeployment() {
		score += 20.0 // Parallel deployment is more resource efficient
	}

	// Shorter timeouts = better resource efficiency
	timeoutMinutes := float64(strategy.GetGlobalTimeout().Minutes())
	if timeoutMinutes < 10 {
		score += 15.0
	} else if timeoutMinutes > 20 {
		score -= 10.0
	}

	// Development strategies are more resource efficient
	if strategy.GetEnvironment() == domain.Development {
		score += 15.0
	}

	return score
}
