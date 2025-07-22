package orchestration

import (
	"context"
	"fmt"
	"time"

	"deploy-cli/domain"
	"deploy-cli/port/logger_port"
)

// StrategyFactory creates deployment strategies based on options
type StrategyFactory struct {
	logger logger_port.LoggerPort
}

// StrategyFactoryPort defines the interface for strategy factory
type StrategyFactoryPort interface {
	CreateStrategy(ctx context.Context, strategyType string, options *domain.DeploymentOptions) (domain.DeploymentStrategy, error)
	GetAvailableStrategies(ctx context.Context) ([]string, error)
	ValidateStrategy(ctx context.Context, strategy domain.DeploymentStrategy) error
}

// NewStrategyFactory creates a new strategy factory
func NewStrategyFactory(logger logger_port.LoggerPort) *StrategyFactory {
	return &StrategyFactory{
		logger: logger,
	}
}

// CreateStrategy creates a deployment strategy based on the strategy type and options
func (s *StrategyFactory) CreateStrategy(ctx context.Context, strategyType string, options *domain.DeploymentOptions) (domain.DeploymentStrategy, error) {
	s.logger.DebugWithContext("creating deployment strategy", map[string]interface{}{
		"strategy_type": strategyType,
		"environment":   options.Environment,
	})

	var strategy domain.DeploymentStrategy
	var err error

	switch strategyType {
	case "sequential":
		strategy = s.createSequentialStrategy(options)
	case "parallel":
		strategy = s.createParallelStrategy(options)
	case "layer_aware":
		strategy = s.createLayerAwareStrategy(options)
	case "blue_green":
		strategy = s.createBlueGreenStrategy(options)
	default:
		err = fmt.Errorf("unsupported strategy type: %s", strategyType)
	}

	if err != nil {
		s.logger.ErrorWithContext("failed to create deployment strategy", map[string]interface{}{
			"strategy_type": strategyType,
			"error":         err.Error(),
		})
		return nil, err
	}

	// Validate the created strategy
	if err := s.ValidateStrategy(ctx, strategy); err != nil {
		s.logger.ErrorWithContext("created strategy validation failed", map[string]interface{}{
			"strategy_type": strategyType,
			"error":         err.Error(),
		})
		return nil, fmt.Errorf("strategy validation failed: %w", err)
	}

	s.logger.DebugWithContext("deployment strategy created successfully", map[string]interface{}{
		"strategy_name": strategy.GetName(),
	})

	return strategy, nil
}

// GetAvailableStrategies returns a list of available deployment strategies
func (s *StrategyFactory) GetAvailableStrategies(ctx context.Context) ([]string, error) {
	strategies := []string{
		"sequential",
		"parallel", 
		"layer_aware",
		"blue_green",
	}

	s.logger.DebugWithContext("returning available deployment strategies", map[string]interface{}{
		"strategy_count": len(strategies),
		"strategies":     strategies,
	})

	return strategies, nil
}

// ValidateStrategy validates a deployment strategy
func (s *StrategyFactory) ValidateStrategy(ctx context.Context, strategy domain.DeploymentStrategy) error {
	if strategy == nil {
		return fmt.Errorf("strategy cannot be nil")
	}

	if strategy.GetName() == "" {
		return fmt.Errorf("strategy name cannot be empty")
	}

	s.logger.DebugWithContext("deployment strategy validated", map[string]interface{}{
		"strategy_name": strategy.GetName(),
	})

	return nil
}

// Strategy creation methods

func (s *StrategyFactory) createSequentialStrategy(options *domain.DeploymentOptions) domain.DeploymentStrategy {
	// Return a concrete implementation that matches the environment
	switch options.Environment {
	case domain.Development:
		return &domain.DevelopmentStrategy{}
	case domain.Staging:
		return &domain.StagingStrategy{}
	case domain.Production:
		return &domain.ProductionStrategy{}
	default:
		return &domain.DevelopmentStrategy{}
	}
}

func (s *StrategyFactory) createParallelStrategy(options *domain.DeploymentOptions) domain.DeploymentStrategy {
	// Return a concrete implementation for parallel deployment
	return &domain.DevelopmentStrategy{} // Use dev strategy as a parallel-friendly default
}

func (s *StrategyFactory) createLayerAwareStrategy(options *domain.DeploymentOptions) domain.DeploymentStrategy {
	// Return production strategy for layer-aware deployment
	return &domain.ProductionStrategy{}
}

func (s *StrategyFactory) createBlueGreenStrategy(options *domain.DeploymentOptions) domain.DeploymentStrategy {
	// Return production strategy for blue-green deployment
	return &domain.ProductionStrategy{}
}

// Helper methods

func (s *StrategyFactory) calculateTimeout(options *domain.DeploymentOptions, baseTimeout time.Duration) time.Duration {
	timeout := baseTimeout

	// Adjust timeout based on deployment options
	if options.Environment == domain.Production {
		timeout = timeout * 2 // Longer for production deployments
	}

	// Ensure minimum timeout
	if timeout < 5*time.Minute {
		timeout = 5 * time.Minute
	}

	return timeout
}

func (s *StrategyFactory) createDefaultValidationRules() []domain.ValidationRule {
	return []domain.ValidationRule{
		{
			Name:        "health_check",
			Type:        "health_check",
			Target:      "deployment",
			Timeout:     5 * time.Minute,
			Required:    true,
			Description: "Verify deployment health after completion",
		},
		{
			Name:        "resource_ready",
			Type:        "resource_ready",
			Target:      "pods",
			Timeout:     3 * time.Minute,
			Required:    true,
			Description: "Ensure all pods are ready",
		},
	}
}

func (s *StrategyFactory) createLayerAwareValidationRules() []domain.ValidationRule {
	rules := s.createDefaultValidationRules()
	
	// Add layer-specific validation
	layerRule := domain.ValidationRule{
		Name:        "layer_dependency",
		Type:        "custom",
		Target:      "layer",
		Timeout:     10 * time.Minute,
		Required:    true,
		Description: "Validate layer dependencies are satisfied",
		Parameters: map[string]interface{}{
			"check_dependencies": true,
			"wait_for_ready":     true,
		},
	}
	
	return append(rules, layerRule)
}

func (s *StrategyFactory) createBlueGreenValidationRules() []domain.ValidationRule {
	rules := s.createDefaultValidationRules()
	
	// Add blue-green specific validations
	trafficRule := domain.ValidationRule{
		Name:        "traffic_validation",
		Type:        "custom",
		Target:      "service",
		Timeout:     5 * time.Minute,
		Required:    true,
		Description: "Validate traffic routing for blue-green deployment",
		Parameters: map[string]interface{}{
			"validate_traffic_split": true,
			"health_check_endpoint":  "/health",
		},
	}
	
	return append(rules, trafficRule)
}