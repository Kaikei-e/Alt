// Package cmd wires the adrdag CLI: a minimal cobra command tree over the
// ADR supersedes DAG, read-only over the ADR files (the only write is the
// optional --out projection, a disposable artifact).
package cmd

import (
	"github.com/spf13/cobra"
)

// Exit codes, mapped in Execute/exitCode:
//
//	0 = success (check: zero ERROR findings; WARN alone stays 0)
//	1 = domain failure (check found errors; resolve id unknown)
//	2 = usage error (bad args/flags/format)
//	3 = I/O error (unreadable --adr-dir, unwritable --out)
const (
	exitOK      = 0
	exitFailure = 1
	exitUsage   = 2
	exitIO      = 3
)

// cliError carries an exit code through cobra's RunE error path.
type cliError struct {
	code int
	msg  string
}

func (e *cliError) Error() string { return e.msg }

// exitCode maps a RunE error to a process exit code.
func exitCode(err error) int {
	return 99 // stub: RED
}

// newRootCmd builds a fresh command tree (constructor, not a package var,
// so every test gets an isolated instance).
func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "adrdag",
		Short: "Derive the latest binding decisions from the ADR supersedes DAG",
	}
	return root // stub: RED — subcommands and flags not wired yet
}

// Execute runs the CLI and returns the process exit code.
func Execute() int {
	return 99 // stub: RED
}
