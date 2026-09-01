package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequireEnv(t *testing.T) {
	t.Run("returns the trimmed value", func(t *testing.T) {
		t.Setenv("REBUILD_TEST_KEY", "  value  ")
		got, err := requireEnv("REBUILD_TEST_KEY")
		require.NoError(t, err)
		assert.Equal(t, "value", got)
	})

	t.Run("an unset key is an error, never an inferred default", func(t *testing.T) {
		t.Setenv("REBUILD_TEST_KEY", "")
		_, err := requireEnv("REBUILD_TEST_KEY")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "REBUILD_TEST_KEY")
	})

	t.Run("a whitespace-only value is an error", func(t *testing.T) {
		t.Setenv("REBUILD_TEST_KEY", "   ")
		_, err := requireEnv("REBUILD_TEST_KEY")
		assert.Error(t, err)
	})
}

func TestRebuildEmbeddingModel_HasNoDefault(t *testing.T) {
	// The model is chosen by evaluation. Inferring one here would rebuild the
	// whole corpus into a vector space nobody picked.
	t.Setenv("EMBEDDING_MODEL", "")
	_, err := rebuildEmbeddingModel()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "EMBEDDING_MODEL")

	t.Setenv("EMBEDDING_MODEL", "some-model")
	got, err := rebuildEmbeddingModel()
	require.NoError(t, err)
	assert.Equal(t, "some-model", got)
}

func TestResolveEmbedderURLs(t *testing.T) {
	t.Run("the replica list wins over the single URL", func(t *testing.T) {
		t.Setenv("EMBEDDER_URLS", "http://a:11434, http://b:11434 ")
		t.Setenv("EMBEDDER_URL", "http://ignored:11434")

		got, err := resolveEmbedderURLs()
		require.NoError(t, err)
		assert.Equal(t, []string{"http://a:11434", "http://b:11434"}, got)
	})

	t.Run("falls back to the single URL", func(t *testing.T) {
		t.Setenv("EMBEDDER_URLS", "")
		t.Setenv("EMBEDDER_URL", "http://only:11434")

		got, err := resolveEmbedderURLs()
		require.NoError(t, err)
		assert.Equal(t, []string{"http://only:11434"}, got)
	})

	t.Run("neither set is an error", func(t *testing.T) {
		t.Setenv("EMBEDDER_URLS", "")
		t.Setenv("EMBEDDER_URL", "")

		_, err := resolveEmbedderURLs()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "EMBEDDER_URL")
	})

	t.Run("a list of separators only is an error", func(t *testing.T) {
		t.Setenv("EMBEDDER_URLS", " , , ")
		t.Setenv("EMBEDDER_URL", "")

		_, err := resolveEmbedderURLs()
		assert.Error(t, err)
	})
}

func TestSingleEmbedderVersion(t *testing.T) {
	t.Run("one replica", func(t *testing.T) {
		got, err := singleEmbedderVersion([]string{"model-a/1024"})
		require.NoError(t, err)
		assert.Equal(t, "model-a/1024", got)
	})

	t.Run("agreeing replicas", func(t *testing.T) {
		got, err := singleEmbedderVersion([]string{"model-a/1024", "model-a/1024", "model-a/1024"})
		require.NoError(t, err)
		assert.Equal(t, "model-a/1024", got)
	})

	t.Run("disagreeing replicas fail fast", func(t *testing.T) {
		// Two models behind one rebuild would write two incompatible vector
		// spaces into one column, and the target could only match one.
		_, err := singleEmbedderVersion([]string{"model-a/1024", "model-b/1024"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "model-a/1024")
		assert.Contains(t, err.Error(), "model-b/1024")
	})

	t.Run("no replica", func(t *testing.T) {
		_, err := singleEmbedderVersion(nil)
		assert.Error(t, err)
	})
}
