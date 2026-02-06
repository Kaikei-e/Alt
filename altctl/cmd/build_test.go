package cmd

import (
	"bytes"
	"testing"

	"github.com/alt-project/altctl/internal/config"
)

func setupBuildTest(t *testing.T) {
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
	buildCmd.Flags().Set("no-deps", "false")
	buildCmd.Flags().Set("no-cache", "false")
	buildCmd.Flags().Set("pull", "false")
	buildCmd.Flags().Set("progress", "auto")
}

func TestBuild_DefaultStacks(t *testing.T) {
	setupBuildTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"build", "--dry-run"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("build command failed: %v", err)
	}
}

func TestBuild_SpecificStack(t *testing.T) {
	setupBuildTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"build", "core", "--dry-run"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("build core failed: %v", err)
	}
}

func TestBuild_UnknownStack(t *testing.T) {
	setupBuildTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"build", "nonexistent", "--dry-run"})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for unknown stack, got nil")
	}
}

func TestBuild_NoDeps(t *testing.T) {
	setupBuildTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"build", "core", "--no-deps", "--dry-run"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("build --no-deps failed: %v", err)
	}
}
