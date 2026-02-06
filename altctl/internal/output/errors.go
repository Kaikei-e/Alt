package output

import (
	"fmt"

	"github.com/fatih/color"
)

// Exit code constants
const (
	ExitSuccess      = 0
	ExitGeneral      = 1
	ExitUsageError   = 2
	ExitComposeError = 3
	ExitConfigError  = 4
	ExitTimeout      = 5
)

// CLIError is a structured error with user-facing context
type CLIError struct {
	Summary    string
	Detail     string
	Suggestion string
	ExitCode   int
}

// Error implements the error interface, returning the summary
func (e *CLIError) Error() string {
	return e.Summary
}

// FormatError prints a structured error message to stderr
func (p *Printer) FormatError(e *CLIError) {
	if p.useColors {
		color.New(color.FgRed, color.Bold).Fprintf(p.err, "Error: %s\n", e.Summary)
		if e.Detail != "" {
			fmt.Fprintf(p.err, "  Cause: %s\n", e.Detail)
		}
		if e.Suggestion != "" {
			color.New(color.FgCyan).Fprintf(p.err, "  Suggestion: %s\n", e.Suggestion)
		}
	} else {
		fmt.Fprintf(p.err, "[ERROR] %s\n", e.Summary)
		if e.Detail != "" {
			fmt.Fprintf(p.err, "  Cause: %s\n", e.Detail)
		}
		if e.Suggestion != "" {
			fmt.Fprintf(p.err, "  Suggestion: %s\n", e.Suggestion)
		}
	}
}
