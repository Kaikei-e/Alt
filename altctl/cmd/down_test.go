package cmd

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func setupDownTest(t *testing.T) {
	t.Helper()
	cfg = testConfig(t, []string{"db", "auth", "core", "workers"})
	dryRun = true
	quiet = false
	downCmd.Flags().Set("volumes", "false")
	downCmd.Flags().Set("with-deps", "false")
	downCmd.Flags().Set("remove-orphans", "false")
}

func TestDown_NoArgs(t *testing.T) {
	setupDownTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"down", "--dry-run"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("down command failed: %v", err)
	}
}

func TestDown_SpecificStack(t *testing.T) {
	setupDownTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"down", "recap", "--dry-run"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("down recap failed: %v", err)
	}
}

func TestDown_UnknownStack(t *testing.T) {
	setupDownTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"down", "nonexistent", "--dry-run"})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for unknown stack, got nil")
	}
}

func TestDown_WithDeps(t *testing.T) {
	setupDownTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"down", "db", "--with-deps", "--dry-run"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("down --with-deps failed: %v", err)
	}
}

func TestDown_Volumes(t *testing.T) {
	setupDownTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"down", "--volumes", "--dry-run"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("down --volumes failed: %v", err)
	}
}

// --- M2: exact argv assertions ---

// TestDown_NoArgs_UsesAggregateFileDown guards the bare-down design: a
// full, unscoped teardown uses `docker compose -f compose/compose.yaml
// down` (a real `down`, not stop+rm -- there's nothing to scope to), not
// the old per-stack-file union (which -- like C3's `up` breakage -- was
// itself broken: core.yaml + dev.yaml both redeclare alt-frontend-sv with
// conflicting resource limits).
func TestDown_NoArgs_UsesAggregateFileDown(t *testing.T) {
	setupDownTest(t)
	fake := installFakeComposeClient(t)

	rootCmd.SetArgs([]string{"down", "--dry-run"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("down failed: %v", err)
	}

	argv, ok := fake.findArgv(" down")
	if !ok {
		t.Fatalf("expected a 'down' invocation, got calls: %v", fake.argvs())
	}
	wantAggregateFile := "-f " + filepath.Join(getComposeDir(), "compose.yaml")
	if !strings.Contains(argv, wantAggregateFile) {
		t.Errorf("bare down argv %q missing %q", argv, wantAggregateFile)
	}
	if strings.Contains(argv, "-f "+filepath.Join(getComposeDir(), "dev.yaml")) {
		t.Errorf("bare down argv %q must not combine dev.yaml with the aggregate (conflicting alt-frontend-sv redeclaration)", argv)
	}
}

// TestDown_SpecificStack_UsesStopThenRemove guards the scoped-down design:
// `altctl down <stack>` must stop + rm the named services rather than a
// plain `docker compose down [SERVICES]` (which doesn't cleanly scope -v to
// just the named services -- see cmd/down.go's runScopedDown doc comment),
// and must never issue an unscoped `down` that would tear down the whole
// aggregate project.
func TestDown_SpecificStack_UsesStopThenRemove(t *testing.T) {
	setupDownTest(t)
	fake := installFakeComposeClient(t)

	rootCmd.SetArgs([]string{"down", "core", "--dry-run"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("down core failed: %v", err)
	}

	stopArgv, ok := fake.findArgv(" stop")
	if !ok {
		t.Fatalf("expected a 'stop' invocation, got calls: %v", fake.argvs())
	}
	rmArgv, ok := fake.findArgv(" rm ", "-f")
	if !ok {
		t.Fatalf("expected an 'rm -f' invocation, got calls: %v", fake.argvs())
	}

	wantAggregateFile := "-f " + filepath.Join(getComposeDir(), "compose.yaml")
	for _, argv := range []string{stopArgv, rmArgv} {
		if !strings.Contains(argv, wantAggregateFile) {
			t.Errorf("down core argv %q missing %q", argv, wantAggregateFile)
		}
		if !strings.Contains(argv, "alt-backend") {
			t.Errorf("down core argv %q missing core service alt-backend", argv)
		}
	}
	for _, argv := range fake.argvs() {
		if strings.HasSuffix(strings.TrimSpace(argv), " down") || strings.Contains(argv, " down ") {
			t.Errorf("down core must never issue a bare 'down' subcommand (it would tear down the whole aggregate project), got argv %q", argv)
		}
	}
}
