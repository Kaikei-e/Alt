package domain

import (
	"time"
)

// LayerDefinition defines a deployment layer with its charts and dependencies
type LayerDefinition struct {
	Name         string    `json:"name"`         // layer name (e.g., "infrastructure", "application")
	Order        int       `json:"order"`        // deployment order
	Charts       []Chart   `json:"charts"`       // charts in this layer
	Dependencies []string  `json:"dependencies"` // other layers this depends on
	Parallel     bool      `json:"parallel"`     // can charts in this layer be deployed in parallel
	WaitReady    bool      `json:"wait_ready"`   // wait for all charts in layer to be ready before proceeding
	Timeout      time.Duration `json:"timeout"`  // layer deployment timeout
	Description  string    `json:"description"`  // human-readable description
}

// Note: Many common types like DeploymentProgress, RollbackOptions, DeploymentStatus, 
// ChartDeploymentResult, LayerStatus, and DeploymentStrategy already exist in other domain files.
// We only define new types that don't exist elsewhere.

// ValidationRule defines validation criteria for deployments
type ValidationRule struct {
	Name        string                 `json:"name"`
	Type        string                 `json:"type"` // health_check, resource_ready, custom
	Target      string                 `json:"target"`
	Timeout     time.Duration          `json:"timeout"`
	Required    bool                   `json:"required"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
	Description string                 `json:"description"`
}