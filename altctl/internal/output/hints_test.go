package output

import (
	"bytes"
	"strings"
	"testing"
)

func TestPrintHints_KnownCommand(t *testing.T) {
	var stdout bytes.Buffer
	p := NewPrinterWithOptions(PrinterOptions{
		ColorMode:    ColorNever,
		ConfigColors: false,
	})
	p.out = &stdout

	p.PrintHints("up")

	out := stdout.String()
	if !strings.Contains(out, "See also") {
		t.Errorf("expected 'See also' in output, got: %q", out)
	}
	if !strings.Contains(out, "status") {
		t.Errorf("expected 'status' hint for 'up' command, got: %q", out)
	}
}

func TestPrintHints_UnknownCommand(t *testing.T) {
	var stdout bytes.Buffer
	p := NewPrinterWithOptions(PrinterOptions{
		ColorMode:    ColorNever,
		ConfigColors: false,
	})
	p.out = &stdout

	p.PrintHints("nonexistent")

	if stdout.Len() != 0 {
		t.Errorf("expected no output for unknown command, got: %q", stdout.String())
	}
}

func TestPrintHints_Quiet(t *testing.T) {
	var stdout bytes.Buffer
	p := NewPrinterWithOptions(PrinterOptions{
		ColorMode:    ColorNever,
		ConfigColors: false,
		Quiet:        true,
	})
	p.out = &stdout

	p.PrintHints("up")

	if stdout.Len() != 0 {
		t.Errorf("expected no output in quiet mode, got: %q", stdout.String())
	}
}
