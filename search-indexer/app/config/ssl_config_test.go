package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDatabaseConfig_BuildPgxConnectionString(t *testing.T) {
	tests := []struct {
		name     string
		config   *DatabaseConfig
		expected string
	}{
		{
			name: "SSL prefer mode",
			config: &DatabaseConfig{
				Host:     "localhost",
				Port:     "5432",
				User:     "user",
				Password: "pass",
				Name:     "testdb",
				SSL:      SSLConfig{Mode: "prefer"},
			},
			expected: "host=localhost port=5432 user=user password=pass dbname=testdb sslmode=prefer",
		},
		{
			name: "SSL require mode",
			config: &DatabaseConfig{
				Host:     "localhost",
				Port:     "5432",
				User:     "user",
				Password: "pass",
				Name:     "testdb",
				SSL:      SSLConfig{Mode: "require"},
			},
			expected: "host=localhost port=5432 user=user password=pass dbname=testdb sslmode=require",
		},
		{
			name: "SSL verify-full with certificates",
			config: &DatabaseConfig{
				Host:     "db.example.com",
				Port:     "5432",
				User:     "appuser",
				Password: "secret",
				Name:     "appdb",
				SSL: SSLConfig{
					Mode:     "verify-full",
					RootCert: "/app/ssl/ca.crt",
					Cert:     "/app/ssl/client.crt",
					Key:      "/app/ssl/client.key",
				},
			},
			expected: "host=db.example.com port=5432 user=appuser password=secret dbname=appdb sslmode=verify-full sslrootcert=/app/ssl/ca.crt sslcert=/app/ssl/client.crt sslkey=/app/ssl/client.key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.BuildPgxConnectionString()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDatabaseConfig_BuildPostgresURL(t *testing.T) {
	tests := []struct {
		name     string
		config   *DatabaseConfig
		expected string
	}{
		{
			name: "SSL prefer mode URL",
			config: &DatabaseConfig{
				Host:     "localhost",
				Port:     "5432",
				User:     "user",
				Password: "pass",
				Name:     "testdb",
				SSL:      SSLConfig{Mode: "prefer"},
			},
			expected: "postgres://user:pass@localhost:5432/testdb?sslmode=prefer",
		},
		{
			name: "SSL verify-full with certificates URL",
			config: &DatabaseConfig{
				Host:     "db.example.com",
				Port:     "5432",
				User:     "appuser",
				Password: "secret",
				Name:     "appdb",
				SSL: SSLConfig{
					Mode:     "verify-full",
					RootCert: "/app/ssl/ca.crt",
				},
			},
			expected: "postgres://appuser:secret@db.example.com:5432/appdb?sslmode=verify-full&sslrootcert=/app/ssl/ca.crt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.BuildPostgresURL()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSSLConfigValidation(t *testing.T) {
	tests := []struct {
		name      string
		sslConfig SSLConfig
		expectErr bool
		errMsg    string
	}{
		{"prefer mode", SSLConfig{Mode: "prefer"}, false, ""},
		{"require mode", SSLConfig{Mode: "require"}, false, ""},
		{"verify-ca with cert", SSLConfig{Mode: "verify-ca", RootCert: "/path/to/ca.crt"}, false, ""},
		{"verify-full with cert", SSLConfig{Mode: "verify-full", RootCert: "/path/to/ca.crt"}, false, ""},
		{"disable mode", SSLConfig{Mode: "disable"}, true, "SSL disable mode is not allowed"},
		{"verify-ca without cert", SSLConfig{Mode: "verify-ca"}, true, "SSL root certificate required"},
		{"invalid mode", SSLConfig{Mode: "invalid"}, true, "invalid SSL mode"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &DatabaseConfig{SSL: tt.sslConfig}
			err := config.ValidateSSLConfig()

			if tt.expectErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestNewDatabaseConfigFromEnv(t *testing.T) {
	// 環境変数をクリア
	os.Clearenv()

	// テスト用環境変数設定
	envVars := map[string]string{
		"DB_HOST":     "testhost",
		"DB_SSL_MODE": "require",
		"DB_USER":     "testuser",
		"DB_PASSWORD": "testpass",
		"DB_NAME":     "testdb",
	}

	for k, v := range envVars {
		os.Setenv(k, v)
	}
	defer os.Clearenv()

	config := NewDatabaseConfigFromEnv()
	assert.NotNil(t, config)

	assert.Equal(t, "testhost", config.Host)
	assert.Equal(t, "require", config.SSL.Mode)
	assert.Equal(t, "testuser", config.User)
	assert.Equal(t, "testpass", config.Password)
	assert.Equal(t, "testdb", config.Name)
}

func TestLoad_InvalidSSL(t *testing.T) {
	os.Clearenv()

	// 必要な環境変数を設定
	envVars := map[string]string{
		"DB_HOST":                    "localhost",
		"DB_PORT":                    "5432",
		"DB_NAME":                    "testdb",
		"SEARCH_INDEXER_DB_USER":     "user",
		"SEARCH_INDEXER_DB_PASSWORD": "pass",
		"MEILISEARCH_HOST":           "http://localhost:7700",
		"DB_SSL_MODE":                "disable", // 無効なSSLモード
	}

	for k, v := range envVars {
		os.Setenv(k, v)
	}
	defer os.Clearenv()

	config, err := Load()
	assert.Error(t, err)
	assert.Nil(t, config)
	assert.Contains(t, err.Error(), "SSL disable mode is not allowed")
}
