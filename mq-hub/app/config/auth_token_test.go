package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveAuthToken(t *testing.T) {
	t.Run("missing both file and env returns error naming MQHUB_AUTH_TOKEN_FILE", func(t *testing.T) {
		os.Unsetenv("MQHUB_AUTH_TOKEN_FILE")
		os.Unsetenv("MQHUB_AUTH_TOKEN")
		_, err := ResolveAuthToken(nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "MQHUB_AUTH_TOKEN_FILE")
	})

	t.Run("empty MQHUB_AUTH_TOKEN_FILE path returns error", func(t *testing.T) {
		t.Setenv("MQHUB_AUTH_TOKEN_FILE", "   ")
		t.Setenv("MQHUB_AUTH_TOKEN", "")
		_, err := ResolveAuthToken(nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "MQHUB_AUTH_TOKEN_FILE is set but empty")
	})

	t.Run("nonexistent MQHUB_AUTH_TOKEN_FILE returns read error", func(t *testing.T) {
		t.Setenv("MQHUB_AUTH_TOKEN_FILE", "/nonexistent/token/path")
		t.Setenv("MQHUB_AUTH_TOKEN", "")
		_, err := ResolveAuthToken(nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "read MQHUB_AUTH_TOKEN_FILE")
	})

	t.Run("empty token file content returns error", func(t *testing.T) {
		tmpDir := t.TempDir()
		tokenFile := filepath.Join(tmpDir, "empty_token")
		require.NoError(t, os.WriteFile(tokenFile, []byte("   \n\t "), 0600))
		t.Setenv("MQHUB_AUTH_TOKEN_FILE", tokenFile)
		t.Setenv("MQHUB_AUTH_TOKEN", "")

		_, err := ResolveAuthToken(nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "resolved to an empty token")
	})

	t.Run("non-ASCII token returns error", func(t *testing.T) {
		tmpDir := t.TempDir()
		tokenFile := filepath.Join(tmpDir, "non_ascii_token")
		require.NoError(t, os.WriteFile(tokenFile, []byte("secret-token-日本語\n"), 0600))
		t.Setenv("MQHUB_AUTH_TOKEN_FILE", tokenFile)
		t.Setenv("MQHUB_AUTH_TOKEN", "")

		_, err := ResolveAuthToken(nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "safe ASCII")
	})

	t.Run("token with control characters returns error", func(t *testing.T) {
		tmpDir := t.TempDir()
		tokenFile := filepath.Join(tmpDir, "control_token")
		require.NoError(t, os.WriteFile(tokenFile, []byte("secret\x00token\n"), 0600))
		t.Setenv("MQHUB_AUTH_TOKEN_FILE", tokenFile)
		t.Setenv("MQHUB_AUTH_TOKEN", "")

		_, err := ResolveAuthToken(nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "safe ASCII")
	})

	t.Run("valid token from file succeeds", func(t *testing.T) {
		tmpDir := t.TempDir()
		tokenFile := filepath.Join(tmpDir, "valid_token")
		require.NoError(t, os.WriteFile(tokenFile, []byte("mqhub-valid-bearer-token-1234\n"), 0600))
		t.Setenv("MQHUB_AUTH_TOKEN_FILE", tokenFile)
		t.Setenv("MQHUB_AUTH_TOKEN", "")

		token, err := ResolveAuthToken(nil)
		require.NoError(t, err)
		assert.Equal(t, "mqhub-valid-bearer-token-1234", token)
	})

	t.Run("valid token from env var fallback succeeds", func(t *testing.T) {
		os.Unsetenv("MQHUB_AUTH_TOKEN_FILE")
		t.Setenv("MQHUB_AUTH_TOKEN", "mqhub-valid-bearer-token-from-env")

		token, err := ResolveAuthToken(nil)
		require.NoError(t, err)
		assert.Equal(t, "mqhub-valid-bearer-token-from-env", token)
	})
}
