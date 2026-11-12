package config

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"net/netip"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func generateTestEd25519Key(t *testing.T) ed25519.PrivateKey {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	return priv
}

func writeEd25519SigningKeyFile(t *testing.T) (string, ed25519.PrivateKey) {
	t.Helper()
	priv := generateTestEd25519Key(t)
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	require.NoError(t, err)
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	keyPath := filepath.Join(t.TempDir(), "signing-key.pem")
	require.NoError(t, os.WriteFile(keyPath, pemBytes, 0o600))
	return keyPath, priv
}

func mustTrustedCIDRs(t *testing.T, cidrs ...string) []netip.Prefix {
	t.Helper()
	out := make([]netip.Prefix, 0, len(cidrs))
	for _, s := range cidrs {
		p, err := netip.ParsePrefix(s)
		require.NoError(t, err)
		out = append(out, p.Masked())
	}
	return out
}

func TestLoad(t *testing.T) {
	keyFile, _ := writeEd25519SigningKeyFile(t)
	t.Setenv("TRUSTED_PROXY_CIDRS", "172.18.0.0/16")

	tests := []struct {
		name        string
		setupEnv    func()
		cleanupEnv  func()
		expected    *Config
		wantErr     bool
		errContains string
	}{
		{
			name: "default configuration when no env vars set (missing CSRF_SECRET)",
			setupEnv: func() {
				os.Unsetenv("KRATOS_URL")
				os.Unsetenv("PORT")
				os.Unsetenv("CACHE_TTL")
				os.Unsetenv("CSRF_SECRET")
				os.Unsetenv("BACKEND_TOKEN_SIGNING_KEY_FILE")
				os.Unsetenv("INTERNAL_AUTH_SECRET")
			},
			cleanupEnv:  func() {},
			expected:    nil,
			wantErr:     true,
			errContains: "CSRF_SECRET is required",
		},
		{
			name: "custom configuration from environment variables",
			setupEnv: func() {
				os.Setenv("KRATOS_URL", "http://custom-kratos:4444")
				os.Setenv("PORT", "9999")
				os.Setenv("CACHE_TTL", "30s")
				os.Setenv("CSRF_SECRET", "this-is-a-valid-csrf-secret-that-is-at-least-32-chars")
				os.Setenv("BACKEND_TOKEN_SIGNING_KEY_FILE", keyFile)
				os.Setenv("INTERNAL_AUTH_SECRET", "this-is-a-distinct-internal-auth-secret-32-chars")
			},
			cleanupEnv: func() {
				os.Unsetenv("KRATOS_URL")
				os.Unsetenv("PORT")
				os.Unsetenv("CACHE_TTL")
				os.Unsetenv("CSRF_SECRET")
				os.Unsetenv("BACKEND_TOKEN_SIGNING_KEY_FILE")
				os.Unsetenv("INTERNAL_AUTH_SECRET")
			},
			expected: &Config{
				KratosURL:  "http://custom-kratos:4444",
				Port:       "9999",
				CacheTTL:   30 * time.Second,
				CSRFSecret: "this-is-a-valid-csrf-secret-that-is-at-least-32-chars",
			},
			wantErr: false,
		},
		{
			name: "invalid cache TTL format returns error",
			setupEnv: func() {
				os.Setenv("CACHE_TTL", "invalid")
			},
			cleanupEnv: func() {
				os.Unsetenv("CACHE_TTL")
			},
			expected:    nil,
			wantErr:     true,
			errContains: "invalid CACHE_TTL",
		},
		{
			name: "partial configuration with defaults",
			setupEnv: func() {
				os.Setenv("KRATOS_URL", "http://localhost:4433")
				os.Setenv("CSRF_SECRET", "this-is-a-valid-csrf-secret-that-is-at-least-32-chars")
				os.Setenv("BACKEND_TOKEN_SIGNING_KEY_FILE", keyFile)
				os.Setenv("INTERNAL_AUTH_SECRET", "this-is-a-distinct-internal-auth-secret-32-chars")
				os.Unsetenv("PORT")
				os.Unsetenv("CACHE_TTL")
			},
			cleanupEnv: func() {
				os.Unsetenv("KRATOS_URL")
				os.Unsetenv("CSRF_SECRET")
				os.Unsetenv("BACKEND_TOKEN_SIGNING_KEY_FILE")
				os.Unsetenv("INTERNAL_AUTH_SECRET")
			},
			expected: &Config{
				KratosURL:  "http://localhost:4433",
				Port:       "8888",
				CacheTTL:   60 * time.Second,
				CSRFSecret: "this-is-a-valid-csrf-secret-that-is-at-least-32-chars",
			},
			wantErr: false,
		},
		{
			name: "INTROSPECT_RATE_LIMIT=NaN is rejected",
			setupEnv: func() {
				os.Setenv("CSRF_SECRET", "this-is-a-valid-csrf-secret-that-is-at-least-32-chars")
				os.Setenv("BACKEND_TOKEN_SIGNING_KEY_FILE", keyFile)
				os.Setenv("INTERNAL_AUTH_SECRET", "this-is-a-distinct-internal-auth-secret-32-chars")
				os.Setenv("INTROSPECT_RATE_LIMIT", "NaN")
			},
			cleanupEnv: func() {
				os.Unsetenv("CSRF_SECRET")
				os.Unsetenv("BACKEND_TOKEN_SIGNING_KEY_FILE")
				os.Unsetenv("INTERNAL_AUTH_SECRET")
				os.Unsetenv("INTROSPECT_RATE_LIMIT")
			},
			expected:    nil,
			wantErr:     true,
			errContains: "must be a finite positive number",
		},
		{
			name: "INTROSPECT_RATE_LIMIT=+Inf is rejected",
			setupEnv: func() {
				os.Setenv("CSRF_SECRET", "this-is-a-valid-csrf-secret-that-is-at-least-32-chars")
				os.Setenv("BACKEND_TOKEN_SIGNING_KEY_FILE", keyFile)
				os.Setenv("INTERNAL_AUTH_SECRET", "this-is-a-distinct-internal-auth-secret-32-chars")
				os.Setenv("INTROSPECT_RATE_LIMIT", "+Inf")
			},
			cleanupEnv: func() {
				os.Unsetenv("CSRF_SECRET")
				os.Unsetenv("BACKEND_TOKEN_SIGNING_KEY_FILE")
				os.Unsetenv("INTERNAL_AUTH_SECRET")
				os.Unsetenv("INTROSPECT_RATE_LIMIT")
			},
			expected:    nil,
			wantErr:     true,
			errContains: "must be a finite positive number",
		},
		{
			name: "INTROSPECT_RATE_LIMIT=-Inf is rejected",
			setupEnv: func() {
				os.Setenv("CSRF_SECRET", "this-is-a-valid-csrf-secret-that-is-at-least-32-chars")
				os.Setenv("BACKEND_TOKEN_SIGNING_KEY_FILE", keyFile)
				os.Setenv("INTERNAL_AUTH_SECRET", "this-is-a-distinct-internal-auth-secret-32-chars")
				os.Setenv("INTROSPECT_RATE_LIMIT", "-Inf")
			},
			cleanupEnv: func() {
				os.Unsetenv("CSRF_SECRET")
				os.Unsetenv("BACKEND_TOKEN_SIGNING_KEY_FILE")
				os.Unsetenv("INTERNAL_AUTH_SECRET")
				os.Unsetenv("INTROSPECT_RATE_LIMIT")
			},
			expected:    nil,
			wantErr:     true,
			errContains: "must be a finite positive number",
		},
		{
			name: "INTROSPECT_RATE_LIMIT=20.0 is accepted",
			setupEnv: func() {
				os.Setenv("CSRF_SECRET", "this-is-a-valid-csrf-secret-that-is-at-least-32-chars")
				os.Setenv("BACKEND_TOKEN_SIGNING_KEY_FILE", keyFile)
				os.Setenv("INTERNAL_AUTH_SECRET", "this-is-a-distinct-internal-auth-secret-32-chars")
				os.Setenv("INTROSPECT_RATE_LIMIT", "20.0")
			},
			cleanupEnv: func() {
				os.Unsetenv("CSRF_SECRET")
				os.Unsetenv("BACKEND_TOKEN_SIGNING_KEY_FILE")
				os.Unsetenv("INTERNAL_AUTH_SECRET")
				os.Unsetenv("INTROSPECT_RATE_LIMIT")
			},
			expected: &Config{
				KratosURL:           "http://kratos:4433",
				Port:                "8888",
				CacheTTL:            60 * time.Second,
				CSRFSecret:          "this-is-a-valid-csrf-secret-that-is-at-least-32-chars",
				IntrospectRateLimit: 20.0,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupEnv()
			defer tt.cleanupEnv()

			got, err := Load()

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}

			assert.NoError(t, err)
			assert.NotNil(t, got)
			assert.Equal(t, tt.expected.KratosURL, got.KratosURL)
			assert.Equal(t, tt.expected.Port, got.Port)
			assert.Equal(t, tt.expected.CacheTTL, got.CacheTTL)
		})
	}
}

