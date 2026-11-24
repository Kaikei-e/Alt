package meta

import (
	"context"
	"fmt"
	"io"
	"time"

	"alt/scripts/altharness/internal/lint"
)

func init() {
	lint.RegisterGateRunner(runGate)
}

func runGate(ctx context.Context, repo, target string, stdout, stderr io.Writer) int {
	steps, err := Catalog(ctx, repo, target)
	if err != nil {
		fmt.Fprintln(stderr, err.Error())
		return 2
	}
	code, err := Run(ctx, steps, Options{Repo: repo, Stdout: stdout, Stderr: stderr, Timeout: 15 * time.Minute})
	if err != nil {
		fmt.Fprintln(stderr, err.Error())
		return 1
	}
	return code
}
