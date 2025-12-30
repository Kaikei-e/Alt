// Package output provides CLI output formatting utilities
package output

import (
	"fmt"
	"io"
	"os"

	"github.com/fatih/color"
)

// Printer handles formatted output to the terminal
type Printer struct {
	out       io.Writer
	err       io.Writer
	useColors bool
}

// NewPrinter creates a new printer with the specified options
func NewPrinter(useColors bool) *Printer {
	return &Printer{
		out:       os.Stdout,
		err:       os.Stderr,
		useColors: useColors,
	}
}

// Info prints an informational message
func (p *Printer) Info(format string, args ...interface{}) {
	if p.useColors {
		color.New(color.FgCyan).Fprintf(p.out, format+"\n", args...)
	} else {
		fmt.Fprintf(p.out, format+"\n", args...)
	}
}

// Success prints a success message
func (p *Printer) Success(format string, args ...interface{}) {
	if p.useColors {
		color.New(color.FgGreen).Fprintf(p.out, "✓ "+format+"\n", args...)
	} else {
		fmt.Fprintf(p.out, "[OK] "+format+"\n", args...)
	}
}

// Warning prints a warning message
func (p *Printer) Warning(format string, args ...interface{}) {
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
	fmt.Fprintf(p.out, format+"\n", args...)
}

// Header prints a section header
func (p *Printer) Header(title string) {
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