func TestLoad_BackendTokenSigningKeyFile(t *testing.T) {
	setBaseEnv := func(t *testing.T) {
		t.Helper()
		t.Setenv("CSRF_SECRET", "this-is-a-valid-csrf-secret-that-is-at-least-32-chars")
		t.Setenv("INTERNAL_AUTH_SECRET", "this-is-a-distinct-internal-auth-secret-32-chars")
		t.Setenv("TRUSTED_PROXY_CIDRS", "172.18.0.0/16")
	}

	t.Run("loads valid PKCS8 Ed25519 private key", func(t *testing.T) {
		setBaseEnv(t)
		keyFile, priv := writeEd25519SigningKeyFile(t)
		t.Setenv("BACKEND_TOKEN_SIGNING_KEY_FILE", keyFile)

		cfg, err := Load()
		require.NoError(t, err)
		assert.True(t, priv.Equal(cfg.BackendTokenSigningKey))
	})

	t.Run("fails when BACKEND_TOKEN_SIGNING_KEY_FILE is unset", func(t *testing.T) {
		setBaseEnv(t)
		os.Unsetenv("BACKEND_TOKEN_SIGNING_KEY_FILE")

		_, err := Load()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "BACKEND_TOKEN_SIGNING_KEY_FILE")
	})

	t.Run("ignores legacy BACKEND_TOKEN_SECRET and BACKEND_TOKEN_SECRET_FILE", func(t *testing.T) {
		setBaseEnv(t)
		os.Unsetenv("BACKEND_TOKEN_SIGNING_KEY_FILE")
		legacyFile := filepath.Join(t.TempDir(), "legacy.txt")
		require.NoError(t, os.WriteFile(legacyFile, []byte("this-is-a-valid-backend-token-secret-32-chars-long"), 0o600))
		t.Setenv("BACKEND_TOKEN_SECRET", "this-is-a-valid-backend-token-secret-32-chars-long")
		t.Setenv("BACKEND_TOKEN_SECRET_FILE", legacyFile)

		_, err := Load()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "BACKEND_TOKEN_SIGNING_KEY_FILE")
	})

	t.Run("fails when key file does not exist", func(t *testing.T) {
		setBaseEnv(t)
		t.Setenv("BACKEND_TOKEN_SIGNING_KEY_FILE", filepath.Join(t.TempDir(), "missing.pem"))

		_, err := Load()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "BACKEND_TOKEN_SIGNING_KEY_FILE")
	})

	t.Run("fails when key file is not PEM", func(t *testing.T) {
		setBaseEnv(t)
		badFile := filepath.Join(t.TempDir(), "bad.pem")
		require.NoError(t, os.WriteFile(badFile, []byte("not-a-pem-file"), 0o600))
		t.Setenv("BACKEND_TOKEN_SIGNING_KEY_FILE", badFile)

		_, err := Load()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "BACKEND_TOKEN_SIGNING_KEY_FILE")
	})

	t.Run("fails when key file is an Ed25519 public key instead of private key", func(t *testing.T) {
		setBaseEnv(t)
		pub, _, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)
		spki, err := x509.MarshalPKIXPublicKey(pub)
		require.NoError(t, err)
		pubFile := filepath.Join(t.TempDir(), "pub.pem")
		require.NoError(t, os.WriteFile(pubFile, pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: spki}), 0o600))
		t.Setenv("BACKEND_TOKEN_SIGNING_KEY_FILE", pubFile)

		_, err = Load()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "BACKEND_TOKEN_SIGNING_KEY_FILE")
	})

	t.Run("fails when PKCS8 private key is ECDSA instead of Ed25519", func(t *testing.T) {
		setBaseEnv(t)
		ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)
		der, err := x509.MarshalPKCS8PrivateKey(ecKey)
		require.NoError(t, err)
		ecFile := filepath.Join(t.TempDir(), "ec.pem")
		require.NoError(t, os.WriteFile(ecFile, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), 0o600))
		t.Setenv("BACKEND_TOKEN_SIGNING_KEY_FILE", ecFile)

		_, err = Load()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "BACKEND_TOKEN_SIGNING_KEY_FILE")
	})
}

