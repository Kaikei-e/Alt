package cmd

import (
	"bytes"
	"testing"
)

func setupStatusTest(t *testing.T) {
	t.Helper()
	cfg = testConfig(t, []string{"db", "auth", "core", "workers"})
	dryRun = true
	quiet = false
}

func TestStatus_DryRun(t *testing.T) {
	setupStatusTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"status", "--dry-run"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("status command failed: %v", err)
	}
}

func TestStatus_JSON_DryRun(t *testing.T) {
	setupStatusTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"status", "--json", "--dry-run"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("status --json failed: %v", err)
	}
}

func TestStatus_NoWatch(t *testing.T) {
	setupStatusTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"status", "--dry-run"})

	// Ensure status command works without --watch (default)
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("status (no watch) failed: %v", err)
	}
}
