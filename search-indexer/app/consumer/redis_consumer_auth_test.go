package consumer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewConsumer_RedisPassword(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.RedisURL = "redis://localhost:6379"
	cfg.RedisPassword = "test-secret-password"

	c, err := NewConsumer(cfg, nil, nil)
	require.NoError(t, err)
	defer c.client.Close()

	assert.Equal(t, "test-secret-password", c.client.Options().Password)
}

func TestConfigFromEnv_RedisPassword(t *testing.T) {
	t.Run("resolves password when file present", func(t *testing.T) {
		tmpDir := t.TempDir()
		pwFile := filepath.Join(tmpDir, "redis_password.txt")
		err := os.WriteFile(pwFile, []byte("my-secret-pw\n"), 0600)
		require.NoError(t, err)

		t.Setenv("REDIS_PASSWORD_FILE", pwFile)

		cfg, err := ConfigFromEnv()
		require.NoError(t, err)
		assert.Equal(t, "my-secret-pw", cfg.RedisPassword)
	})

	t.Run("fails when file missing", func(t *testing.T) {
		t.Setenv("REDIS_PASSWORD_FILE", "/non/existent/file")

		_, err := ConfigFromEnv()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "REDIS_PASSWORD_FILE")
	})

	t.Run("fails when neither REDIS_PASSWORD_FILE nor REDIS_AUTH is set", func(t *testing.T) {
		t.Setenv("REDIS_PASSWORD_FILE", "")
		if err := os.Unsetenv("REDIS_PASSWORD_FILE"); err != nil {
			t.Fatal(err)
		}
		t.Setenv("REDIS_AUTH", "")
		if err := os.Unsetenv("REDIS_AUTH"); err != nil {
			t.Fatal(err)
		}

		_, err := ConfigFromEnv()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "REDIS_PASSWORD_FILE")
		assert.Contains(t, err.Error(), "REDIS_AUTH")
	})

	t.Run("returns empty password when REDIS_AUTH=disabled", func(t *testing.T) {
		t.Setenv("REDIS_PASSWORD_FILE", "")
		if err := os.Unsetenv("REDIS_PASSWORD_FILE"); err != nil {
			t.Fatal(err)
		}
		t.Setenv("REDIS_AUTH", "disabled")

		cfg, err := ConfigFromEnv()
		require.NoError(t, err)
		assert.Empty(t, cfg.RedisPassword)
	})
}
