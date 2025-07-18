package deployment_usecase

import (
	"fmt"
	"deploy-cli/domain"
	"deploy-cli/port/logger_port"
)

// StrategyFactory creates deployment strategies based on environment or explicit strategy selection
type StrategyFactory struct {
	logger logger_port.LoggerPort
}

// NewStrategyFactory creates a new strategy factory
func NewStrategyFactory(logger logger_port.LoggerPort) *StrategyFactory {
	return &StrategyFactory{
		logger: logger,
	}
}

// CreateStrategy creates a deployment strategy based on environment
func (f *StrategyFactory) CreateStrategy(env domain.Environment) (domain.DeploymentStrategy, error) {
	switch env {
	case domain.Development:
		f.logger.InfoWithContext("creating development deployment strategy", map[string]interface{}{
			"environment": env.String(),
			"strategy": "development",
		})
		return &domain.DevelopmentStrategy{}, nil
	case domain.Staging:
		f.logger.InfoWithContext("creating staging deployment strategy", map[string]interface{}{
			"environment": env.String(),
			"strategy": "staging",
		})
		return &domain.StagingStrategy{}, nil
	case domain.Production:
		f.logger.InfoWithContext("creating production deployment strategy", map[string]interface{}{
			"environment": env.String(),
			"strategy": "production",
		})
		return &domain.ProductionStrategy{}, nil
	default:
		return nil, fmt.Errorf("unsupported environment: %s", env.String())
	}
}

// CreateStrategyByName creates a deployment strategy by explicit name
func (f *StrategyFactory) CreateStrategyByName(strategyName string) (domain.DeploymentStrategy, error) {
	switch strategyName {
	case "development":
		f.logger.InfoWithContext("creating development deployment strategy", map[string]interface{}{
			"strategy": strategyName,
		})
		return &domain.DevelopmentStrategy{}, nil
	case "staging":
		f.logger.InfoWithContext("creating staging deployment strategy", map[string]interface{}{
			"strategy": strategyName,
		})
		return &domain.StagingStrategy{}, nil
	case "production":
		f.logger.InfoWithContext("creating production deployment strategy", map[string]interface{}{
			"strategy": strategyName,
		})
		return &domain.ProductionStrategy{}, nil
	case "disaster-recovery":
		f.logger.InfoWithContext("creating disaster recovery deployment strategy", map[string]interface{}{
			"strategy": strategyName,
		})
		return &domain.DisasterRecoveryStrategy{}, nil
	default:
		return nil, fmt.Errorf("unsupported strategy: %s", strategyName)
	}
}

// ValidateStrategyForEnvironment validates if a strategy is compatible with an environment
func (f *StrategyFactory) ValidateStrategyForEnvironment(strategy domain.DeploymentStrategy, env domain.Environment) error {
	// Allow disaster recovery strategy for production environment
	if strategy.GetName() == "disaster-recovery" && env == domain.Production {
		return nil
	}
	
	// For other strategies, environment must match
	if strategy.GetEnvironment() != env {
		return fmt.Errorf("strategy '%s' is not compatible with environment '%s'", strategy.GetName(), env.String())
	}
	
	return nil
}

// GetAvailableStrategies returns all available strategies
func (f *StrategyFactory) GetAvailableStrategies() []string {
	return []string{
		"development",
		"staging", 
		"production",
		"disaster-recovery",
	}
}

// GetStrategyDescription returns a description of the strategy
func (f *StrategyFactory) GetStrategyDescription(strategyName string) (string, error) {
	descriptions := map[string]string{
		"development": "Fast deployment with minimal health checks, parallel processing, and reduced timeouts. Optimized for development speed.",
		"staging": "Comprehensive validation with extended health checks, full service deployment, and thorough dependency verification. Optimized for testing.",
		"production": "Conservative, sequential deployment with full validation, extended timeouts, and zero-downtime patterns. Optimized for reliability.",
		"disaster-recovery": "Emergency deployment of critical services only with rapid startup and minimal dependency validation. Optimized for recovery speed.",
	}
	
	description, exists := descriptions[strategyName]
	if !exists {
		return "", fmt.Errorf("unknown strategy: %s", strategyName)
	}
	
	return description, nil
}

// GetRecommendedStrategy returns the recommended strategy for an environment
func (f *StrategyFactory) GetRecommendedStrategy(env domain.Environment) string {
	switch env {
	case domain.Development:
		return "development"
	case domain.Staging:
		return "staging"
	case domain.Production:
		return "production"
	default:
		return "development" // Default fallback
	}
}

// GetStrategy creates a deployment strategy based on environment (alias for CreateStrategy)
func (f *StrategyFactory) GetStrategy(env domain.Environment) domain.DeploymentStrategy {
	strategy, err := f.CreateStrategy(env)
	if err != nil {
		// Fallback to development strategy if creation fails
		f.logger.WarnWithContext("failed to create strategy, falling back to development", map[string]interface{}{
			"environment": env.String(),
			"error": err.Error(),
		})
		devStrategy, _ := f.CreateStrategy(domain.Development)
		return devStrategy
	}
	return strategy
}