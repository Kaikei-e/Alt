package config

import (
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
	t.Setenv("ADMIN_TOKEN", "")
	t.Setenv("ADMIN_TOKEN_FILE", "")
	t.Setenv("ADMIN_AUTH", "disabled")

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, ":9500", cfg.ListenAddr)
	assert.Equal(t, ":9501", cfg.MetricsAddr)
	assert.Equal(t, "/data/snapshots", cfg.SnapshotDir)
	assert.Equal(t, "/tmp/archives", cfg.ArchiveDir)
	assert.Equal(t, "dev", cfg.BuildRef)
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

	_, err := Load()
	require.Error(t, err)
}
