package stack

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeComposeFixture creates dir/name with the given YAML body and returns
// the file's stem-relative helpers useful in table-driven assertions.
func writeComposeFixture(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatalf("writing fixture %s: %v", name, err)
	}
}

// newFixtureComposeDir builds a small compose/*.yaml tree exercising every
// derivation case: a real stack (db), the shared-resources file with no
// services (base), an overlay file that must not become a stack even though
// it has services, and an empty-services file that must be excluded.
func newFixtureComposeDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	writeComposeFixture(t, dir, "base.yaml", "name: alt\nsecrets:\n  x:\n    file: y\n")
	writeComposeFixture(t, dir, "db.yaml", "services:\n  db:\n    image: postgres\n  meilisearch:\n    image: meilisearch\n")
	writeComposeFixture(t, dir, "empty.yaml", "services: {}\n")
	writeComposeFixture(t, dir, "compose.dev.yaml", "name: alt\nservices:\n  db:\n    pull_policy: never\n")
	writeComposeFixture(t, dir, "newstack.yaml", "services:\n  fooservice:\n    image: foo\n")

	return dir
}

func baseFixtureSemantics() *SemanticsConfig {
	return &SemanticsConfig{
		Stacks: map[string]StackSemantics{
			"base": {Description: "shared resources", DependsOn: []string{}, Optional: false},
			"db":   {Description: "database", DependsOn: []string{"base"}, Optional: false},
		},
		Overlays: []string{"compose.dev.yaml"},
	}
}

func TestNewRegistryFromSemantics_DerivesServicesFromComposeFile(t *testing.T) {
	dir := newFixtureComposeDir(t)
	registry, err := NewRegistryFromSemantics(dir, baseFixtureSemantics())
	if err != nil {
		t.Fatalf("NewRegistryFromSemantics failed: %v", err)
	}

	db, ok := registry.Get("db")
	if !ok {
		t.Fatal("expected db stack to be registered")
	}
	want := []string{"db", "meilisearch"}
	if !equalStrings(db.Services, want) {
		t.Errorf("db.Services = %v, want %v", db.Services, want)
	}
	if db.ComposeFile != "db.yaml" {
		t.Errorf("db.ComposeFile = %q, want db.yaml", db.ComposeFile)
	}
}

func TestNewRegistryFromSemantics_DeclaredStackAppliesSemantics(t *testing.T) {
	dir := newFixtureComposeDir(t)
	registry, err := NewRegistryFromSemantics(dir, baseFixtureSemantics())
	if err != nil {
		t.Fatalf("NewRegistryFromSemantics failed: %v", err)
	}

	db, ok := registry.Get("db")
	if !ok {
		t.Fatal("expected db stack to be registered")
	}
	if db.Optional {
		t.Error("expected db to be non-optional per declared semantics")
	}
	if !equalStrings(db.DependsOn, []string{"base"}) {
		t.Errorf("db.DependsOn = %v, want [base]", db.DependsOn)
	}
}

func TestNewRegistryFromSemantics_BaseWithEmptyServicesStaysAStack(t *testing.T) {
	dir := newFixtureComposeDir(t)
	registry, err := NewRegistryFromSemantics(dir, baseFixtureSemantics())
	if err != nil {
		t.Fatalf("NewRegistryFromSemantics failed: %v", err)
	}

	base, ok := registry.Get("base")
	if !ok {
		t.Fatal("expected base stack to be registered even though base.yaml has no services")
	}
	if len(base.Services) != 0 {
		t.Errorf("base.Services = %v, want empty", base.Services)
	}
}

func TestNewRegistryFromSemantics_EmptyServicesFileNotDeclaredIsExcluded(t *testing.T) {
	dir := newFixtureComposeDir(t)
	registry, err := NewRegistryFromSemantics(dir, baseFixtureSemantics())
	if err != nil {
		t.Fatalf("NewRegistryFromSemantics failed: %v", err)
	}

	if _, ok := registry.Get("empty"); ok {
		t.Error("expected empty.yaml (no services, not declared) to be excluded from the registry")
	}
}

func TestNewRegistryFromSemantics_OverlayFileExcludedEvenWithServices(t *testing.T) {
	dir := newFixtureComposeDir(t)
	registry, err := NewRegistryFromSemantics(dir, baseFixtureSemantics())
	if err != nil {
		t.Fatalf("NewRegistryFromSemantics failed: %v", err)
	}

	if _, ok := registry.Get("compose.dev"); ok {
		t.Error("expected compose.dev.yaml (declared overlay) to never become a stack")
	}
	// Also guard the failure mode where the overlay's own services (which
	// happen to reuse the name "db") get merged into the real db stack.
	db, _ := registry.Get("db")
	if len(db.Services) != 2 {
		t.Errorf("expected overlay file not to alter db's services, got %v", db.Services)
	}
}

