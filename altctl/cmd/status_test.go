package cmd

import (
	"bytes"
	"testing"

	"github.com/alt-project/altctl/internal/config"
)

func setupStatusTest(t *testing.T) {
	t.Helper()
	cfg = &config.Config{
		Output:   config.OutputConfig{Colors: false},
		Logging:  config.LoggingConfig{Level: "info", Format: "text"},
		Defaults: config.DefaultsConfig{Stacks: []string{"db", "auth", "core", "workers"}},
		Project:  config.ProjectConfig{Root: t.TempDir()},
		Compose:  config.ComposeConfig{Dir: "compose"},
	}
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
