package doctor

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"

	"gopkg.in/yaml.v3"
)

// checkDotEnv reports a Finding when .env is missing at the project root.
// Several compose files (via env_file: / variable interpolation) hard-fail
// `docker compose config`/`ps` without it -- this is doctor's most load-
// bearing preflight check, since its absence otherwise surfaces only as a
// cryptic "env file ... not found" error from the aggregate probe below.
func checkDotEnv(projectDir string) (Finding, bool) {
	p := filepath.Join(projectDir, ".env")
	if _, err := os.Stat(p); err == nil {
		return Finding{}, false
	}
	return Finding{
		Severity: SeverityError,
		Category: "preflight",
		Message:  "missing .env at repo root",
		Detail:   p + " does not exist -- docker compose config/ps for the full stack needs it for env_file: directives and variable interpolation",
		Prescription: []string{
			"cp .env.example .env",
			"altctl init",
		},
	}, true
}

// composeSecretsDoc decodes only the top-level `secrets:` key of a compose
// file, the same yaml.Node approach as composeIncludeDoc/composeFileDoc.
type composeSecretsDoc struct {
	Secrets yaml.Node `yaml:"secrets"`
}

// readComposeSecretFiles returns name -> file path (as written in the
// compose file, typically "../secrets/x.txt") for every top-level secret
// declared in a compose file's `secrets:` block.
func readComposeSecretFiles(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc composeSecretsDoc
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	if doc.Secrets.Kind != yaml.MappingNode {
		return nil, nil
	}
	result := make(map[string]string)
	for i := 0; i+1 < len(doc.Secrets.Content); i += 2 {
		name := doc.Secrets.Content[i].Value
		val := doc.Secrets.Content[i+1]
		if val.Kind != yaml.MappingNode {
			continue
		}
		for j := 0; j+1 < len(val.Content); j += 2 {
			if val.Content[j].Value == "file" {
				result[name] = val.Content[j+1].Value
			}
		}
	}
	return result, nil
}

// checkSecrets compares compose/base.yaml's secrets: block against what's
// actually present on disk, returning one Finding listing everything
// missing (or a Finding explaining why the check itself couldn't run).
func checkSecrets(composeDir string) []Finding {
	basePath := filepath.Join(composeDir, "base.yaml")
	secretFiles, err := readComposeSecretFiles(basePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return []Finding{{
			Severity: SeverityWarning,
			Category: "preflight",
			Message:  "could not read compose/base.yaml's secrets: block",
			Detail:   err.Error(),
		}}
	}

	var missing []string
	for name, rel := range secretFiles {
		abs := rel
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(composeDir, rel)
		}
		if _, statErr := os.Stat(abs); statErr != nil {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	sort.Strings(missing)
	return []Finding{{
		Severity: SeverityError,
		Category: "preflight",
		Message:  formatMissingSecretsMessage(len(missing), len(secretFiles)),
		Detail:   strings.Join(missing, ", "),
		Prescription: []string{
			"provision the missing secrets/*.txt files (see docs/services/altctl.md)",
			"altctl init",
		},
	}}
}

func formatMissingSecretsMessage(missing, total int) string {
	if missing == total {
		return "secrets/ directory is missing or empty (all " + strconv.Itoa(total) + " declared secrets absent)"
	}
	return strconv.Itoa(missing) + " of " + strconv.Itoa(total) + " declared secret file(s) missing under secrets/"
}

// DockerSocket is the host socket docker-socket-proxy-ro bind-mounts.
const DockerSocket = "/var/run/docker.sock"

const socketProxyService = "docker-socket-proxy-ro"

type composeServiceUsersDoc struct {
	Services map[string]struct {
		User string `yaml:"user"`
	} `yaml:"services"`
}

// dockerSocketGroupFinding reports when the host Docker socket's group is
// not the gid compose/logging.yaml pins in docker-socket-proxy-ro's user:.
// The proxy reaches the socket through that group alone, so a mismatch
// leaves every log forwarder and cAdvisor behind it without a Docker API.
func dockerSocketGroupFinding(composeDir, socketPath string) (Finding, bool) {
	loggingPath := filepath.Join(composeDir, "logging.yaml")
	data, err := os.ReadFile(loggingPath)
	if err != nil {
		return Finding{
			Severity: SeverityWarning,
			Category: "preflight",
			Stack:    "logging",
			Message:  "could not read compose/logging.yaml to check the Docker socket group",
			Detail:   err.Error(),
		}, true
	}
	var doc composeServiceUsersDoc
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return Finding{
			Severity: SeverityWarning,
			Category: "preflight",
			Stack:    "logging",
			Message:  "could not parse compose/logging.yaml to check the Docker socket group",
			Detail:   err.Error(),
		}, true
	}
	proxy, declared := doc.Services[socketProxyService]
	if !declared {
		return Finding{}, false
	}
	_, wantGID, hasGroup := strings.Cut(proxy.User, ":")
	if !hasGroup || wantGID == "" {
		return Finding{
			Severity: SeverityError,
			Category: "preflight",
			Stack:    "logging",
			Message:  socketProxyService + " user: " + strconv.Quote(proxy.User) + " names no group",
			Detail:   "the proxy reads " + socketPath + " through its group; compose/logging.yaml must pin it as \"uid:gid\"",
		}, true
	}

	info, err := os.Stat(socketPath)
	if err != nil {
		return Finding{
			Severity: SeverityWarning,
			Category: "preflight",
			Stack:    "logging",
			Message:  "could not inspect the Docker socket's group",
			Detail:   err.Error(),
		}, true
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return Finding{
			Severity: SeverityWarning,
			Category: "preflight",
			Stack:    "logging",
			Message:  "could not inspect the Docker socket's group",
			Detail:   "no ownership information for " + socketPath + " on this platform",
		}, true
	}
	gotGID := strconv.FormatUint(uint64(stat.Gid), 10)
	if gotGID == wantGID {
		return Finding{}, false
	}
	return Finding{
		Severity: SeverityError,
		Category: "preflight",
		Stack:    "logging",
		Message:  "Docker socket group " + gotGID + " does not match " + socketProxyService + "'s gid " + wantGID,
		Detail:   socketPath + " is group " + gotGID + ", but compose/logging.yaml runs " + socketProxyService + " as " + strconv.Quote(proxy.User) + "; the proxy cannot open the socket and the log forwarders and cAdvisor lose the Docker API",
		Prescription: []string{
			"getent group docker",
			"set docker-socket-proxy-ro's user: in compose/logging.yaml and the docker-socket owner in deploy/host-prereqs.yaml to the host's docker gid",
		},
	}, true
}
