package cmd

import (
	"bytes"
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
