package alt_db

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDatabaseConfig_BuildConnectionString(t *testing.T) {
	tests := []struct {
		name     string
		config   *DatabaseConfig
		expected string
	}{
		{
			name: "disable mode",
			config: &DatabaseConfig{
				Host:     "localhost",
				Port:     "5432",
				User:     "testuser",
				Password: "testpass",
				DBName:   "testdb",
				SSLMode:  "disable",
			},
			expected: "host=localhost port=5432 user=testuser password=testpass dbname=testdb sslmode=disable search_path=public connect_timeout=90",
		},
		{
			name: "require mode",
			config: &DatabaseConfig{
				Host:     "postgres.alt-database.svc.cluster.local",
				Port:     "5432",
				User:     "alt_db_user",
				Password: "ProductionPassword123",
				DBName:   "alt",
				SSLMode:  "require",
			},
			expected: "host=postgres.alt-database.svc.cluster.local port=5432 user=alt_db_user password=ProductionPassword123 dbname=alt sslmode=require search_path=public connect_timeout=90",
		},
		{
			name: "verify-ca mode",
			config: &DatabaseConfig{
				Host:     "db",
				Port:     "5432",
				User:     "alt_db_user",
				Password: "secret",
				DBName:   "alt",
				SSLMode:  "verify-ca",
			},
			expected: "host=db port=5432 user=alt_db_user password=secret dbname=alt sslmode=verify-ca search_path=public connect_timeout=90",
		},
		{
			name: "verify-full mode",
			config: &DatabaseConfig{
				Host:     "db",
				Port:     "5432",
				User:     "alt_db_user",
				Password: "secret",
				DBName:   "alt",
				SSLMode:  "verify-full",
			},
			expected: "host=db port=5432 user=alt_db_user password=secret dbname=alt sslmode=verify-full search_path=public connect_timeout=90",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.BuildConnectionString()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestLoadDatabaseConfigFromEnv_PasswordFailFast(t *testing.T) {
	t.Run("missing password and password file returns error", func(t *testing.T) {
		t.Setenv("DB_PASSWORD", "")
		t.Setenv("DB_PASSWORD_FILE", "")
		t.Setenv("DB_SSL_MODE", "disable")

		cfg, err := LoadDatabaseConfigFromEnv()
		require.Error(t, err)
		assert.Nil(t, cfg)
		assert.Contains(t, err.Error(), "DB_PASSWORD")
	})

	t.Run("unreadable password file returns error without falling back to DB_PASSWORD", func(t *testing.T) {
		missingFile := filepath.Join(t.TempDir(), "missing-secret")
		t.Setenv("DB_PASSWORD_FILE", missingFile)
		t.Setenv("DB_PASSWORD", "fallback-should-not-be-used")
		t.Setenv("DB_SSL_MODE", "disable")

		cfg, err := LoadDatabaseConfigFromEnv()
		require.Error(t, err)
		assert.Nil(t, cfg)
		assert.Contains(t, err.Error(), "DB_PASSWORD_FILE")
	})

	t.Run("empty password file returns error", func(t *testing.T) {
		emptyFile := filepath.Join(t.TempDir(), "empty-secret")
		require.NoError(t, os.WriteFile(emptyFile, []byte("   \n"), 0o600))
		t.Setenv("DB_PASSWORD_FILE", emptyFile)
		t.Setenv("DB_PASSWORD", "")
		t.Setenv("DB_SSL_MODE", "disable")

		cfg, err := LoadDatabaseConfigFromEnv()
		require.Error(t, err)
		assert.Nil(t, cfg)
		assert.Contains(t, err.Error(), "DB_PASSWORD_FILE")
	})

	t.Run("readable password file loads trimmed secret", func(t *testing.T) {
		secretFile := filepath.Join(t.TempDir(), "db-secret")
		require.NoError(t, os.WriteFile(secretFile, []byte("file-secret-value\n"), 0o600))
		t.Setenv("DB_PASSWORD_FILE", secretFile)
		t.Setenv("DB_PASSWORD", "")
		t.Setenv("DB_SSL_MODE", "require")

		cfg, err := LoadDatabaseConfigFromEnv()
		require.NoError(t, err)
		require.NotNil(t, cfg)
		assert.Equal(t, "file-secret-value", cfg.Password)
	})
}

func TestLoadDatabaseConfigFromEnv_SSLModeValidation(t *testing.T) {
	t.Run("missing DB_SSL_MODE returns error", func(t *testing.T) {
		t.Setenv("DB_PASSWORD", "testpass")
		t.Setenv("DB_PASSWORD_FILE", "")
		t.Setenv("DB_SSL_MODE", "")

		cfg, err := LoadDatabaseConfigFromEnv()
		require.Error(t, err)
		assert.Nil(t, cfg)
		assert.Contains(t, err.Error(), "DB_SSL_MODE")
	})

	for _, invalidMode := range []string{"prefer", "allow", "true", "invalid"} {
		t.Run("invalid_"+invalidMode, func(t *testing.T) {
			t.Setenv("DB_PASSWORD", "testpass")
			t.Setenv("DB_PASSWORD_FILE", "")
			t.Setenv("DB_SSL_MODE", invalidMode)

			cfg, err := LoadDatabaseConfigFromEnv()
			require.Error(t, err)
			assert.Nil(t, cfg)
			assert.Contains(t, err.Error(), "DB_SSL_MODE")
			assert.Contains(t, err.Error(), invalidMode)
		})
	}

	for _, allowedMode := range []string{"disable", "require", "verify-ca", "verify-full"} {
		t.Run("allowed_"+allowedMode, func(t *testing.T) {
			t.Setenv("DB_HOST", "testhost")
			t.Setenv("DB_PASSWORD", "testpass")
			t.Setenv("DB_PASSWORD_FILE", "")
			t.Setenv("DB_SSL_MODE", allowedMode)

			cfg, err := LoadDatabaseConfigFromEnv()
			require.NoError(t, err)
			require.NotNil(t, cfg)
			assert.Equal(t, "testhost", cfg.Host)
			assert.Equal(t, "5432", cfg.Port)
			assert.Equal(t, allowedMode, cfg.SSLMode)
			assert.Contains(t, cfg.BuildConnectionString(), "sslmode="+allowedMode)
		})
	}
}

func TestHTTPOnlyConnection_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	t.Setenv("DB_HOST", "localhost")
	t.Setenv("DB_PORT", "5432")
	t.Setenv("DB_USER", "testuser")
	t.Setenv("DB_PASSWORD", "testpass")
	t.Setenv("DB_PASSWORD_FILE", "")
	t.Setenv("DB_NAME", "testdb")
	t.Setenv("DB_SSL_MODE", "disable")

	t.Run("explicit disable mode connection string generation", func(t *testing.T) {
		config, err := LoadDatabaseConfigFromEnv()
		require.NoError(t, err)
		connStr := config.BuildConnectionString()

		assert.Contains(t, connStr, "sslmode=disable")
		assert.NotContains(t, connStr, "sslrootcert")
		assert.NotContains(t, connStr, "sslcert")
		assert.NotContains(t, connStr, "sslkey")
	})
}
