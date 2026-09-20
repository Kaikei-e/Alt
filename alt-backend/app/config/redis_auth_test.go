package config

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveRedisPassword(t *testing.T) {
	t.Run("neither REDIS_PASSWORD_FILE nor REDIS_AUTH returns startup error", func(t *testing.T) {
		t.Setenv("REDIS_PASSWORD_FILE", "")
		if err := os.Unsetenv("REDIS_PASSWORD_FILE"); err != nil {
			t.Fatal(err)
		}
		t.Setenv("REDIS_AUTH", "")
		if err := os.Unsetenv("REDIS_AUTH"); err != nil {
			t.Fatal(err)
		}

		pwd, err := ResolveRedisPassword(nil)
		require.Error(t, err)
		assert.Empty(t, pwd)
		assert.Contains(t, err.Error(), "REDIS_PASSWORD_FILE")
		assert.Contains(t, err.Error(), "REDIS_AUTH")
	})

	t.Run("explicit REDIS_AUTH=disabled logs redis_auth_disabled and returns empty password", func(t *testing.T) {
		t.Setenv("REDIS_PASSWORD_FILE", "")
		if err := os.Unsetenv("REDIS_PASSWORD_FILE"); err != nil {
			t.Fatal(err)
		}
		t.Setenv("REDIS_AUTH", "disabled")

		var buf bytes.Buffer
		testLogger := slog.New(slog.NewTextHandler(&buf, nil))

		pwd, err := ResolveRedisPassword(testLogger)
		require.NoError(t, err)
		assert.Empty(t, pwd)
		assert.Contains(t, buf.String(), "redis_auth_disabled")
	})

	t.Run("REDIS_AUTH=disabled is case-insensitive", func(t *testing.T) {
		t.Setenv("REDIS_PASSWORD_FILE", "")
		if err := os.Unsetenv("REDIS_PASSWORD_FILE"); err != nil {
			t.Fatal(err)
		}
		t.Setenv("REDIS_AUTH", "Disabled")

		pwd, err := ResolveRedisPassword(nil)
		require.NoError(t, err)
		assert.Empty(t, pwd)
	})

	t.Run("file present returns trimmed password", func(t *testing.T) {
		tmpDir := t.TempDir()
		pwFile := filepath.Join(tmpDir, "redis_password.txt")
		err := os.WriteFile(pwFile, []byte("  secret-redis-password \n"), 0600)
		require.NoError(t, err)

		t.Setenv("REDIS_PASSWORD_FILE", pwFile)

		pwd, err := ResolveRedisPassword(nil)
		require.NoError(t, err)
		assert.Equal(t, "secret-redis-password", pwd)
	})

	t.Run("file missing returns startup error", func(t *testing.T) {
		tmpDir := t.TempDir()
		missingFile := filepath.Join(tmpDir, "non_existent_password.txt")

		t.Setenv("REDIS_PASSWORD_FILE", missingFile)

		pwd, err := ResolveRedisPassword(nil)
		require.Error(t, err)
		assert.Empty(t, pwd)
		assert.Contains(t, err.Error(), "REDIS_PASSWORD_FILE")
	})

	t.Run("file empty returns startup error", func(t *testing.T) {
		tmpDir := t.TempDir()
		emptyFile := filepath.Join(tmpDir, "empty_password.txt")
		err := os.WriteFile(emptyFile, []byte("   \n\t "), 0600)
		require.NoError(t, err)

		t.Setenv("REDIS_PASSWORD_FILE", emptyFile)

		pwd, err := ResolveRedisPassword(nil)
		require.Error(t, err)
		assert.Empty(t, pwd)
		assert.Contains(t, err.Error(), "empty password")
	})

	t.Run("variable set to empty string returns startup error", func(t *testing.T) {
		t.Setenv("REDIS_PASSWORD_FILE", "")

		pwd, err := ResolveRedisPassword(nil)
		require.Error(t, err)
		assert.Empty(t, pwd)
		assert.Contains(t, err.Error(), "REDIS_PASSWORD_FILE is set but empty")
	})
}

// cmd/datahub and cmd/notifier share NewConfig with cmd/backend and
// cmd/harvester but have no HOST_RATE_LIMITER_REDIS_URL capability at all; a
// binary running the explicit local mode has nothing to authenticate
// against either. Both must not be tripped up by a REDIS_PASSWORD_FILE that
// happens to be set for an unrelated reason (e.g. a shared env_file).
func TestResolveCoordinationRedisPassword(t *testing.T) {
	t.Run("coordination unset never touches REDIS_PASSWORD_FILE", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("REDIS_PASSWORD_FILE", filepath.Join(tmpDir, "non_existent_password.txt"))

		pwd, err := resolveCoordinationRedisPassword("")
		require.NoError(t, err, "an unset coordination URL must short-circuit before reading REDIS_PASSWORD_FILE at all")
		assert.Empty(t, pwd)
	})

	t.Run("coordination set resolves the password", func(t *testing.T) {
		tmpDir := t.TempDir()
		pwFile := filepath.Join(tmpDir, "redis_password.txt")
		require.NoError(t, os.WriteFile(pwFile, []byte("coordination-secret"), 0600))
		t.Setenv("REDIS_PASSWORD_FILE", pwFile)

		pwd, err := resolveCoordinationRedisPassword("redis://redis-streams:6379/3")
		require.NoError(t, err)
		assert.Equal(t, "coordination-secret", pwd)
	})

	t.Run("coordination set with REDIS_AUTH=disabled succeeds", func(t *testing.T) {
		t.Setenv("REDIS_PASSWORD_FILE", "")
		if err := os.Unsetenv("REDIS_PASSWORD_FILE"); err != nil {
			t.Fatal(err)
		}
		t.Setenv("REDIS_AUTH", "disabled")

		pwd, err := resolveCoordinationRedisPassword("redis://redis-streams:6379/3")
		require.NoError(t, err)
		assert.Empty(t, pwd)
	})

	t.Run("coordination set with neither REDIS_PASSWORD_FILE nor REDIS_AUTH fails fast", func(t *testing.T) {
		t.Setenv("REDIS_PASSWORD_FILE", "")
		if err := os.Unsetenv("REDIS_PASSWORD_FILE"); err != nil {
			t.Fatal(err)
		}
		t.Setenv("REDIS_AUTH", "")
		if err := os.Unsetenv("REDIS_AUTH"); err != nil {
			t.Fatal(err)
		}

		pwd, err := resolveCoordinationRedisPassword("redis://redis-streams:6379/3")
		require.Error(t, err)
		assert.Empty(t, pwd)
		assert.Contains(t, err.Error(), "REDIS_PASSWORD_FILE")
		assert.Contains(t, err.Error(), "REDIS_AUTH")
	})

	t.Run("coordination set but REDIS_PASSWORD_FILE unreadable fails fast", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("REDIS_PASSWORD_FILE", filepath.Join(tmpDir, "non_existent_password.txt"))
		t.Setenv("REDIS_AUTH", "")
		if err := os.Unsetenv("REDIS_AUTH"); err != nil {
			t.Fatal(err)
		}

		pwd, err := resolveCoordinationRedisPassword("redis://redis-streams:6379/3")
		require.Error(t, err)
		assert.Empty(t, pwd)
	})
}
