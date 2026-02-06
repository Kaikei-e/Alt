package stack

import (
	"strings"
	"testing"
)

func TestResolve_Chain(t *testing.T) {
	registry := NewRegistry()
	resolver := NewDependencyResolver(registry)

	stacks, err := resolver.Resolve([]string{"core"})
	if err != nil {
		t.Fatalf("Resolve(core) failed: %v", err)
	}

	// core depends on base, db, auth - all should be included
	names := stackNames(stacks)
	for _, expected := range []string{"base", "db", "auth", "core"} {
		if !contains(names, expected) {
			t.Errorf("expected %q in resolved stacks %v", expected, names)
		}
	}

	// base should come before core
	baseIdx := indexOf(names, "base")
	coreIdx := indexOf(names, "core")
	if baseIdx >= coreIdx {
		t.Errorf("base (idx=%d) should come before core (idx=%d)", baseIdx, coreIdx)
	}
}

func TestResolve_UnknownStack(t *testing.T) {
	registry := NewRegistry()
	resolver := NewDependencyResolver(registry)

	_, err := resolver.Resolve([]string{"nonexistent"})
	if err == nil {
		t.Fatal("expected error for unknown stack")
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Errorf("expected error to mention 'nonexistent', got: %v", err)
	}
}

func TestResolveReverse(t *testing.T) {
	registry := NewRegistry()
	resolver := NewDependencyResolver(registry)

	stacks, err := resolver.ResolveReverse([]string{"core"})
	if err != nil {
		t.Fatalf("ResolveReverse(core) failed: %v", err)
	}

	names := stackNames(stacks)
	// In reverse order, core should come before base
	coreIdx := indexOf(names, "core")
	baseIdx := indexOf(names, "base")
	if coreIdx >= baseIdx {
		t.Errorf("core (idx=%d) should come before base (idx=%d) in reverse", coreIdx, baseIdx)
	}
}

func TestResolveWithDependents(t *testing.T) {
	registry := NewRegistry()
	resolver := NewDependencyResolver(registry)

	stacks, err := resolver.ResolveWithDependents([]string{"db"})
	if err != nil {
		t.Fatalf("ResolveWithDependents(db) failed: %v", err)
	}

	names := stackNames(stacks)
	// db's dependents should include core, workers, etc.
	if !contains(names, "core") {
		t.Errorf("expected 'core' as dependent of 'db', got: %v", names)
	}
	if !contains(names, "workers") {
		t.Errorf("expected 'workers' as dependent of 'db', got: %v", names)
	}
}

func TestDetectCycles(t *testing.T) {
	registry := NewRegistry()
	resolver := NewDependencyResolver(registry)

	// Default registry should have no cycles
	if err := resolver.DetectCycles(); err != nil {
		t.Errorf("unexpected cycle detected: %v", err)
	}
}

func TestGetDependencyGraph(t *testing.T) {
	registry := NewRegistry()
	resolver := NewDependencyResolver(registry)

	graph := resolver.GetDependencyGraph()

	// base should have no dependencies
	if deps, ok := graph["base"]; ok && len(deps) > 0 {
		t.Errorf("base should have no dependencies, got: %v", deps)
	}

	// core should depend on base, db, auth
	coreDeps := graph["core"]
	for _, expected := range []string{"base", "db", "auth"} {
		if !contains(coreDeps, expected) {
			t.Errorf("expected core to depend on %q, got: %v", expected, coreDeps)
		}
	}
}

// Helper functions

func stackNames(stacks []*Stack) []string {
	names := make([]string, len(stacks))
	for i, s := range stacks {
		names[i] = s.Name
	}
	return names
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func indexOf(slice []string, item string) int {
	for i, s := range slice {
		if s == item {
			return i
		}
	}
	return -1
}
