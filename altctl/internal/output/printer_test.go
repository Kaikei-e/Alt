package output

import (
	"bytes"
	"os"
	"testing"
)

func TestParseColorMode_Valid(t *testing.T) {
	tests := []struct {
		input string
		want  ColorMode
	}{
		{"auto", ColorAuto},
		{"always", ColorAlways},
		{"never", ColorNever},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseColorMode(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("ParseColorMode(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseColorMode_Invalid(t *testing.T) {
	_, err := ParseColorMode("invalid")
	if err == nil {
		t.Error("expected error for invalid color mode, got nil")
	}
}

func TestResolveColors_Always(t *testing.T) {
	// Even with NO_COLOR set, ColorAlways should return true
	t.Setenv("NO_COLOR", "1")
	got := ResolveColors(ColorAlways, false)
	if !got {
		t.Error("ResolveColors(ColorAlways, false) with NO_COLOR=1 should return true")
	}
}

func TestResolveColors_Never(t *testing.T) {
	// Even with config=true, ColorNever should return false
	got := ResolveColors(ColorNever, true)
	if got {
		t.Error("ResolveColors(ColorNever, true) should return false")
	}
}

func TestResolveColors_NoColorEnv(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	got := ResolveColors(ColorAuto, true)
	if got {
		t.Error("ResolveColors(ColorAuto, true) with NO_COLOR set should return false")
	}
}

func TestResolveColors_TermDumb(t *testing.T) {
	// Unset NO_COLOR to test TERM=dumb path
	os.Unsetenv("NO_COLOR")
	t.Setenv("TERM", "dumb")
	got := ResolveColors(ColorAuto, true)
	if got {
		t.Error("ResolveColors(ColorAuto, true) with TERM=dumb should return false")
	}
}

func TestResolveColors_AutoDefault(t *testing.T) {
	os.Unsetenv("NO_COLOR")
	t.Setenv("TERM", "xterm-256color")

	// Should follow config value
	if !ResolveColors(ColorAuto, true) {
		t.Error("ResolveColors(ColorAuto, true) should return true when no overrides")
	}
	if ResolveColors(ColorAuto, false) {
		t.Error("ResolveColors(ColorAuto, false) should return false when no overrides")
	}
}

func TestQuietMode_InfoSuppressed(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	p := NewPrinterWithOptions(PrinterOptions{
		ColorMode:    ColorNever,
		ConfigColors: false,
		Quiet:        true,
	})
	p.out = &stdout
	p.err = &stderr

	p.Info("should not appear")
	p.Success("should not appear")
	p.Warning("should not appear")
	p.Header("should not appear")
	p.Print("should not appear")

	if stdout.Len() != 0 {
		t.Errorf("expected empty stdout in quiet mode, got: %q", stdout.String())
	}
	// Warning goes to stderr but should also be suppressed in quiet mode
	if stderr.Len() != 0 {
		t.Errorf("expected empty stderr in quiet mode (except Error), got: %q", stderr.String())
	}
}

func TestQuietMode_ErrorNotSuppressed(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	p := NewPrinterWithOptions(PrinterOptions{
		ColorMode:    ColorNever,
		ConfigColors: false,
		Quiet:        true,
	})
	p.out = &stdout
	p.err = &stderr

	p.Error("this should appear")

	if stderr.Len() == 0 {
		t.Error("Error output should not be suppressed in quiet mode")
	}
}

func TestNewPrinter_BackwardsCompatible(t *testing.T) {
	// NewPrinter(true) should still work
	p := NewPrinter(true)
	if p == nil {
		t.Fatal("NewPrinter returned nil")
	}
	if p.quiet {
		t.Error("NewPrinter should not enable quiet mode")
	}
}

func TestNewPrinterWithOptions_QuietAndVerboseExclusion(t *testing.T) {
	// This just tests that the options are correctly stored
	p := NewPrinterWithOptions(PrinterOptions{
		ColorMode:    ColorNever,
		ConfigColors: false,
		Quiet:        true,
	})
	if !p.quiet {
		t.Error("expected quiet to be true")
	}
}

func TestIsQuiet(t *testing.T) {
	p := NewPrinterWithOptions(PrinterOptions{Quiet: true})
	if !p.IsQuiet() {
		t.Error("IsQuiet should return true")
	}
	p2 := NewPrinterWithOptions(PrinterOptions{Quiet: false})
	if p2.IsQuiet() {
		t.Error("IsQuiet should return false")
	}
}
