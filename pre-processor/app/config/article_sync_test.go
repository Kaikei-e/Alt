package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveArticleSyncOwnerUserID(t *testing.T) {
	t.Run("fails when ARTICLE_SYNC_OWNER_USER_ID is unset", func(t *testing.T) {
		unsetEnvForTest(t, "ARTICLE_SYNC_OWNER_USER_ID")

		ownerID, err := ResolveArticleSyncOwnerUserID()

		require.Error(t, err)
		assert.Empty(t, ownerID)
		assert.Contains(t, err.Error(), "ARTICLE_SYNC_OWNER_USER_ID")
	})

	t.Run("fails when ARTICLE_SYNC_OWNER_USER_ID is empty or whitespace", func(t *testing.T) {
		for _, val := range []string{"", "   ", "\t\n"} {
			t.Setenv("ARTICLE_SYNC_OWNER_USER_ID", val)

			ownerID, err := ResolveArticleSyncOwnerUserID()

			require.Error(t, err)
			assert.Empty(t, ownerID)
			assert.Contains(t, err.Error(), "ARTICLE_SYNC_OWNER_USER_ID")
		}
	})

	t.Run("fails when ARTICLE_SYNC_OWNER_USER_ID is not a valid UUID", func(t *testing.T) {
		for _, val := range []string{"not-a-uuid", "12345", "11111111-2222-3333-4444"} {
			t.Setenv("ARTICLE_SYNC_OWNER_USER_ID", val)

			ownerID, err := ResolveArticleSyncOwnerUserID()

			require.Error(t, err)
			assert.Empty(t, ownerID)
			assert.Contains(t, err.Error(), "ARTICLE_SYNC_OWNER_USER_ID")
		}
	})

	t.Run("fails when ARTICLE_SYNC_OWNER_USER_ID is the nil UUID", func(t *testing.T) {
		t.Setenv("ARTICLE_SYNC_OWNER_USER_ID", "00000000-0000-0000-0000-000000000000")

		ownerID, err := ResolveArticleSyncOwnerUserID()

		require.Error(t, err)
		assert.Empty(t, ownerID)
		assert.Contains(t, err.Error(), "ARTICLE_SYNC_OWNER_USER_ID")
	})

	t.Run("returns canonical UUID string when valid", func(t *testing.T) {
		t.Setenv("ARTICLE_SYNC_OWNER_USER_ID", "  AAAAAAAA-BBBB-4CCC-8DDD-EEEEEEEEEEEE  ")

		ownerID, err := ResolveArticleSyncOwnerUserID()

		require.NoError(t, err)
		assert.Equal(t, "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee", ownerID)
	})
}

func TestLoadConfig_ArticleSyncOwnerUserIDValidation(t *testing.T) {
	t.Run("rejects malformed ARTICLE_SYNC_OWNER_USER_ID when set", func(t *testing.T) {
		t.Setenv("ARTICLE_SYNC_OWNER_USER_ID", "invalid-uuid")

		cfg, err := LoadConfig()

		require.Error(t, err)
		assert.Nil(t, cfg)
		assert.Contains(t, err.Error(), "ARTICLE_SYNC_OWNER_USER_ID")
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
