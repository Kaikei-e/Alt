package config

import (
	"os"
	"path/filepath"
	"testing"
)

// imageProxyEnvKeys are reset before every case so a leaked value from a
// neighbouring test cannot decide the outcome of a fail-fast assertion.
var imageProxyEnvKeys = []string{
	"APP_ENV",
	"IMAGE_PROXY_ENABLED",
	"IMAGE_PROXY_SECRET",
	"IMAGE_PROXY_SECRET_FILE",
	"BACKEND_TOKEN_SECRET",
	"BACKEND_TOKEN_SECRET_FILE",
}

// applyEnv clears the image-proxy / app-env variables and then applies the
// case's overrides. t.Setenv restores the previous values on cleanup.
func applyEnv(t *testing.T, envVars map[string]string) {
	t.Helper()
	for _, key := range imageProxyEnvKeys {
		t.Setenv(key, "")
	}
	for key, value := range envVars {
		t.Setenv(key, value)
	}
}

// writeSecretFile writes content to a temp file and returns its path,
// emulating the Docker Secrets mount compose/core.yaml wires as
// IMAGE_PROXY_SECRET_FILE=/run/secrets/image_proxy_secret.
func writeSecretFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "image_proxy_secret")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write secret file: %v", err)
	}
	return path
}

// TestNewConfig_ImageProxyFailsFastWithoutSecret pins CLAUDE.md rule 9 for the
// image proxy: IMAGE_PROXY_ENABLED defaults to true and compose/core.yaml
// mounts IMAGE_PROXY_SECRET_FILE, so an empty (or whitespace-only) secret file
// used to leave the proxy enabled-but-unwired. The guard must not depend on
// APP_ENV — APP_ENV is set in no compose file, which made the production-only
// panic in di/image_module.go permanently unreachable.
func TestNewConfig_ImageProxyFailsFastWithoutSecret(t *testing.T) {
	emptySecretFile := writeSecretFile(t, "")
	whitespaceSecretFile := writeSecretFile(t, "  \n\t ")
	goodSecretFile := writeSecretFile(t, "dev-image-proxy-secret\n")

	tests := []struct {
		name    string
		envVars map[string]string
		wantErr bool
		errMsg  string
	}{
		{
			name: "empty secret file fails fast",
			envVars: map[string]string{
				"IMAGE_PROXY_SECRET_FILE": emptySecretFile,
			},
			wantErr: true,
			errMsg:  "IMAGE_PROXY_ENABLED=true requires a non-empty secret",
		},
		{
			name: "whitespace-only secret file fails fast",
			envVars: map[string]string{
				"IMAGE_PROXY_SECRET_FILE": whitespaceSecretFile,
			},
			wantErr: true,
			errMsg:  "IMAGE_PROXY_ENABLED=true requires a non-empty secret",
		},
		{
			name:    "no secret configured at all fails fast",
			envVars: map[string]string{},
			wantErr: true,
			errMsg:  "IMAGE_PROXY_ENABLED=true requires a non-empty secret",
		},
		{
			name: "development is not exempt from the guard",
			envVars: map[string]string{
				"APP_ENV":                 "development",
				"IMAGE_PROXY_SECRET_FILE": emptySecretFile,
			},
			wantErr: true,
			errMsg:  "IMAGE_PROXY_ENABLED=true requires a non-empty secret",
		},
		{
			name: "explicitly disabled proxy starts",
			envVars: map[string]string{
				"IMAGE_PROXY_ENABLED": "false",
			},
			wantErr: false,
		},
		{
			name: "non-empty secret file starts",
			envVars: map[string]string{
				"IMAGE_PROXY_SECRET_FILE": goodSecretFile,
			},
			wantErr: false,
		},
		{
			name: "IMAGE_PROXY_SECRET env var starts",
			envVars: map[string]string{
				"IMAGE_PROXY_SECRET": "dev-image-proxy-secret",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			applyEnv(t, tt.envVars)

			cfg, err := NewConfig()
			if tt.wantErr {
				if err == nil {
					t.Error("NewConfig() expected error but got none")
				} else if tt.errMsg != "" && !contains(err.Error(), tt.errMsg) {
					t.Errorf("NewConfig() error = %v, want to contain %s", err, tt.errMsg)
				}
				return
			}

			if err != nil {
				t.Fatalf("NewConfig() unexpected error: %v", err)
			}
			if cfg.ImageProxy.Enabled && cfg.ImageProxy.Secret == "" {
				t.Error("NewConfig() returned an enabled image proxy with an empty secret")
			}
		})
	}
}

// TestNewConfig_AppEnvMustBeAKnownValue pins CLAUDE.md rule 9 for APP_ENV
// itself: an unrecognised value (a "prod" / "prd" typo) must not silently
// resolve to the permissive development behaviour of the remaining
// environment-keyed guards.
func TestNewConfig_AppEnvMustBeAKnownValue(t *testing.T) {
	tests := []struct {
		name       string
		envVars    map[string]string
		wantErr    bool
		errMsg     string
		wantAppEnv string
	}{
		{
			name: "unset APP_ENV resolves to development",
			envVars: map[string]string{
				"IMAGE_PROXY_ENABLED": "false",
			},
			wantAppEnv: "development",
		},
		{
			name: "staging is accepted",
			envVars: map[string]string{
				"APP_ENV":             "staging",
				"IMAGE_PROXY_ENABLED": "false",
			},
			wantAppEnv: "staging",
		},
		{
			name: "production is accepted",
			envVars: map[string]string{
				"APP_ENV":              "production",
				"IMAGE_PROXY_ENABLED":  "false",
				"BACKEND_TOKEN_SECRET": "this-is-a-long-enough-backend-token-secret",
			},
			wantAppEnv: "production",
		},
		{
			name: "typo is rejected instead of degrading to development",
			envVars: map[string]string{
				"APP_ENV":             "prod",
				"IMAGE_PROXY_ENABLED": "false",
			},
			wantErr: true,
			errMsg:  "APP_ENV must be one of",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			applyEnv(t, tt.envVars)

			cfg, err := NewConfig()
			if tt.wantErr {
				if err == nil {
					t.Error("NewConfig() expected error but got none")
				} else if tt.errMsg != "" && !contains(err.Error(), tt.errMsg) {
					t.Errorf("NewConfig() error = %v, want to contain %s", err, tt.errMsg)
				}
				return
			}

			if err != nil {
				t.Fatalf("NewConfig() unexpected error: %v", err)
			}
			if cfg.AppEnv != tt.wantAppEnv {
				t.Errorf("AppEnv = %s, want %s", cfg.AppEnv, tt.wantAppEnv)
			}
		})
	}
}
