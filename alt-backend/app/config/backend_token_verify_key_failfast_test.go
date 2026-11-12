package config

import (
	"crypto/ed25519"
	"os"
	"path/filepath"
	"testing"
)

func writeBackendVerifyKeyFile(t *testing.T, content []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "backend_token_verify_key")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("failed to write verify key file: %v", err)
	}
	return path
}

func TestValidateBackendConfig_VerifyKeyFileFailsFast(t *testing.T) {
	validPub, validPEM := marshalTestEd25519SPKIPEM(t)
	ecdsaPEM := marshalTestECDSASPKIPEM(t)
	privatePEM := marshalTestEd25519PKCS8PrivatePEM(t)

	emptyKeyFile := writeBackendVerifyKeyFile(t, nil)
	whitespaceKeyFile := writeBackendVerifyKeyFile(t, []byte("  \n\t "))
	notPEMKeyFile := writeBackendVerifyKeyFile(t, []byte("not-a-pem-key"))
	trailingKeyFile := writeBackendVerifyKeyFile(t, append(append([]byte{}, validPEM...), []byte("trailing-bytes")...))
	ecdsaKeyFile := writeBackendVerifyKeyFile(t, ecdsaPEM)
	privateKeyFile := writeBackendVerifyKeyFile(t, privatePEM)
	goodKeyFile := writeBackendVerifyKeyFile(t, validPEM)
	missingKeyFile := filepath.Join(t.TempDir(), "never-mounted")

	tests := []struct {
		name    string
		envVars map[string]string
		wantErr bool
		errMsg  string
		wantKey ed25519.PublicKey
	}{
		{
			name:    "unset BACKEND_TOKEN_VERIFY_KEY_FILE fails fast",
			envVars: map[string]string{},
			wantErr: true,
			errMsg:  "BACKEND_TOKEN_VERIFY_KEY_FILE",
		},
		{
			name: "inline BACKEND_TOKEN_VERIFY_KEY env is ignored and fails fast",
			envVars: map[string]string{
				"BACKEND_TOKEN_VERIFY_KEY": string(validPEM),
			},
			wantErr: true,
			errMsg:  "BACKEND_TOKEN_VERIFY_KEY_FILE",
		},
		{
			name: "empty key file fails fast",
			envVars: map[string]string{
				"BACKEND_TOKEN_VERIFY_KEY_FILE": emptyKeyFile,
			},
			wantErr: true,
			errMsg:  "BACKEND_TOKEN_VERIFY_KEY_FILE",
		},
		{
			name: "whitespace-only key file fails fast",
			envVars: map[string]string{
				"BACKEND_TOKEN_VERIFY_KEY_FILE": whitespaceKeyFile,
			},
			wantErr: true,
			errMsg:  "BACKEND_TOKEN_VERIFY_KEY_FILE",
		},
		{
			name: "unreadable key file fails fast",
			envVars: map[string]string{
				"BACKEND_TOKEN_VERIFY_KEY_FILE": missingKeyFile,
			},
			wantErr: true,
			errMsg:  "BACKEND_TOKEN_VERIFY_KEY_FILE",
		},
		{
			name: "non-PEM content fails fast",
			envVars: map[string]string{
				"BACKEND_TOKEN_VERIFY_KEY_FILE": notPEMKeyFile,
			},
			wantErr: true,
			errMsg:  "BACKEND_TOKEN_VERIFY_KEY_FILE",
		},
		{
			name: "trailing data after PEM block fails fast",
			envVars: map[string]string{
				"BACKEND_TOKEN_VERIFY_KEY_FILE": trailingKeyFile,
			},
			wantErr: true,
			errMsg:  "BACKEND_TOKEN_VERIFY_KEY_FILE",
		},
		{
			name: "non-Ed25519 SPKI public key fails fast",
			envVars: map[string]string{
				"BACKEND_TOKEN_VERIFY_KEY_FILE": ecdsaKeyFile,
			},
			wantErr: true,
			errMsg:  "Ed25519",
		},
		{
			name: "Ed25519 PKCS8 private key fails fast",
			envVars: map[string]string{
				"BACKEND_TOKEN_VERIFY_KEY_FILE": privateKeyFile,
			},
			wantErr: true,
			errMsg:  "PUBLIC KEY",
		},
		{
			name: "valid SPKI Ed25519 public key starts",
			envVars: map[string]string{
				"BACKEND_TOKEN_VERIFY_KEY_FILE": goodKeyFile,
			},
			wantKey: validPub,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			envVars := map[string]string{
				"IMAGE_PROXY_ENABLED":           "false",
				"SOVEREIGN_URL":                 "disabled",
				"SOVEREIGN_EVENT_AUTH":          "disabled",
				"RAG_API_AUTH":                  "disabled",
				"VAPID_PUBLIC_KEY":              "a-vapid-public-key-value",
				"BACKEND_TOKEN_VERIFY_KEY":      "",
				"BACKEND_TOKEN_VERIFY_KEY_FILE": "",
			}
			for key, value := range tt.envVars {
				envVars[key] = value
			}
			applyEnv(t, envVars)

			cfg, err := NewConfig()
			if err != nil {
				t.Fatalf("NewConfig() unexpected error: %v", err)
			}

			err = ValidateBackendConfig(cfg)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ValidateBackendConfig() expected error but got none")
				}
				if tt.errMsg != "" && !contains(err.Error(), tt.errMsg) {
					t.Errorf("ValidateBackendConfig() error = %v, want to contain %s", err, tt.errMsg)
				}
				return
			}

			if err != nil {
				t.Fatalf("ValidateBackendConfig() unexpected error: %v", err)
			}
			if !cfg.Auth.BackendTokenPublicKey.Equal(tt.wantKey) {
				t.Errorf("Auth.BackendTokenPublicKey = %x, want %x", cfg.Auth.BackendTokenPublicKey, tt.wantKey)
			}
		})
	}
}
