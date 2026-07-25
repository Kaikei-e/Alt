package cmd

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alt-project/altctl/internal/output"
	"github.com/alt-project/altctl/internal/stack"
)

// --- levenshtein / closestMatches (pure, no registry needed) ---

func TestLevenshtein_Basic(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"core", "core", 0},
		{"core", "cor", 1},
		{"core", "core-x", 2},
		{"kitten", "sitting", 3},
		{"", "abc", 3},
		{"abc", "", 3},
	}
	for _, c := range cases {
		if got := levenshtein(c.a, c.b); got != c.want {
			t.Errorf("levenshtein(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestClosestMatches_TypoSuggestsExactName(t *testing.T) {
	candidates := []string{"core", "db", "auth", "workers"}
	got := closestMatches("cor", candidates, 3)
	found := false
	for _, m := range got {
		if m == "core" {
			found = true
		}
	}
	if !found {
		t.Errorf("closestMatches(%q, %v) = %v, expected to contain %q", "cor", candidates, got, "core")
	}
}

func TestClosestMatches_NoisyInputYieldsNoMatch(t *testing.T) {
	candidates := []string{"core", "db", "auth", "workers"}
	got := closestMatches("zzzzzzzzzzzzzzzzzzzz", candidates, 3)
	if len(got) != 0 {
		t.Errorf("closestMatches for a wildly different string should return no matches, got %v", got)
	}
}

func TestClosestMatches_RespectsMax(t *testing.T) {
	candidates := []string{"aaaa", "aaab", "aaac", "aaad", "aaae"}
	got := closestMatches("aaaa", candidates, 2)
	if len(got) > 2 {
		t.Errorf("closestMatches should respect max=2, got %d results: %v", len(got), got)
	}
}

// --- resolveRebuildTargets (real registry against the repo's compose/*.yaml + .altctl.yaml) ---

func realRegistryForTest(t *testing.T) *stack.Registry {
	t.Helper()
	root := repoRootForTest(t)
	reg, err := stack.NewRegistry(root+"/compose", root+"/.altctl.yaml")
	if err != nil {
		t.Fatalf("building registry: %v", err)
	}
	return reg
}

func TestResolveRebuildTargets_ServiceName(t *testing.T) {
	registry := realRegistryForTest(t)

	// auth-hub is only ever declared in auth.yaml (unlike e.g. alt-backend,
	// which the local-dev "dev" stack also redeclares under its own
	// service: key for an alternate local-dev definition -- FindByService
	// now deterministically prefers the aggregate-covered stack ("core")
	// over the isolated one ("dev") for that name; see
	// TestRealRepo_FindByService_PrefersAggregateCoveredStack in
	// internal/stack for that guarantee directly).
	rt, err := resolveRebuildTargets(registry, []string{"auth-hub"})
	if err != nil {
		t.Fatalf("resolveRebuildTargets failed: %v", err)
	}
	if len(rt.services) != 1 || rt.services[0] != "auth-hub" {
		t.Errorf("services = %v, want [auth-hub]", rt.services)
	}
	// auth is AggregateCovered, so its file resolves to the aggregate
	// compose.yaml (C3 fix), not the bare "auth.yaml" -- see
	// cmd/compose_target.go's buildStackInvocation.
	if len(rt.files) != 1 || rt.files[0] != stack.AggregateComposeFile {
		t.Errorf("files = %v, want [%s]", rt.files, stack.AggregateComposeFile)
	}
	if len(rt.stacks) != 1 || rt.stacks[0].Name != "auth" {
		t.Fatalf("stacks = %v, want a single synthetic 'auth' stack", rt.stacks)
	}
	// The synthetic stack must be narrowed to only the targeted service, not
	// the whole auth.yaml service list -- otherwise Ready-wait/diagnostics
	// would wait on services rebuild never touched.
	if len(rt.stacks[0].Services) != 1 || rt.stacks[0].Services[0] != "auth-hub" {
		t.Errorf("synthetic stack services = %v, want [auth-hub] only", rt.stacks[0].Services)
	}
}

func TestResolveRebuildTargets_StackName(t *testing.T) {
	registry := realRegistryForTest(t)

	dbStack, ok := registry.Get("db")
	if !ok {
		t.Fatal("expected registry to have a 'db' stack")
	}

	rt, err := resolveRebuildTargets(registry, []string{"db"})
	if err != nil {
		t.Fatalf("resolveRebuildTargets failed: %v", err)
	}
	if len(rt.services) != len(dbStack.Services) {
		t.Errorf("services = %v, want all %d services of the db stack (%v)", rt.services, len(dbStack.Services), dbStack.Services)
	}
	// db is AggregateCovered, so it resolves to the aggregate compose.yaml,
	// not the bare "db.yaml" (C3 fix).
	if len(rt.files) != 1 || rt.files[0] != stack.AggregateComposeFile {
		t.Errorf("files = %v, want [%s]", rt.files, stack.AggregateComposeFile)
	}
}

func TestResolveRebuildTargets_DedupAcrossArgs(t *testing.T) {
	registry := realRegistryForTest(t)

	coreStack, ok := registry.Get("core")
	if !ok {
		t.Fatal("expected registry to have a 'core' stack")
	}

	rt, err := resolveRebuildTargets(registry, []string{"core", "alt-backend"})
	if err != nil {
		t.Fatalf("resolveRebuildTargets failed: %v", err)
	}
	if len(rt.services) != len(coreStack.Services) {
		t.Errorf("services = %v, want exactly the %d core services (no duplicate alt-backend)", rt.services, len(coreStack.Services))
	}
	count := 0
	for _, svc := range rt.services {
		if svc == "alt-backend" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("alt-backend appeared %d times in services, want exactly 1", count)
	}
}

func TestResolveRebuildTargets_MultipleStacksAcrossFiles(t *testing.T) {
	registry := realRegistryForTest(t)

	rt, err := resolveRebuildTargets(registry, []string{"db", "auth"})
	if err != nil {
		t.Fatalf("resolveRebuildTargets failed: %v", err)
	}
	// Both db and auth are AggregateCovered, so their combined invocation
	// still collapses to a single aggregate file -- not two separate -f
	// files (which would hit the C3 project-validation failure).
	if len(rt.files) != 1 || rt.files[0] != stack.AggregateComposeFile {
		t.Errorf("files = %v, want [%s]", rt.files, stack.AggregateComposeFile)
	}
	if len(rt.stacks) != 2 {
		t.Errorf("stacks = %v, want 2 synthetic stacks", rt.stacks)
	}
}

func TestResolveRebuildTargets_UnknownArg(t *testing.T) {
	registry := realRegistryForTest(t)

	_, err := resolveRebuildTargets(registry, []string{"totally-not-a-thing"})
	if err == nil {
		t.Fatal("expected error for unknown target, got nil")
	}
	cliErr, ok := err.(*output.CLIError)
	if !ok {
		t.Fatalf("expected *output.CLIError, got %T: %v", err, err)
	}
	if cliErr.ExitCode != output.ExitUsageError {
		t.Errorf("ExitCode = %d, want %d", cliErr.ExitCode, output.ExitUsageError)
	}
	if !strings.Contains(cliErr.Summary, "totally-not-a-thing") {
		t.Errorf("Summary = %q, want it to mention the unknown arg", cliErr.Summary)
	}
}

func TestResolveRebuildTargets_UnknownArg_SuggestsCloseMatch(t *testing.T) {
	registry := realRegistryForTest(t)

	_, err := resolveRebuildTargets(registry, []string{"cor"})
	if err == nil {
		t.Fatal("expected error for unknown target, got nil")
	}
	cliErr, ok := err.(*output.CLIError)
	if !ok {
		t.Fatalf("expected *output.CLIError, got %T: %v", err, err)
	}
	if !strings.Contains(cliErr.Suggestion, "core") {
		t.Errorf("Suggestion = %q, want it to suggest 'core' for typo 'cor'", cliErr.Suggestion)
	}
}

// --- CLI level (dry-run, mirrors up_test.go / restart_test.go conventions) ---

func setupRebuildTest(t *testing.T) {
	t.Helper()
	cfg = testConfig(t, []string{"db", "auth", "core", "workers"})
	dryRun = true
	quiet = false
	rebuildCmd.Flags().Set("no-cache", "false")
	rebuildCmd.Flags().Set("detach", "false")
}

func TestRebuild_DryRun_SpecificService(t *testing.T) {
	setupRebuildTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"rebuild", "alt-backend", "--dry-run"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("rebuild alt-backend failed: %v", err)
	}
}

func TestRebuild_DryRun_SpecificStack(t *testing.T) {
	setupRebuildTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"rebuild", "db", "--dry-run"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("rebuild db failed: %v", err)
	}
}