func TestLoad_TrustedProxyCIDRs(t *testing.T) {
	keyFile, _ := writeEd25519SigningKeyFile(t)
	setBaseEnv := func(t *testing.T) {
		t.Helper()
		t.Setenv("CSRF_SECRET", "this-is-a-valid-csrf-secret-that-is-at-least-32-chars")
		t.Setenv("BACKEND_TOKEN_SIGNING_KEY_FILE", keyFile)
		t.Setenv("INTERNAL_AUTH_SECRET", "this-is-a-distinct-internal-auth-secret-32-chars")
	}

	t.Run("parses comma-separated CIDRs", func(t *testing.T) {
		setBaseEnv(t)
		t.Setenv("TRUSTED_PROXY_CIDRS", "172.18.0.0/16, 10.10.0.5/32, fd00::/8")

		cfg, err := Load()
		require.NoError(t, err)
		assert.False(t, cfg.TrustedProxiesDisabled)
		assert.Equal(t, mustTrustedCIDRs(t, "172.18.0.0/16", "10.10.0.5/32", "fd00::/8"), cfg.TrustedProxyCIDRs)
	})

	t.Run("accepts explicit none value case-insensitively", func(t *testing.T) {
		for _, val := range []string{"none", "NONE", "  None  "} {
			setBaseEnv(t)
			t.Setenv("TRUSTED_PROXY_CIDRS", val)

			cfg, err := Load()
			require.NoError(t, err, "value %q", val)
			assert.True(t, cfg.TrustedProxiesDisabled, "value %q", val)
			assert.Empty(t, cfg.TrustedProxyCIDRs, "value %q", val)
		}
	})

	t.Run("fails when none is combined with CIDRs", func(t *testing.T) {
		for _, val := range []string{"none,10.0.0.0/8", "10.0.0.0/8,none", "NONE, 172.18.0.0/16"} {
			setBaseEnv(t)
			t.Setenv("TRUSTED_PROXY_CIDRS", val)

			_, err := Load()
			require.Error(t, err, "value %q", val)
			assert.Contains(t, err.Error(), "TRUSTED_PROXY_CIDRS")
		}
	})

	t.Run("fails when TRUSTED_PROXY_CIDRS is unset", func(t *testing.T) {
		setBaseEnv(t)
		t.Setenv("TRUSTED_PROXY_CIDRS", "")
		require.NoError(t, os.Unsetenv("TRUSTED_PROXY_CIDRS"))

		_, err := Load()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "TRUSTED_PROXY_CIDRS")
	})

	t.Run("fails when TRUSTED_PROXY_CIDRS is whitespace", func(t *testing.T) {
		setBaseEnv(t)
		t.Setenv("TRUSTED_PROXY_CIDRS", "   ")

		_, err := Load()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "TRUSTED_PROXY_CIDRS")
	})

	t.Run("fails when TRUSTED_PROXY_CIDRS contains unparsable CIDR", func(t *testing.T) {
		setBaseEnv(t)
		t.Setenv("TRUSTED_PROXY_CIDRS", "172.18.0.0/16,not-a-cidr")

		_, err := Load()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "TRUSTED_PROXY_CIDRS")
	})

	t.Run("fails when TRUSTED_PROXY_CIDRS contains bare IP without prefix length", func(t *testing.T) {
		setBaseEnv(t)
		t.Setenv("TRUSTED_PROXY_CIDRS", "172.18.0.5")

		_, err := Load()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "TRUSTED_PROXY_CIDRS")
	})

	t.Run("fails when TRUSTED_PROXY_CIDRS has empty comma segment", func(t *testing.T) {
		setBaseEnv(t)
		t.Setenv("TRUSTED_PROXY_CIDRS", "172.18.0.0/16,")

		_, err := Load()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "TRUSTED_PROXY_CIDRS")
	})

	t.Run("fails when TRUSTED_PROXY_CIDRS contains IPv4-mapped IPv6 prefix", func(t *testing.T) {
		setBaseEnv(t)
		t.Setenv("TRUSTED_PROXY_CIDRS", "::ffff:172.18.0.0/112")

		_, err := Load()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "TRUSTED_PROXY_CIDRS")
		assert.Contains(t, err.Error(), "IPv4")
	})
}

