package setup

import (
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
func CheckPrerequisites() []CheckResult {
	return []CheckResult{
		checkDockerCLI(),
		checkDockerCompose(),
		checkDockerDaemon(),
	}
}

func checkDockerCLI() CheckResult {
	r := CheckResult{Name: "Docker CLI"}

	path, err := exec.LookPath("docker")
	if err != nil {
		r.Detail = "docker not found in PATH"
		return r
	}

	out, err := exec.Command(path, "version", "--format", "{{.Client.Version}}").Output()
	if err != nil {
		r.Detail = "failed to get docker version"
		return r
	}

	r.OK = true
	r.Version = strings.TrimSpace(string(out))
	return r
}

func checkDockerCompose() CheckResult {
	r := CheckResult{Name: "Docker Compose"}

	out, err := exec.Command("docker", "compose", "version", "--short").Output()
	if err != nil {
		r.Detail = "docker compose not available (requires Docker Compose V2)"
		return r
	}

	r.OK = true
	r.Version = strings.TrimSpace(string(out))
	return r
}

func checkDockerDaemon() CheckResult {
	r := CheckResult{Name: "Docker Daemon"}

	err := exec.Command("docker", "info").Run()
	if err != nil {
		r.Detail = "Docker daemon is not running"
		return r
	}

	r.OK = true
	r.Detail = "running"
	return r
}
