package doctor

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/alt-project/altctl/internal/compose"
)

// aggregateComposeFile is compose/compose.yaml -- the top-level `include:`
// aggregate that combines (almost) every stack's compose file. Several
// per-stack files (e.g. core.yaml) transitively `include: pki.yaml`, whose
// pki-agent sidecars depend_on services scattered across many other stacks
// (recap-subworker, acolyte-orchestrator, tag-generator, ...). Passing a
// narrow -f subset that happens to include one of those files fails compose
// project validation ("depends on undefined service"), even though the
// caller only asked about one stack. compose.yaml already resolves this by
// including everything that's meant to be run together, so it is used as
// the primary source of truth for `ps`/`config` whenever possible.
const aggregateComposeFile = "compose.yaml"

// psEntry is one line of `docker compose ps --format json`. Docker Compose
// outputs one JSON object per service per line; the fields used here go
// beyond internal/compose.ServiceStatus (which lacks ExitCode/Service),
// hence doctor decodes its own copy instead of reusing that type.
type psEntry struct {
	Name     string `json:"Name"`
	Service  string `json:"Service"`
	State    string `json:"State"`
	Status   string `json:"Status"`
	Health   string `json:"Health"`
	ExitCode int    `json:"ExitCode"`
}

// composeDependsOn mirrors one entry of a service's `depends_on` block in
// `docker compose config --format json` output.
type composeDependsOn struct {
	Condition string `json:"condition"`
	Required  bool   `json:"required"`
}

type composeServiceConfig struct {
	DependsOn   map[string]composeDependsOn `json:"depends_on,omitempty"`
	Healthcheck json.RawMessage             `json:"healthcheck,omitempty"`
}

// HasHealthcheck reports whether this service has a healthcheck: block at
// all (regardless of its content).
func (c composeServiceConfig) HasHealthcheck() bool {
	return len(c.Healthcheck) > 0
}

type composeConfigDoc struct {
	Services map[string]composeServiceConfig `json:"services"`
}

// envFileArgs mirrors compose.Client.buildFileArgs's --env-file handling:
// when a .env exists at the project root it's passed explicitly so variable
// interpolation works regardless of which compose file directory -f points
// into. compose.Client doesn't expose this, so doctor reimplements it
// against the Executor interface directly (per file-ownership: internal/
// compose is not to be edited).
func envFileArgs(projectDir string) []string {
	p := filepath.Join(projectDir, ".env")
	if _, err := os.Stat(p); err == nil {
		return []string{"--env-file", p}
	}
	return nil
}

func filePath(composeDir, name string) string {
	if filepath.IsAbs(name) {
		return name
	}
	return filepath.Join(composeDir, name)
}

func composeArgs(composeDir, projectDir string, files []string, sub string, extra ...string) []string {
	args := []string{"compose"}
	args = append(args, envFileArgs(projectDir)...)
	for _, f := range files {
		args = append(args, "-f", filePath(composeDir, f))
	}
	args = append(args, sub)
	args = append(args, extra...)
	return args
}

// checkDockerDaemon reports whether the docker daemon is reachable at all,
// distinct from any compose-project-level failure below it.
func checkDockerDaemon(ctx context.Context, exec compose.Executor) error {
	_, err := exec.RunWithOutput(ctx, "docker", []string{"info"})
	return err
}

// runComposePS runs `docker compose -f ... ps --format json` and decodes
// the newline-delimited JSON objects it prints, one per service.
func runComposePS(ctx context.Context, exec compose.Executor, composeDir, projectDir string, files []string) ([]psEntry, error) {
	args := composeArgs(composeDir, projectDir, files, "ps", "--format", "json", "--all")
	out, err := exec.RunWithOutput(ctx, "docker", args)
	if err != nil {
		return nil, err
	}
	return parsePSOutput(out)
}

func parsePSOutput(out []byte) ([]psEntry, error) {
	var entries []psEntry
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var e psEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			return nil, fmt.Errorf("parsing docker compose ps output: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, nil
}

// runComposeConfig runs `docker compose -f ... config --format json` and
// decodes it into a service-name-keyed map of depends_on/healthcheck info.
func runComposeConfig(ctx context.Context, exec compose.Executor, composeDir, projectDir string, files []string) (*composeConfigDoc, error) {
	args := composeArgs(composeDir, projectDir, files, "config", "--format", "json")
	out, err := exec.RunWithOutput(ctx, "docker", args)
	if err != nil {
		return nil, err
	}
	var doc composeConfigDoc
	if err := json.Unmarshal(out, &doc); err != nil {
		return nil, fmt.Errorf("parsing docker compose config output: %w", err)
	}
	return &doc, nil
}

// runComposeLogs runs `docker compose -f ... logs --tail N --no-color <svc>`
// and returns the captured lines.
func runComposeLogs(ctx context.Context, exec compose.Executor, composeDir, projectDir string, files []string, service string, tail int) ([]string, error) {
	args := composeArgs(composeDir, projectDir, files, "logs", "--tail", strconv.Itoa(tail), "--no-color", service)
	out, err := exec.RunWithOutput(ctx, "docker", args)
	if err != nil {
		return nil, err
	}
	text := strings.TrimRight(string(out), "\n")
	if text == "" {
		return nil, nil
	}
	return strings.Split(text, "\n"), nil
}
