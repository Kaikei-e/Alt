package mtlsedge

import (
	"bytes"
	"context"
	"io"
	"os/exec"
	"strings"

	"github.com/alt-project/altctl/internal/compose"
)

type frozenExecutor struct {
	workDir string
	env     []string
}

// DefaultExecutorFactory creates a compose.Executor that runs commands with an explicit frozen environment.
func DefaultExecutorFactory(workDir string, env []string) compose.Executor {
	return &frozenExecutor{
		workDir: workDir,
		env:     append([]string(nil), env...),
	}
}

func (e *frozenExecutor) Run(ctx context.Context, cmd string, args []string) error {
	var stdout, stderr bytes.Buffer
	return e.RunWithPipes(ctx, cmd, args, &stdout, &stderr)
}

func (e *frozenExecutor) RunWithOutput(ctx context.Context, cmd string, args []string) ([]byte, error) {
	var stdout, stderr bytes.Buffer
	if err := e.RunWithPipes(ctx, cmd, args, &stdout, &stderr); err != nil {
		return nil, err
	}
	return stdout.Bytes(), nil
}

func (e *frozenExecutor) RunWithPipes(ctx context.Context, cmd string, args []string, stdout, stderr io.Writer) error {
	c := exec.CommandContext(ctx, cmd, args...)
	c.Dir = e.workDir
	c.Env = e.env
	c.Stdout = stdout
	c.Stderr = stderr
	return c.Run()
}

func probeDockerAndMeta(ctx context.Context, runner compose.Executor) (reachable bool, dockerContext string, composeVersion string) {
	var out, errBuf bytes.Buffer
	if err := runner.RunWithPipes(ctx, "docker", []string{"info", "--format", "{{json .ServerVersion}}"}, &out, &errBuf); err != nil {
		return false, StatusNotObserved, StatusNotObserved
	}

	dockerContext = StatusNotObserved
	out.Reset()
	errBuf.Reset()
	if err := runner.RunWithPipes(ctx, "docker", []string{"context", "show"}, &out, &errBuf); err == nil {
		if v := strings.TrimSpace(out.String()); v != "" {
			dockerContext = v
		}
	}

	composeVersion = StatusNotObserved
	out.Reset()
	errBuf.Reset()
	if err := runner.RunWithPipes(ctx, "docker", []string{"compose", "version", "--short"}, &out, &errBuf); err == nil {
		if v := strings.TrimSpace(out.String()); v != "" {
			composeVersion = v
		}
	}

	return true, dockerContext, composeVersion
}
