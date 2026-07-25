package doctor

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

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

// dockerGroupIDFinding returns a Finding when DOCKER_GROUP_ID is unset,
// for callers that have already determined the logging stack is in scope
// (compose/logging.yaml's rask-log-forwarder sidecars require it via a
// `${DOCKER_GROUP_ID:?...}` hard-fail interpolation).
func dockerGroupIDFinding() (Finding, bool) {
	if os.Getenv("DOCKER_GROUP_ID") != "" {
		return Finding{}, false
	}
	return Finding{
		Severity: SeverityError,
		Category: "preflight",
		Stack:    "logging",
		Message:  "DOCKER_GROUP_ID is not set",
		Detail:   "required by the logging stack's rask-log-forwarder sidecars (compose/logging.yaml); docker compose will hard-fail to even parse the logging stack without it",
		Prescription: []string{
			"export DOCKER_GROUP_ID=$(./scripts/get-docker-gid.sh)",
		},
	}, true
}
