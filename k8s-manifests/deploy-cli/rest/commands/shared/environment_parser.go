// PHASE R3: Shared environment parsing utilities
package shared

import (
	"fmt"

	"deploy-cli/domain"
)

// EnvironmentParser provides shared environment parsing functionality
type EnvironmentParser struct {
	shared *CommandShared
}

// NewEnvironmentParser creates a new environment parser
func NewEnvironmentParser(shared *CommandShared) *EnvironmentParser {
	return &EnvironmentParser{
		shared: shared,
	}
}

// ParseEnvironment parses environment from command arguments
func (e *EnvironmentParser) ParseEnvironment(args []string) (domain.Environment, error) {
	var env domain.Environment = domain.Development
	
	if len(args) > 0 {
		parsedEnv, err := domain.ParseEnvironment(args[0])
		if err != nil {
			return "", fmt.Errorf("invalid environment '%s': %w", args[0], err)
		}
		env = parsedEnv
	}

	e.shared.Logger.InfoWithContext("environment parsed", map[string]interface{}{
		"environment": env,
	})

	return env, nil
}

// ParseEnvironmentWithDefault parses environment with a custom default
func (e *EnvironmentParser) ParseEnvironmentWithDefault(args []string, defaultEnv domain.Environment) (domain.Environment, error) {
	var env domain.Environment = defaultEnv
	
	if len(args) > 0 {
		parsedEnv, err := domain.ParseEnvironment(args[0])
		if err != nil {
			return "", fmt.Errorf("invalid environment '%s': %w", args[0], err)
		}
		env = parsedEnv
	}

	e.shared.Logger.InfoWithContext("environment parsed with default", map[string]interface{}{
		"environment": env,
		"default":     defaultEnv,
		"args_count":  len(args),
	})

	return env, nil
}

// ValidateEnvironment validates that the environment is allowed for the operation
func (e *EnvironmentParser) ValidateEnvironment(env domain.Environment, allowedEnvs []domain.Environment) error {
	for _, allowed := range allowedEnvs {
		if env == allowed {
			return nil
		}
	}
	
	return fmt.Errorf("environment '%s' is not allowed for this operation. Allowed: %v", env, allowedEnvs)
}

// IsProductionEnvironment checks if the environment is production
func (e *EnvironmentParser) IsProductionEnvironment(env domain.Environment) bool {
	return env == domain.Production
}

// IsDevelopmentEnvironment checks if the environment is development
func (e *EnvironmentParser) IsDevelopmentEnvironment(env domain.Environment) bool {
	return env == domain.Development
}

// RequireConfirmationForEnvironment checks if the environment requires confirmation for destructive operations
func (e *EnvironmentParser) RequireConfirmationForEnvironment(env domain.Environment) bool {
	return e.IsProductionEnvironment(env) || env == domain.Staging
}

// GetEnvironmentSpecificTimeout returns environment-specific timeout values
func (e *EnvironmentParser) GetEnvironmentSpecificTimeout(env domain.Environment) string {
	switch env {
	case domain.Production:
		return "600s" // 10 minutes for production
	case domain.Staging:
		return "300s" // 5 minutes for staging
	case domain.Development:
		return "180s" // 3 minutes for development
	default:
		return "300s" // 5 minutes default
	}
}

// GetEnvironmentNamespacePrefix returns the namespace prefix for the environment
func (e *EnvironmentParser) GetEnvironmentNamespacePrefix(env domain.Environment) string {
	switch env {
	case domain.Production:
		return "alt-production"
	case domain.Staging:
		return "alt-staging"
	case domain.Development:
		return "alt-dev"
	default:
		return "alt-default"
	}
}