func TestNewRegistryFromSemantics_UnknownFileWithServicesAutoRegistered(t *testing.T) {
	dir := newFixtureComposeDir(t)
	registry, err := NewRegistryFromSemantics(dir, baseFixtureSemantics())
	if err != nil {
		t.Fatalf("NewRegistryFromSemantics failed: %v", err)
	}

	ns, ok := registry.Get("newstack")
	if !ok {
		t.Fatal("expected newstack.yaml (has services, no declared semantics) to be auto-registered")
	}
	if !ns.Optional {
		t.Error("expected auto-registered stack to default to optional=true")
	}
	if !equalStrings(ns.DependsOn, []string{"base"}) {
		t.Errorf("expected auto-registered stack to default depends_on=[base], got %v", ns.DependsOn)
	}
	if !equalStrings(ns.Services, []string{"fooservice"}) {
		t.Errorf("newstack.Services = %v, want [fooservice]", ns.Services)
	}
}

func TestNewRegistryFromSemantics_DeclaredStackWithoutComposeFileIsHardError(t *testing.T) {
	dir := newFixtureComposeDir(t)
	sem := baseFixtureSemantics()
	sem.Stacks["ghost"] = StackSemantics{DependsOn: []string{"base"}, Optional: true}

	_, err := NewRegistryFromSemantics(dir, sem)
	if err == nil {
		t.Fatal("expected error when a declared stack has no matching compose file")
	}
	if !strings.Contains(err.Error(), "ghost") {
		t.Errorf("expected error to mention the offending stack name, got: %v", err)
	}
}

func TestNewRegistryFromSemantics_NilSemanticsUsesPureDefaults(t *testing.T) {
	dir := newFixtureComposeDir(t)
	registry, err := NewRegistryFromSemantics(dir, nil)
	if err != nil {
		t.Fatalf("NewRegistryFromSemantics(nil) failed: %v", err)
	}

	// With no declared semantics at all, base.yaml (no services) is never a
	// stack, and compose.dev.yaml (not declared as an overlay) becomes a
	// bogus auto-registered stack -- which is exactly why overlays/excluded
	// must be declared explicitly rather than inferred.
	if _, ok := registry.Get("base"); ok {
		t.Error("expected base to be absent when not declared (its file has no services)")
	}
	if _, ok := registry.Get("db"); !ok {
		t.Error("expected db to still be auto-registered from db.yaml")
	}
}

func TestNewRegistry_MissingConfigFileFallsBackToDefaults(t *testing.T) {
	dir := newFixtureComposeDir(t)
	registry, err := NewRegistry(dir, filepath.Join(dir, "does-not-exist.yaml"))
	if err != nil {
		t.Fatalf("NewRegistry with missing config file should not error, got: %v", err)
	}
	if _, ok := registry.Get("db"); !ok {
		t.Error("expected db to be auto-registered even with no config file")
	}
}

func TestNewRegistry_MissingComposeDirErrors(t *testing.T) {
	_, err := NewRegistry(filepath.Join(t.TempDir(), "does-not-exist"), "")
	if err == nil {
		t.Fatal("expected error when compose directory does not exist")
	}
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// --- Real repository consistency checks -------------------------------
//
// These replace the old hardcoded-list-vs-YAML sync tests: instead of
// comparing a Go literal against compose/*.yaml, they run the derivation
// itself against the real project tree and assert on properties that must
// hold no matter how compose/*.yaml or .altctl.yaml evolve.

// realProjectRoot locates the Alt monorepo root (the directory containing
// both compose/ and altctl/) by walking up from the test's working
// directory.
func realProjectRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getting working directory: %v", err)
	}
	for {
		if info, statErr := os.Stat(filepath.Join(dir, "compose")); statErr == nil && info.IsDir() {
			if _, altErr := os.Stat(filepath.Join(dir, "altctl")); altErr == nil {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Skip("Alt project root (compose/ + altctl/) not found; skipping")
		}
		dir = parent
	}
}

func realRegistry(t *testing.T) *Registry {
	t.Helper()
	root := realProjectRoot(t)
	registry, err := NewRegistry(filepath.Join(root, "compose"), filepath.Join(root, ".altctl.yaml"))
	if err != nil {
		t.Fatalf("loading real registry: %v", err)
	}
	return registry
}

