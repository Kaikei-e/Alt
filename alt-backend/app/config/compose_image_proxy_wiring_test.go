package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// composeFilesDefiningAltBackend lists every compose file that declares an
// alt-backend service. validateImageProxyConfig makes a missing image-proxy
// secret a non-zero exit, so each of these is a deployment that either starts
// or does not — and `go test ./...` cannot see any of them.
//
// Keep this list in sync with `grep -n '^  alt-backend:' compose/*.yaml`. A
// fourth compose file that forgets the image proxy is exactly the regression
// this guard exists to catch, so TestComposeAltBackendDefinitionsAreEnumerated
// below fails when one appears that is not listed here.
var composeFilesDefiningAltBackend = []string{
	"compose/core.yaml",
	"compose/dev.yaml",
	"compose/compose.staging.yaml",
}

// altBackendService is the slice of a compose service definition this guard
// needs. `environment:` accepts both the list form ("- KEY=value", which all
// three files use today) and the map form, so ImageProxyEnv is normalised in
// parseEnvironment rather than at the assertion site.
type altBackendService struct {
	Environment yaml.Node `yaml:"environment"`
	EnvFile     yaml.Node `yaml:"env_file"`
	Secrets     []string  `yaml:"secrets"`
}

// hasEnvFile reports whether the service loads an env_file. Compose accepts
// both a bare string and a list.
func (s altBackendService) hasEnvFile() bool {
	return s.EnvFile.Kind == yaml.ScalarNode || s.EnvFile.Kind == yaml.SequenceNode
}

type composeFile struct {
	Services map[string]altBackendService `yaml:"services"`
}

// parseEnvironment normalises compose's two `environment:` spellings into a
// map. Only the `environment:` block counts: `env_file: ../.env` points at an
// operator-supplied file that is not in the repo, so a compose file that leans
// on it is not self-sufficient and cannot be verified here.
func parseEnvironment(node *yaml.Node) (map[string]string, error) {
	env := map[string]string{}
	switch node.Kind {
	case 0: // absent
		return env, nil
	case yaml.SequenceNode:
		var entries []string
		if err := node.Decode(&entries); err != nil {
			return nil, err
		}
		for _, entry := range entries {
			key, value, _ := strings.Cut(entry, "=")
			env[key] = value
		}
	case yaml.MappingNode:
		if err := node.Decode(&env); err != nil {
			return nil, err
		}
	}
	return env, nil
}

// findRepoFile walks up from the package directory to the repo root.
func findRepoFile(t *testing.T, relPath string) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		candidate := filepath.Join(dir, filepath.FromSlash(relPath))
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			// Fail rather than skip, for the same reason the Prometheus
			// scrape-config guard does: the backend CI job checks out only the
			// directories named in .github/workflows/backend-go.yaml, so
			// dropping `compose` from that list must turn the build red
			// instead of silently disabling this check.
			t.Fatalf("%s not found walking up from the package dir; add `compose` to the sparse-checkout in .github/workflows/backend-go.yaml", relPath)
		}
		dir = parent
	}
}

func loadAltBackendService(t *testing.T, relPath string) altBackendService {
	t.Helper()
	raw, err := os.ReadFile(findRepoFile(t, relPath))
	if err != nil {
		t.Fatalf("read %s: %v", relPath, err)
	}

	var parsed composeFile
	if err := yaml.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("parse %s: %v", relPath, err)
	}

	service, ok := parsed.Services["alt-backend"]
	if !ok {
		t.Fatalf("%s declares no alt-backend service; parser is broken or the service was renamed", relPath)
	}
	return service
}

