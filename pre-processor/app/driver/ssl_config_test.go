package driver

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
			name: "SSL prefer mode",
			config: &DatabaseConfig{
				Host:            "localhost",
				Port:            "5432",
				User:            "testuser",
				Password:        "testpass",
				DBName:          "testdb",
				SSL:             DatabaseSSLConfig{Mode: "prefer"},
				MaxConns:        20,
				MinConns:        5,
				MaxConnLifetime: "1h",
				MaxConnIdleTime: "30m",
			},
			expected: "host=localhost port=5432 user=testuser password=testpass dbname=testdb sslmode=prefer pool_max_conns=20 pool_min_conns=5 pool_max_conn_lifetime=1h pool_max_conn_idle_time=30m",
		},
		{
			name: "SSL require mode with certificates",
			config: &DatabaseConfig{
				Host:     "db.example.com",
				Port:     "5432",
				User:     "appuser",
				Password: "secret",
				DBName:   "appdb",
				SSL: DatabaseSSLConfig{
					Mode:     "verify-full",
					RootCert: "/app/ssl/ca.crt",
					Cert:     "/app/ssl/client.crt",
					Key:      "/app/ssl/client.key",
				},
				MaxConns:        10,
				MinConns:        2,
				MaxConnLifetime: "2h",
				MaxConnIdleTime: "15m",
			},
			expected: "host=db.example.com port=5432 user=appuser password=secret dbname=appdb sslmode=verify-full sslrootcert=/app/ssl/ca.crt sslcert=/app/ssl/client.crt sslkey=/app/ssl/client.key pool_max_conns=10 pool_min_conns=2 pool_max_conn_lifetime=2h pool_max_conn_idle_time=15m",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.BuildConnectionString()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNewDatabaseConfig(t *testing.T) {
	// 環境変数をクリア
	os.Clearenv()

	// テスト用環境変数設定
	_ = os.Setenv("DB_HOST", "testhost")
	_ = os.Setenv("DB_SSL_MODE", "require")
	_ = os.Setenv("DB_MAX_CONNS", "15")

	defer func() {
		os.Clearenv()
	}()

	config := NewDatabaseConfig()

	assert.Equal(t, "testhost", config.Host)
	assert.Equal(t, "require", config.SSL.Mode)
	assert.Equal(t, int32(15), config.MaxConns)
	assert.Equal(t, "5432", config.Port) // デフォルト値
}

func TestNewDatabaseConfigWithPrefix(t *testing.T) {
	os.Clearenv()

	_ = os.Setenv("PP_DB_HOST", "pre-processor-db")
	_ = os.Setenv("PP_DB_PORT", "5437")
	_ = os.Setenv("PP_DB_NAME", "pre_processor")
	_ = os.Setenv("PP_DB_USER", "pp_user")
	_ = os.Setenv("PP_DB_PASSWORD", "pp_secret")

	defer func() {
		os.Clearenv()
	}()

	config := NewDatabaseConfigWithPrefix("PP_")

	assert.Equal(t, "pre-processor-db", config.Host)
	assert.Equal(t, "5437", config.Port)
	assert.Equal(t, "pre_processor", config.DBName)
	assert.Equal(t, "pp_user", config.User)
	assert.Equal(t, "pp_secret", config.Password)
	assert.Equal(t, "disable", config.SSL.Mode)
}

func TestNewDatabaseConfigWithPrefix_PasswordFile(t *testing.T) {
	os.Clearenv()

	// Create a temp file for password
	tmpFile, err := os.CreateTemp("", "pp_password")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	_, _ = tmpFile.WriteString("secret_from_file\n")
	_ = tmpFile.Close()

	_ = os.Setenv("PP_DB_HOST", "ppdb")
	_ = os.Setenv("PP_DB_PASSWORD_FILE", tmpFile.Name())

	defer func() {
		os.Clearenv()
	}()

	config := NewDatabaseConfigWithPrefix("PP_")

	assert.Equal(t, "ppdb", config.Host)
	assert.Equal(t, "secret_from_file", config.Password)
}

func TestNewDatabaseConfigWithPrefix_Defaults(t *testing.T) {
	os.Clearenv()

	defer func() {
		os.Clearenv()
	}()

	config := NewDatabaseConfigWithPrefix("PP_")

	assert.Equal(t, "localhost", config.Host)
	assert.Equal(t, "5432", config.Port)
	assert.Equal(t, "postgres", config.DBName)
	assert.Equal(t, "postgres", config.User)
	assert.Equal(t, "disable", config.SSL.Mode)
}

func TestDatabaseConfig_SSLValidation(t *testing.T) {
	// Create temporary certificate file for testing
	tempCertFile, err := os.CreateTemp("", "test_ca.crt")
	if err != nil {
		t.Fatalf("Failed to create temp cert file: %v", err)
	}
	defer func() {
		_ = os.Remove(tempCertFile.Name())
	}()

	// Write some dummy content to make it a valid file
	_, _ = tempCertFile.WriteString("-----BEGIN CERTIFICATE-----\nDUMMY CERTIFICATE\n-----END CERTIFICATE-----")
	_ = tempCertFile.Close()

	tests := []struct {
		name      string
		sslMode   string
		rootCert  string
		expectErr bool
	}{
		{"prefer mode", "prefer", "", false},
		{"require mode", "require", "", false},
		{"verify-ca with cert", "verify-ca", tempCertFile.Name(), false},
		{"verify-full with cert", "verify-full", tempCertFile.Name(), false},
		{"verify-ca without cert", "verify-ca", "", true},
		{"invalid mode", "invalid", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &DatabaseConfig{
				SSL: DatabaseSSLConfig{
					Mode:     tt.sslMode,
					RootCert: tt.rootCert,
				},
			}

			err := config.ValidateSSLConfig()
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
