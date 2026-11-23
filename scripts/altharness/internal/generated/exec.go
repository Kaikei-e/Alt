package generated

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"alt/scripts/altharness/internal/execx"
)

const subprocessTimeout = 120 * time.Second

// checker is one Run invocation. Canonical-check results are memoized on the
// call stack (canonMemo), never in a package-level cache.
type checker struct {
	lookPath func(string) (string, error)
	// forcePython skips the in-process codegen checker and shells out once.
	forcePython bool
}

func (c *checker) which(name string) (string, error) {
	if c != nil && c.lookPath != nil {
		return c.lookPath(name)
	}
	return exec.LookPath(name)
}

func (c *checker) exec(ctx context.Context, args []string, dir string) (stdout, stderr string, code int, err error) {
	if err := ctx.Err(); err != nil {
		return "", "", 1, err
	}
	if len(args) == 0 {
		return "", "", 1, errors.New("empty command")
	}
	cctx, cancel := context.WithTimeout(ctx, subprocessTimeout)
	defer cancel()
	stdout, stderr, code, err = execx.Combined(cctx, execx.Cmd{Args: args, Dir: dir})
	if ctx.Err() != nil {
		return stdout, stderr, code, ctx.Err()
	}
	if err != nil && errors.Is(cctx.Err(), context.DeadlineExceeded) {
		return stdout, stderr, code, fmt.Errorf("%s: %w", args[0], context.DeadlineExceeded)
	}
	return stdout, stderr, code, err
}

func commandDetail(stdout, stderr string) string {
	if strings.TrimSpace(stderr) != "" {
		return strings.TrimSpace(stderr)
	}
	return strings.TrimSpace(stdout)
}

func outputDetail(stdout, stderr string, err error) string {
	if msg := commandDetail(stdout, stderr); msg != "" {
		return msg
	}
	if err != nil {
		return err.Error()
	}
	return ""
}
