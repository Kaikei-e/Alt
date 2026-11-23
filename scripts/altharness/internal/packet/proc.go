package packet

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"syscall"
	"time"
)

type cmdResult struct {
	Code    int
	Stdout  []byte
	Stderr  []byte
	Elapsed time.Duration
	Timeout bool
	Err     error
}

func runProcess(ctx context.Context, argv []string, dir string, timeout time.Duration) cmdResult {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return cmdResult{Code: 1, Err: err}
	}
	if len(argv) == 0 || argv[0] == "" {
		return cmdResult{Code: 1, Err: errors.New("empty command")}
	}
	path, err := exec.LookPath(argv[0])
	if err != nil {
		return cmdResult{Code: 127, Err: fmt.Errorf("%s executable not found in PATH", argv[0])}
	}
	runCtx := ctx
	cancel := func() {}
	if timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, timeout)
	}
	defer cancel()
	cmd := exec.CommandContext(runCtx, path, argv[1:]...)
	cmd.Dir = dir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		return cmd.Process.Kill()
	}
	cmd.WaitDelay = 2 * time.Second
	start := time.Now()
	err = cmd.Run()
	elapsed := time.Since(start)
	res := cmdResult{Stdout: stdout.Bytes(), Stderr: stderr.Bytes(), Elapsed: elapsed}
	if err == nil {
		return res
	}
	if ctx.Err() != nil {
		res.Err = ctx.Err()
		res.Code = 1
		return res
	}
	if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		res.Timeout = true
		res.Code = timeoutExitCode
		return res
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		res.Code = exitErr.ExitCode()
		return res
	}
	res.Err = err
	res.Code = 1
	return res
}

func runBash(ctx context.Context, command, cwd string, timeout time.Duration) (int, float64, []byte, []byte, error) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		return 0, 0, nil, nil, pErr("bash executable not found in PATH")
	}
	res := runProcess(ctx, []string{bash, "-c", command}, cwd, timeout)
	elapsed := roundSeconds(res.Elapsed)
	if res.Timeout {
		return timeoutExitCode, elapsed, res.Stdout, res.Stderr, nil
	}
	if res.Err != nil {
		return res.Code, elapsed, res.Stdout, res.Stderr, res.Err
	}
	return res.Code, elapsed, res.Stdout, res.Stderr, nil
}

func timeDur(seconds int64) time.Duration {
	return time.Duration(seconds) * time.Second
}

func roundSeconds(d time.Duration) float64 {
	ms := float64(d) / float64(time.Second)
	return float64(int64(ms*1000+0.5)) / 1000
}

func lookPython() (string, error) {
	path, err := exec.LookPath("python3")
	if err != nil {
		return "", pErr("python3 executable not found in PATH")
	}
	return path, nil
}

func lookGit() (string, error) {
	path, err := exec.LookPath("git")
	if err != nil {
		return "", pErr("git executable not found in PATH")
	}
	return path, nil
}

func runGit(ctx context.Context, repo string, args ...string) ([]byte, []byte, int, error) {
	git, err := lookGit()
	if err != nil {
		return nil, nil, 127, err
	}
	argv := make([]string, 0, len(args)+3)
	argv = append(argv, git, "-C", repo)
	argv = append(argv, args...)
	res := runProcess(ctx, argv, "", subprocessTimeoutSeconds*time.Second)
	if res.Err != nil {
		if res.Timeout {
			return res.Stdout, res.Stderr, res.Code, pErr("git command timed out after %ds", subprocessTimeoutSeconds)
		}
		return res.Stdout, res.Stderr, res.Code, res.Err
	}
	return res.Stdout, res.Stderr, res.Code, nil
}

func splitNull(b []byte) []string {
	if len(b) == 0 {
		return nil
	}
	parts := bytes.Split(b, []byte{0})
	var out []string
	for _, p := range parts {
		if len(p) == 0 {
			continue
		}
		out = append(out, string(p))
	}
	return out
}

func splitNullKeep(b []byte) []string {
	if len(b) == 0 {
		return nil
	}
	parts := bytes.Split(b, []byte{0})
	out := make([]string, len(parts))
	for i, p := range parts {
		out[i] = string(p)
	}
	return out
}