func TestRebuild_DryRun_MultipleTargets(t *testing.T) {
	setupRebuildTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"rebuild", "alt-backend", "db", "--dry-run"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("rebuild alt-backend db failed: %v", err)
	}
}

func TestRebuild_UnknownTarget_CLI(t *testing.T) {
	setupRebuildTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"rebuild", "nonexistent", "--dry-run"})

	if err := rootCmd.Execute(); err == nil {
		t.Fatal("expected error for unknown target, got nil")
	}
}

func TestRebuild_NoCacheFlag_DryRun(t *testing.T) {
	setupRebuildTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"rebuild", "alt-backend", "--no-cache", "--dry-run"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("rebuild --no-cache failed: %v", err)
	}
}

// TestRebuild_DetachSkipsReadyWait guards the same fire-and-forget contract
// 'up --detach' has: rebuild must report success right after the (dry-run)
// up phase without attempting a Ready-wait.
func TestRebuild_DetachSkipsReadyWait(t *testing.T) {
	setupRebuildTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"rebuild", "alt-backend", "--detach", "--dry-run"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("rebuild --detach failed: %v", err)
	}
}

// --- M2: exact argv assertions (the kind that would have caught C3/C4/H2) ---

// TestRebuild_AltBackend_ResolvesToCoreAggregateFile guards C4 (FindByService
// determinism: alt-backend must always resolve to "core", never "dev") and
// C3 (the resolved file must be the aggregate compose.yaml, not the bare
// "core.yaml" -- which fails alone: "migrate depends on undefined service
// db") in one shot, on the real composed argv.
func TestRebuild_AltBackend_ResolvesToCoreAggregateFile(t *testing.T) {
	setupRebuildTest(t)
	fake := installFakeComposeClient(t)

	rootCmd.SetArgs([]string{"rebuild", "alt-backend", "--dry-run"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("rebuild alt-backend failed: %v", err)
	}

	buildArgv, ok := fake.findArgv(" build ")
	if !ok {
		t.Fatalf("expected a 'build' invocation, got calls: %v", fake.argvs())
	}
	upArgv, ok := fake.findArgv(" up ", "-d")
	if !ok {
		t.Fatalf("expected an 'up' invocation, got calls: %v", fake.argvs())
	}

	wantAggregateFile := "-f " + filepath.Join(getComposeDir(), "compose.yaml")
	for _, argv := range []string{buildArgv, upArgv} {
		if !strings.Contains(argv, wantAggregateFile) {
			t.Errorf("rebuild alt-backend argv %q missing aggregate file arg %q", argv, wantAggregateFile)
		}
		if strings.Contains(argv, "-f "+filepath.Join(getComposeDir(), "dev.yaml")) {
			t.Errorf("rebuild alt-backend argv %q must never touch dev.yaml (C4: alt-backend must resolve to core, not dev)", argv)
		}
		if !strings.Contains(argv, "alt-backend") {
			t.Errorf("rebuild alt-backend argv %q missing the target service", argv)
		}
	}
	// rebuild must always force recreation and never touch dependents/
	// dependencies (see altctl/CLAUDE.md's "rebuild" section, ADR-000761 /
	// PM-2026-005).
	if !strings.Contains(upArgv, "--no-deps") {
		t.Errorf("rebuild up argv %q missing --no-deps", upArgv)
	}
	if !strings.Contains(upArgv, "--force-recreate") {
		t.Errorf("rebuild up argv %q missing --force-recreate", upArgv)
	}
}

func TestRebuild_NoArgs_UsageError(t *testing.T) {
	setupRebuildTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"rebuild", "--dry-run"})

	if err := rootCmd.Execute(); err == nil {
		t.Fatal("expected usage error when no target is given, got nil")
	}
}
