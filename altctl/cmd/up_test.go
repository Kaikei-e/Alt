package cmd

import (
	"bytes"
	"testing"

	"github.com/alt-project/altctl/internal/config"
)

func setupUpTest(t *testing.T) {
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
	// Reset flags that persist between test runs
	upCmd.Flags().Set("all", "false")
	upCmd.Flags().Set("no-deps", "false")
	upCmd.Flags().Set("build", "false")
	upCmd.Flags().Set("remove-orphans", "false")
	upCmd.Flags().Set("progress", "auto")
}

func TestUp_DefaultStacks(t *testing.T) {
	setupUpTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"up", "--dry-run"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("up command failed: %v", err)
	}
}

func TestUp_SpecificStack(t *testing.T) {
	setupUpTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"up", "recap", "--dry-run"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("up recap failed: %v", err)
	}
}

func TestUp_UnknownStack(t *testing.T) {
	setupUpTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"up", "nonexistent", "--dry-run"})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for unknown stack, got nil")
	}
}

func TestUp_NoDeps(t *testing.T) {
	setupUpTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"up", "core", "--no-deps", "--dry-run"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("up --no-deps failed: %v", err)
	}
}

func TestUp_All(t *testing.T) {
	setupUpTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"up", "--all", "--dry-run"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("up --all failed: %v", err)
	}
}

func TestUp_UnknownStack_NoDeps(t *testing.T) {
	setupUpTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"up", "nonexistent", "--no-deps", "--dry-run"})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for unknown stack with --no-deps, got nil")
	}
}

func TestUp_DryRunDoesNotFail(t *testing.T) {
	setupUpTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"up", "ai", "--dry-run"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("up ai --dry-run failed: %v", err)
	}
}
