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

// --force overwrites .env and regenerates the random secrets — rotating DB
// passwords and tokens — while operator-provided and step-ca provisioner
// secrets are never rewritten. The help must not promise more than that.
func TestInit_HelpDescribesForceAsRandomSecretRotation(t *testing.T) {
	setupInitTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"init", "--help"})
	t.Cleanup(func() { _ = initCmd.Flags().Set("help", "false") })
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("init --help failed: %v", err)
	}

	out := buf.String()
	for _, stale := range []string{
		"Overwrite existing .env and secrets",
		"overwrite existing .env and secret files",
		"existing files are not overwritten unless --force is used",
	} {
		if strings.Contains(out, stale) {
			t.Errorf("help still claims %q:\n%s", stale, out)
		}
	}
	for _, want := range []string{"random secrets", "operator-provided"} {
		if !strings.Contains(out, want) {
			t.Errorf("help must mention %q:\n%s", want, out)
		}
	}
}

// Plain `altctl init` creates whatever is missing; --force would also rotate
// every random secret that already exists, DB passwords included.
func TestInit_MissingFilesSuggestsPlainInit(t *testing.T) {
	err := missingFilesError(2)

	if err.Summary != "2 required files missing" {
		t.Errorf("summary = %q", err.Summary)
	}
	if strings.Contains(err.Suggestion, "--force") {
		t.Errorf("suggestion must not steer to --force: %q", err.Suggestion)
	}
	if !strings.Contains(err.Suggestion, "altctl init") {
		t.Errorf("suggestion must point at plain altctl init: %q", err.Suggestion)
	}
	if err.ExitCode != output.ExitConfigError {
		t.Errorf("exit code = %d", err.ExitCode)
	}
}
