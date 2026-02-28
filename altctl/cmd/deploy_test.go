package cmd

import (
	"bytes"
	"testing"

	"github.com/alt-project/altctl/internal/config"
)

func setupDeployTest(t *testing.T) {
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
	deployCmd.Flags().Set("no-pull", "false")
	deployCmd.Flags().Set("no-smoke", "false")
	deployCmd.Flags().Set("no-cache", "false")
	deployCmd.Flags().Set("pull", "false")
	deployCmd.Flags().Set("no-deps", "false")
}

func TestDeploy_DefaultStacks(t *testing.T) {
	setupDeployTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"deploy", "--dry-run"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("deploy command failed: %v", err)
	}
}

func TestDeploy_SpecificStack(t *testing.T) {
	setupDeployTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"deploy", "core", "--dry-run"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("deploy core failed: %v", err)
	}
}

func TestDeploy_UnknownStack(t *testing.T) {
	setupDeployTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"deploy", "nonexistent", "--dry-run"})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for unknown stack, got nil")
	}
}

func TestDeploy_NoPull(t *testing.T) {
	setupDeployTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"deploy", "--no-pull", "--dry-run"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("deploy --no-pull failed: %v", err)
	}
}

func TestDeploy_NoDeps(t *testing.T) {
	setupDeployTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"deploy", "core", "--no-deps", "--dry-run"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("deploy --no-deps failed: %v", err)
	}
}

func TestDeploy_UnknownStack_NoDeps(t *testing.T) {
	setupDeployTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"deploy", "nonexistent", "--no-deps", "--dry-run"})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for unknown stack with --no-deps, got nil")
	}
}
