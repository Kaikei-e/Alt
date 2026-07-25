package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestExecuteWithSuccess(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := executeWith([]string{"--adr-dir", fixture("ok"), "check"}, &out, &errBuf)
	if code != exitOK {
		t.Fatalf("exit = %d, want %d (stderr: %s)", code, exitOK, errBuf.String())
	}
	if want := "OK: 3 ADRs, 2 supersedes edges, no cycles, status aligned\n"; out.String() != want {
		t.Errorf("stdout = %q, want %q", out.String(), want)
	}
}

func TestExecuteWithUnknownFlagPrintsErrorAndExitsUsage(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := executeWith([]string{"--bogus"}, &out, &errBuf)
	if code != exitUsage {
		t.Fatalf("exit = %d, want %d", code, exitUsage)
	}
	if !strings.Contains(errBuf.String(), "Error: unknown flag: --bogus") {
		t.Errorf("stderr = %q, want contains %q", errBuf.String(), "Error: unknown flag: --bogus")
	}
}

func TestExecuteWithBadFormatPrintsError(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := executeWith([]string{"--adr-dir", fixture("ok"), "check", "--format", "yaml"}, &out, &errBuf)
	if code != exitUsage {
		t.Fatalf("exit = %d, want %d", code, exitUsage)
	}
	if !strings.Contains(errBuf.String(), "Error: invalid --format") {
		t.Errorf("stderr = %q, want contains %q", errBuf.String(), "Error: invalid --format")
	}
}

func TestExecuteWithDomainFailureStaysQuiet(t *testing.T) {
	// check already prints its own ERROR diagnostics; the empty-msg cliError
	// must not add a redundant "Error:" line on top.
	var out, errBuf bytes.Buffer
	code := executeWith([]string{"--adr-dir", fixture("status-drift"), "check"}, &out, &errBuf)
	if code != exitFailure {
		t.Fatalf("exit = %d, want %d", code, exitFailure)
	}
	if strings.Contains(errBuf.String(), "Error:") {
		t.Errorf("stderr = %q, must not contain a redundant Error: line", errBuf.String())
	}
}
