package setup

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

// SecretSpec defines a secret file to be generated
type SecretSpec struct {
	Filename     string
	Description  string
	AutoGenerate bool // false = user-provided (empty placeholder)
	Length       int  // bytes for random generation (default 32)
	Truncate     int  // if > 0, truncate the encoded output to this many characters
}

// SecretsResult holds the outcome of a secrets generation run
type SecretsResult struct {
	Created []string
	Skipped []string
}

// secretMeta declares the generation strategy for one known secret file:
// whether it's auto-generated or a user-provided placeholder, and (for
// auto-generated secrets) the random byte length and any truncation.
//
// This table is metadata only -- it does NOT decide which secrets are
// required. That list is derived at runtime from compose/base.yaml's
// secrets: block (see secretNamesFromComposeFile / DefaultSecretSpecs),
// the same "derive from compose/*.yaml, don't hardcode" philosophy
// internal/stack uses for the stack registry. A secret base.yaml declares
// that has no entry here still gets generated, via defaultSecretMeta.
var knownSecretMeta = map[string]secretMeta{
	"backend_token_secret.txt":              {"Backend JWT token secret", true, 32, 0},
	"postgres_password.txt":                 {"PostgreSQL superuser password", true, 32, 0},
	"db_password.txt":                       {"Application database password", true, 32, 0},
	"pre_processor_sidecar_db_password.txt": {"Pre-processor sidecar DB password", true, 32, 0},
	"recap_db_password.txt":                 {"Recap database password", true, 32, 0},
	// ADR-000371: DB passwords must be URL-safe base64 -- PgBouncer parses
	// DATABASE_URL as a URL, so '+' / '/' / padding in the password breaks
	// parsing. generateRandomSecret always uses base64.RawURLEncoding, so
	// this holds for every auto-generated secret below, not just DB ones.
	"sovereign_db_password.txt":           {"Knowledge Sovereign database password", true, 32, 0},
	"rag_db_password.txt":                 {"RAG database password", true, 32, 0},
	"kratos_db_password.txt":              {"Kratos database password", true, 32, 0},
	"kratos_cookie_secret.txt":            {"Kratos cookie encryption secret", true, 32, 0},
	"kratos_cipher_secret.txt":            {"Kratos cipher secret (exactly 32 chars)", true, 32, 32},
	"meili_master_key.txt":                {"Meilisearch master key", true, 32, 0},
	"meili_search_key.txt":                {"Meilisearch search-only API key", true, 32, 0},
	"clickhouse_password.txt":             {"ClickHouse password", true, 32, 0},
	"csrf_secret.txt":                     {"CSRF token secret (min 32 chars)", true, 32, 0},
	"grafana_admin_password.txt":          {"Grafana admin password", true, 32, 0},
	"restic_password.txt":                 {"Restic backup encryption password", true, 32, 0},
	"acolyte_db_password.txt":             {"Acolyte database password", true, 32, 0},
	"pp_db_password.txt":                  {"Pre-processor dedicated DB password", true, 32, 0},
	"image_proxy_secret.txt":              {"Image proxy HMAC secret", true, 32, 0},
	"step_ca_root_password.txt":           {"step-ca root CA password", true, 32, 0},
	"pact_broker_basic_auth_password.txt": {"Pact Broker basic-auth password", true, 32, 0},
	"pact_db_password.txt":                {"Pact Broker database password", true, 32, 0},
	"internal_auth_token.txt":             {"auth-token-manager internal auth token (X-Internal-Auth)", true, 32, 0},
	// User-provided secrets: empty placeholders, operator fills them in.
	"hugging_face_token.txt":      {"Hugging Face API token (for AI features)", false, 0, 0},
	"inoreader_client_id.txt":     {"Inoreader OAuth client ID", false, 0, 0},
	"inoreader_client_secret.txt": {"Inoreader OAuth client secret", false, 0, 0},
}

// secretMeta is the per-secret generation strategy attached to a derived
// secret filename.
type secretMeta struct {
	Description  string
	AutoGenerate bool
	Length       int
	Truncate     int
}

// defaultSecretMeta is applied to any secret compose/base.yaml declares
// that isn't in knownSecretMeta yet -- e.g. a secret just added to
// base.yaml before someone remembers to describe it here. Auto-generated,
// 32 random bytes, URL-safe base64 (ADR-000371) is a safe default for a
// brand-new secret: it's always safer to generate one than to silently
// skip it and leave a container failing PASSWORD_FILE reads at startup.
var defaultSecretMeta = secretMeta{AutoGenerate: true, Length: 32}

// composeSecretsDoc decodes only the top-level "secrets:" key of a compose
// YAML document as a raw yaml.Node, mirroring internal/stack's
// composeFileDoc technique -- we only need the mapping's keys/values, not
// full anchor/alias resolution.
type composeSecretsDoc struct {
	Secrets yaml.Node `yaml:"secrets"`
}

// secretFileEntry is a single `secrets:` block entry: `name: {file: path}`.
type secretFileEntry struct {
	File string `yaml:"file"`
}