func TestLoad_KratosAdminURL_Default(t *testing.T) {
	keyFile, _ := writeEd25519SigningKeyFile(t)
	t.Setenv("TRUSTED_PROXY_CIDRS", "172.18.0.0/16")
	os.Unsetenv("KRATOS_ADMIN_URL")
	os.Setenv("CSRF_SECRET", "this-is-a-valid-csrf-secret-that-is-at-least-32-chars")
	os.Setenv("BACKEND_TOKEN_SIGNING_KEY_FILE", keyFile)
	os.Setenv("INTERNAL_AUTH_SECRET", "this-is-a-distinct-internal-auth-secret-32-chars")
	defer func() {
		os.Unsetenv("CSRF_SECRET")
		os.Unsetenv("BACKEND_TOKEN_SIGNING_KEY_FILE")
		os.Unsetenv("INTERNAL_AUTH_SECRET")
	}()

	cfg, err := Load()
	assert.NoError(t, err)
	assert.Equal(t, "http://kratos-admin:4434", cfg.KratosAdminURL)
}

func TestLoad_KratosAdminURL_EnvOverride(t *testing.T) {
	keyFile, _ := writeEd25519SigningKeyFile(t)
	t.Setenv("TRUSTED_PROXY_CIDRS", "172.18.0.0/16")
	os.Setenv("KRATOS_ADMIN_URL", "http://custom-kratos-admin:4434")
	os.Setenv("CSRF_SECRET", "this-is-a-valid-csrf-secret-that-is-at-least-32-chars")
	os.Setenv("BACKEND_TOKEN_SIGNING_KEY_FILE", keyFile)
	os.Setenv("INTERNAL_AUTH_SECRET", "this-is-a-distinct-internal-auth-secret-32-chars")
	defer func() {
		os.Unsetenv("KRATOS_ADMIN_URL")
		os.Unsetenv("CSRF_SECRET")
		os.Unsetenv("BACKEND_TOKEN_SIGNING_KEY_FILE")
		os.Unsetenv("INTERNAL_AUTH_SECRET")
	}()

	cfg, err := Load()
	assert.NoError(t, err)
	assert.Equal(t, "http://custom-kratos-admin:4434", cfg.KratosAdminURL)
}

func TestLoad_ValidateRateLimit_Default(t *testing.T) {
	keyFile, _ := writeEd25519SigningKeyFile(t)
	t.Setenv("TRUSTED_PROXY_CIDRS", "172.18.0.0/16")
	os.Setenv("CSRF_SECRET", "this-is-a-valid-csrf-secret-that-is-at-least-32-chars")
	os.Setenv("BACKEND_TOKEN_SIGNING_KEY_FILE", keyFile)
	os.Setenv("INTERNAL_AUTH_SECRET", "this-is-a-distinct-internal-auth-secret-32-chars")
	defer func() {
		os.Unsetenv("CSRF_SECRET")
		os.Unsetenv("BACKEND_TOKEN_SIGNING_KEY_FILE")
		os.Unsetenv("INTERNAL_AUTH_SECRET")
		os.Unsetenv("VALIDATE_RATE_LIMIT")
	}()

	cfg, err := Load()
	assert.NoError(t, err)
	assert.InDelta(t, 100.0/60.0, cfg.ValidateRateLimit, 0.001)
}

func TestLoad_ValidateRateLimit_EnvOverride(t *testing.T) {
	keyFile, _ := writeEd25519SigningKeyFile(t)
	t.Setenv("TRUSTED_PROXY_CIDRS", "172.18.0.0/16")
	os.Setenv("CSRF_SECRET", "this-is-a-valid-csrf-secret-that-is-at-least-32-chars")
	os.Setenv("BACKEND_TOKEN_SIGNING_KEY_FILE", keyFile)
	os.Setenv("INTERNAL_AUTH_SECRET", "this-is-a-distinct-internal-auth-secret-32-chars")
	os.Setenv("VALIDATE_RATE_LIMIT", "50.0")
	defer func() {
		os.Unsetenv("CSRF_SECRET")
		os.Unsetenv("BACKEND_TOKEN_SIGNING_KEY_FILE")
		os.Unsetenv("INTERNAL_AUTH_SECRET")
		os.Unsetenv("VALIDATE_RATE_LIMIT")
	}()

	cfg, err := Load()
	assert.NoError(t, err)
	assert.InDelta(t, 50.0, cfg.ValidateRateLimit, 0.001)
}

