// Package output provides CLI output formatting utilities
package output

import (
	"fmt"
	"io"
	"os"

	"github.com/fatih/color"
)

// ColorMode represents color output mode
type ColorMode int

const (
	// ColorAuto enables colors based on environment (default)
	ColorAuto ColorMode = iota
	// ColorAlways forces colors on
	ColorAlways
	// ColorNever forces colors off
	ColorNever
)

// PrinterOptions configures the Printer
type PrinterOptions struct {
	ColorMode    ColorMode
	ConfigColors bool // .altctl.yaml output.colors value
	Quiet        bool
}

// Printer handles formatted output to the terminal
type Printer struct {
	out       io.Writer
	err       io.Writer
	useColors bool
	quiet     bool
}

// ParseColorMode parses a string into a ColorMode
func ParseColorMode(s string) (ColorMode, error) {
	switch s {
	case "auto":
		return ColorAuto, nil
	case "always":
		return ColorAlways, nil
	case "never":
		return ColorNever, nil
	default:
		return ColorAuto, fmt.Errorf("invalid color mode %q: must be auto, always, or never", s)
	}
}

// ResolveColors determines whether to use colors based on mode and environment
func ResolveColors(mode ColorMode, configColors bool) bool {
	switch mode {
	case ColorAlways:
		return true
	case ColorNever:
		return false
	default: // ColorAuto
		if _, ok := os.LookupEnv("NO_COLOR"); ok {
			return false
		}
		if os.Getenv("TERM") == "dumb" {
			return false
		}
		return configColors
	}
}

// NewPrinter creates a new printer with the specified color setting (backwards compatible)
func NewPrinter(useColors bool) *Printer {
	return &Printer{
		out:       os.Stdout,
		err:       os.Stderr,
		useColors: useColors,
	}
}

// NewPrinterWithOptions creates a new printer with full options
func NewPrinterWithOptions(opts PrinterOptions) *Printer {
	return &Printer{
		out:       os.Stdout,
		err:       os.Stderr,
		useColors: ResolveColors(opts.ColorMode, opts.ConfigColors),
		quiet:     opts.Quiet,
	}
}

// IsQuiet returns whether the printer is in quiet mode
func (p *Printer) IsQuiet() bool {
	return p.quiet
}

// Info prints an informational message
func (p *Printer) Info(format string, args ...interface{}) {
	if p.quiet {
		return
	}
	if p.useColors {
		color.New(color.FgCyan).Fprintf(p.out, format+"\n", args...)
	} else {
		fmt.Fprintf(p.out, format+"\n", args...)
	}
}

// Success prints a success message
func (p *Printer) Success(format string, args ...interface{}) {
	if p.quiet {
		return
	}
	if p.useColors {
		color.New(color.FgGreen).Fprintf(p.out, "✓ "+format+"\n", args...)
	} else {
		fmt.Fprintf(p.out, "[OK] "+format+"\n", args...)
	}
}

// Warning prints a warning message
func (p *Printer) Warning(format string, args ...interface{}) {
	if p.quiet {
		return
	}
	if p.useColors {
		color.New(color.FgYellow).Fprintf(p.err, "⚠ "+format+"\n", args...)
	} else {
		fmt.Fprintf(p.err, "[WARN] "+format+"\n", args...)
	}
}

// Error prints an error message
func (p *Printer) Error(format string, args ...interface{}) {
	if p.useColors {
		color.New(color.FgRed).Fprintf(p.err, "✗ "+format+"\n", args...)
	} else {
		fmt.Fprintf(p.err, "[ERROR] "+format+"\n", args...)
	}
}

// Print prints a plain message
func (p *Printer) Print(format string, args ...interface{}) {
	if p.quiet {
		return
	}
	fmt.Fprintf(p.out, format+"\n", args...)
}

// Header prints a section header
func (p *Printer) Header(title string) {
	if p.quiet {
		return
	}
	if p.useColors {
		color.New(color.FgWhite, color.Bold).Fprintf(p.out, "\n%s\n", title)
		color.New(color.FgWhite).Fprintf(p.out, "%s\n", repeatChar('─', len(title)))
	} else {
		fmt.Fprintf(p.out, "\n%s\n%s\n", title, repeatChar('-', len(title)))
	}
}

// StatusBadge prints a colored status badge
func (p *Printer) StatusBadge(status string) string {
	if !p.useColors {
		return fmt.Sprintf("[%s]", status)
	}

	switch status {
	case "running", "healthy", "Up":
		return color.GreenString("●")
	case "exited", "stopped", "Exit":
		return color.RedString("●")
	case "starting", "restarting":
		return color.YellowString("●")
	default:
		return color.WhiteString("○")
	}
}

// Bold returns text in bold
func (p *Printer) Bold(text string) string {
	if p.useColors {
		return color.New(color.Bold).Sprint(text)
	}
	return text
}

// Dim returns dimmed text
func (p *Printer) Dim(text string) string {
	if p.useColors {
		return color.New(color.Faint).Sprint(text)
	}
	return text
}

func repeatChar(char rune, count int) string {
	result := make([]rune, count)
	for i := range result {
		result[i] = char
	}
	return string(result)
}
