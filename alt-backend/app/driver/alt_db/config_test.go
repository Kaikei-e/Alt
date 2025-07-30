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
			name: "HTTP-only connection for Linkerd mTLS",
			config: &DatabaseConfig{
				Host:     "localhost",
				Port:     "5432",
				User:     "testuser",
				Password: "testpass",
				DBName:   "testdb",
			},
			expected: "host=localhost port=5432 user=testuser password=testpass dbname=testdb sslmode=disable search_path=public pool_max_conns=20 pool_min_conns=5",
		},
		{
			name: "production config HTTP-only",
			config: &DatabaseConfig{
				Host:     "postgres.alt-database.svc.cluster.local",
				Port:     "5432",
				User:     "alt_db_user",
				Password: "ProductionPassword123",
				DBName:   "alt",
			},
			expected: "host=postgres.alt-database.svc.cluster.local port=5432 user=alt_db_user password=ProductionPassword123 dbname=alt sslmode=disable search_path=public pool_max_conns=20 pool_min_conns=5",
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
	// 環境変数設定 - SSL設定は無視される
	os.Setenv("DB_HOST", "testhost")
	os.Setenv("DB_SSL_MODE", "require") // この設定は無視される
	defer func() {
		os.Unsetenv("DB_HOST")
		os.Unsetenv("DB_SSL_MODE")
	}()

	config := NewDatabaseConfigFromEnv()

	assert.Equal(t, "testhost", config.Host)
	assert.Equal(t, "5432", config.Port) // デフォルト値
	// SSL設定は完全除去されているため検証なし
}

// TestSSLConfig_ValidateSSLConfig はSSL設定除去により削除
// Linkerd mTLSにより暗号化はプロキシレベルで自動処理される

func TestHTTPOnlyConnection_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// テスト用の環境変数設定 - HTTP-only接続
	os.Setenv("DB_HOST", "localhost")
	os.Setenv("DB_PORT", "5432")
	os.Setenv("DB_USER", "testuser")
	os.Setenv("DB_PASSWORD", "testpass")
	os.Setenv("DB_NAME", "testdb")
	defer func() {
		os.Unsetenv("DB_HOST")
		os.Unsetenv("DB_PORT")
		os.Unsetenv("DB_USER")
		os.Unsetenv("DB_PASSWORD")
		os.Unsetenv("DB_NAME")
	}()

	t.Run("HTTP-only connection string generation", func(t *testing.T) {
		config := NewDatabaseConfigFromEnv()
		connStr := config.BuildConnectionString()
		
		// sslmode=disableが含まれていることを確認
		assert.Contains(t, connStr, "sslmode=disable")
		// SSL証明書パラメータが含まれていないことを確認
		assert.NotContains(t, connStr, "sslrootcert")
		assert.NotContains(t, connStr, "sslcert")
		assert.NotContains(t, connStr, "sslkey")
	})
}