// secretNamesFromComposeFile reads a compose YAML file (compose/base.yaml)
// and returns the base filenames declared by its top-level secrets: block's
// `file: ../secrets/X.txt` entries, sorted for deterministic output. This
// is the single source of truth for which secret files `altctl init` must
// generate -- compose/base.yaml, not a hand-maintained Go list.
func secretNamesFromComposeFile(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading compose file %s: %w", path, err)
	}

	var doc composeSecretsDoc
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parsing compose file %s: %w", path, err)
	}

	if doc.Secrets.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("compose file %s has no top-level secrets: block", path)
	}

	var names []string
	for i := 0; i+1 < len(doc.Secrets.Content); i += 2 {
		valueNode := doc.Secrets.Content[i+1]
		var entry secretFileEntry
		if err := valueNode.Decode(&entry); err != nil || entry.File == "" {
			continue
		}
		names = append(names, filepath.Base(entry.File))
	}
	sort.Strings(names)
	return names, nil
}

// specsFromNames builds SecretSpecs for the given secret filenames,
// attaching known generation-strategy metadata where declared in
// knownSecretMeta and defaultSecretMeta otherwise.
func specsFromNames(names []string) []SecretSpec {
	specs := make([]SecretSpec, 0, len(names))
	for _, name := range names {
		meta, ok := knownSecretMeta[name]
		if !ok {
			meta = defaultSecretMeta
			meta.Description = fmt.Sprintf("%s (new secret found in compose/base.yaml; using safe auto-generate default)", name)
		}
		specs = append(specs, SecretSpec{
			Filename:     name,
			Description:  meta.Description,
			AutoGenerate: meta.AutoGenerate,
			Length:       meta.Length,
			Truncate:     meta.Truncate,
		})
	}
	return specs
}

// DeriveSecretSpecs derives the full secret spec list from a compose base
// YAML file's secrets: block (e.g. compose/base.yaml). Exported so callers
// (and tests) can point it at a specific file instead of relying on
// DefaultSecretSpecs' runtime repo-root discovery.
func DeriveSecretSpecs(baseComposeFile string) ([]SecretSpec, error) {
	names, err := secretNamesFromComposeFile(baseComposeFile)
	if err != nil {
		return nil, err
	}
	return specsFromNames(names), nil
}

// findBaseComposeFile locates compose/base.yaml by walking up from the
// current working directory looking for a compose/base.yaml file directly
// (not just a directory named "compose" -- altctl/internal/compose is a Go
// package directory of that same name, which a looser check would match
// first when walking up from this package's own `go test` cwd). This works
// both for a real `altctl init` run (cwd somewhere under the repo) and for
// `go test` (cwd = this package's dir).
func findBaseComposeFile() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir := cwd
	for {
		candidate := filepath.Join(dir, "compose", "base.yaml")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("compose/base.yaml not found (walked up from %s)", cwd)
		}
		dir = parent
	}
}

// DefaultSecretSpecs returns the complete list of secret file
// specifications, derived at runtime from compose/base.yaml's secrets:
// block (single source of truth -- see secretNamesFromComposeFile), with
// per-secret generation strategy (length/truncation/auto-vs-user-provided)
// attached as declared metadata from knownSecretMeta, falling back to a
// safe default for any secret base.yaml declares that isn't in that table
// yet.
//
// C2: this used to silently fall back to a 3-secret placeholder list when
// compose/base.yaml couldn't be located or parsed, and callers (cmd/init.go)
// both generated AND validated against that same shrunken list -- so `altctl
// init` reported "All required files present" while only 3 of the 24
// required secrets existed. Critical Rule 8 forbids exactly this shape of
// silent degrade for an unwired/broken dependency: a parse failure here must
// surface loudly, not be indistinguishable from "everything's fine." Callers
// now get the error and must hard-fail.
func DefaultSecretSpecs() ([]SecretSpec, error) {
	path, err := findBaseComposeFile()
	if err != nil {
		return nil, fmt.Errorf("locating compose/base.yaml: %w", err)
	}
	specs, err := DeriveSecretSpecs(path)
	if err != nil {
		return nil, fmt.Errorf("deriving secret specs from %s: %w", path, err)
	}
	return specs, nil
}

// GenerateSecrets creates secret files in the given directory.
// Existing files are skipped unless force is true.
func GenerateSecrets(dir string, specs []SecretSpec, force bool) (*SecretsResult, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("creating secrets directory: %w", err)
	}

	result := &SecretsResult{}

	for _, spec := range specs {
		path := filepath.Join(dir, spec.Filename)

		if !force {
			if _, err := os.Stat(path); err == nil {
				result.Skipped = append(result.Skipped, spec.Filename)
				continue
			}
		}

		var content string
		if spec.AutoGenerate {
			length := spec.Length
			if length == 0 {
				length = 32
			}
			var err error
			content, err = generateRandomSecret(length)
			if err != nil {
				return nil, fmt.Errorf("generating secret %s: %w", spec.Filename, err)
			}
			if spec.Truncate > 0 && len(content) > spec.Truncate {
				content = content[:spec.Truncate]
			}
		}

		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			return nil, fmt.Errorf("writing secret %s: %w", spec.Filename, err)
		}
		result.Created = append(result.Created, spec.Filename)
	}

	return result, nil
}

// generateRandomSecret produces a URL-safe base64-encoded random string.
// Uses RawURLEncoding (no padding, no '+' or '/') so secrets are safe
// for embedding in DATABASE_URL and similar connection strings.
func generateRandomSecret(length int) (string, error) {
	buf := make([]byte, length)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("reading random bytes: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
