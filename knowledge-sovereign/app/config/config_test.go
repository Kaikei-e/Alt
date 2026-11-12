package config

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_RequiresDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	_, err := Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "DATABASE_URL")
}

func TestLoad_DefaultAddresses(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test:test@localhost:5432/test")
	t.Setenv("LISTEN_ADDR", "")
	t.Setenv("METRICS_ADDR", "")
	t.Setenv("SERVICE_SECRET", "")
	t.Setenv("SERVICE_SECRET_FILE", "")
	t.Setenv("SNAPSHOT_DIR", "")
	t.Setenv("ARCHIVE_DIR", "")
	t.Setenv("BUILD_REF", "")
	t.Setenv("SCHEMA_VERSION", "")
	t.Setenv("ADMIN_TOKEN", "")
	t.Setenv("ADMIN_TOKEN_FILE", "")
	t.Setenv("ADMIN_AUTH", "disabled")
	t.Setenv("EVENT_TOKEN", "")
	t.Setenv("EVENT_TOKEN_FILE", "")
	t.Setenv("EVENT_AUTH", "disabled")

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, ":9500", cfg.ListenAddr)
	assert.Equal(t, ":9501", cfg.MetricsAddr)
	assert.Equal(t, "/data/snapshots", cfg.SnapshotDir)
	assert.Equal(t, "/tmp/archives", cfg.ArchiveDir)
	assert.Equal(t, "dev", cfg.BuildRef)
	assert.Equal(t, "00032", cfg.SchemaVersion)
	assert.Equal(t, 5*time.Second, cfg.ProjectorTickInterval)
}

// The admin surface writes snapshots to a host bind mount and exports whole
// partitions. An unset ADMIN_TOKEN used to disable that gate silently, which
// is exactly the "config omission is indistinguishable from intentional
// disable" shape CLAUDE.md Rule 9 forbids.
func TestLoad_RequiresAdminTokenUnlessExplicitlyDisabled(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test:test@localhost:5432/test")
	t.Setenv("ADMIN_TOKEN", "")
	t.Setenv("ADMIN_TOKEN_FILE", "")
	t.Setenv("ADMIN_AUTH", "")

	_, err := Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ADMIN_TOKEN")
}

func TestLoad_AdminTokenFromEnv(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test:test@localhost:5432/test")
	t.Setenv("ADMIN_TOKEN", "s3cret-admin-token-value-long")
	t.Setenv("ADMIN_TOKEN_FILE", "")
	t.Setenv("ADMIN_AUTH", "")
	t.Setenv("EVENT_AUTH", "disabled")

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "s3cret-admin-token-value-long", cfg.AdminToken)
	assert.True(t, cfg.AdminAuthEnabled)
}

func TestLoad_AdminTokenFromFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "admin_token")
	require.NoError(t, os.WriteFile(path, []byte("  file-token-that-is-long-enough\n"), 0o600))

	t.Setenv("DATABASE_URL", "postgres://test:test@localhost:5432/test")
	t.Setenv("ADMIN_TOKEN", "")
	t.Setenv("ADMIN_TOKEN_FILE", path)
	t.Setenv("ADMIN_AUTH", "")
	t.Setenv("EVENT_AUTH", "disabled")

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "file-token-that-is-long-enough", cfg.AdminToken)
	assert.True(t, cfg.AdminAuthEnabled)
}

// Disabling admin auth must be an explicit choice, never inferred.
func TestLoad_AdminAuthExplicitlyDisabled(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test:test@localhost:5432/test")
	t.Setenv("ADMIN_TOKEN", "")
	t.Setenv("ADMIN_TOKEN_FILE", "")
	t.Setenv("ADMIN_AUTH", "disabled")
	t.Setenv("EVENT_AUTH", "disabled")

	cfg, err := Load()
	require.NoError(t, err)
	assert.False(t, cfg.AdminAuthEnabled)
	assert.Empty(t, cfg.AdminToken)
}

