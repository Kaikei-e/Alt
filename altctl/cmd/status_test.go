package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
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

// TestPSError_DockerGroupIDHint is the H1 regression test: when the
// underlying compose failure mentions DOCKER_GROUP_ID (the
// compose/logging.yaml `${DOCKER_GROUP_ID:?...}` interpolation failure that
// fires before the docker daemon is ever touched), psError must point at
// the real remediation (export via scripts/get-docker-gid.sh) instead of
// its generic "is the daemon running" suggestion, which would otherwise
// send an operator chasing a dead daemon that was never the problem.
func TestPSError_DockerGroupIDHint(t *testing.T) {
	underlying := fmt.Errorf(`exit status 1: variable "DOCKER_GROUP_ID" is not set: environment variable required but not set`)
	err := psError(underlying)

	var cliErr *output.CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("psError(%v) should return a *output.CLIError, got %T", underlying, err)
	}
	if !strings.Contains(cliErr.Suggestion, "get-docker-gid.sh") {
		t.Errorf("Suggestion = %q, want it to reference scripts/get-docker-gid.sh", cliErr.Suggestion)
	}
	if !strings.Contains(cliErr.Suggestion, "DOCKER_GROUP_ID") {
		t.Errorf("Suggestion = %q, want it to name DOCKER_GROUP_ID explicitly", cliErr.Suggestion)
	}
}

// TestShowStatus_DryRun_DockerGroupIDUnset exercises showStatus's own
// construction path (as opposed to psError in isolation) with
// DOCKER_GROUP_ID unset, mirroring cmd/doctor_test.go's
// TestNewDoctorExecutor_InjectsPlaceholderDockerGroupIDWhenUnset: showStatus
// must not fail wiring together its compose client just because the
// environment has no DOCKER_GROUP_ID, since it now injects the same
// read-only placeholder doctor does (doctor.EnsureDockerGroupIDEnv).
func TestShowStatus_DryRun_DockerGroupIDUnset(t *testing.T) {
	setupStatusTest(t)
	t.Setenv("DOCKER_GROUP_ID", "")

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"status", "--dry-run"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("status --dry-run with DOCKER_GROUP_ID unset failed: %v", err)
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
