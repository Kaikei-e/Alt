package cmd

import (
	"bytes"
	"testing"

	"github.com/alt-project/altctl/internal/config"
)

func setupConfigTest(t *testing.T) {
	t.Helper()
	cfg = &config.Config{
		Output:   config.OutputConfig{Colors: false},
		Logging:  config.LoggingConfig{Level: "info", Format: "text"},
		Defaults: config.DefaultsConfig{Stacks: []string{"db", "auth", "core", "workers"}},
		Project:  config.ProjectConfig{Root: t.TempDir()},
		Compose:  config.ComposeConfig{Dir: "compose"},
	}
	dryRun = false
	quiet = false
}

func TestConfig_Default(t *testing.T) {
	setupConfigTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"config"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("config command failed: %v", err)
	}
}

func TestConfig_JSON(t *testing.T) {
	setupConfigTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"config", "--json"})

	// config --json writes to os.Stdout directly; verify no error
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("config --json failed: %v", err)
	}
}

func TestConfig_Path(t *testing.T) {
	setupConfigTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"config", "--path"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("config --path failed: %v", err)
	}
}
