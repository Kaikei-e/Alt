package domain

import (
	"fmt"
	"strings"
)

// Environment represents the deployment environment
type Environment string

const (
	Development Environment = "development"
	Staging     Environment = "staging"
	Production  Environment = "production"
)

// String returns the string representation of the environment
func (e Environment) String() string {
	return string(e)
}

// IsValid checks if the environment is valid
func (e Environment) IsValid() bool {
	switch e {
	case Development, Staging, Production:
		return true
	default:
		return false
	}
}

// ParseEnvironment parses a string to Environment
func ParseEnvironment(s string) (Environment, error) {
	env := Environment(strings.ToLower(s))
	if !env.IsValid() {
		return "", fmt.Errorf("invalid environment: %s (must be development, staging, or production)", s)
	}
	return env, nil
}

// DefaultNamespace returns the default namespace for the environment
func (e Environment) DefaultNamespace() string {
	switch e {
	case Development:
		return "alt-dev"
	case Staging:
		return "alt-staging"
	case Production:
		return "alt-production"
	default:
		return fmt.Sprintf("alt-%s", e)
	}
}
