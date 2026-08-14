package config

import (
	"os"
	"path/filepath"
	"testing"
)

// writeBackendTokenSecretFile emulates the Docker Secrets mount compose wires
// as BACKEND_TOKEN_SECRET_FILE=/run/secrets/backend_token_secret.
func writeBackendTokenSecretFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "backend_token_secret")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write secret file: %v", err)
	}
	return path
}

// TestNewConfig_BackendTokenSecretFileFailsFast pins CLAUDE.md rule 9 for the
// JWT signing secret. Every compose file points BACKEND_TOKEN_SECRET_FILE at a
// mounted secret, and middleware/jwt_middleware.go plus
// connect/v2/middleware/auth_interceptor.go verify every browser token against
// the value it resolves to. An empty result is therefore not a disabled
// feature: the container stays healthy while answering 401 to every
// authenticated REST and Connect call, with nothing in the log to say why.
//
// The guard must not be keyed on APP_ENV — APP_ENV is set in no compose file,
// so validateAuthConfig's env == "production" branch never fires in a real
// deployment (the same trap validateImageProxyConfig documents).
func TestNewConfig_BackendTokenSecretFileFailsFast(t *testing.T) {
	emptySecretFile := writeBackendTokenSecretFile(t, "")
	whitespaceSecretFile := writeBackendTokenSecretFile(t, "  \n\t ")
	goodSecretFile := writeBackendTokenSecretFile(t, "dev-backend-token-secret-long-enough\n")
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
				"BACKEND_TOKEN_SECRET_FILE": emptySecretFile,
			},
			wantErr: true,
			errMsg:  "resolved to an empty secret",
		},
		{
			name: "whitespace-only secret file fails fast",
			envVars: map[string]string{
				"BACKEND_TOKEN_SECRET_FILE": whitespaceSecretFile,
			},
			wantErr: true,
			errMsg:  "resolved to an empty secret",
		},
		{
			name: "unreadable secret file fails fast",
			envVars: map[string]string{
				"BACKEND_TOKEN_SECRET_FILE": missingSecretFile,
			},
			wantErr: true,
			errMsg:  "BACKEND_TOKEN_SECRET_FILE",
		},
		{
			name: "development is not exempt from the guard",
			envVars: map[string]string{
				"APP_ENV":                   "development",
				"BACKEND_TOKEN_SECRET_FILE": emptySecretFile,
			},
			wantErr: true,
			errMsg:  "resolved to an empty secret",
		},
		{
			// The old block re-read the file and assigned the trimmed result
			// unconditionally, so an empty mount overwrote a perfectly good
			// BACKEND_TOKEN_SECRET with "". A contradictory pair is a
			// misconfiguration either way, so it exits non-zero rather than
			// picking a winner silently.
			name: "an empty mount is rejected even when BACKEND_TOKEN_SECRET is set",
			envVars: map[string]string{
				"BACKEND_TOKEN_SECRET":      "env-supplied-backend-token-secret",
				"BACKEND_TOKEN_SECRET_FILE": emptySecretFile,
			},
			wantErr: true,
			errMsg:  "resolved to an empty secret",
		},
		{
			name: "non-empty secret file starts",
			envVars: map[string]string{
				"BACKEND_TOKEN_SECRET_FILE": goodSecretFile,
			},
			wantSecret: "dev-backend-token-secret-long-enough",
		},
		{
			name: "BACKEND_TOKEN_SECRET env var starts",
			envVars: map[string]string{
				"BACKEND_TOKEN_SECRET": "env-supplied-backend-token-secret",
			},
			wantSecret: "env-supplied-backend-token-secret",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			envVars := map[string]string{"IMAGE_PROXY_ENABLED": "false"}
			for key, value := range tt.envVars {
				envVars[key] = value
			}
			applyEnv(t, envVars)

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
			if cfg.Auth.BackendTokenSecret != tt.wantSecret {
				t.Errorf("Auth.BackendTokenSecret = %q, want %q", cfg.Auth.BackendTokenSecret, tt.wantSecret)
			}
		})
	}
}
