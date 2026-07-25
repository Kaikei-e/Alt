package cmd

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func setupLogsTest(t *testing.T) {
	t.Helper()
	cfg = testConfig(t, []string{"db", "auth", "core", "workers"})
	dryRun = true
	quiet = false
}

func TestLogs_ServiceName(t *testing.T) {
	setupLogsTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"logs", "alt-backend", "--dry-run"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("logs alt-backend failed: %v", err)
	}
}

func TestLogs_StackName(t *testing.T) {
	setupLogsTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"logs", "recap", "--dry-run"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("logs recap (stack) failed: %v", err)
	}
}

// --- M2: exact argv assertions (the kind that would have caught H2) ---

// TestLogs_AltBackend_IncludesAggregateFileArg is the H2 regression guard:
// `client.Logs` used to be called with no -f argument at all, so every real
// (non-dry-run) `altctl logs <service>` invocation died immediately with
// "no configuration file provided". alt-backend resolves to the
// AggregateCovered "core" stack, so its logs invocation must carry the
// aggregate compose.yaml as -f.
func TestLogs_AltBackend_IncludesAggregateFileArg(t *testing.T) {
	setupLogsTest(t)
	fake := installFakeComposeClient(t)

	rootCmd.SetArgs([]string{"logs", "alt-backend", "--dry-run"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("logs alt-backend failed: %v", err)
	}

	argv, ok := fake.findArgv(" logs ")
	if !ok {
		t.Fatalf("expected a 'logs' invocation, got calls: %v", fake.argvs())
	}

	wantAggregateFile := "-f " + filepath.Join(getComposeDir(), "compose.yaml")
	if !strings.Contains(argv, wantAggregateFile) {
		t.Errorf("logs alt-backend argv %q missing -f arg %q (H2 regression: exec/logs used to pass no -f at all)", argv, wantAggregateFile)
	}
	if !strings.Contains(argv, "alt-backend") {
		t.Errorf("logs alt-backend argv %q missing the target service", argv)
	}
}

// TestLogs_Dev_UsesIsolatedFileSet mirrors TestUp_Dev_UsesIsolatedFileSet
// for logs: a service that only exists in an isolated stack (mock-auth is
// declared only in dev.yaml/frontend-dev.yaml) must never be queried
// through the aggregate file.
func TestLogs_Dev_UsesIsolatedFileSet(t *testing.T) {
	setupLogsTest(t)
	fake := installFakeComposeClient(t)

	rootCmd.SetArgs([]string{"logs", "dev", "--dry-run"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("logs dev failed: %v", err)
	}

	argv, ok := fake.findArgv(" logs ")
	if !ok {
		t.Fatalf("expected a 'logs' invocation, got calls: %v", fake.argvs())
	}

	wantDevFile := "-f " + filepath.Join(getComposeDir(), "dev.yaml")
	if !strings.Contains(argv, wantDevFile) {
		t.Errorf("logs dev argv %q missing %q", argv, wantDevFile)
	}
	wantAggregateFile := "-f " + filepath.Join(getComposeDir(), "compose.yaml")
	if strings.Contains(argv, wantAggregateFile) {
		t.Errorf("logs dev argv %q must not contain the aggregate file", argv)
	}
}

func TestLogs_UnknownTarget(t *testing.T) {
	setupLogsTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"logs", "nonexistent", "--dry-run"})

	// Should not error (pass through to docker compose with warning)
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("logs nonexistent failed: %v", err)
	}
}
