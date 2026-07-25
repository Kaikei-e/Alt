package cmd

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func setupExecTest(t *testing.T) {
	t.Helper()
	cfg = testConfig(t, []string{"db", "auth", "core", "workers"})
	dryRun = true
	quiet = false
	// Reset cobra args state
	rootCmd.SetArgs(nil)
}

func TestExec_DryRun(t *testing.T) {
	setupExecTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"exec", "alt-backend", "--dry-run", "--", "echo", "hello"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("exec command failed: %v", err)
	}
}

func TestExec_UnknownService(t *testing.T) {
	setupExecTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"exec", "nonexistent", "--dry-run", "--", "echo"})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for unknown service, got nil")
	}
}

// TestExec_IncludesAggregateFileArg is the other half of the H2 regression
// guard: `client.Exec` used to be called with no -f argument at all, so
// every real (non-dry-run) `altctl exec <service>` invocation died
// immediately with "no configuration file provided".
func TestExec_IncludesAggregateFileArg(t *testing.T) {
	setupExecTest(t)
	fake := installFakeComposeClient(t)

	rootCmd.SetArgs([]string{"exec", "alt-backend", "--dry-run", "--", "sh"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("exec alt-backend failed: %v", err)
	}

	argv, ok := fake.findArgv(" exec ")
	if !ok {
		t.Fatalf("expected an 'exec' invocation, got calls: %v", fake.argvs())
	}
	wantAggregateFile := "-f " + filepath.Join(getComposeDir(), "compose.yaml")
	if !strings.Contains(argv, wantAggregateFile) {
		t.Errorf("exec alt-backend argv %q missing -f arg %q (H2 regression)", argv, wantAggregateFile)
	}
	if !strings.Contains(argv, "exec alt-backend sh") {
		t.Errorf("exec alt-backend argv %q missing 'exec alt-backend sh'", argv)
	}
}

func TestExec_WithMultipleArgs(t *testing.T) {
	setupExecTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"exec", "alt-backend", "--dry-run", "--", "ls", "-la"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("exec with multiple args failed: %v", err)
	}
}
