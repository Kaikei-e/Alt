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

// TestNewConfig_InternalAuthSecret pins CLAUDE.md rule 9 for the /internal
// shared bearer cmd/datahub sends on auth-hub GetSystemUser calls: a
// mounted-but-empty or unreadable secret is a misconfiguration, not a disabled
// feature.
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
			name: "non-empty secret file starts",
			envVars: map[string]string{
				"INTERNAL_AUTH_SECRET_FILE": goodSecretFile,
			},
			wantSecret: "dev-internal-auth-secret-long-enough",
		},
		{
			name: "INTERNAL_AUTH_SECRET env var starts",
			envVars: map[string]string{
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
		})
	}
}