func TestLoad_ValidateRateLimit_Invalid(t *testing.T) {
	keyFile, _ := writeEd25519SigningKeyFile(t)
	t.Setenv("TRUSTED_PROXY_CIDRS", "172.18.0.0/16")
	os.Setenv("CSRF_SECRET", "this-is-a-valid-csrf-secret-that-is-at-least-32-chars")
	os.Setenv("BACKEND_TOKEN_SIGNING_KEY_FILE", keyFile)
	os.Setenv("INTERNAL_AUTH_SECRET", "this-is-a-distinct-internal-auth-secret-32-chars")
	os.Setenv("VALIDATE_RATE_LIMIT", "not-a-number")
	defer func() {
		os.Unsetenv("CSRF_SECRET")
		os.Unsetenv("BACKEND_TOKEN_SIGNING_KEY_FILE")
		os.Unsetenv("INTERNAL_AUTH_SECRET")
		os.Unsetenv("VALIDATE_RATE_LIMIT")
	}()

	_, err := Load()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid VALIDATE_RATE_LIMIT")
}

func TestLoad_CSRFRateLimit_Default(t *testing.T) {
	keyFile, _ := writeEd25519SigningKeyFile(t)
	t.Setenv("TRUSTED_PROXY_CIDRS", "172.18.0.0/16")
	os.Setenv("CSRF_SECRET", "this-is-a-valid-csrf-secret-that-is-at-least-32-chars")
	os.Setenv("BACKEND_TOKEN_SIGNING_KEY_FILE", keyFile)
	os.Setenv("INTERNAL_AUTH_SECRET", "this-is-a-distinct-internal-auth-secret-32-chars")
	defer func() {
		os.Unsetenv("CSRF_SECRET")
		os.Unsetenv("BACKEND_TOKEN_SIGNING_KEY_FILE")
		os.Unsetenv("INTERNAL_AUTH_SECRET")
		os.Unsetenv("CSRF_RATE_LIMIT")
	}()

	cfg, err := Load()
	assert.NoError(t, err)
	assert.InDelta(t, 100.0, cfg.CSRFRateLimit, 0.001)
}

func TestLoad_CSRFRateLimit_EnvOverride(t *testing.T) {
	keyFile, _ := writeEd25519SigningKeyFile(t)
	t.Setenv("TRUSTED_PROXY_CIDRS", "172.18.0.0/16")
	os.Setenv("CSRF_SECRET", "this-is-a-valid-csrf-secret-that-is-at-least-32-chars")
	os.Setenv("BACKEND_TOKEN_SIGNING_KEY_FILE", keyFile)
	os.Setenv("INTERNAL_AUTH_SECRET", "this-is-a-distinct-internal-auth-secret-32-chars")
	os.Setenv("CSRF_RATE_LIMIT", "50.0")
	defer func() {
		os.Unsetenv("CSRF_SECRET")
		os.Unsetenv("BACKEND_TOKEN_SIGNING_KEY_FILE")
		os.Unsetenv("INTERNAL_AUTH_SECRET")
		os.Unsetenv("CSRF_RATE_LIMIT")
	}()

	cfg, err := Load()
	assert.NoError(t, err)
	assert.InDelta(t, 50.0, cfg.CSRFRateLimit, 0.001)
}

func TestLoad_CSRFRateLimit_Invalid(t *testing.T) {
	keyFile, _ := writeEd25519SigningKeyFile(t)
	t.Setenv("TRUSTED_PROXY_CIDRS", "172.18.0.0/16")
	os.Setenv("CSRF_SECRET", "this-is-a-valid-csrf-secret-that-is-at-least-32-chars")
	os.Setenv("BACKEND_TOKEN_SIGNING_KEY_FILE", keyFile)
	os.Setenv("INTERNAL_AUTH_SECRET", "this-is-a-distinct-internal-auth-secret-32-chars")
	os.Setenv("CSRF_RATE_LIMIT", "not-a-number")
	defer func() {
		os.Unsetenv("CSRF_SECRET")
		os.Unsetenv("BACKEND_TOKEN_SIGNING_KEY_FILE")
		os.Unsetenv("INTERNAL_AUTH_SECRET")
		os.Unsetenv("CSRF_RATE_LIMIT")
	}()

	_, err := Load()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid CSRF_RATE_LIMIT")
}

