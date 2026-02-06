package output

import (
	"bytes"
	"strings"
	"testing"
)

func TestCLIError_Error(t *testing.T) {
	err := &CLIError{
		Summary:    "something failed",
		Detail:     "because of reasons",
		Suggestion: "try again",
		ExitCode:   ExitGeneral,
	}

	if err.Error() != "something failed" {
		t.Errorf("Error() = %q, want %q", err.Error(), "something failed")
	}
}

func TestFormatError_AllFields(t *testing.T) {
	var stderr bytes.Buffer
	p := NewPrinterWithOptions(PrinterOptions{
		ColorMode:    ColorNever,
		ConfigColors: false,
	})
	p.err = &stderr

	cliErr := &CLIError{
		Summary:    "unknown stack: foo",
		Detail:     "stack 'foo' is not registered",
		Suggestion: "Run 'altctl list' to see available stacks",
		ExitCode:   ExitUsageError,
	}

	p.FormatError(cliErr)

	out := stderr.String()
	if !strings.Contains(out, "unknown stack: foo") {
		t.Errorf("missing summary in output: %q", out)
	}
	if !strings.Contains(out, "stack 'foo' is not registered") {
		t.Errorf("missing detail in output: %q", out)
	}
	if !strings.Contains(out, "Run 'altctl list' to see available stacks") {
		t.Errorf("missing suggestion in output: %q", out)
	}
}

func TestFormatError_NoDetail(t *testing.T) {
	var stderr bytes.Buffer
	p := NewPrinterWithOptions(PrinterOptions{
		ColorMode:    ColorNever,
		ConfigColors: false,
	})
	p.err = &stderr

	cliErr := &CLIError{
		Summary:    "config file not found",
		Suggestion: "Check .altctl.yaml syntax or use --config flag",
		ExitCode:   ExitConfigError,
	}

	p.FormatError(cliErr)

	out := stderr.String()
	if !strings.Contains(out, "config file not found") {
		t.Errorf("missing summary in output: %q", out)
	}
	if strings.Contains(out, "Cause:") {
		t.Errorf("should not contain Cause line when Detail is empty: %q", out)
	}
	if !strings.Contains(out, "Check .altctl.yaml syntax or use --config flag") {
		t.Errorf("missing suggestion in output: %q", out)
	}
}

func TestExitCodes(t *testing.T) {
	// Verify exit code constants have expected values
	if ExitSuccess != 0 {
		t.Errorf("ExitSuccess = %d, want 0", ExitSuccess)
	}
	if ExitGeneral != 1 {
		t.Errorf("ExitGeneral = %d, want 1", ExitGeneral)
	}
	if ExitUsageError != 2 {
		t.Errorf("ExitUsageError = %d, want 2", ExitUsageError)
	}
}
