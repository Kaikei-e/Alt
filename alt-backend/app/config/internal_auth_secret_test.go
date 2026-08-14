package config

import (
	"os"
	"path/filepath"
	"testing"
)

// internalAuthEnvKeys are reset before every case so a value leaked from a
// neighbouring test cannot decide a fail-fast assertion.
var internalAuthEnvKeys = []string{
	"APP_ENV",
	"IMAGE_PROXY_ENABLED",
	"BACKEND_TOKEN_SECRET",
	"BACKEND_TOKEN_SECRET_FILE",
	"INTERNAL_AUTH_SECRET",
	"INTERNAL_AUTH_SECRET_FILE",
}

func applyInternalAuthEnv(t *testing.T, envVars map[string]string) {
	t.Helper()
	for _, key := range internalAuthEnvKeys {
		t.Setenv(key, "")
	}
	t.Setenv("IMAGE_PROXY_ENABLED", "false")
	for key, value := range envVars {
		t.Setenv(key, value)
	}
}

// writeInternalAuthSecretFile emulates the Docker Secrets mount compose wires
// as INTERNAL_AUTH_SECRET_FILE=/run/secrets/internal_auth_secret.
func writeInternalAuthSecretFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "internal_auth_secret")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write secret file: %v", err)
	}
	return path
}

// TestNewConfig_InternalAuthSecret separates the /internal shared bearer from
// the HS256 key that signs backend JWTs.
//
// cmd/datahub sends this value in a plaintext X-Internal-Auth header on every
// auth-hub GetSystemUser call, so it reaches nginx access logs and OTel span
// attributes. The signing key must never travel there, which makes the two
// secrets two different values — and a config that sets them to the same
// string exits non-zero rather than quietly restoring the old exposure.
//
// The _FILE guard follows BACKEND_TOKEN_SECRET_FILE's shape (CLAUDE.md rule 9):
// a mounted-but-empty or unreadable secret is a misconfiguration, not a
// disabled feature.
func TestNewConfig_InternalAuthSecret(t *testing.T) {
	emptySecretFile := writeInternalAuthSecretFile(t, "")
	whitespaceSecretFile := writeInternalAuthSecretFile(t, "  \n\t ")
	goodSecretFile := writeInternalAuthSecretFile(t, "dev-internal-auth-secret-long-enough\n")
	missingSecretFile := filepath.Join(t.TempDir(), "never-mounted")

	tests := []struct {
		name       string
		envVars    map[string]string
		wantErr    bool
		errMsg     string
		wantSecret string
	}{
		{
			name: "empty secret file fails fast",
			envVars: map[string]string{
				"INTERNAL_AUTH_SECRET_FILE": emptySecretFile,
			},
			wantErr: true,
			errMsg:  "resolved to an empty secret",
		},
		{
			name: "whitespace-only secret file fails fast",
			envVars: map[string]string{
				"INTERNAL_AUTH_SECRET_FILE": whitespaceSecretFile,
			},
			wantErr: true,
			errMsg:  "resolved to an empty secret",
		},
		{
			name: "unreadable secret file fails fast",
			envVars: map[string]string{
				"INTERNAL_AUTH_SECRET_FILE": missingSecretFile,
			},
			wantErr: true,
			errMsg:  "INTERNAL_AUTH_SECRET_FILE",
		},
		{
			name: "development is not exempt from the guard",
			envVars: map[string]string{
				"APP_ENV":                   "development",
				"INTERNAL_AUTH_SECRET_FILE": emptySecretFile,
			},
			wantErr: true,
			errMsg:  "resolved to an empty secret",
		},
		{
			name: "reusing the JWT signing key is rejected",
			envVars: map[string]string{
				"BACKEND_TOKEN_SECRET": "one-secret-doing-two-jobs-is-the-bug",
				"INTERNAL_AUTH_SECRET": "one-secret-doing-two-jobs-is-the-bug",
			},
			wantErr: true,
			errMsg:  "INTERNAL_AUTH_SECRET must not equal BACKEND_TOKEN_SECRET",
		},
		{
			name: "reusing the JWT signing key is rejected across env and mount",
			envVars: map[string]string{
				"BACKEND_TOKEN_SECRET":      "dev-internal-auth-secret-long-enough",
				"INTERNAL_AUTH_SECRET_FILE": goodSecretFile,
			},
			wantErr: true,
			errMsg:  "INTERNAL_AUTH_SECRET must not equal BACKEND_TOKEN_SECRET",
		},
		{
			name: "non-empty secret file starts",
			envVars: map[string]string{
				"BACKEND_TOKEN_SECRET":      "env-supplied-backend-token-secret",
				"INTERNAL_AUTH_SECRET_FILE": goodSecretFile,
			},
			wantSecret: "dev-internal-auth-secret-long-enough",
		},
		{
			name: "INTERNAL_AUTH_SECRET env var starts",
			envVars: map[string]string{
				"BACKEND_TOKEN_SECRET": "env-supplied-backend-token-secret",
				"INTERNAL_AUTH_SECRET": "env-supplied-internal-auth-secret",
			},
			wantSecret: "env-supplied-internal-auth-secret",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			applyInternalAuthEnv(t, tt.envVars)

			cfg, err := NewConfig()
			if tt.wantErr {
				if err == nil {
					t.Fatalf("NewConfig() expected error but got none")
				}
				if tt.errMsg != "" && !contains(err.Error(), tt.errMsg) {
					t.Errorf("NewConfig() error = %v, want to contain %s", err, tt.errMsg)
				}
				return
			}

			if err != nil {
				t.Fatalf("NewConfig() unexpected error: %v", err)
			}
			if cfg.Auth.InternalAuthSecret != tt.wantSecret {
				t.Errorf("Auth.InternalAuthSecret = %q, want %q", cfg.Auth.InternalAuthSecret, tt.wantSecret)
			}
			if cfg.Auth.InternalAuthSecret == cfg.Auth.BackendTokenSecret {
				t.Errorf("Auth.InternalAuthSecret must differ from Auth.BackendTokenSecret, both are %q",
					cfg.Auth.InternalAuthSecret)
			}
		})
	}
}
