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
	// AggregateCovered reports whether this stack's ComposeFile is reachable
	// through compose/compose.yaml's top-level `include:` graph (directly or
	// transitively), i.e. whether `docker compose -f compose/compose.yaml`
	// alone already contains this stack's services. Computed once at
	// registry construction (see Registry's aggregateCoveredFiles) by
	// reading compose.yaml's own `include:` list -- never hand-maintained.
	//
	// This drives the C3 fix (see internal/doctor/probe.go's
	// aggregateComposeFile doc, which discovered the underlying problem
	// first): assembling a narrow per-stack `-f` subset is structurally
	// broken because several per-stack files transitively `include:
	// pki.yaml`, whose pki-agent sidecars depend_on services scattered
	// across many other stacks -- so lifecycle commands use the aggregate
	// file whenever every involved stack is AggregateCovered, and fall back
	// to a stack's own file (or a small isolated file set) only for the
	// stacks compose.yaml deliberately leaves out (today: dev,
	// frontend-dev, load-test -- local-dev-only overlays). Base.yaml is
	// always AggregateCovered (it has no services of its own and is
	// included everywhere), so its presence alone never forces the
	// aggregate strategy -- see cmd's buildStackInvocation.
	AggregateCovered bool `json:"aggregate_covered"`

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