func TestLoad_RejectsShortAdminToken(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test:test@localhost:5432/test")
	t.Setenv("ADMIN_TOKEN", "short")
	t.Setenv("ADMIN_TOKEN_FILE", "")
	t.Setenv("ADMIN_AUTH", "")
	t.Setenv("EVENT_AUTH", "disabled")

	_, err := Load()
	require.Error(t, err)
}

func TestLoad_RequiresEventTokenUnlessExplicitlyDisabled(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test:test@localhost:5432/test")
	t.Setenv("ADMIN_AUTH", "disabled")
	t.Setenv("EVENT_TOKEN", "")
	t.Setenv("EVENT_TOKEN_FILE", "")
	t.Setenv("EVENT_AUTH", "")

	_, err := Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "EVENT_TOKEN")
}

func TestLoad_EventTokenFromEnv(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test:test@localhost:5432/test")
	t.Setenv("ADMIN_AUTH", "disabled")
	t.Setenv("EVENT_TOKEN", "s3cret-event-token-value-long")
	t.Setenv("EVENT_TOKEN_FILE", "")
	t.Setenv("EVENT_AUTH", "")

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "s3cret-event-token-value-long", cfg.EventToken)
	assert.True(t, cfg.EventAuthEnabled)
}

func TestLoad_EventTokenFromFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "event_token")
	require.NoError(t, os.WriteFile(path, []byte("  file-event-token-that-is-long-enough\n"), 0o600))

	t.Setenv("DATABASE_URL", "postgres://test:test@localhost:5432/test")
	t.Setenv("ADMIN_AUTH", "disabled")
	t.Setenv("EVENT_TOKEN", "")
	t.Setenv("EVENT_TOKEN_FILE", path)
	t.Setenv("EVENT_AUTH", "")

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "file-event-token-that-is-long-enough", cfg.EventToken)
	assert.True(t, cfg.EventAuthEnabled)
}

func TestLoad_EventAuthExplicitlyDisabled(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test:test@localhost:5432/test")
	t.Setenv("ADMIN_AUTH", "disabled")
	t.Setenv("EVENT_TOKEN", "")
	t.Setenv("EVENT_TOKEN_FILE", "")
	t.Setenv("EVENT_AUTH", "disabled")

	cfg, err := Load()
	require.NoError(t, err)
	assert.False(t, cfg.EventAuthEnabled)
	assert.Empty(t, cfg.EventToken)
}

func TestLoad_RejectsShortEventToken(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test:test@localhost:5432/test")
	t.Setenv("ADMIN_AUTH", "disabled")
	t.Setenv("EVENT_TOKEN", "short")
	t.Setenv("EVENT_TOKEN_FILE", "")
	t.Setenv("EVENT_AUTH", "")

	_, err := Load()
	require.Error(t, err)
}

func TestLoad_EventAuthDisabledTakesPrecedenceOverTokenFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "event_token")
	require.NoError(t, os.WriteFile(path, []byte("file-event-token-that-is-long-enough"), 0o600))

	t.Setenv("DATABASE_URL", "postgres://test:test@localhost:5432/test")
	t.Setenv("ADMIN_AUTH", "disabled")
	t.Setenv("EVENT_TOKEN", "")
	t.Setenv("EVENT_TOKEN_FILE", path)
	t.Setenv("EVENT_AUTH", "disabled")

	cfg, err := Load()
	require.NoError(t, err)
	assert.False(t, cfg.EventAuthEnabled)
	assert.Empty(t, cfg.EventToken)
}

func TestReadSecretFile_RejectsEmptyAndRootPaths(t *testing.T) {
	for _, invalid := range []string{"", ".", "/"} {
		_, err := readSecretFile(invalid)
		require.Error(t, err, "path %q should be rejected", invalid)
	}
}

