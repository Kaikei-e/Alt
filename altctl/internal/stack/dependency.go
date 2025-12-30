package stack

import (
	"fmt"
	"slices"
)

// DependencyResolver handles stack dependency resolution
type DependencyResolver struct {
	registry *Registry
}

// NewDependencyResolver creates a new resolver with the given registry
func NewDependencyResolver(registry *Registry) *DependencyResolver {
	return &DependencyResolver{registry: registry}
}

// Resolve returns all stacks needed to start the requested stacks,
// in the correct order (dependencies first).
// Uses topological sort to ensure proper ordering.
func (r *DependencyResolver) Resolve(stackNames []string) ([]*Stack, error) {
	visited := make(map[string]bool)
	var result []*Stack

	var visit func(name string) error
	visit = func(name string) error {
		if visited[name] {
			return nil
		}

		stack, ok := r.registry.Get(name)
		if !ok {
			return fmt.Errorf("unknown stack: %s", name)
		}

		// Visit dependencies first (depth-first)
		for _, dep := range stack.DependsOn {
			if err := visit(dep); err != nil {
				return err
			}
		}

		visited[name] = true
		result = append(result, stack)
		return nil
	}

	for _, name := range stackNames {
		if err := visit(name); err != nil {
			return nil, err
		}
	}

	return result, nil
}

// ResolveReverse returns stacks in reverse dependency order
// (for graceful shutdown - dependents first, then dependencies)
func (r *DependencyResolver) ResolveReverse(stackNames []string) ([]*Stack, error) {
	stacks, err := r.Resolve(stackNames)
	if err != nil {
		return nil, err
	}
	slices.Reverse(stacks)
	return stacks, nil
}

// ResolveWithDependents returns all stacks that would need to be stopped
// if the given stacks are stopped (includes stacks that depend on them)
func (r *DependencyResolver) ResolveWithDependents(stackNames []string) ([]*Stack, error) {
	// Build a set of stacks to stop
	toStop := make(map[string]bool)
	for _, name := range stackNames {
		toStop[name] = true
	}

	// Find all stacks that depend on the ones being stopped
	changed := true
	for changed {
		changed = false
		for _, s := range r.registry.All() {
			if toStop[s.Name] {
				continue
			}
			// Check if this stack depends on any stack being stopped
			for _, dep := range s.DependsOn {
				if toStop[dep] {
					toStop[s.Name] = true
					changed = true
					break
				}
			}
		}
	}

	// Convert to list and sort by dependency order (reverse)
	var names []string
	for name := range toStop {
		names = append(names, name)
	}
	return r.ResolveReverse(names)
}

// GetDependents returns stacks that directly depend on the given stack
func (r *DependencyResolver) GetDependents(stackName string) []*Stack {
	var dependents []*Stack
	for _, s := range r.registry.All() {
		for _, dep := range s.DependsOn {
			if dep == stackName {
				dependents = append(dependents, s)
				break
			}
		}
	}
	return dependents
}

// DetectCycles checks for circular dependencies using Kahn's algorithm
func (r *DependencyResolver) DetectCycles() error {
	// Calculate in-degree for each stack
	inDegree := make(map[string]int)
	for _, stack := range r.registry.All() {
		if _, ok := inDegree[stack.Name]; !ok {
			inDegree[stack.Name] = 0
		}
		for _, dep := range stack.DependsOn {
			inDegree[dep]++
		}
	}

	// Start with stacks that have no incoming edges
	var queue []string
	for name, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, name)
		}
	}

	// Process queue
	var sorted []string
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		sorted = append(sorted, name)

		stack, ok := r.registry.Get(name)
		if !ok {
			continue
		}

		for _, dep := range stack.DependsOn {
			inDegree[dep]--
			if inDegree[dep] == 0 {
				queue = append(queue, dep)
			}
		}
	}

	// If we couldn't sort all stacks, there's a cycle
	if len(sorted) != len(r.registry.All()) {
		return fmt.Errorf("circular dependency detected in stack definitions")
	}
	return nil
}

// GetDependencyGraph returns a visual representation of the dependency graph
func (r *DependencyResolver) GetDependencyGraph() map[string][]string {
	graph := make(map[string][]string)
	for _, s := range r.registry.All() {
		graph[s.Name] = s.DependsOn
	}
	return graph
}
