// Package command is the process entry contract for every ported harness check.
// Implementations must be deterministic, fail closed, and honour ctx cancellation.
package command

import (
	"context"
	"io"
)

// Func runs one CLI. args excludes the subcommand. The return value is the process exit code.
type Func func(ctx context.Context, args []string, stdout, stderr io.Writer) int

// Result is a finished step. Output is combined only for callers that ask for it;
// the Func itself writes to the provided streams.
type Result struct {
	Name string
	Code int
	Err  error
}
