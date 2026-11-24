package ledgers

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"alt/scripts/altharness/internal/execx"
)

type gitResult struct {
	Stdout  string
	Stderr  string
	Code    int
	Missing bool
	Err     error
}

func runGit(ctx context.Context, cwd string, args ...string) gitResult {
	if ctx == nil {
		return gitResult{Code: 2, Err: errors.New("nil context")}
	}
	if ctx.Err() != nil {
		return gitResult{Code: 2, Err: ctx.Err()}
	}
	if _, err := exec.LookPath("git"); err != nil {
		return gitResult{Code: 127, Missing: true, Err: err}
	}
	var outBuf, errBuf bytes.Buffer
	err := execx.Run(ctx, execx.Cmd{
		Args: append([]string{"git"}, args...),
		Dir:  cwd,
	}, &outBuf, &errBuf)
	res := gitResult{Stdout: outBuf.String(), Stderr: strings.TrimSpace(errBuf.String())}
	if err == nil {
		return res
	}
	var exitErr *execx.ExitError
	if errors.As(err, &exitErr) {
		res.Code = exitErr.Code
		if res.Code == 0 {
			res.Code = 1
		}
		return res
	}
	res.Code = 1
	res.Err = err
	if res.Stderr == "" {
		res.Stderr = err.Error()
	}
	return res
}

func gitMissing(stderr io.Writer) {
	fmt.Fprintln(stderr, "error: git executable not found on PATH")
}

func nullSplit(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, "\x00")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
