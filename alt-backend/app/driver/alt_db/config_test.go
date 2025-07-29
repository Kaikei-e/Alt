package alt_db

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDatabaseConfig_BuildConnectionString(t *testing.T) {
	tests := []struct {
		name     string
		config   *DatabaseConfig
		expected string
	}{
		{
			name: "basic SSL prefer",
			config: &DatabaseConfig{
				Host:     "localhost",
				Port:     "5432",
				User:     "testuser",
				Password: "testpass",
				DBName:   "testdb",
				SSL:      SSLConfig{Mode: "prefer"},
			},
			expected: "host=localhost port=5432 user=testuser password=testpass dbname=testdb sslmode=prefer search_path=public pool_max_conns=20 pool_min_conns=5",
		},
		{
			name: "SSL with certificates",
			config: &DatabaseConfig{
				Host:     "localhost",
				Port:     "5432",
				User:     "testuser",
				Password: "testpass",
				DBName:   "testdb",
				SSL: SSLConfig{
					Mode:     "verify-full",
					RootCert: "/path/to/ca.crt",
					Cert:     "/path/to/client.crt",
					Key:      "/path/to/client.key",
				},
			},
			expected: "host=localhost port=5432 user=testuser password=testpass dbname=testdb sslmode=verify-full sslrootcert=/path/to/ca.crt sslcert=/path/to/client.crt sslkey=/path/to/client.key search_path=public pool_max_conns=20 pool_min_conns=5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.BuildConnectionString()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNewDatabaseConfigFromEnv(t *testing.T) {
	// 環境変数設定
	os.Setenv("DB_HOST", "testhost")
	os.Setenv("DB_SSL_MODE", "require")
	defer func() {
		os.Unsetenv("DB_HOST")
		os.Unsetenv("DB_SSL_MODE")
	}()

	config := NewDatabaseConfigFromEnv()

	assert.Equal(t, "testhost", config.Host)
	assert.Equal(t, "require", config.SSL.Mode)
	assert.Equal(t, "5432", config.Port) // デフォルト値
}

func TestSSLConfig_ValidateSSLConfig(t *testing.T) {
	tests := []struct {
		name      string
		config    *DatabaseConfig
		wantError bool
	}{
		{
			name: "valid prefer mode",
			config: &DatabaseConfig{
				SSL: SSLConfig{Mode: "prefer"},
			},
			wantError: false,
		},
		{
			name: "valid verify-full with certificates",
			config: &DatabaseConfig{
				SSL: SSLConfig{
					Mode:     "verify-full",
					RootCert: "/path/to/ca.crt",
				},
			},
			wantError: false,
		},
		{
			name: "invalid verify-full without certificates",
			config: &DatabaseConfig{
				SSL: SSLConfig{
					Mode:     "verify-full",
					RootCert: "",
				},
			},
			wantError: true,
		},
		{
			name: "invalid ssl mode",
			config: &DatabaseConfig{
				SSL: SSLConfig{Mode: "invalid"},
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.ValidateSSLConfig()
			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestInitDBConnectionPool_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// テスト用の環境変数設定
	os.Setenv("DB_SSL_MODE", "prefer")
	os.Setenv("DB_HOST", "localhost")
	os.Setenv("DB_PORT", "5432")
	os.Setenv("DB_USER", "testuser")
	os.Setenv("DB_PASSWORD", "testpass")
	os.Setenv("DB_NAME", "testdb")
	defer func() {
		os.Unsetenv("DB_SSL_MODE")
		os.Unsetenv("DB_HOST")
		os.Unsetenv("DB_PORT")
		os.Unsetenv("DB_USER")
		os.Unsetenv("DB_PASSWORD")
		os.Unsetenv("DB_NAME")
	}()

	// 新しいInitDB関数をテスト
	// この時点ではまだ実装されていないのでエラーになる想定
	t.Run("test new InitDB with SSL config", func(t *testing.T) {
		// このテストは新しいInitDB実装後に動作する
		t.Skip("InitDB not yet refactored to use SSL config")
	})
}
