package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/alt-project/altctl/internal/config"
)

func setupRootTest(t *testing.T) {
	t.Helper()
	cfg = &config.Config{
		Output:   config.OutputConfig{Colors: false},
		Logging:  config.LoggingConfig{Level: "info", Format: "text"},
		Defaults: config.DefaultsConfig{Stacks: []string{"db", "auth", "core", "workers"}},
		Project:  config.ProjectConfig{Root: t.TempDir()},
		Compose:  config.ComposeConfig{Dir: "compose"},
	}
}

func TestRootCmd_Help(t *testing.T) {
	setupRootTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"--help"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("root --help failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "altctl") {
		t.Errorf("expected help output to contain 'altctl', got:\n%s", out)
	}
}

func TestRootCmd_UnknownCommand(t *testing.T) {
	setupRootTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"nonexistent-command"})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for unknown command, got nil")
	}
}

func TestRootCmd_SubcommandsList(t *testing.T) {
	setupRootTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"--help"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("root --help failed: %v", err)
	}

	out := buf.String()
	for _, cmd := range []string{"up", "down", "status", "list", "logs", "config", "version", "restart", "exec"} {
		if !strings.Contains(out, cmd) {
			t.Errorf("expected help output to list %q command, got:\n%s", cmd, out)
		}
	}
}
