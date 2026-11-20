package setup

import (
	"context"
	"os/exec"
	"strings"
)

// CheckResult represents the outcome of a single prerequisite check
type CheckResult struct {
	Name    string
	OK      bool
	Version string
	Detail  string
}

// CheckPrerequisites verifies that required tools are available
func CheckPrerequisites(ctx context.Context) []CheckResult {
	if ctx == nil {
		ctx = context.Background()
	}
	return []CheckResult{
		checkDockerCLI(ctx),
		checkDockerCompose(ctx),
		checkDockerDaemon(ctx),
	}
}

func checkDockerCLI(ctx context.Context) CheckResult {
	r := CheckResult{Name: "Docker CLI"}

	path, err := exec.LookPath("docker")
	if err != nil {
		r.Detail = "docker not found in PATH"
		return r
	}

	// #nosec G204 -- path resolved by exec.LookPath("docker") with constant argv; no shell
	out, err := exec.CommandContext(ctx, path, "version", "--format", "{{.Client.Version}}").Output()
	if err != nil {
		r.Detail = "failed to get docker version"
		return r
	}

	r.OK = true
	r.Version = strings.TrimSpace(string(out))
	return r
}

func checkDockerCompose(ctx context.Context) CheckResult {
	r := CheckResult{Name: "Docker Compose"}

	out, err := exec.CommandContext(ctx, "docker", "compose", "version", "--short").Output()
	if err != nil {
		r.Detail = "docker compose not available (requires Docker Compose V2)"
		return r
	}

	r.OK = true
	r.Version = strings.TrimSpace(string(out))
	return r
}

func checkDockerDaemon(ctx context.Context) CheckResult {
	r := CheckResult{Name: "Docker Daemon"}

	err := exec.CommandContext(ctx, "docker", "info").Run()
	if err != nil {
		r.Detail = "Docker daemon is not running"
		return r
	}

	r.OK = true
	r.Detail = "running"
	return r
}
