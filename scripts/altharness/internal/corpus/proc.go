package corpus

import (
	"alt/scripts/altharness/internal/fsperm"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"alt/scripts/altharness/internal/execx"
)

func runCaptured(ctx context.Context, timeout time.Duration, dir, label string, args []string) (string, string, int, string) {
	if ctx == nil {
		return "", "", -1, label + " exec error: nil context"
	}
	if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
		return "", "", -1, label + " exec error: empty command"
	}
	if timeout <= 0 {
		timeout = subprocessTimeout()
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	stdout, stderr, code, err := execx.Combined(cctx, execx.Cmd{Args: args, Dir: dir})
	if err == nil {
		return stdout, stderr, code, ""
	}
	if errors.Is(cctx.Err(), context.DeadlineExceeded) && ctx.Err() == nil {
		secs := int(timeout / time.Second)
		if secs <= 0 {
			secs = SubprocessTimeoutSecond
		}
		return stdout, stderr, -1, fmt.Sprintf("%s timed out after %ds", label, secs)
	}
	return stdout, stderr, -1, fmt.Sprintf("%s exec error: %v", label, err)
}

func outcomeFrom(stdout, stderr string, code int, errMsg string) CheckOutcome {
	if errMsg != "" {
		return CheckOutcome{ExitCode: -1, RunnerError: errMsg}
	}
	return CheckOutcome{ExitCode: code, Output: strings.TrimSpace(stdout + "\n" + stderr)}
}

func runSimple(ctx context.Context, timeout time.Duration, dir, label string, args []string) CheckOutcome {
	stdout, stderr, code, errMsg := runCaptured(ctx, timeout, dir, label, args)
	return outcomeFrom(stdout, stderr, code, errMsg)
}

func runInProcess(ctx context.Context, timeout time.Duration, label string, fn func(context.Context, io.Writer, io.Writer) int) CheckOutcome {
	if ctx == nil {
		return CheckOutcome{ExitCode: -1, RunnerError: label + " exec error: nil context"}
	}
	if timeout <= 0 {
		timeout = subprocessTimeout()
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var stdout, stderr bytes.Buffer
	code := fn(cctx, &stdout, &stderr)
	if errors.Is(cctx.Err(), context.DeadlineExceeded) && ctx.Err() == nil {
		secs := int(timeout / time.Second)
		if secs <= 0 {
			secs = SubprocessTimeoutSecond
		}
		return CheckOutcome{ExitCode: -1, RunnerError: fmt.Sprintf("%s timed out after %ds", label, secs)}
	}
	if ctx.Err() != nil {
		return CheckOutcome{ExitCode: -1, RunnerError: fmt.Sprintf("%s exec error: %v", label, ctx.Err())}
	}
	return outcomeFrom(stdout.String(), stderr.String(), code, "")
}

func lookExecutable(envKey, name string, extras []string) (string, string) {
	if env := strings.TrimSpace(os.Getenv(envKey)); env != "" {
		if p, ok := executableFile(env); ok {
			return p, ""
		}
		if p, err := exec.LookPath(env); err == nil {
			return p, ""
		}
		return "", fmt.Sprintf("%s=%s does not point to an executable binary", envKey, pyRepr(env))
	}
	if p, err := exec.LookPath(name); err == nil {
		return p, ""
	}
	for _, extra := range extras {
		if p, ok := executableFile(extra); ok {
			return p, ""
		}
	}
	return "", ""
}

func executableFile(path string) (string, bool) {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return "", false
	}
	if info.Mode()&0o111 == 0 {
		return "", false
	}
	return path, true
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open %s: %w", src, err)
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), fsperm.Dir); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(dst), err)
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, fsperm.File)
	if err != nil {
		return fmt.Errorf("create %s: %w", dst, err)
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return fmt.Errorf("copy %s: %w", dst, copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close %s: %w", dst, closeErr)
	}
	return nil
}

func markExecutableIfShell(path string) {
	if filepath.Ext(path) != ".sh" {
		return
	}
	info, err := os.Stat(path)
	if err != nil {
		return
	}
	_ = os.Chmod(path, info.Mode().Perm()|0o100)
}

func subprocessTimeout() time.Duration {
	return time.Duration(SubprocessTimeoutSecond) * time.Second
}
