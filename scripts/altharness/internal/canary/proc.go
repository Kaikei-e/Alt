package canary

import (
	"alt/scripts/altharness/internal/fsperm"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"
)

var ansiRE = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)

type cmdResult struct {
	stdout string
	stderr string
	code   int
	err    error
}

func (r cmdResult) combined() string {
	return r.stdout + "\n" + r.stderr
}

func look(bin string) string {
	path, err := exec.LookPath(bin)
	if err != nil {
		return ""
	}
	return path
}

func runCmd(ctx context.Context, timeout time.Duration, bin string, args []string, dir string, env []string) cmdResult {
	if ctx == nil {
		return cmdResult{code: 1, err: errors.New("nil context")}
	}
	if err := ctx.Err(); err != nil {
		return cmdResult{code: 1, err: err}
	}
	cctx := ctx
	cancel := func() {}
	if timeout > 0 {
		cctx, cancel = context.WithTimeout(ctx, timeout)
	}
	defer cancel()

	cmd := exec.CommandContext(cctx, bin, args...)
	cmd.Dir = dir
	if env != nil {
		cmd.Env = env
	}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return cmd.Process.Kill()
	}
	cmd.WaitDelay = 2 * time.Second
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	res := cmdResult{stdout: stdout.String(), stderr: stderr.String()}
	if err == nil {
		return res
	}
	if errors.Is(cctx.Err(), context.DeadlineExceeded) && ctx.Err() == nil {
		res.code = 1
		res.err = fmt.Errorf("timeout after %s", timeout)
		return res
	}
	if ctx.Err() != nil {
		res.code = 1
		res.err = ctx.Err()
		return res
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		res.code = exitErr.ExitCode()
		return res
	}
	res.code = 1
	res.err = err
	return res
}

func mergedEnv(overrides map[string]string) []string {
	if len(overrides) == 0 {
		return nil
	}
	merged := map[string]string{}
	for _, kv := range os.Environ() {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		merged[k] = v
	}
	for k, v := range overrides {
		merged[k] = v
	}
	keys := make([]string, 0, len(merged))
	for k := range merged {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, k+"="+merged[k])
	}
	return out
}

func withWorkDir(st streams, workDir, prefix, label string, fn func(dir string) int) int {
	dir := workDir
	cleanup := func() {}
	if dir == "" {
		tmp, err := os.MkdirTemp("", prefix)
		if err != nil {
			st.error(fmt.Sprintf("ERROR: CANARY AUDIT FAILED (%s): work dir: %s", label, err.Error()))
			return exitFail
		}
		dir = tmp
		cleanup = func() { _ = os.RemoveAll(tmp) }
	} else if err := os.MkdirAll(dir, fsperm.Dir); err != nil {
		st.error(fmt.Sprintf("ERROR: CANARY AUDIT FAILED (%s): work dir: %s", label, err.Error()))
		return exitFail
	}
	defer cleanup()
	return fn(dir)
}
