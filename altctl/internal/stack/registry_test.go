package stack

import (
	"testing"
)

func TestDevStackExists(t *testing.T) {
	registry := NewRegistry()
	dev, ok := registry.Get("dev")

	if !ok {
		t.Fatal("expected dev stack to exist in registry")
	}

	if dev.Name != "dev" {
		t.Errorf("expected stack name to be 'dev', got '%s'", dev.Name)
	}
}

func TestDevStackDependencies(t *testing.T) {
	registry := NewRegistry()
	dev, ok := registry.Get("dev")

	if !ok {
		t.Fatal("expected dev stack to exist")
	}

	// Dev stack should only depend on base (no auth required)
	if len(dev.DependsOn) != 1 {
		t.Errorf("expected dev stack to have exactly 1 dependency, got %d", len(dev.DependsOn))
	}

	if dev.DependsOn[0] != "base" {
		t.Errorf("expected dev stack to depend on 'base', got '%s'", dev.DependsOn[0])
	}
}

func TestDevStackIsOptional(t *testing.T) {
	registry := NewRegistry()
	dev, ok := registry.Get("dev")

	if !ok {
		t.Fatal("expected dev stack to exist")
	}

	if !dev.Optional {
		t.Error("expected dev stack to be optional")
	}
}

func TestDevStackServices(t *testing.T) {
	registry := NewRegistry()
	dev, ok := registry.Get("dev")

	if !ok {
		t.Fatal("expected dev stack to exist")
	}

	requiredServices := []string{"mock-auth", "alt-frontend-sv", "alt-backend", "db", "migrate"}

	for _, svc := range requiredServices {
		found := false
		for _, s := range dev.Services {
			if s == svc {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected dev stack to include service '%s'", svc)
		}
	}
}

func TestDevStackComposeFile(t *testing.T) {
	registry := NewRegistry()
	dev, ok := registry.Get("dev")

	if !ok {
		t.Fatal("expected dev stack to exist")
	}

	if dev.ComposeFile != "dev.yaml" {
		t.Errorf("expected compose file to be 'dev.yaml', got '%s'", dev.ComposeFile)
	}
}

func TestDevStackProfile(t *testing.T) {
	registry := NewRegistry()
	dev, ok := registry.Get("dev")

	if !ok {
		t.Fatal("expected dev stack to exist")
	}

	if dev.Profile != "dev" {
		t.Errorf("expected profile to be 'dev', got '%s'", dev.Profile)
	}
}

func TestObservabilityStackExists(t *testing.T) {
	registry := NewRegistry()
	obs, ok := registry.Get("observability")

	if !ok {
		t.Fatal("expected observability stack to exist in registry")
	}

	if obs.Name != "observability" {
		t.Errorf("expected stack name to be 'observability', got '%s'", obs.Name)
	}
}

func TestObservabilityStackDependencies(t *testing.T) {
	registry := NewRegistry()
	obs, ok := registry.Get("observability")

	if !ok {
		t.Fatal("expected observability stack to exist")
	}

	expectedDeps := map[string]bool{"base": false, "db": false, "core": false}
	for _, dep := range obs.DependsOn {
		if _, ok := expectedDeps[dep]; ok {
			expectedDeps[dep] = true
		}
	}

	for dep, found := range expectedDeps {
		if !found {
			t.Errorf("expected observability stack to depend on '%s'", dep)
		}
	}
}

func TestObservabilityStackIsOptional(t *testing.T) {
	registry := NewRegistry()
	obs, ok := registry.Get("observability")

	if !ok {
		t.Fatal("expected observability stack to exist")
	}

	if !obs.Optional {
		t.Error("expected observability stack to be optional")
	}
}

func TestObservabilityStackServices(t *testing.T) {
	registry := NewRegistry()
	obs, ok := registry.Get("observability")

	if !ok {
		t.Fatal("expected observability stack to exist")
	}

	requiredServices := []string{"nginx-exporter", "prometheus", "grafana"}

	for _, svc := range requiredServices {
		found := false
		for _, s := range obs.Services {
			if s == svc {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected observability stack to include service '%s'", svc)
		}
	}
}

func TestObservabilityStackComposeFile(t *testing.T) {
	registry := NewRegistry()
	obs, ok := registry.Get("observability")

	if !ok {
		t.Fatal("expected observability stack to exist")
	}

	if obs.ComposeFile != "observability.yaml" {
		t.Errorf("expected compose file to be 'observability.yaml', got '%s'", obs.ComposeFile)
	}
}

func TestObservabilityStackProfile(t *testing.T) {
	registry := NewRegistry()
	obs, ok := registry.Get("observability")

	if !ok {
		t.Fatal("expected observability stack to exist")
	}

	if obs.Profile != "observability" {
		t.Errorf("expected profile to be 'observability', got '%s'", obs.Profile)
	}
}

func TestObservabilityStackProvidesFeature(t *testing.T) {
	registry := NewRegistry()
	obs, ok := registry.Get("observability")

	if !ok {
		t.Fatal("expected observability stack to exist")
	}

	if !obs.ProvidesFeature(FeatureObservability) {
		t.Error("expected observability stack to provide observability feature")
	}
}