func TestConfig_Validate(t *testing.T) {
	validKey := generateTestEd25519Key(t)
	trustedCIDRs := mustTrustedCIDRs(t, "172.18.0.0/16")

	tests := []struct {
		name        string
		config      *Config
		wantErr     bool
		errContains string
	}{
		{
			name: "valid configuration",
			config: &Config{
				KratosURL:              "http://kratos:4433",
				Port:                   "8888",
				CacheTTL:               60 * time.Second,
				BackendTokenTTL:        5 * time.Minute,
				CSRFSecret:             "this-is-a-valid-csrf-secret-that-is-at-least-32-chars",
				BackendTokenSigningKey: validKey,
				InternalAuthSecret:     "this-is-a-distinct-internal-auth-secret-32-chars",
				TrustedProxyCIDRs:      trustedCIDRs,
			},
			wantErr: false,
		},
		{
			name: "missing Kratos URL",
			config: &Config{
				KratosURL:              "",
				Port:                   "8888",
				CacheTTL:               60 * time.Second,
				BackendTokenTTL:        5 * time.Minute,
				CSRFSecret:             "this-is-a-valid-csrf-secret-that-is-at-least-32-chars",
				BackendTokenSigningKey: validKey,
				InternalAuthSecret:     "this-is-a-distinct-internal-auth-secret-32-chars",
				TrustedProxyCIDRs:      trustedCIDRs,
			},
			wantErr:     true,
			errContains: "KRATOS_URL",
		},
		{
			name: "missing port",
			config: &Config{
				KratosURL:              "http://kratos:4433",
				Port:                   "",
				CacheTTL:               60 * time.Second,
				BackendTokenTTL:        5 * time.Minute,
				CSRFSecret:             "this-is-a-valid-csrf-secret-that-is-at-least-32-chars",
				BackendTokenSigningKey: validKey,
				InternalAuthSecret:     "this-is-a-distinct-internal-auth-secret-32-chars",
				TrustedProxyCIDRs:      trustedCIDRs,
			},
			wantErr:     true,
			errContains: "PORT",
		},
		{
			name: "invalid cache TTL (zero)",
			config: &Config{
				KratosURL:              "http://kratos:4433",
				Port:                   "8888",
				CacheTTL:               0,
				BackendTokenTTL:        5 * time.Minute,
				CSRFSecret:             "this-is-a-valid-csrf-secret-that-is-at-least-32-chars",
				BackendTokenSigningKey: validKey,
				InternalAuthSecret:     "this-is-a-distinct-internal-auth-secret-32-chars",
				TrustedProxyCIDRs:      trustedCIDRs,
			},
			wantErr:     true,
			errContains: "CACHE_TTL",
		},
		{
			name: "invalid cache TTL (negative)",
			config: &Config{
				KratosURL:              "http://kratos:4433",
				Port:                   "8888",
				CacheTTL:               -1 * time.Minute,
				BackendTokenTTL:        5 * time.Minute,
				CSRFSecret:             "this-is-a-valid-csrf-secret-that-is-at-least-32-chars",
				BackendTokenSigningKey: validKey,
				InternalAuthSecret:     "this-is-a-distinct-internal-auth-secret-32-chars",
				TrustedProxyCIDRs:      trustedCIDRs,
			},
			wantErr:     true,
			errContains: "CACHE_TTL",
		},
		{
			name: "missing CSRF secret",
			config: &Config{
				KratosURL:              "http://kratos:4433",
				Port:                   "8888",
				CacheTTL:               60 * time.Second,
				BackendTokenTTL:        5 * time.Minute,
				CSRFSecret:             "",
				BackendTokenSigningKey: validKey,
				InternalAuthSecret:     "this-is-a-distinct-internal-auth-secret-32-chars",
				TrustedProxyCIDRs:      trustedCIDRs,
			},
			wantErr:     true,
			errContains: "CSRF_SECRET",
		},
		{
			name: "CSRF secret too short",
			config: &Config{
				KratosURL:              "http://kratos:4433",
				Port:                   "8888",
				CacheTTL:               60 * time.Second,
				BackendTokenTTL:        5 * time.Minute,
				CSRFSecret:             "short-secret",
				BackendTokenSigningKey: validKey,
				InternalAuthSecret:     "this-is-a-distinct-internal-auth-secret-32-chars",
				TrustedProxyCIDRs:      trustedCIDRs,
			},
			wantErr:     true,
			errContains: "CSRF_SECRET must be at least 32 characters",
		},
		{
			name: "valid CSRF secret",
			config: &Config{
				KratosURL:              "http://kratos:4433",
				Port:                   "8888",
				CacheTTL:               60 * time.Second,
				BackendTokenTTL:        5 * time.Minute,
				CSRFSecret:             "this-is-a-valid-csrf-secret-that-is-at-least-32-chars",
				BackendTokenSigningKey: validKey,
				InternalAuthSecret:     "this-is-a-distinct-internal-auth-secret-32-chars",
				TrustedProxyCIDRs:      trustedCIDRs,
			},
			wantErr: false,
		},
		{
			name: "missing backend token signing key",
			config: &Config{
				KratosURL:              "http://kratos:4433",
				Port:                   "8888",
				CacheTTL:               60 * time.Second,
				BackendTokenTTL:        5 * time.Minute,
				CSRFSecret:             "this-is-a-valid-csrf-secret-that-is-at-least-32-chars",
				BackendTokenSigningKey: nil,
				InternalAuthSecret:     "this-is-a-distinct-internal-auth-secret-32-chars",
				TrustedProxyCIDRs:      trustedCIDRs,
			},
			wantErr:     true,
			errContains: "BACKEND_TOKEN_SIGNING_KEY_FILE",
		},
		{
			name: "invalid backend token signing key length",
			config: &Config{
				KratosURL:              "http://kratos:4433",
				Port:                   "8888",
				CacheTTL:               60 * time.Second,
				BackendTokenTTL:        5 * time.Minute,
				CSRFSecret:             "this-is-a-valid-csrf-secret-that-is-at-least-32-chars",
				BackendTokenSigningKey: ed25519.PrivateKey("short"),
				InternalAuthSecret:     "this-is-a-distinct-internal-auth-secret-32-chars",
				TrustedProxyCIDRs:      trustedCIDRs,
			},
			wantErr:     true,
			errContains: "BACKEND_TOKEN_SIGNING_KEY_FILE",
		},
		{
			name: "missing trusted proxy CIDRs",
			config: &Config{
				KratosURL:              "http://kratos:4433",
				Port:                   "8888",
				CacheTTL:               60 * time.Second,
				BackendTokenTTL:        5 * time.Minute,
				CSRFSecret:             "this-is-a-valid-csrf-secret-that-is-at-least-32-chars",
				BackendTokenSigningKey: validKey,
				InternalAuthSecret:     "this-is-a-distinct-internal-auth-secret-32-chars",
				TrustedProxyCIDRs:      nil,
			},
			wantErr:     true,
			errContains: "TRUSTED_PROXY_CIDRS",
		},
		{
			name: "valid configuration with trusted proxies explicitly disabled",
			config: &Config{
				KratosURL:              "http://kratos:4433",
				Port:                   "8888",
				CacheTTL:               60 * time.Second,
				BackendTokenTTL:        5 * time.Minute,
				CSRFSecret:             "this-is-a-valid-csrf-secret-that-is-at-least-32-chars",
				BackendTokenSigningKey: validKey,
				InternalAuthSecret:     "this-is-a-distinct-internal-auth-secret-32-chars",
				TrustedProxiesDisabled: true,
			},
			wantErr: false,
		},
		{
			name: "invalid configuration combining disabled trusted proxies with CIDRs",
			config: &Config{
				KratosURL:              "http://kratos:4433",
				Port:                   "8888",
				CacheTTL:               60 * time.Second,
				BackendTokenTTL:        5 * time.Minute,
				CSRFSecret:             "this-is-a-valid-csrf-secret-that-is-at-least-32-chars",
				BackendTokenSigningKey: validKey,
				InternalAuthSecret:     "this-is-a-distinct-internal-auth-secret-32-chars",
				TrustedProxyCIDRs:      trustedCIDRs,
				TrustedProxiesDisabled: true,
			},
			wantErr:     true,
			errContains: "TRUSTED_PROXY_CIDRS",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestConfig_BoundedSessionCacheAndJWT(t *testing.T) {
	keyFile, validKey := writeEd25519SigningKeyFile(t)
	trustedCIDRs := mustTrustedCIDRs(t, "172.18.0.0/16")
	t.Setenv("TRUSTED_PROXY_CIDRS", "172.18.0.0/16")

	t.Run("Load caps CACHE_TTL to 60s", func(t *testing.T) {
		os.Setenv("CSRF_SECRET", "this-is-a-valid-csrf-secret-that-is-at-least-32-chars")
		os.Setenv("BACKEND_TOKEN_SIGNING_KEY_FILE", keyFile)
		os.Setenv("INTERNAL_AUTH_SECRET", "this-is-a-distinct-internal-auth-secret-32-chars")
		os.Setenv("CACHE_TTL", "10m")
		defer func() {
			os.Unsetenv("CSRF_SECRET")
			os.Unsetenv("BACKEND_TOKEN_SIGNING_KEY_FILE")
			os.Unsetenv("INTERNAL_AUTH_SECRET")
			os.Unsetenv("CACHE_TTL")
		}()

		cfg, err := Load()
		assert.NoError(t, err)
		assert.Equal(t, 60*time.Second, cfg.CacheTTL, "CACHE_TTL must be capped to <= 60s")
	})

	t.Run("default CacheTTL is 60s", func(t *testing.T) {
		os.Setenv("CSRF_SECRET", "this-is-a-valid-csrf-secret-that-is-at-least-32-chars")
		os.Setenv("BACKEND_TOKEN_SIGNING_KEY_FILE", keyFile)
		os.Setenv("INTERNAL_AUTH_SECRET", "this-is-a-distinct-internal-auth-secret-32-chars")
		os.Unsetenv("CACHE_TTL")
		defer func() {
			os.Unsetenv("CSRF_SECRET")
			os.Unsetenv("BACKEND_TOKEN_SIGNING_KEY_FILE")
			os.Unsetenv("INTERNAL_AUTH_SECRET")
		}()

		cfg, err := Load()
		assert.NoError(t, err)
		assert.Equal(t, 60*time.Second, cfg.CacheTTL, "Default CacheTTL must be 60s")
	})

	t.Run("Load caps BACKEND_TOKEN_TTL to 5m", func(t *testing.T) {
		os.Setenv("CSRF_SECRET", "this-is-a-valid-csrf-secret-that-is-at-least-32-chars")
		os.Setenv("BACKEND_TOKEN_SIGNING_KEY_FILE", keyFile)
		os.Setenv("INTERNAL_AUTH_SECRET", "this-is-a-distinct-internal-auth-secret-32-chars")
		os.Setenv("BACKEND_TOKEN_TTL", "30m")
		defer func() {
			os.Unsetenv("CSRF_SECRET")
			os.Unsetenv("BACKEND_TOKEN_SIGNING_KEY_FILE")
			os.Unsetenv("INTERNAL_AUTH_SECRET")
			os.Unsetenv("BACKEND_TOKEN_TTL")
		}()

		cfg, err := Load()
		assert.NoError(t, err)
		assert.Equal(t, 5*time.Minute, cfg.BackendTokenTTL, "BACKEND_TOKEN_TTL must be capped to <= 5m")
	})

	t.Run("default BackendTokenTTL is 5m", func(t *testing.T) {
		os.Setenv("CSRF_SECRET", "this-is-a-valid-csrf-secret-that-is-at-least-32-chars")
		os.Setenv("BACKEND_TOKEN_SIGNING_KEY_FILE", keyFile)
		os.Setenv("INTERNAL_AUTH_SECRET", "this-is-a-distinct-internal-auth-secret-32-chars")
		os.Unsetenv("BACKEND_TOKEN_TTL")
		os.Unsetenv("JWT_ISSUER_TTL")
		defer func() {
			os.Unsetenv("CSRF_SECRET")
			os.Unsetenv("BACKEND_TOKEN_SIGNING_KEY_FILE")
			os.Unsetenv("INTERNAL_AUTH_SECRET")
		}()

		cfg, err := Load()
		assert.NoError(t, err)
		assert.Equal(t, 5*time.Minute, cfg.BackendTokenTTL, "Default BackendTokenTTL must be 5m")
	})

	t.Run("Load clamps CacheTTL when JWT_ISSUER_TTL is smaller than CACHE_TTL", func(t *testing.T) {
		os.Setenv("CSRF_SECRET", "this-is-a-valid-csrf-secret-that-is-at-least-32-chars")
		os.Setenv("BACKEND_TOKEN_SIGNING_KEY_FILE", keyFile)
		os.Setenv("INTERNAL_AUTH_SECRET", "this-is-a-distinct-internal-auth-secret-32-chars")
		os.Setenv("CACHE_TTL", "60s")
		os.Unsetenv("BACKEND_TOKEN_TTL")
		os.Setenv("JWT_ISSUER_TTL", "15s")
		defer func() {
			os.Unsetenv("CSRF_SECRET")
			os.Unsetenv("BACKEND_TOKEN_SIGNING_KEY_FILE")
			os.Unsetenv("INTERNAL_AUTH_SECRET")
			os.Unsetenv("CACHE_TTL")
			os.Unsetenv("JWT_ISSUER_TTL")
		}()

		cfg, err := Load()
		assert.NoError(t, err)
		assert.Equal(t, 15*time.Second, cfg.BackendTokenTTL)
		assert.Equal(t, 15*time.Second, cfg.CacheTTL, "CacheTTL must be clamped to jwt_issuer_ttl when jwt_issuer_ttl < cache_ttl")
	})

	t.Run("Validate enforces bounds and positive values", func(t *testing.T) {
		baseCfg := func() *Config {
			return &Config{
				KratosURL:              "http://kratos:4433",
				Port:                   "8888",
				CacheTTL:               60 * time.Second,
				BackendTokenTTL:        5 * time.Minute,
				CSRFSecret:             "this-is-a-valid-csrf-secret-that-is-at-least-32-chars",
				BackendTokenSigningKey: validKey,
				InternalAuthSecret:     "this-is-a-distinct-internal-auth-secret-32-chars",
				TrustedProxyCIDRs:      trustedCIDRs,
			}
		}

		assert.NoError(t, baseCfg().Validate())

		c := baseCfg()
		c.CacheTTL = 61 * time.Second
		err := c.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "CACHE_TTL cannot exceed 60s")

		c = baseCfg()
		c.BackendTokenTTL = 0
		err = c.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "BACKEND_TOKEN_TTL must be positive")

		c = baseCfg()
		c.BackendTokenTTL = 6 * time.Minute
		err = c.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "BACKEND_TOKEN_TTL cannot exceed 5m")

		c = baseCfg()
		c.IntrospectRateLimit = -1
		err = c.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "INTROSPECT_RATE_LIMIT cannot be negative")

		c = baseCfg()
		c.IntrospectBurst = -1
		err = c.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "INTROSPECT_BURST cannot be negative")
	})

	t.Run("introspect rate limit env parsing", func(t *testing.T) {
		os.Setenv("CSRF_SECRET", "this-is-a-valid-csrf-secret-that-is-at-least-32-chars")
		os.Setenv("BACKEND_TOKEN_SIGNING_KEY_FILE", keyFile)
		os.Setenv("INTERNAL_AUTH_SECRET", "this-is-a-distinct-internal-auth-secret-32-chars")
		os.Setenv("INTROSPECT_RATE_LIMIT", "25.5")
		os.Setenv("INTROSPECT_BURST", "150")
		defer func() {
			os.Unsetenv("CSRF_SECRET")
			os.Unsetenv("BACKEND_TOKEN_SIGNING_KEY_FILE")
			os.Unsetenv("INTERNAL_AUTH_SECRET")
			os.Unsetenv("INTROSPECT_RATE_LIMIT")
			os.Unsetenv("INTROSPECT_BURST")
		}()

		cfg, err := Load()
		assert.NoError(t, err)
		assert.Equal(t, 25.5, cfg.IntrospectRateLimit)
		assert.Equal(t, 150, cfg.IntrospectBurst)
	})
}
