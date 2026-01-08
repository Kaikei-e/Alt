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
