package stack

import (
	"testing"
)

func TestCheckMissingFeatures_CoreWithoutWorkers(t *testing.T) {
	registry := NewRegistry()
	resolver := NewFeatureResolver(registry)

	// Starting only core stack (with its dependencies), but not workers
	warnings := resolver.CheckMissingFeatures([]string{"base", "db", "auth", "core"})

	if len(warnings) == 0 {
		t.Error("expected warning for missing search feature")
	}

	found := false
	for _, w := range warnings {
		if w.Stack == "core" && w.MissingFeature == FeatureSearch {
			found = true
			if len(w.ProvidedBy) == 0 {
				t.Error("expected providers list to be non-empty")
			}
			hasWorkers := false
			for _, p := range w.ProvidedBy {
				if p == "workers" {
					hasWorkers = true
					break
				}
			}
			if !hasWorkers {
				t.Error("expected workers to be in providers list")
			}
		}
	}

	if !found {
		t.Error("expected search feature warning for core stack")
	}
}

func TestCheckMissingFeatures_CoreWithWorkers(t *testing.T) {
	registry := NewRegistry()
	resolver := NewFeatureResolver(registry)

	// Starting core and workers together
	warnings := resolver.CheckMissingFeatures([]string{"base", "db", "auth", "core", "workers"})

	for _, w := range warnings {
		if w.Stack == "core" && w.MissingFeature == FeatureSearch {
			t.Error("should not warn about search when workers is included")
		}
	}
}

func TestSuggestAdditionalStacks(t *testing.T) {
	registry := NewRegistry()
	resolver := NewFeatureResolver(registry)

	// Only core stack (without its dependencies for simplicity)
	suggested := resolver.SuggestAdditionalStacks([]string{"core"})

	if len(suggested) == 0 {
		t.Error("expected suggestions")
	}

	hasWorkers := false
	for _, s := range suggested {
		if s == "workers" {
			hasWorkers = true
		}
	}

	if !hasWorkers {
		t.Error("expected workers to be suggested for core")
	}
}

func TestCheckMissingFeatures_NoWarningsForCompleteStack(t *testing.T) {
	registry := NewRegistry()
	resolver := NewFeatureResolver(registry)

	// Default complete stack
	warnings := resolver.CheckMissingFeatures([]string{"base", "db", "auth", "core", "workers"})

	// Should have no critical warnings
	for _, w := range warnings {
		if w.Severity == SeverityCritical {
			t.Errorf("unexpected critical warning: stack=%s, feature=%s", w.Stack, w.MissingFeature)
		}
	}
}

func TestFindFeatureProviders(t *testing.T) {
	registry := NewRegistry()
	resolver := NewFeatureResolver(registry)

	providers := resolver.findFeatureProviders(FeatureSearch)

	if len(providers) == 0 {
		t.Error("expected at least one provider for search feature")
	}

	hasWorkers := false
	for _, p := range providers {
		if p == "workers" {
			hasWorkers = true
			break
		}
	}

	if !hasWorkers {
		t.Error("expected workers to provide search feature")
	}
}

func TestProvidesFeature(t *testing.T) {
	stack := &Stack{
		Name:     "test",
		Provides: []Feature{FeatureSearch, FeatureDatabase},
	}

	if !stack.ProvidesFeature(FeatureSearch) {
		t.Error("expected stack to provide search feature")
	}

	if !stack.ProvidesFeature(FeatureDatabase) {
		t.Error("expected stack to provide database feature")
	}

	if stack.ProvidesFeature(FeatureAI) {
		t.Error("expected stack to NOT provide AI feature")
	}
}

func TestFindFeatureProviders_Observability(t *testing.T) {
	registry := NewRegistry()
	resolver := NewFeatureResolver(registry)

	providers := resolver.findFeatureProviders(FeatureObservability)

	if len(providers) == 0 {
		t.Error("expected at least one provider for observability feature")
	}

	hasObservability := false
	for _, p := range providers {
		if p == "observability" {
			hasObservability = true
			break
		}
	}

	if !hasObservability {
		t.Error("expected observability stack to provide observability feature")
	}
}