// TestComposeAltBackendImageProxyIsExplicitlyWired pins the deployment side of
// the CLAUDE.md rule 9 fix for the image proxy.
//
// validateImageProxyConfig turns "IMAGE_PROXY_ENABLED=true with no secret" into
// a startup error, and IMAGE_PROXY_ENABLED defaults to true. That makes silence
// fatal: a compose file that mentions neither IMAGE_PROXY_ENABLED nor a secret
// source no longer starts alt-backend at all. Only compose/core.yaml wired the
// secret, so this guard is what keeps the dev stack and the staging Hurl slice
// from being broken by a config-package change that their own test suites never
// exercise.
//
// Each definition must therefore make one of two explicit choices:
//   - supply a secret (IMAGE_PROXY_SECRET or IMAGE_PROXY_SECRET_FILE), or
//   - opt out loudly with IMAGE_PROXY_ENABLED=false.
func TestComposeAltBackendImageProxyIsExplicitlyWired(t *testing.T) {
	for _, relPath := range composeFilesDefiningAltBackend {
		t.Run(relPath, func(t *testing.T) {
			service := loadAltBackendService(t, relPath)

			env, err := parseEnvironment(&service.Environment)
			if err != nil {
				t.Fatalf("parse %s environment: %v", relPath, err)
			}
			if len(env) == 0 {
				t.Fatalf("%s: alt-backend has an empty environment block; parser is broken", relPath)
			}

			enabled, hasEnabled := env["IMAGE_PROXY_ENABLED"]
			secret, hasSecret := env["IMAGE_PROXY_SECRET"]
			secretFile, hasSecretFile := env["IMAGE_PROXY_SECRET_FILE"]

			// The explicit opt-out. di/image_module.go logs image_proxy_disabled
			// for this, which is the loud "disabled is a config value" state
			// CLAUDE.md rule 9 asks for.
			if hasEnabled && enabled == "false" {
				return
			}

			if !hasSecret && !hasSecretFile {
				t.Fatalf("%s: alt-backend sets neither IMAGE_PROXY_SECRET nor IMAGE_PROXY_SECRET_FILE, "+
					"and does not opt out with IMAGE_PROXY_ENABLED=false. IMAGE_PROXY_ENABLED defaults to true, "+
					"so config.validateImageProxyConfig rejects this and the container exits non-zero at startup",
					relPath)
			}
			if hasSecret && strings.TrimSpace(secret) == "" {
				t.Errorf("%s: IMAGE_PROXY_SECRET is empty, which validateImageProxyConfig rejects", relPath)
			}

			// A *_FILE pointing into /run/secrets only resolves if the service
			// also mounts that Docker secret; otherwise config/env.go fails the
			// read before validateImageProxyConfig even runs.
			if hasSecretFile && strings.HasPrefix(secretFile, "/run/secrets/") {
				name := strings.TrimPrefix(secretFile, "/run/secrets/")
				if !containsString(service.Secrets, name) {
					t.Errorf("%s: IMAGE_PROXY_SECRET_FILE points at /run/secrets/%s but the service's secrets: "+
						"list is %v; config/env.go fails the read and alt-backend exits non-zero",
						relPath, name, service.Secrets)
				}
			}
		})
	}
}

// TestComposeAltBackendAppEnvHasOneSource pins the delivery path for APP_ENV.
//
// .env.example and .env.template both document APP_ENV in the repo-root .env,
// which compose loads through `env_file: ../.env`. An `environment:` entry for
// the same key takes precedence over env_file, and `${APP_ENV}` interpolates
// from the compose project directory rather than that root .env — so declaring
// APP_ENV in `environment:` silently pins the operator's documented setting
// back to the default. That is the same "documented knob reaches nothing"
// failure this task was opened to fix, so it must not come back.
func TestComposeAltBackendAppEnvHasOneSource(t *testing.T) {
	for _, relPath := range composeFilesDefiningAltBackend {
		t.Run(relPath, func(t *testing.T) {
			service := loadAltBackendService(t, relPath)
			if !service.hasEnvFile() {
				// No env_file, so `environment:` is the only source and
				// there is nothing to shadow.
				return
			}

			env, err := parseEnvironment(&service.Environment)
			if err != nil {
				t.Fatalf("parse %s environment: %v", relPath, err)
			}
			if value, ok := env["APP_ENV"]; ok {
				t.Errorf("%s: alt-backend sets APP_ENV=%s in environment: while also loading env_file. "+
					"environment: wins, so APP_ENV set in the root .env (as .env.example documents) is "+
					"silently ignored. Delete the environment: entry and let env_file deliver it",
					relPath, value)
			}
		})
	}
}

// TestComposeAltBackendDefinitionsAreEnumerated keeps
// composeFilesDefiningAltBackend honest. Without it, adding a fourth compose
// file that defines alt-backend would leave the image-proxy guard above
// passing while that deployment fails to start.
func TestComposeAltBackendDefinitionsAreEnumerated(t *testing.T) {
	composeDir := filepath.Dir(findRepoFile(t, composeFilesDefiningAltBackend[0]))
	entries, err := filepath.Glob(filepath.Join(composeDir, "*.yaml"))
	if err != nil {
		t.Fatalf("glob %s: %v", composeDir, err)
	}
	if len(entries) == 0 {
		t.Fatalf("no compose files found in %s; glob is broken", composeDir)
	}

	for _, path := range entries {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		var parsed composeFile
		if err := yaml.Unmarshal(raw, &parsed); err != nil {
			// Not every compose file is a plain service map; skip what this
			// narrow struct cannot read rather than failing on unrelated syntax.
			continue
		}
		if _, ok := parsed.Services["alt-backend"]; !ok {
			continue
		}

		relPath := "compose/" + filepath.Base(path)
		if !containsString(composeFilesDefiningAltBackend, relPath) {
			t.Errorf("%s defines an alt-backend service but is missing from composeFilesDefiningAltBackend; "+
				"add it so its image-proxy wiring is checked", relPath)
		}
	}
}

func containsString(haystack []string, needle string) bool {
	for _, item := range haystack {
		if item == needle {
			return true
		}
	}
	return false
}
