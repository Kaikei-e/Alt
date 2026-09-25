package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var sovereignAuthEnvKeys = []string{
	"APP_ENV",
	"IMAGE_PROXY_ENABLED",
	"BACKEND_TOKEN_SECRET",
	"SOVEREIGN_URL",
	"SOVEREIGN_EVENT_TOKEN",
	"SOVEREIGN_EVENT_TOKEN_FILE",
	"SOVEREIGN_EVENT_AUTH",
}

func applySovereignAuthEnv(t *testing.T, envVars map[string]string) {
	t.Helper()
	for _, key := range sovereignAuthEnvKeys {
		t.Setenv(key, "")
	}
	t.Setenv("IMAGE_PROXY_ENABLED", "false")
	t.Setenv("BACKEND_TOKEN_SECRET", "dev-backend-token-secret-sufficient-length")
	for key, value := range envVars {
		t.Setenv(key, value)
	}
}

func writeSovereignTokenFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sovereign_event_token")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write token file: %v", err)
	}
	return path
}

func TestNewConfig_SovereignEventAuth(t *testing.T) {
	emptyTokenFile := writeSovereignTokenFile(t, "")
	whitespaceTokenFile := writeSovereignTokenFile(t, "  \n\t  ")
	goodTokenFile := writeSovereignTokenFile(t, "test-sovereign-token-value-1234567890\n")
	missingTokenFile := filepath.Join(t.TempDir(), "never-mounted-token")

	tests := []struct {
		name      string
		envVars   map[string]string
		wantErr   bool
		errMsg    string
		wantToken string
	}{
		{
			name: "unset token and unset auth fails fast when SOVEREIGN_URL is set",
			envVars: map[string]string{
				"SOVEREIGN_URL": "http://knowledge-sovereign:9500",
			},
			wantErr: true,
			errMsg:  "SOVEREIGN_EVENT_TOKEN_FILE or SOVEREIGN_EVENT_TOKEN is required",
		},
		{
			name: "empty token file fails fast",
			envVars: map[string]string{
				"SOVEREIGN_URL":              "http://knowledge-sovereign:9500",
				"SOVEREIGN_EVENT_TOKEN_FILE": emptyTokenFile,
			},
			wantErr: true,
			errMsg:  "must be at least 24 characters",
		},
		{
			name: "whitespace-only token file fails fast",
			envVars: map[string]string{
				"SOVEREIGN_URL":              "http://knowledge-sovereign:9500",
				"SOVEREIGN_EVENT_TOKEN_FILE": whitespaceTokenFile,
			},
			wantErr: true,
			errMsg:  "must be at least 24 characters",
		},
		{
			name: "short token file (< 24 chars) fails fast",
			envVars: map[string]string{
				"SOVEREIGN_URL":              "http://knowledge-sovereign:9500",
				"SOVEREIGN_EVENT_TOKEN_FILE": writeSovereignTokenFile(t, "short-secret"),
			},
			wantErr: true,
			errMsg:  "must be at least 24 characters",
		},
		{
			name: "valid plain env token loads successfully",
			envVars: map[string]string{
				"SOVEREIGN_URL":         "http://knowledge-sovereign:9500",
				"SOVEREIGN_EVENT_TOKEN": "plain-env-token-with-at-least-24-chars",
			},
			wantErr:   false,
			wantToken: "plain-env-token-with-at-least-24-chars",
		},
		{
			name: "short plain env token (< 24 chars) fails fast",
			envVars: map[string]string{
				"SOVEREIGN_URL":         "http://knowledge-sovereign:9500",
				"SOVEREIGN_EVENT_TOKEN": "too-short",
			},
			wantErr: true,
			errMsg:  "must be at least 24 characters",
		},
		{
			name: "unreadable token file fails fast",
			envVars: map[string]string{
				"SOVEREIGN_URL":              "http://knowledge-sovereign:9500",
				"SOVEREIGN_EVENT_TOKEN_FILE": missingTokenFile,
			},
			wantErr: true,
			errMsg:  "SOVEREIGN_EVENT_TOKEN_FILE",
		},
		{
			name: "disabled auth is allowed in development",
			envVars: map[string]string{
				"APP_ENV":              "development",
				"SOVEREIGN_URL":        "http://knowledge-sovereign:9500",
				"SOVEREIGN_EVENT_AUTH": "disabled",
			},
			wantErr:   false,
			wantToken: "",
		},
		{
			name: "disabled auth is rejected in production",
			envVars: map[string]string{
				"APP_ENV":              "production",
				"SOVEREIGN_URL":        "http://knowledge-sovereign:9500",
				"SOVEREIGN_EVENT_AUTH": "disabled",
			},
			wantErr: true,
			errMsg:  "not permitted in production",
		},
		{
			name: "valid token file loads successfully",
			envVars: map[string]string{
				"SOVEREIGN_URL":              "http://knowledge-sovereign:9500",
				"SOVEREIGN_EVENT_TOKEN_FILE": goodTokenFile,
			},
			wantErr:   false,
			wantToken: "test-sovereign-token-value-1234567890",
		},
		{
			name: "when SOVEREIGN_URL is empty token is not required",
			envVars: map[string]string{
				"SOVEREIGN_URL": "",
			},
			wantErr:   false,
			wantToken: "",
		},
		{
			name: "when SOVEREIGN_URL is empty unreadable token file is ignored",
			envVars: map[string]string{
				"SOVEREIGN_URL":              "",
				"SOVEREIGN_EVENT_TOKEN_FILE": missingTokenFile,
			},
			wantErr:   false,
			wantToken: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			applySovereignAuthEnv(t, tt.envVars)

			cfg, err := NewConfig()
			if tt.wantErr {
				if err == nil {
					t.Fatalf("NewConfig() expected error but got none")
				}
				if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("NewConfig() error = %v, want to contain %s", err, tt.errMsg)
				}
				return
			}

			if err != nil {
				t.Fatalf("NewConfig() unexpected error: %v", err)
			}
			if cfg.Sovereign.EventToken != tt.wantToken {
				t.Errorf("Sovereign.EventToken = %q, want %q", cfg.Sovereign.EventToken, tt.wantToken)
			}
		})
	}
}
