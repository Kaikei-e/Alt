package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"testing"

	"github.com/alt-project/altctl/internal/output"
)

// TestPSError_WrapsAsCLIError guards against a down Docker daemon being
// indistinguishable from an empty stack: client.PS failing (daemon
// unreachable, permission denied, etc.) must surface as a real,
// non-zero-exit error -- not get logged at Debug and papered over with
// "No services running".
func TestPSError_WrapsAsCLIError(t *testing.T) {
	underlying := fmt.Errorf("dial unix /var/run/docker.sock: connect: permission denied")
	err := psError(underlying)

	var cliErr *output.CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("psError(%v) should return a *output.CLIError, got %T", underlying, err)
	}
	if cliErr.ExitCode != output.ExitComposeError {
		t.Errorf("ExitCode = %d, want %d (ExitComposeError)", cliErr.ExitCode, output.ExitComposeError)
	}
	if cliErr.Detail == "" {
		t.Error("Detail should carry the underlying docker error, not swallow it")
	}
	if cliErr.Summary == "" {
		t.Error("Summary should be non-empty")
	}
}

func setupStatusTest(t *testing.T) {
	t.Helper()
	cfg = testConfig(t, []string{"db", "auth", "core", "workers"})
	dryRun = true
	quiet = false
}

func TestStatus_DryRun(t *testing.T) {
	setupStatusTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"status", "--dry-run"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("status command failed: %v", err)
	}
}

func TestStatus_JSON_DryRun(t *testing.T) {
	setupStatusTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"status", "--json", "--dry-run"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("status --json failed: %v", err)
	}
}

func TestStatus_NoWatch(t *testing.T) {
	setupStatusTest(t)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"status", "--dry-run"})

	// Ensure status command works without --watch (default)
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("status (no watch) failed: %v", err)
	}
}
