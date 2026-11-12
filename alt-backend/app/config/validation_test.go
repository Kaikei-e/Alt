package config

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"
)

func TestValidateAuthConfig(t *testing.T) {
	validPub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate ed25519 key: %v", err)
	}

	tests := []struct {
		name    string
		config  AuthConfig
		env     string
		wantErr bool
	}{
		{
			name:    "shared config loader allows unset verify key in production for non-verifying binaries",
			config:  AuthConfig{},
			env:     "production",
			wantErr: false,
		},
		{
			name:    "shared config loader allows unset verify key in development",
			config:  AuthConfig{},
			env:     "development",
			wantErr: false,
		},
		{
			name: "valid Ed25519 public key passes",
			config: AuthConfig{
				BackendTokenPublicKey: validPub,
			},
			env:     "production",
			wantErr: false,
		},
		{
			name: "malformed public key length fails",
			config: AuthConfig{
				BackendTokenPublicKey: ed25519.PublicKey("short"),
			},
			env:     "development",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAuthConfig(&tt.config, tt.env)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateAuthConfig() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}