// TestRealRepo_LoadsWithoutError is the fail-fast smoke test: if
// .altctl.yaml declares a stack whose compose file went away, or a new
// compose file with services isn't reflected as expected, this either
// fails to load (declared-but-missing) or would silently auto-register
// (missing-but-present, which is intentionally non-fatal -- see the notice
// printed by newRegistry).
func TestRealRepo_LoadsWithoutError(t *testing.T) {
	realRegistry(t)
}

// TestRealRepo_KnownStacksPresent guards the specific drift this refactor
// was written to fix: acolyte.yaml and pki.yaml previously had no stack at
// all.
func TestRealRepo_KnownStacksPresent(t *testing.T) {
	registry := realRegistry(t)
	for _, name := range []string{
		"base", "db", "pgbouncer", "auth", "sovereign", "core", "mq", "workers",
		"ai", "recap", "logging", "rag", "perf", "observability", "bff",
		"dev", "frontend-dev", "backup", "load-test", "pact", "acolyte", "pki",
	} {
		if _, ok := registry.Get(name); !ok {
			t.Errorf("expected stack %q to be present", name)
		}
	}
}

// TestRealRepo_OverlaysAreNotStacks guards against compose.dev.yaml /
// compose.staging.yaml silently becoming bogus stacks because they have a
// services: key.
func TestRealRepo_OverlaysAreNotStacks(t *testing.T) {
	registry := realRegistry(t)
	for _, name := range []string{"compose.dev", "compose.staging", "compose"} {
		if _, ok := registry.Get(name); ok {
			t.Errorf("expected %q to not be registered as a stack", name)
		}
	}
}

// TestRealRepo_CoreUsesPlectoProxyNotNginx guards the specific drift
// reported for the core stack: it used to hardcode "nginx" as a service,
// but compose/core.yaml defines "plecto-proxy".
func TestRealRepo_CoreUsesPlectoProxyNotNginx(t *testing.T) {
	registry := realRegistry(t)
	core, ok := registry.Get("core")
	if !ok {
		t.Fatal("expected core stack to exist")
	}
	if !contains(core.Services, "plecto-proxy") {
		t.Errorf("expected core.Services to include plecto-proxy, got %v", core.Services)
	}
	if contains(core.Services, "nginx") {
		t.Errorf("expected core.Services to not include stale service 'nginx', got %v", core.Services)
	}
}

// TestRealRepo_ObservabilityDoesNotListRemovedService guards the specific
// drift reported for observability: "nginx-exporter" no longer exists.
func TestRealRepo_ObservabilityDoesNotListRemovedService(t *testing.T) {
	registry := realRegistry(t)
	obs, ok := registry.Get("observability")
	if !ok {
		t.Fatal("expected observability stack to exist")
	}
	if contains(obs.Services, "nginx-exporter") {
		t.Errorf("expected observability.Services to not include removed service 'nginx-exporter', got %v", obs.Services)
	}
}

// TestRealRepo_DBIncludesPreProcessorDB guards the specific drift reported
// for db: it used to miss pre-processor-db{,-migrator}.
func TestRealRepo_DBIncludesPreProcessorDB(t *testing.T) {
	registry := realRegistry(t)
	db, ok := registry.Get("db")
	if !ok {
		t.Fatal("expected db stack to exist")
	}
	for _, svc := range []string{"pre-processor-db", "pre-processor-db-migrator"} {
		if !contains(db.Services, svc) {
			t.Errorf("expected db.Services to include %q, got %v", svc, db.Services)
		}
	}
}

// TestRealRepo_NoOrphanComposeFiles ensures every *.yaml/*.yml file in
// compose/ is accounted for: either it became a stack, is declared as an
// overlay, is declared as excluded, or has no services (and so is
// legitimately not a stack, like base.yaml/compose.yaml).
func TestRealRepo_NoOrphanComposeFiles(t *testing.T) {
	root := realProjectRoot(t)
	composeDir := filepath.Join(root, "compose")
	registry := realRegistry(t)

	semantics, err := LoadSemanticsConfig(filepath.Join(root, ".altctl.yaml"))
	if err != nil {
		t.Fatalf("loading semantics config: %v", err)
	}
	skip := stringSet(semantics.Overlays)
	for k, v := range stringSet(semantics.Excluded) {
		skip[k] = v
	}

	entries, err := os.ReadDir(composeDir)
	if err != nil {
		t.Fatalf("reading compose dir: %v", err)
	}

	registeredFiles := make(map[string]bool)
	for _, s := range registry.All() {
		registeredFiles[s.ComposeFile] = true
	}

	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}
		if skip[name] || registeredFiles[name] {
			continue
		}
		services, err := composeFileServices(filepath.Join(composeDir, name))
		if err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}
		if len(services) > 0 {
			t.Errorf("compose file %q has services but is neither a registered stack nor declared as an overlay/excluded file", name)
		}
	}
}
