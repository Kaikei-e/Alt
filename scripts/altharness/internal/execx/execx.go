// Package execx runs external tools with context cancellation, no inherited shell,
// and captured or streamed output. Missing binaries fail closed.
package execx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// Cmd is one external process. Args[0] is the executable name or path.
type Cmd struct {
	Args []string
	Dir  string
	Env  []string // extra KEY=VAL; the parent environment is still inherited
}

// Run executes c. A cancelled ctx kills the process. Exit codes are returned
// as *ExitError; lookup failures are wrapped and are not exit codes.
func Run(ctx context.Context, c Cmd, stdout, stderr io.Writer) error {
	if len(c.Args) == 0 || strings.TrimSpace(c.Args[0]) == "" {
		return errors.New("execx: empty command")
	}
	if ctx == nil {
		return errors.New("execx: nil context")
	}
	path, err := exec.LookPath(c.Args[0])
	if err != nil {
		return fmt.Errorf("execx: command not found: %s: %w", c.Args[0], err)
	}
	cmd := exec.CommandContext(ctx, path, c.Args[1:]...)
	cmd.Dir = c.Dir
	if len(c.Env) > 0 {
		cmd.Env = append(os.Environ(), c.Env...)
	}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return cmd.Process.Kill()
	}
	if err := cmd.Run(); err != nil {
		if errors.Is(ctx.Err(), context.Canceled) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("execx: %s: %w", c.Args[0], ctx.Err())
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return &ExitError{Name: c.Args[0], Code: exitErr.ExitCode(), Err: err}
		}
		return fmt.Errorf("execx: %s: %w", c.Args[0], err)
	}
	return nil
}

// ExitError is a non-zero tool status. Code is never zero.
type ExitError struct {
	Name string
	Code int
	Err  error
}

func (e *ExitError) Error() string {
	return fmt.Sprintf("%s exited %d", e.Name, e.Code)
}

func (e *ExitError) Unwrap() error { return e.Err }

// Combined runs c and returns stdout, stderr, and the exit code.
// A missing binary returns code 127.
func Combined(ctx context.Context, c Cmd) (stdout, stderr string, code int, err error) {
	var outBuf, errBuf bytes.Buffer
	runErr := Run(ctx, c, &outBuf, &errBuf)
	if runErr == nil {
		return outBuf.String(), errBuf.String(), 0, nil
	}
	var exitErr *ExitError
	if errors.As(runErr, &exitErr) {
		return outBuf.String(), errBuf.String(), exitErr.Code, nil
	}
	if errors.Is(runErr, exec.ErrNotFound) || strings.Contains(runErr.Error(), "command not found") {
		return outBuf.String(), errBuf.String(), 127, runErr
	}
	return outBuf.String(), errBuf.String(), 1, runErr
}
