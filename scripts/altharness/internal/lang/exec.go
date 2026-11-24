package lang

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func prepare(ctx context.Context, stdout, stderr io.Writer) (context.Context, io.Writer, io.Writer, int) {
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	if ctx == nil {
		fmt.Fprintln(stderr, "error: nil context")
		return nil, stdout, stderr, 2
	}
	return ctx, stdout, stderr, 0
}

func canceled(ctx context.Context, stderr io.Writer) int {
	if ctx.Err() == nil {
		return 0
	}
	fmt.Fprintf(stderr, "error: %s\n", ctx.Err().Error())
	return 1
}

// run executes args[0] with context cancellation. A missing binary is an error,
// never a skipped success. Extra env keys override the parent environment.
func run(ctx context.Context, stdout, stderr io.Writer, dir string, env []string, args ...string) (int, error) {
	if ctx == nil {
		return 2, errors.New("nil context")
	}
	if ctx.Err() != nil {
		return 1, ctx.Err()
	}
	if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
		return 2, errors.New("empty command")
	}
	path, err := exec.LookPath(args[0])
	if err != nil {
		return 127, fmt.Errorf("command not found: %s: %w", args[0], err)
	}
	cmd := exec.CommandContext(ctx, path, args[1:]...)
	cmd.Dir = dir
	if len(env) > 0 {
		cmd.Env = mergeEnv(env)
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
			return 1, ctx.Err()
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			code := exitErr.ExitCode()
			if code == 0 {
				code = 1
			}
			return code, nil
		}
		return 1, err
	}
	return 0, nil
}

func capture(ctx context.Context, dir string, env []string, args ...string) (string, string, int, error) {
	var outBuf, errBuf bytes.Buffer
	code, err := run(ctx, &outBuf, &errBuf, dir, env, args...)
	return outBuf.String(), errBuf.String(), code, err
}

func mergeEnv(extra []string) []string {
	base := os.Environ()
	index := map[string]int{}
	for i, kv := range base {
		key, _, ok := strings.Cut(kv, "=")
		if ok {
			index[key] = i
		}
	}
	out := append([]string{}, base...)
	for _, kv := range extra {
		key, _, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		if i, exists := index[key]; exists {
			out[i] = kv
			continue
		}
		index[key] = len(out)
		out = append(out, kv)
	}
	return out
}

func commandMissing(stderr io.Writer, name string) {
	fmt.Fprintf(stderr, "ERROR: Required command not found in PATH: %s\n", name)
}

func commandMissingHint(stderr io.Writer, name, hint string) {
	fmt.Fprintf(stderr, "ERROR: Required command not found in PATH: %s (%s)\n", name, hint)
}

func requireCommands(stderr io.Writer, names ...string) int {
	for _, name := range names {
		if _, err := exec.LookPath(name); err != nil {
			commandMissing(stderr, name)
			return 1
		}
	}
	return 0
}

func cacheHome() (string, error) {
	if v := strings.TrimSpace(os.Getenv("XDG_CACHE_HOME")); v != "" {
		return v, nil
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "", errors.New("HOME must be set")
	}
	return filepath.Join(home, ".cache"), nil
}
