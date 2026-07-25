// Package cmd wires the adrdag CLI: a minimal cobra command tree over the
// ADR supersedes DAG, read-only over the ADR files (the only write is the
// optional --out projection, a disposable artifact).
package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/alt-project/adrdag/internal/adr"
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

// cliError carries an exit code through cobra's RunE error path. An empty
// msg means the command already printed its own diagnostics.
type cliError struct {
	code int
	msg  string
}

func (e *cliError) Error() string { return e.msg }

func domainErr(format string, a ...any) *cliError {
	return &cliError{code: exitFailure, msg: fmt.Sprintf(format, a...)}
}

func usageErr(format string, a ...any) *cliError {
	return &cliError{code: exitUsage, msg: fmt.Sprintf(format, a...)}
}

func ioErr(err error) *cliError {
	return &cliError{code: exitIO, msg: err.Error()}
}

// exitCode maps a RunE error to a process exit code. Anything cobra itself
// produced (unknown flags, arg-count validation) is a usage error.
func exitCode(err error) int {
	if err == nil {
		return exitOK
	}
	var ce *cliError
	if errors.As(err, &ce) {
		return ce.code
	}
	return exitUsage
}

// loadCorpus loads the --adr-dir corpus, mapping read failures to exit 3.
func loadCorpus(cmd *cobra.Command) (map[string]adr.ADR, error) {
	dir, err := cmd.Flags().GetString("adr-dir")
	if err != nil {
		return nil, usageErr("%v", err)
	}
	adrs, err := adr.LoadDir(dir)
	if err != nil {
		return nil, ioErr(err)
	}
	return adrs, nil
}

func validFormat(format string, allowed ...string) error {
	for _, a := range allowed {
		if format == a {
			return nil
		}
	}
	return usageErr("invalid --format %q (allowed: %v)", format, allowed)
}

func defaultADRDir() string {
	if dir := os.Getenv("ADRDAG_ADR_DIR"); dir != "" {
		return dir
	}
	return "docs/ADR"
}

// newRootCmd builds a fresh command tree (constructor, not a package var,
// so every test gets an isolated instance).
func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "adrdag",
		Short:         "Derive the latest binding decisions from the ADR supersedes DAG",
		Long:          "adrdag validates and queries the docs/ADR supersedes DAG.\nSemantics-compatible successor to scripts/adr_graph.py:\nbinding(A) ⇔ status=accepted ∧ no inbound supersedes edge.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().String("adr-dir", defaultADRDir(), "ADR directory (env ADRDAG_ADR_DIR)")
	root.AddCommand(newCheckCmd(), newResolveCmd(), newBindingCmd(), newGraphCmd())
	return root
}

// Execute runs the CLI and returns the process exit code.
func Execute() int {
	return executeWith(os.Args[1:], os.Stdout, os.Stderr)
}

// executeWith is Execute with injectable args and streams so the
// error-printing and exit-code mapping path is testable.
func executeWith(args []string, out, errW io.Writer) int {
	root := newRootCmd()
	root.SetOut(out)
	root.SetErr(errW)
	root.SetArgs(args)
	err := root.Execute()
	if err != nil && err.Error() != "" {
		fmt.Fprintf(errW, "Error: %v\n", err)
	}
	return exitCode(err)
}
