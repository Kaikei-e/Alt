package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeTempTokenFile(t *testing.T, filename, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, filename)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func TestLoadSovereignOperatorToken(t *testing.T) {
	const validToken = "valid-operator-token-long-enough-12345"

	t.Run("empty sovereign URL disables operator client without error", func(t *testing.T) {
		token, enabled, err := LoadSovereignOperatorToken("")
		require.NoError(t, err)
		assert.False(t, enabled)
		assert.Empty(t, token)
	})

	t.Run("resolves valid token from SOVEREIGN_OPERATOR_TOKEN_FILE", func(t *testing.T) {
		path := writeTempTokenFile(t, "sovereign_operator_token", validToken+"\n")
		t.Setenv("SOVEREIGN_OPERATOR_TOKEN_FILE", path)
		t.Setenv("SOVEREIGN_OPERATOR_TOKEN", "")

		token, enabled, err := LoadSovereignOperatorToken("http://knowledge-sovereign:9500")
		require.NoError(t, err)
		assert.True(t, enabled)
		assert.Equal(t, validToken, token)
	})

	t.Run("resolves valid token from SOVEREIGN_OPERATOR_TOKEN env", func(t *testing.T) {
		t.Setenv("SOVEREIGN_OPERATOR_TOKEN_FILE", "")
		t.Setenv("SOVEREIGN_OPERATOR_TOKEN", validToken)

		token, enabled, err := LoadSovereignOperatorToken("http://knowledge-sovereign:9500")
		require.NoError(t, err)
		assert.True(t, enabled)
		assert.Equal(t, validToken, token)
	})

	t.Run("short token file fails fast", func(t *testing.T) {
		path := writeTempTokenFile(t, "sovereign_operator_token", "too-short")
		t.Setenv("SOVEREIGN_OPERATOR_TOKEN_FILE", path)
		t.Setenv("SOVEREIGN_OPERATOR_TOKEN", "")

		_, _, err := LoadSovereignOperatorToken("http://knowledge-sovereign:9500")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "at least 24 characters")
	})

	t.Run("missing token file fails fast", func(t *testing.T) {
		t.Setenv("SOVEREIGN_OPERATOR_TOKEN_FILE", "/nonexistent/sovereign_operator_token")
		t.Setenv("SOVEREIGN_OPERATOR_TOKEN", "")

		_, _, err := LoadSovereignOperatorToken("http://knowledge-sovereign:9500")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "read SOVEREIGN_OPERATOR_TOKEN_FILE")
	})

	t.Run("missing both token file and env fails fast when sovereign URL is set", func(t *testing.T) {
		t.Setenv("SOVEREIGN_OPERATOR_TOKEN_FILE", "")
		t.Setenv("SOVEREIGN_OPERATOR_TOKEN", "")

		_, _, err := LoadSovereignOperatorToken("http://knowledge-sovereign:9500")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "SOVEREIGN_OPERATOR_TOKEN_FILE or SOVEREIGN_OPERATOR_TOKEN is required")
	})
}

func TestLoadOperatorAuth_RefusesDisabledWhenPrivilegedOperatorEnabled(t *testing.T) {
	const validToken = "valid-operator-token-long-enough-12345"

	t.Run("OPERATOR_AUTH=disabled refused when SOVEREIGN_OPERATOR_TOKEN_FILE is configured", func(t *testing.T) {
		path := writeTempTokenFile(t, "sovereign_operator_token", validToken)
		t.Setenv("OPERATOR_AUTH", "disabled")
		t.Setenv("SOVEREIGN_OPERATOR_TOKEN_FILE", path)
		t.Setenv("SOVEREIGN_OPERATOR_TOKEN", "")

		_, _, err := LoadOperatorAuth()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "OPERATOR_AUTH=disabled is refused when privileged sovereign operator client is enabled")
	})

	t.Run("OPERATOR_AUTH=disabled refused when SOVEREIGN_OPERATOR_TOKEN is configured", func(t *testing.T) {
		t.Setenv("OPERATOR_AUTH", "disabled")
		t.Setenv("SOVEREIGN_OPERATOR_TOKEN_FILE", "")
		t.Setenv("SOVEREIGN_OPERATOR_TOKEN", validToken)

		_, _, err := LoadOperatorAuth()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "OPERATOR_AUTH=disabled is refused when privileged sovereign operator client is enabled")
	})

	t.Run("OPERATOR_AUTH=disabled accepted when privileged operator token is not configured", func(t *testing.T) {
		t.Setenv("OPERATOR_AUTH", "disabled")
		t.Setenv("SOVEREIGN_OPERATOR_TOKEN_FILE", "")
		t.Setenv("SOVEREIGN_OPERATOR_TOKEN", "")

		token, enabled, err := LoadOperatorAuth()
		require.NoError(t, err)
		assert.False(t, enabled)
		assert.Empty(t, token)
	})
}
