package stack

import (
	"os"
	"path/filepath"
	"testing"
)

// --- AggregateCovered: real repo ---------------------------------------

// TestRealRepo_AggregateCoveredMatchesComposeYamlInclude guards the C3 fix's
// foundation: every stack whose compose file is named (directly) in
// compose/compose.yaml's `include:` list must be AggregateCovered, and the
// three local-dev-only stacks compose.yaml deliberately excludes (dev,
// frontend-dev, load-test) must not be.
func TestRealRepo_AggregateCoveredMatchesComposeYamlInclude(t *testing.T) {
	registry := realRegistry(t)

	covered := []string{
		"base", "db", "pgbouncer", "auth", "sovereign", "core", "mq",
		"workers", "ai", "recap", "logging", "rag", "perf", "observability",
		"bff", "backup", "pact", "acolyte", "pki",
	}
	for _, name := range covered {
		s, ok := registry.Get(name)
		if !ok {
			t.Fatalf("expected stack %q to exist", name)
		}
		if !s.AggregateCovered {
			t.Errorf("expected stack %q to be AggregateCovered (its compose file is in compose.yaml's include: graph)", name)
		}
	}

	isolated := []string{"dev", "frontend-dev", "load-test"}
	for _, name := range isolated {
		s, ok := registry.Get(name)
		if !ok {
			t.Fatalf("expected stack %q to exist", name)
		}
		if s.AggregateCovered {
			t.Errorf("expected stack %q to NOT be AggregateCovered (compose.yaml's include: list omits it)", name)
		}
	}
}

// --- AggregateCovered: fixture registry (no aggregate file) -------------

func TestNewRegistryFromSemantics_NoAggregateFile_NothingIsCovered(t *testing.T) {
	dir := newFixtureComposeDir(t) // no compose.yaml written
	registry, err := NewRegistryFromSemantics(dir, baseFixtureSemantics())
	if err != nil {
		t.Fatalf("NewRegistryFromSemantics failed: %v", err)
	}

	db, ok := registry.Get("db")
	if !ok {
		t.Fatal("expected db stack to exist")
	}
	if db.AggregateCovered {
		t.Error("expected db.AggregateCovered = false when compose.yaml doesn't exist at all")
	}
}

func TestNewRegistryFromSemantics_AggregateFilePresent_MarksIncludedStacks(t *testing.T) {
	dir := newFixtureComposeDir(t)
	writeComposeFixture(t, dir, "compose.yaml", "name: alt\ninclude:\n  - db.yaml\n")

	registry, err := NewRegistryFromSemantics(dir, baseFixtureSemantics())
	if err != nil {
		t.Fatalf("NewRegistryFromSemantics failed: %v", err)
	}

	db, ok := registry.Get("db")
	if !ok {
		t.Fatal("expected db stack to exist")
	}
	if !db.AggregateCovered {
		t.Error("expected db.AggregateCovered = true (db.yaml is in compose.yaml's include: list)")
	}

	base, ok := registry.Get("base")
	if !ok {
		t.Fatal("expected base stack to exist")
	}
	if !base.AggregateCovered {
		t.Error("expected base.AggregateCovered = true unconditionally (base.yaml has no services and is always covered)")
	}
}

func TestNewRegistryFromSemantics_AggregateFilePresent_UnlistedStackNotCovered(t *testing.T) {
	dir := newFixtureComposeDir(t)
	// compose.yaml exists but doesn't include newstack.yaml.
	writeComposeFixture(t, dir, "compose.yaml", "name: alt\ninclude:\n  - db.yaml\n")

	registry, err := NewRegistryFromSemantics(dir, baseFixtureSemantics())
	if err != nil {
		t.Fatalf("NewRegistryFromSemantics failed: %v", err)
	}

	ns, ok := registry.Get("newstack")
	if !ok {
		t.Fatal("expected newstack to be auto-registered")
	}
	if ns.AggregateCovered {
		t.Error("expected newstack.AggregateCovered = false (not named in compose.yaml's include: list)")
	}
}

// --- FindByService: determinism (C4) -------------------------------------

// TestRealRepo_FindByService_PrefersAggregateCoveredStack guards the C4 fix
// directly: "alt-backend" is declared both in core.yaml (AggregateCovered)
// and dev.yaml's local-dev override (not AggregateCovered). Before the fix,
// FindByService iterated a Go map and returned "core" or "dev" nearly
// 50/50 across runs; it must now always return "core".
func TestRealRepo_FindByService_PrefersAggregateCoveredStack(t *testing.T) {
	registry := realRegistry(t)

	for i := 0; i < 20; i++ {
		s, err := registry.FindByService("alt-backend")
		if err != nil {
			t.Fatalf("FindByService(alt-backend) returned an error: %v", err)
		}
		if s == nil {
			t.Fatal("FindByService(alt-backend) returned nil")
		}
		if s.Name != "core" {
			t.Fatalf("FindByService(alt-backend) = %q, want %q (run %d)", s.Name, "core", i)
		}
	}
}

func TestRealRepo_FindByService_UnambiguousService(t *testing.T) {
	registry := realRegistry(t)

	s, err := registry.FindByService("auth-hub")
	if err != nil {
		t.Fatalf("FindByService(auth-hub) returned an error: %v", err)
	}
	if s == nil || s.Name != "auth" {
		t.Fatalf("FindByService(auth-hub) = %v, want stack 'auth'", s)
	}
}

func TestRealRepo_FindByService_UnknownService(t *testing.T) {
	registry := realRegistry(t)

	s, err := registry.FindByService("totally-not-a-service")
	if err != nil {
		t.Fatalf("expected nil error for an unknown service, got: %v", err)
	}
	if s != nil {
		t.Fatalf("expected nil stack for an unknown service, got %v", s)
	}
}

// TestFindByService_AmbiguousAcrossTwoIsolatedStacks_ReturnsError exercises
// the genuinely-ambiguous branch (step 3 of FindByService's doc comment):
// a service declared in two stacks that are BOTH outside the aggregate, so
// preferring "AggregateCovered" can't break the tie.
func TestFindByService_AmbiguousAcrossTwoIsolatedStacks_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	composeDir := filepath.Join(dir, "compose")
	if err := os.MkdirAll(composeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeComposeFixture(t, composeDir, "compose.yaml", "name: alt\ninclude:\n  - core.yaml\n")
	writeComposeFixture(t, composeDir, "core.yaml", "services:\n  web:\n    image: web\n")
	writeComposeFixture(t, composeDir, "iso-a.yaml", "services:\n  shared-svc:\n    image: a\n")
	writeComposeFixture(t, composeDir, "iso-b.yaml", "services:\n  shared-svc:\n    image: b\n")

	semantics := &SemanticsConfig{
		Stacks: map[string]StackSemantics{
			"core":  {Optional: false},
			"iso-a": {Optional: true},
			"iso-b": {Optional: true},
		},
	}
	registry, err := NewRegistryFromSemantics(composeDir, semantics)
	if err != nil {
		t.Fatalf("NewRegistryFromSemantics failed: %v", err)
	}

	s, err := registry.FindByService("shared-svc")
	if err == nil {
		t.Fatalf("expected an error for a service ambiguous across two isolated stacks, got stack %v", s)
	}
	if s != nil {
		t.Errorf("expected a nil stack alongside the error, got %v", s)
	}
}
