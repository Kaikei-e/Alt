package cmd

import (
	"bytes"
	"testing"

	"github.com/alt-project/altctl/internal/config"
)

func setupDownTest(t *testing.T) {
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