func TestLoad_RejectsDisabledAdminAuthInProduction(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("DATABASE_URL", "postgres://test:test@localhost:5432/test")
	t.Setenv("ADMIN_AUTH", "disabled")
	t.Setenv("EVENT_TOKEN", "s3cret-event-token-value-long")
	t.Setenv("EVENT_AUTH", "")

	_, err := Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ADMIN_AUTH")
	assert.Contains(t, err.Error(), "production")
}

func TestLoad_RejectsDisabledEventAuthInProduction(t *testing.T) {
	t.Run("without policy file", func(t *testing.T) {
		t.Setenv("APP_ENV", "production")
		t.Setenv("DATABASE_URL", "postgres://test:test@localhost:5432/test")
		t.Setenv("ADMIN_TOKEN", "s3cret-admin-token-value-long")
		t.Setenv("ADMIN_AUTH", "")
		t.Setenv("EVENT_AUTH", "disabled")

		_, err := Load()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "EVENT_AUTH")
		assert.Contains(t, err.Error(), "production")
	})

	t.Run("with policy file", func(t *testing.T) {
		tok := writeTempTokenFile(t, "tok", "service-token-that-is-long-enough-12345")
		policyJSON := `{
			"version": 1,
			"services": [{
				"name": "alt-backend",
				"token_file": "` + tok + `",
				"allowed_methods": ["/services.sovereign.v1.KnowledgeSovereignService/GetTrailFootprints"]
			}]
		}`
		policyFile := writeTempTokenFile(t, "policy.json", policyJSON)

		t.Setenv("APP_ENV", "production")
		t.Setenv("DATABASE_URL", "postgres://test:test@localhost:5432/test")
		t.Setenv("ADMIN_TOKEN", "s3cret-admin-token-value-long")
		t.Setenv("ADMIN_AUTH", "")
		t.Setenv("EVENT_AUTH_POLICY_FILE", policyFile)
		t.Setenv("EVENT_AUTH", "disabled")

		_, err := Load()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "EVENT_AUTH")
		assert.Contains(t, err.Error(), "production")
	})
}

func TestLoad_AcceptsProductionWhenBothAuthModesEnabled(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("DATABASE_URL", "postgres://test:test@localhost:5432/test")
	t.Setenv("ADMIN_TOKEN", "s3cret-admin-token-value-long")
	t.Setenv("ADMIN_AUTH", "")
	t.Setenv("EVENT_TOKEN", "s3cret-event-token-value-long")
	t.Setenv("EVENT_AUTH", "")

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "production", cfg.AppEnv)
	assert.True(t, cfg.AdminAuthEnabled)
	assert.True(t, cfg.EventAuthEnabled)
}

func TestLoad_RejectsUnknownAppEnv(t *testing.T) {
	t.Setenv("APP_ENV", "prod")
	t.Setenv("DATABASE_URL", "postgres://test:test@localhost:5432/test")
	t.Setenv("ADMIN_TOKEN", "s3cret-admin-token-value-long")
	t.Setenv("EVENT_TOKEN", "s3cret-event-token-value-long")

	_, err := Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "APP_ENV")
}

func TestLoad_LogsWarningWhenAuthDisabledOutsideProduction(t *testing.T) {
	for _, env := range []string{"", "development", "test", "staging"} {
		t.Run("APP_ENV="+env, func(t *testing.T) {
			var buf bytes.Buffer
			prev := slog.Default()
			slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})))
			t.Cleanup(func() { slog.SetDefault(prev) })

			t.Setenv("APP_ENV", env)
			t.Setenv("DATABASE_URL", "postgres://test:test@localhost:5432/test")
			t.Setenv("ADMIN_AUTH", "disabled")
			t.Setenv("EVENT_AUTH", "disabled")

			cfg, err := Load()
			require.NoError(t, err)
			assert.False(t, cfg.AdminAuthEnabled)
			assert.False(t, cfg.EventAuthEnabled)

			logged := buf.String()
			assert.Contains(t, logged, "ADMIN_AUTH")
			assert.Contains(t, logged, "EVENT_AUTH")
		})
	}
}
