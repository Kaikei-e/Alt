package config_test

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"auth-service/app/config"
)

func TestConfig_Load(t *testing.T) {
	tests := []struct {
		name    string
		envVars map[string]string
		want    *config.Config
		wantErr bool
	}{
		{
			name: "default configuration",
			envVars: map[string]string{
				"DATABASE_URL":      "postgres://auth_user:password@auth-postgres:5432/auth_db?sslmode=require",
				"KRATOS_PUBLIC_URL": "http://kratos-public:4433",
				"KRATOS_ADMIN_URL":  "http://kratos-admin:4434",
				"DB_PASSWORD":       "test_password",
			},
			want: &config.Config{
				Port:             "9500",
				Host:             "0.0.0.0",
				LogLevel:         "info",
				DatabaseURL:      "postgres://auth_user:password@auth-postgres:5432/auth_db?sslmode=require",
				DatabaseHost:     "auth-postgres",
				DatabasePort:     "5432",
				DatabaseName:     "auth_db",
				DatabaseUser:     "auth_user",
				DatabasePassword: "test_password",
				DatabaseSSLMode:  "require",
				KratosPublicURL:  "http://kratos-public:4433",
				KratosAdminURL:   "http://kratos-admin:4434",
				CSRFTokenLength:  32,
				SessionTimeout:   24 * time.Hour,
				EnableAuditLog:   true,
				EnableMetrics:    true,
			},
			wantErr: false,
		},
		{
			name: "custom configuration",
			envVars: map[string]string{
				"PORT":              "8080",
				"HOST":              "127.0.0.1",
				"LOG_LEVEL":         "debug",
				"DATABASE_URL":      "postgres://custom_user:custom_pass@custom-host:5433/custom_db",
				"DB_HOST":           "custom-host",
				"DB_PORT":           "5433",
				"DB_NAME":           "custom_db",
				"DB_USER":           "custom_user",
				"DB_PASSWORD":       "custom_pass",
				"DB_SSL_MODE":       "disable",
				"KRATOS_PUBLIC_URL": "http://custom-kratos:4433",
				"KRATOS_ADMIN_URL":  "http://custom-kratos:4434",
				"CSRF_TOKEN_LENGTH": "64",
				"SESSION_TIMEOUT":   "12h",
				"ENABLE_AUDIT_LOG":  "false",
				"ENABLE_METRICS":    "false",
			},
			want: &config.Config{
				Port:             "8080",
				Host:             "127.0.0.1",
				LogLevel:         "debug",
				DatabaseURL:      "postgres://custom_user:custom_pass@custom-host:5433/custom_db",
				DatabaseHost:     "custom-host",
				DatabasePort:     "5433",
				DatabaseName:     "custom_db",
				DatabaseUser:     "custom_user",
				DatabasePassword: "custom_pass",
				DatabaseSSLMode:  "disable",
				KratosPublicURL:  "http://custom-kratos:4433",
				KratosAdminURL:   "http://custom-kratos:4434",
				CSRFTokenLength:  64,
				SessionTimeout:   12 * time.Hour,
				EnableAuditLog:   false,
				EnableMetrics:    false,
			},
			wantErr: false,
		},
		{
			name: "missing required fields",
			envVars: map[string]string{
				"PORT": "9500",
				// Missing DATABASE_URL, KRATOS_PUBLIC_URL, KRATOS_ADMIN_URL, DB_PASSWORD
			},
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup environment variables
			for key, value := range tt.envVars {
				os.Setenv(key, value)
				defer os.Unsetenv(key)
			}

			got, err := config.Load()

			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, got)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  *config.Config
		wantErr bool
	}{
		{
			name: "valid configuration",
			config: &config.Config{
				Port:             "9500",
				Host:             "0.0.0.0",
				LogLevel:         "info",
				DatabaseURL:      "postgres://auth_user:password@auth-postgres:5432/auth_db",
				DatabaseHost:     "auth-postgres",
				DatabasePort:     "5432",
				DatabaseName:     "auth_db",
				DatabaseUser:     "auth_user",
				DatabasePassword: "password",
				KratosPublicURL:  "http://kratos-public:4433",
				KratosAdminURL:   "http://kratos-admin:4434",
				CSRFTokenLength:  32,
				SessionTimeout:   24 * time.Hour,
				EnableAuditLog:   true,
				EnableMetrics:    true,
			},
			wantErr: false,
		},
		{
			name: "invalid port",
			config: &config.Config{
				Port:             "invalid_port",
				DatabaseURL:      "postgres://auth_user:password@auth-postgres:5432/auth_db",
				KratosPublicURL:  "http://kratos-public:4433",
				KratosAdminURL:   "http://kratos-admin:4434",
				DatabasePassword: "password",
			},
			wantErr: true,
		},
		{
			name: "invalid log level",
			config: &config.Config{
				Port:             "9500",
				LogLevel:         "invalid_level",
				DatabaseURL:      "postgres://auth_user:password@auth-postgres:5432/auth_db",
				KratosPublicURL:  "http://kratos-public:4433",
				KratosAdminURL:   "http://kratos-admin:4434",
				DatabasePassword: "password",
			},
			wantErr: true,
		},
		{
			name: "invalid CSRF token length",
			config: &config.Config{
				Port:             "9500",
				LogLevel:         "info",
				DatabaseURL:      "postgres://auth_user:password@auth-postgres:5432/auth_db",
				KratosPublicURL:  "http://kratos-public:4433",
				KratosAdminURL:   "http://kratos-admin:4434",
				DatabasePassword: "password",
				CSRFTokenLength:  8, // Too short
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
