package setup

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
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

// DefaultSecretSpecs returns the complete list of secret file specifications
func DefaultSecretSpecs() []SecretSpec {
	return []SecretSpec{
		// Auto-generated secrets
		{Filename: "postgres_password.txt", Description: "PostgreSQL superuser password", AutoGenerate: true, Length: 32},
		{Filename: "db_password.txt", Description: "Application database password", AutoGenerate: true, Length: 32},
		{Filename: "pre_processor_db_password.txt", Description: "Pre-processor service DB password", AutoGenerate: true, Length: 32},
		{Filename: "pre_processor_sidecar_db_password.txt", Description: "Pre-processor sidecar DB password", AutoGenerate: true, Length: 32},
		{Filename: "tag_generator_db_password.txt", Description: "Tag generator service DB password", AutoGenerate: true, Length: 32},
		{Filename: "search_indexer_db_password.txt", Description: "Search indexer service DB password", AutoGenerate: true, Length: 32},
		{Filename: "recap_db_password.txt", Description: "Recap database password", AutoGenerate: true, Length: 32},
		{Filename: "rag_db_password.txt", Description: "RAG database password", AutoGenerate: true, Length: 32},
		{Filename: "kratos_db_password.txt", Description: "Kratos database password", AutoGenerate: true, Length: 32},
		{Filename: "kratos_cookie_secret.txt", Description: "Kratos cookie encryption secret", AutoGenerate: true, Length: 32},
		{Filename: "kratos_cipher_secret.txt", Description: "Kratos cipher secret (exactly 32 chars)", AutoGenerate: true, Length: 32, Truncate: 32},
		{Filename: "meili_master_key.txt", Description: "Meilisearch master key", AutoGenerate: true, Length: 32},
		{Filename: "clickhouse_password.txt", Description: "ClickHouse password", AutoGenerate: true, Length: 32},
		{Filename: "csrf_secret.txt", Description: "CSRF token secret (min 32 chars)", AutoGenerate: true, Length: 32},
		{Filename: "auth_shared_secret.txt", Description: "Auth shared secret", AutoGenerate: true, Length: 32},
		{Filename: "backend_token_secret.txt", Description: "Backend JWT token secret", AutoGenerate: true, Length: 32},
		{Filename: "pp_db_password.txt", Description: "Pre-processor dedicated DB password", AutoGenerate: true, Length: 32},
		{Filename: "image_proxy_secret.txt", Description: "Image proxy HMAC secret", AutoGenerate: true, Length: 32},
		// User-provided secrets (empty placeholders)
		{Filename: "hugging_face_token.txt", Description: "Hugging Face API token (for AI features)", AutoGenerate: false},
		{Filename: "inoreader_client_id.txt", Description: "Inoreader OAuth client ID", AutoGenerate: false},
		{Filename: "inoreader_client_secret.txt", Description: "Inoreader OAuth client secret", AutoGenerate: false},
	}
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

		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
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
