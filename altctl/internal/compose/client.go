package compose

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"time"
)

// Client provides Docker Compose operations
type Client struct {
	executor   *DefaultExecutor
	projectDir string
	composeDir string
	logger     *slog.Logger
}

// UpOptions configures the up command
type UpOptions struct {
	Files         []string
	Detach        bool
	Build         bool
	NoDeps        bool
	Timeout       time.Duration
	RemoveOrphans bool
}

// DownOptions configures the down command
type DownOptions struct {
	Files         []string
	Volumes       bool
	RemoveOrphans bool
	Timeout       time.Duration
}

// BuildOptions configures the build command
type BuildOptions struct {
	Files    []string
	NoCache  bool
	Pull     bool
	Parallel bool
}

// LogsOptions configures the logs command
type LogsOptions struct {
	Follow     bool
	Tail       int
	Timestamps bool
	Since      string
}

// ServiceStatus represents the status of a running service
type ServiceStatus struct {
	Name   string `json:"Name"`
	State  string `json:"State"`
	Health string `json:"Health"`
	Ports  string `json:"Ports"`
}

// NewClient creates a new Docker Compose client
func NewClient(projectDir, composeDir string, logger *slog.Logger, dryRun bool) *Client {
	return &Client{
		executor:   NewExecutor(projectDir, logger, dryRun),
		projectDir: projectDir,
		composeDir: composeDir,
		logger:     logger,
	}
}

// Up starts services defined in the compose files
func (c *Client) Up(ctx context.Context, opts UpOptions) error {
	args := c.buildFileArgs(opts.Files)
	args = append(args, "up")

	if opts.Detach {
		args = append(args, "-d")
	}
	if opts.Build {
		args = append(args, "--build")
	}
	if opts.NoDeps {
		args = append(args, "--no-deps")
	}
	if opts.RemoveOrphans {
		args = append(args, "--remove-orphans")
	}
	if opts.Timeout > 0 {
		args = append(args, "--timeout", fmt.Sprintf("%d", int(opts.Timeout.Seconds())))
	}

	return c.executor.Run(ctx, "docker", append([]string{"compose"}, args...))
}

// Down stops and removes services
func (c *Client) Down(ctx context.Context, opts DownOptions) error {
	args := c.buildFileArgs(opts.Files)
	args = append(args, "down")

	if opts.Volumes {
		args = append(args, "-v")
	}
	if opts.RemoveOrphans {
		args = append(args, "--remove-orphans")
	}
	if opts.Timeout > 0 {
		args = append(args, "--timeout", fmt.Sprintf("%d", int(opts.Timeout.Seconds())))
	}

	return c.executor.Run(ctx, "docker", append([]string{"compose"}, args...))
}

// Build builds service images
func (c *Client) Build(ctx context.Context, opts BuildOptions) error {
	args := c.buildFileArgs(opts.Files)
	args = append(args, "build")

	if opts.NoCache {
		args = append(args, "--no-cache")
	}
	if opts.Pull {
		args = append(args, "--pull")
	}
	if opts.Parallel {
		args = append(args, "--parallel")
	}

	return c.executor.Run(ctx, "docker", append([]string{"compose"}, args...))
}

// Logs streams logs from a service
func (c *Client) Logs(ctx context.Context, service string, opts LogsOptions) error {
	args := []string{"compose", "logs"}

	if opts.Follow {
		args = append(args, "-f")
	}
	if opts.Tail > 0 {
		args = append(args, "--tail", fmt.Sprintf("%d", opts.Tail))
	}
	if opts.Timestamps {
		args = append(args, "-t")
	}
	if opts.Since != "" {
		args = append(args, "--since", opts.Since)
	}

	args = append(args, service)
	return c.executor.Run(ctx, "docker", args)
}

// PS returns the status of running services
func (c *Client) PS(ctx context.Context, files []string) ([]ServiceStatus, error) {
	args := c.buildFileArgs(files)
	args = append(args, "ps", "--format", "json")

	output, err := c.executor.RunWithOutput(ctx, "docker", append([]string{"compose"}, args...))
	if err != nil {
		return nil, fmt.Errorf("getting service status: %w", err)
	}

	if len(output) == 0 {
		return nil, nil
	}

	var statuses []ServiceStatus

	// Docker compose outputs one JSON object per line
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		var status ServiceStatus
		if err := json.Unmarshal([]byte(line), &status); err != nil {
			c.logger.Warn("failed to parse service status", "line", line, "error", err)
			continue
		}
		statuses = append(statuses, status)
	}

	return statuses, nil
}

// Config validates and displays the compose configuration
func (c *Client) Config(ctx context.Context, files []string) ([]byte, error) {
	args := c.buildFileArgs(files)
	args = append(args, "config")

	return c.executor.RunWithOutput(ctx, "docker", append([]string{"compose"}, args...))
}

// Exec runs a command in a running container
func (c *Client) Exec(ctx context.Context, service string, command []string, stdout, stderr io.Writer) error {
	args := []string{"compose", "exec", service}
	args = append(args, command...)

	return c.executor.RunWithPipes(ctx, "docker", args, stdout, stderr)
}

// buildFileArgs constructs the -f arguments for compose files
func (c *Client) buildFileArgs(files []string) []string {
	var args []string
	for _, file := range files {
		// Convert to absolute path if needed
		if !filepath.IsAbs(file) {
			file = filepath.Join(c.composeDir, file)
		}
		args = append(args, "-f", file)
	}
	return args
}

// GetComposeFilePath returns the full path to a compose file
func (c *Client) GetComposeFilePath(filename string) string {
	return filepath.Join(c.composeDir, filename)
}
