package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alt-project/altctl/internal/config"
	"github.com/alt-project/altctl/internal/output"
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

// assertInitDeterministicOutcome runs the already-configured init command
// through captureStdout and checks the one thing that holds regardless of
// whether Docker happens to be present in the test environment: the
// Prerequisites phase always runs and is always reported first, and if it
// fails, init must fail with exactly the structured "prerequisites not met"
// CLIError — not a panic, not some unrelated error, and not a silent
// success. If prerequisites do pass (e.g. a real Docker daemon is
// reachable), dry-run must complete successfully without touching the
// filesystem.
func assertInitDeterministicOutcome(t *testing.T, tmpDir string) {
	t.Helper()

	var err error
	out := captureStdout(t, func() {
		err = rootCmd.Execute()
	})

	if !strings.Contains(out, "Prerequisites") {
		t.Errorf("expected output to contain the Prerequisites header, got:\n%s", out)
	}

	if err != nil {
		cliErr, ok := err.(*output.CLIError)
		if !ok {
			t.Fatalf("expected *output.CLIError on failure, got %T: %v", err, err)
		}
		if cliErr.Summary != "prerequisites not met" {
			t.Errorf("expected prerequisites-not-met error, got summary %q (detail: %q)", cliErr.Summary, cliErr.Detail)
		}
		if cliErr.ExitCode != output.ExitConfigError {
			t.Errorf("expected ExitConfigError (%d), got %d", output.ExitConfigError, cliErr.ExitCode)
		}
		return
	}

	if !strings.Contains(out, "Initialization complete") {
		t.Errorf("expected successful dry-run to report completion, got:\n%s", out)
	}
	if _, statErr := os.Stat(filepath.Join(tmpDir, ".env")); statErr == nil {
		t.Error("dry-run must not create .env")
	}
}

func TestInit_DryRun(t *testing.T) {
	tmpDir := setupInitTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"init", "--dry-run"})

	assertInitDeterministicOutcome(t, tmpDir)
}

func TestInit_SkipSecrets(t *testing.T) {
	tmpDir := setupInitTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"init", "--skip-secrets", "--dry-run"})

	assertInitDeterministicOutcome(t, tmpDir)
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
