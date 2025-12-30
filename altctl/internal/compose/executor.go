// Package compose provides Docker Compose operations for altctl
package compose

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strings"
)

// Executor runs shell commands
type Executor interface {
	Run(ctx context.Context, cmd string, args []string) error
	RunWithOutput(ctx context.Context, cmd string, args []string) ([]byte, error)
	RunWithPipes(ctx context.Context, cmd string, args []string, stdout, stderr io.Writer) error
}

// DefaultExecutor implements Executor using os/exec
type DefaultExecutor struct {
	workDir string
	env     []string
	logger  *slog.Logger
	dryRun  bool
}

// NewExecutor creates a new command executor
func NewExecutor(workDir string, logger *slog.Logger, dryRun bool) *DefaultExecutor {
	return &DefaultExecutor{
		workDir: workDir,
		env:     os.Environ(),
		logger:  logger,
		dryRun:  dryRun,
	}
}

// Run executes a command and waits for completion
func (e *DefaultExecutor) Run(ctx context.Context, cmd string, args []string) error {
	e.logger.Debug("executing command",
		"cmd", cmd,
		"args", args,
		"workdir", e.workDir,
	)

	if e.dryRun {
		fmt.Printf("[dry-run] %s %s\n", cmd, strings.Join(args, " "))
		return nil
	}

	c := exec.CommandContext(ctx, cmd, args...)
	c.Dir = e.workDir
	c.Env = e.env
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Stdin = os.Stdin

	return c.Run()
}

// RunWithOutput executes a command and returns its output
func (e *DefaultExecutor) RunWithOutput(ctx context.Context, cmd string, args []string) ([]byte, error) {
	e.logger.Debug("executing command with output capture",
		"cmd", cmd,
		"args", args,
		"workdir", e.workDir,
	)

	if e.dryRun {
		fmt.Printf("[dry-run] %s %s\n", cmd, strings.Join(args, " "))
		return nil, nil
	}

	c := exec.CommandContext(ctx, cmd, args...)
	c.Dir = e.workDir
	c.Env = e.env

	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr

	err := c.Run()
	if err != nil {
		return nil, fmt.Errorf("%w: %s", err, stderr.String())
	}

	return stdout.Bytes(), nil
}

// RunWithPipes executes a command with custom stdout/stderr writers
func (e *DefaultExecutor) RunWithPipes(ctx context.Context, cmd string, args []string, stdout, stderr io.Writer) error {
	e.logger.Debug("executing command with pipes",
		"cmd", cmd,
		"args", args,
		"workdir", e.workDir,
	)

	if e.dryRun {
		fmt.Fprintf(stdout, "[dry-run] %s %s\n", cmd, strings.Join(args, " "))
		return nil
	}

	c := exec.CommandContext(ctx, cmd, args...)
	c.Dir = e.workDir
	c.Env = e.env
	c.Stdout = stdout
	c.Stderr = stderr

	return c.Run()
}

// SetEnv adds or updates an environment variable
func (e *DefaultExecutor) SetEnv(key, value string) {
	// Remove existing key if present
	prefix := key + "="
	newEnv := make([]string, 0, len(e.env)+1)
	for _, env := range e.env {
		if !strings.HasPrefix(env, prefix) {
			newEnv = append(newEnv, env)
		}
	}
	newEnv = append(newEnv, prefix+value)
	e.env = newEnv
}
