package compose

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Client provides Docker Compose operations
type Client struct {
	executor   Executor
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
	// ForceRecreate passes --force-recreate. Needed because `docker compose
	// up` alone will not recreate a container whose image tag is
	// unchanged -- a freshly rebuilt image with the same tag would
	// otherwise leave the stale container running (documented failure
	// pattern ADR-000761 / PM-2026-005). Used by `altctl rebuild` for
	// exactly that reason.
	ForceRecreate bool
	// Services restricts the operation to specific service names instead
	// of every service defined across Files. Empty means "all services"
	// (existing up/restart behavior is unchanged).
	Services []string
	// Profiles adds `--profile <p>` for every entry, activating
	// compose-profile-gated services (see compose/*.yaml's `profiles:`
	// blocks and stack.Stack.Profile) alongside whatever Services names.
	// Compose already activates a profiled service's own profile when it's
	// named explicitly (naming a service on the command line overrides the
	// profile gate for that service and its dependencies), so Profiles is
	// mainly needed for parity with Down/Stop/Remove/Build, which don't
	// always name every profiled service the same way Up's Services does.
	Profiles []string
}

// DownOptions configures the down command
type DownOptions struct {
	Files         []string
	Volumes       bool
	RemoveOrphans bool
	Timeout       time.Duration
}

// StopOptions configures the stop command -- used instead of Down for a
// stack-scoped teardown (`altctl down <stack>`/`restart <stack>`'s stop
// phase), since `docker compose down [SERVICES]` scopes containers/networks
// to the named services but not volumes (`-v` still targets every named
// volume declared anywhere in the project's -f files, not just the ones
// belonging to the named services) -- `stop` + `rm -f -v` (see
// RemoveOptions) scope cleanly to just the named services' own containers
// and anonymous volumes instead.
type StopOptions struct {
	Files    []string
	Services []string
	Profiles []string
	Timeout  time.Duration
}

// RemoveOptions configures the rm command (paired with StopOptions -- see
// its doc comment for why `stop` + `rm` replaces a scoped `down`).
type RemoveOptions struct {
	Files    []string
	Services []string
	Profiles []string
	// Volumes passes -v to `rm`, removing anonymous volumes attached to the
	// named services' containers only -- not project-wide named volumes
	// (that's what a full, unscoped `altctl down --volumes` is for).
	Volumes bool
}

// BuildOptions configures the build command
type BuildOptions struct {
	Files    []string
	NoCache  bool
	Pull     bool
	Parallel bool
	Progress string
	// Services restricts the build to specific service names instead of
	// every service defined across Files. Empty means "all services"
	// (existing build behavior is unchanged).
	Services []string
	// Profiles adds `--profile <p>` for every entry -- see UpOptions.Profiles.
	Profiles []string
}

// LogsOptions configures the logs command
type LogsOptions struct {
	Follow     bool
	Tail       int
	Timestamps bool
	Since      string
}

