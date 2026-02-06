package cmd

import (
	"bytes"
	"testing"

	"github.com/alt-project/altctl/internal/config"
)

func setupExecTest(t *testing.T) {
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

func TestExec_WithMultipleArgs(t *testing.T) {
	setupExecTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"exec", "alt-backend", "--dry-run", "--", "ls", "-la"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("exec with multiple args failed: %v", err)
	}
}
