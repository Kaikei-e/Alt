package config

import (
	"bytes"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseSyncMode(t *testing.T) {
	t.Run("accepts enabled and disabled", func(t *testing.T) {
		mode, err := ParseSyncMode("enabled")
		require.NoError(t, err)
		assert.Equal(t, SyncEnabled, mode)

		mode, err = ParseSyncMode("disabled")
		require.NoError(t, err)
		assert.Equal(t, SyncDisabled, mode)
	})

	t.Run("rejects empty and any other value", func(t *testing.T) {
		for _, val := range []string{"", " ", "true", "false", "1", "0", "off", "on", "ENABLED", "DISABLED"} {
			mode, err := ParseSyncMode(val)
			require.Error(t, err, "expected error for %q", val)
			assert.Empty(t, mode)
			assert.Contains(t, err.Error(), "INOREADER_SYNC")
		}
	})
}

func TestResolveArticleSyncConfig(t *testing.T) {
	t.Run("fails when INOREADER_SYNC is unset", func(t *testing.T) {
		unsetEnvForTest(t, "INOREADER_SYNC")
		t.Setenv("ARTICLE_SYNC_OWNER_USER_ID", "11111111-2222-4333-8444-555555555555")

		mode, ownerID, err := ResolveArticleSyncConfig(slog.Default())

		require.Error(t, err)
		assert.Empty(t, mode)
		assert.Empty(t, ownerID)
		assert.Contains(t, err.Error(), "INOREADER_SYNC")
	})

	t.Run("fails when INOREADER_SYNC has an unaccepted value", func(t *testing.T) {
		for _, val := range []string{"", "true", "false", "auto"} {
			t.Setenv("INOREADER_SYNC", val)
			t.Setenv("ARTICLE_SYNC_OWNER_USER_ID", "11111111-2222-4333-8444-555555555555")

			mode, ownerID, err := ResolveArticleSyncConfig(slog.Default())

			require.Error(t, err)
			assert.Empty(t, mode)
			assert.Empty(t, ownerID)
			assert.Contains(t, err.Error(), "INOREADER_SYNC")
		}
	})

	t.Run("disabled mode does not require ARTICLE_SYNC_OWNER_USER_ID and logs once", func(t *testing.T) {
		t.Setenv("INOREADER_SYNC", "disabled")
		unsetEnvForTest(t, "ARTICLE_SYNC_OWNER_USER_ID")

		var buf bytes.Buffer
		log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))

		mode, ownerID, err := ResolveArticleSyncConfig(log)

		require.NoError(t, err)
		assert.Equal(t, SyncDisabled, mode)
		assert.Empty(t, ownerID)
		assert.Contains(t, buf.String(), "article_sync_disabled")
		assert.Equal(t, 1, strings.Count(strings.TrimSpace(buf.String()), "\n")+1)
	})

	t.Run("disabled mode accepts empty ARTICLE_SYNC_OWNER_USER_ID from compose interpolation", func(t *testing.T) {
		t.Setenv("INOREADER_SYNC", "disabled")
		t.Setenv("ARTICLE_SYNC_OWNER_USER_ID", "")

		var buf bytes.Buffer
		log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))

		mode, ownerID, err := ResolveArticleSyncConfig(log)

		require.NoError(t, err)
		assert.Equal(t, SyncDisabled, mode)
		assert.Empty(t, ownerID)
		assert.Contains(t, buf.String(), "article_sync_disabled")
	})

	t.Run("disabled mode still validates ARTICLE_SYNC_OWNER_USER_ID when set", func(t *testing.T) {
		t.Setenv("INOREADER_SYNC", "disabled")
		for _, val := range []string{"   ", "not-a-uuid", "00000000-0000-0000-0000-000000000000"} {
			t.Setenv("ARTICLE_SYNC_OWNER_USER_ID", val)

			mode, ownerID, err := ResolveArticleSyncConfig(slog.Default())

			require.Error(t, err, "expected error for %q", val)
			assert.Empty(t, mode)
			assert.Empty(t, ownerID)
			assert.Contains(t, err.Error(), "ARTICLE_SYNC_OWNER_USER_ID")
		}

		t.Setenv("ARTICLE_SYNC_OWNER_USER_ID", "  AAAAAAAA-BBBB-4CCC-8DDD-EEEEEEEEEEEE  ")
		mode, ownerID, err := ResolveArticleSyncConfig(slog.Default())
		require.NoError(t, err)
		assert.Equal(t, SyncDisabled, mode)
		assert.Equal(t, "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee", ownerID)
	})

	t.Run("enabled mode requires valid non-nil ARTICLE_SYNC_OWNER_USER_ID and logs once", func(t *testing.T) {
		t.Setenv("INOREADER_SYNC", "enabled")
		unsetEnvForTest(t, "ARTICLE_SYNC_OWNER_USER_ID")

		_, _, err := ResolveArticleSyncConfig(slog.Default())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ARTICLE_SYNC_OWNER_USER_ID")

		for _, val := range []string{"", "   ", "not-a-uuid", "00000000-0000-0000-0000-000000000000"} {
			t.Setenv("ARTICLE_SYNC_OWNER_USER_ID", val)
			_, _, err := ResolveArticleSyncConfig(slog.Default())
			require.Error(t, err, "expected error for %q", val)
			assert.Contains(t, err.Error(), "ARTICLE_SYNC_OWNER_USER_ID")
		}

		var buf bytes.Buffer
		log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
		t.Setenv("ARTICLE_SYNC_OWNER_USER_ID", "  AAAAAAAA-BBBB-4CCC-8DDD-EEEEEEEEEEEE  ")

		mode, ownerID, err := ResolveArticleSyncConfig(log)

		require.NoError(t, err)
		assert.Equal(t, SyncEnabled, mode)
		assert.Equal(t, "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee", ownerID)
		assert.Contains(t, buf.String(), "article_sync_enabled")
		assert.Equal(t, 1, strings.Count(strings.TrimSpace(buf.String()), "\n")+1)
	})
}

