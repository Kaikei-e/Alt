package cmd

import (
	"bytes"
	"testing"

	"github.com/alt-project/altctl/internal/config"
)

func setupRestartTest(t *testing.T) {
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
	restartCmd.Flags().Set("build", "false")
}

func TestRestart_DefaultStacks(t *testing.T) {
	setupRestartTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"restart", "--dry-run"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("restart command failed: %v", err)
	}
}

func TestRestart_SpecificStack(t *testing.T) {
	setupRestartTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"restart", "recap", "--dry-run"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("restart recap failed: %v", err)
	}
}

func TestRestart_UnknownStack(t *testing.T) {
	setupRestartTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"restart", "nonexistent", "--dry-run"})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for unknown stack, got nil")
	}
}
