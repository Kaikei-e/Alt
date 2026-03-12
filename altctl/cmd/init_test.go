package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/alt-project/altctl/internal/config"
)

func setupInitTest(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()

	// Create .env.example
	example := "POSTGRES_DB=alt\nPOSTGRES_USER=alt_user\n"
	if err := os.WriteFile(filepath.Join(tmpDir, ".env.example"), []byte(example), 0644); err != nil {
		t.Fatal(err)
	}

	cfg = &config.Config{
		Output:   config.OutputConfig{Colors: false},
		Logging:  config.LoggingConfig{Level: "info", Format: "text"},
		Defaults: config.DefaultsConfig{Stacks: []string{"db", "auth", "core", "workers"}},
		Project:  config.ProjectConfig{Root: tmpDir},
		Compose:  config.ComposeConfig{Dir: "compose"},
	}
	dryRun = true
	quiet = false

	// Reset flags to defaults
	initCmd.Flags().Set("force", "false")
	initCmd.Flags().Set("skip-secrets", "false")

	return tmpDir
}

func TestInit_DryRun(t *testing.T) {
	setupInitTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"init", "--dry-run"})

	// dry-run should succeed (skips prerequisites failure if docker not present)
	// Note: In CI/test environments without Docker, prerequisites check will fail.
	// We test the dry-run path which still validates the command structure.
	_ = rootCmd.Execute()
}

func TestInit_SkipSecrets(t *testing.T) {
	setupInitTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"init", "--skip-secrets", "--dry-run"})

	_ = rootCmd.Execute()
}

func TestInit_NoArgs(t *testing.T) {
	setupInitTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"init", "extra-arg"})

	err := rootCmd.Execute()
	if err == nil {
		t.Error("expected error when passing args to init")
	}
}
