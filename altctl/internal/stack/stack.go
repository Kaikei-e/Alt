// Package stack provides stack definitions and dependency management for altctl
package stack

import (
	"time"
)

// Stack represents a logical grouping of Docker Compose services
type Stack struct {
	Name        string        `json:"name"`
	Description string        `json:"description"`
	ComposeFile string        `json:"compose_file"`
	Services    []string      `json:"services"`
	DependsOn   []string      `json:"depends_on"`
	Profile     string        `json:"profile,omitempty"`
	Optional    bool          `json:"optional"`
	RequiresGPU bool          `json:"requires_gpu"`
	Timeout     time.Duration `json:"timeout"`

	// Feature-based dependencies
	Provides         []Feature `json:"provides,omitempty"`          // Features this stack provides
	RequiresFeatures []Feature `json:"requires_features,omitempty"` // Features this stack needs to function
}

// IsDefault returns true if this stack should be started by default
func (s *Stack) IsDefault() bool {
	return !s.Optional
}

// HasProfile returns true if this stack requires a Docker Compose profile
func (s *Stack) HasProfile() bool {
	return s.Profile != ""
}

// GetTimeout returns the startup timeout for this stack
func (s *Stack) GetTimeout() time.Duration {
	if s.Timeout > 0 {
		return s.Timeout
	}
	// Default timeout
	return 5 * time.Minute
}

// ProvidesFeature checks if this stack provides a specific feature
func (s *Stack) ProvidesFeature(f Feature) bool {
	for _, provided := range s.Provides {
		if provided == f {
			return true
		}
	}
	return false
}