func TestLoadConfig_ArticleSyncOwnerUserIDValidation(t *testing.T) {
	t.Run("rejects malformed or whitespace ARTICLE_SYNC_OWNER_USER_ID when set", func(t *testing.T) {
		for _, val := range []string{"   ", "invalid-uuid"} {
			t.Setenv("ARTICLE_SYNC_OWNER_USER_ID", val)

			cfg, err := LoadConfig()

			require.Error(t, err)
			assert.Nil(t, cfg)
			assert.Contains(t, err.Error(), "ARTICLE_SYNC_OWNER_USER_ID")
		}
	})

	t.Run("rejects nil UUID ARTICLE_SYNC_OWNER_USER_ID when set", func(t *testing.T) {
		t.Setenv("ARTICLE_SYNC_OWNER_USER_ID", "00000000-0000-0000-0000-000000000000")

		cfg, err := LoadConfig()

		require.Error(t, err)
		assert.Nil(t, cfg)
		assert.Contains(t, err.Error(), "ARTICLE_SYNC_OWNER_USER_ID")
	})

	t.Run("normalizes valid ARTICLE_SYNC_OWNER_USER_ID when set", func(t *testing.T) {
		t.Setenv("ARTICLE_SYNC_OWNER_USER_ID", "11111111-2222-4333-8444-555555555555")

		cfg, err := LoadConfig()

		require.NoError(t, err)
		require.NotNil(t, cfg)
		assert.Equal(t, "11111111-2222-4333-8444-555555555555", cfg.ArticleSync.OwnerUserID)
	})
}

func unsetEnvForTest(t *testing.T, key string) {
	t.Helper()
	t.Setenv(key, "placeholder")
	require.NoError(t, os.Unsetenv(key))
}