// ServiceStatus represents the status of a running service.
//
// Name is the CONTAINER name ("alt-alt-backend-1", or the container_name:
// override); Service is the compose service name ("alt-backend"). Anything
// comparing against stack.Stack.Services must key on Service — keying on
// Name only matches the coincidental subset of services whose
// container_name: equals the service name.
type ServiceStatus struct {
	Name    string `json:"Name"`
	Service string `json:"Service"`
	State   string `json:"State"`
	Health  string `json:"Health"`
	Ports   string `json:"Ports"`
	// ExitCode is the container's exit code once it has exited. It is 0
	// while the container is still running. Used by internal/health to
	// distinguish a clean one-shot exit (migrator/init job) from a crash.
	ExitCode int `json:"ExitCode"`
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

// NewClientWithExecutor builds a Client against a caller-supplied Executor
// instead of a real *DefaultExecutor -- primarily for cmd package tests
// that need to capture the exact argv a command builds (file list,
// --profile, service names, --no-deps/--force-recreate, ...) without
// parsing dry-run log text. Production code should use NewClient.
func NewClientWithExecutor(executor Executor, projectDir, composeDir string, logger *slog.Logger) *Client {
	if logger == nil {
		logger = slog.Default()
	}
	return &Client{
		executor:   executor,
		projectDir: projectDir,
		composeDir: composeDir,
		logger:     logger,
	}
}

// Up starts services defined in the compose files
func (c *Client) Up(ctx context.Context, opts UpOptions) error {
	args := c.buildFileArgs(opts.Files)
	args = append(args, c.buildProfileArgs(opts.Profiles)...)
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
	if opts.ForceRecreate {
		args = append(args, "--force-recreate")
	}
	if opts.RemoveOrphans {
		args = append(args, "--remove-orphans")
	}
	if opts.Timeout > 0 {
		args = append(args, "--timeout", fmt.Sprintf("%d", int(opts.Timeout.Seconds())))
	}
	// Restrict to specific services when set (e.g. `altctl rebuild`, which
	// must only touch the targeted services, not every service in the
	// compose file). Compose treats trailing positional args after `up`'s
	// flags as the service names to operate on; omitted, it operates on
	// every service defined across -f files.
	args = append(args, opts.Services...)

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

// Stop stops (but does not remove) the named services -- see
// StopOptions/RemoveOptions doc comments for why a scoped teardown uses
// stop+rm instead of `down [SERVICES]`.
func (c *Client) Stop(ctx context.Context, opts StopOptions) error {
	args := c.buildFileArgs(opts.Files)
	args = append(args, c.buildProfileArgs(opts.Profiles)...)
	args = append(args, "stop")

	if opts.Timeout > 0 {
		args = append(args, "--timeout", fmt.Sprintf("%d", int(opts.Timeout.Seconds())))
	}
	args = append(args, opts.Services...)

	return c.executor.Run(ctx, "docker", append([]string{"compose"}, args...))
}

// Remove removes stopped service containers (and, with opts.Volumes, their
// anonymous volumes) for the named services only.
func (c *Client) Remove(ctx context.Context, opts RemoveOptions) error {
	args := c.buildFileArgs(opts.Files)
	args = append(args, c.buildProfileArgs(opts.Profiles)...)
	args = append(args, "rm", "-f")

	if opts.Volumes {
		args = append(args, "-v")
	}
	args = append(args, opts.Services...)

	return c.executor.Run(ctx, "docker", append([]string{"compose"}, args...))
}

// Build builds service images
func (c *Client) Build(ctx context.Context, opts BuildOptions) error {
	args := c.buildFileArgs(opts.Files)
	args = append(args, c.buildProfileArgs(opts.Profiles)...)
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
	if opts.Progress != "" {
		args = append(args, "--progress", opts.Progress)
	}
	// Restrict to specific services when set (see Up's opts.Services doc).
	args = append(args, opts.Services...)

	return c.executor.Run(ctx, "docker", append([]string{"compose"}, args...))
}

// Logs streams logs from one or more services in a single `docker compose
// logs` invocation. Compose accepts multiple service args natively, so all
// resolved services must be passed together here -- calling this once per
// service and looping is wrong for --follow: `docker compose logs -f` never
// exits, so a per-service loop sticks on the first service forever and the
// rest of the stack's logs are never tailed.
//
// files is the -f argument list (H2 fix: this used to be omitted entirely,
// so every real `altctl logs` invocation died with "no configuration file
// provided" the moment dry-run mode was off -- see cmd/logs.go's caller for
// how files is resolved via buildStackInvocation).
func (c *Client) Logs(ctx context.Context, files []string, services []string, opts LogsOptions) error {
	args := []string{"compose"}
	args = append(args, c.buildFileArgs(files)...)
	args = append(args, "logs")

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

	args = append(args, services...)
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

// Exec runs a command in a running container.
//
// files is the -f argument list (H2 fix: this used to be omitted entirely,
// so every real `altctl exec` invocation died with "no configuration file
// provided" -- see cmd/exec.go's caller for how files is resolved via
// buildStackInvocation).
func (c *Client) Exec(ctx context.Context, files []string, service string, command []string, stdout, stderr io.Writer) error {
	args := []string{"compose"}
	args = append(args, c.buildFileArgs(files)...)
	args = append(args, "exec", service)
	args = append(args, command...)

	return c.executor.RunWithPipes(ctx, "docker", args, stdout, stderr)
}

// buildFileArgs constructs the -f arguments for compose files.
// When .env exists in the project root, --env-file is prepended so that
// variable interpolation works regardless of the compose file location.
func (c *Client) buildFileArgs(files []string) []string {
	var args []string

	// Explicitly point to the project root .env for variable interpolation.
	// When -f is used, Docker Compose resolves .env relative to the first
	// compose file's directory, which may not be the project root.
	envFile := filepath.Join(c.projectDir, ".env")
	if _, err := os.Stat(envFile); err == nil {
		args = append(args, "--env-file", envFile)
	}

	for _, file := range files {
		if !filepath.IsAbs(file) {
			file = filepath.Join(c.composeDir, file)
		}
		args = append(args, "-f", file)
	}
	return args
}

// buildProfileArgs constructs `--profile <p>` global flags (repeatable, one
// per profile) for the compose-profile-gated stacks in play. Must be placed
// before the subcommand (up/down/build/...) alongside -f/--env-file --
// callers append this right after buildFileArgs's result.
func (c *Client) buildProfileArgs(profiles []string) []string {
	var args []string
	for _, p := range profiles {
		args = append(args, "--profile", p)
	}
	return args
}

// GetComposeFilePath returns the full path to a compose file
func (c *Client) GetComposeFilePath(filename string) string {
	return filepath.Join(c.composeDir, filename)
}
