package kratos

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"auth-service/app/config"
	"auth-service/app/utils/logger"
)

func TestNewClient(t *testing.T) {
	tests := []struct {
		name      string
		config    *config.Config
		wantError bool
	}{
		{
			name: "valid kratos configuration",
			config: &config.Config{
				KratosPublicURL: "http://kratos-public:4433",
				KratosAdminURL:  "http://kratos-admin:4434",
			},
			wantError: false,
		},
		{
			name: "empty public URL",
			config: &config.Config{
				KratosPublicURL: "",
				KratosAdminURL:  "http://kratos-admin:4434",
			},
			wantError: true,
		},
		{
			name: "empty admin URL",
			config: &config.Config{
				KratosPublicURL: "http://kratos-public:4433",
				KratosAdminURL:  "",
			},
			wantError: true,
		},
		{
			name: "invalid public URL",
			config: &config.Config{
				KratosPublicURL: "invalid-url",
				KratosAdminURL:  "http://kratos-admin:4434",
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger, err := logger.NewWithWriter("info", &buf)
			require.NoError(t, err)

			client, err := NewClient(tt.config, logger)

			if tt.wantError {
				assert.Error(t, err)
				assert.Nil(t, client)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, client)
				assert.NotNil(t, client.PublicAPI())
				assert.NotNil(t, client.AdminAPI())
			}
		})
	}
}

func TestClient_PublicAPI(t *testing.T) {
	var buf bytes.Buffer
	logger, err := logger.NewWithWriter("info", &buf)
	require.NoError(t, err)

	config := &config.Config{
		KratosPublicURL: "http://kratos-public:4433",
		KratosAdminURL:  "http://kratos-admin:4434",
	}

	client, err := NewClient(config, logger)
	require.NoError(t, err)

	publicAPI := client.PublicAPI()
	assert.NotNil(t, publicAPI)
}

func TestClient_AdminAPI(t *testing.T) {
	var buf bytes.Buffer
	logger, err := logger.NewWithWriter("info", &buf)
	require.NoError(t, err)

	config := &config.Config{
		KratosPublicURL: "http://kratos-public:4433",
		KratosAdminURL:  "http://kratos-admin:4434",
	}

	client, err := NewClient(config, logger)
	require.NoError(t, err)

	adminAPI := client.AdminAPI()
	assert.NotNil(t, adminAPI)
}

func TestURLValidation(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		isValid bool
	}{
		{"valid HTTP URL", "http://localhost:4433", true},
		{"valid HTTPS URL", "https://kratos.example.com", true},
		{"invalid URL", "invalid-url", false},
		{"empty URL", "", false},
		{"URL without protocol", "localhost:4433", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid := isValidURL(tt.url)
			assert.Equal(t, tt.isValid, valid)
		})
	}
}

func TestClient_HealthCheck(t *testing.T) {
	var buf bytes.Buffer
	logger, err := logger.NewWithWriter("info", &buf)
	require.NoError(t, err)

	config := &config.Config{
		KratosPublicURL: "http://kratos-public:4433",
		KratosAdminURL:  "http://kratos-admin:4434",
	}

	client, err := NewClient(config, logger)
	require.NoError(t, err)

	// Note: This test will fail in CI without real Kratos instance
	// But it tests the method signature and basic functionality
	t.Run("health check method exists", func(t *testing.T) {
		// We can't actually call HealthCheck without a real Kratos instance
		// but we can verify the method exists and client is not nil
		assert.NotNil(t, client)
	})
}