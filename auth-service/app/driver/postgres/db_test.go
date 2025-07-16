package postgres

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"auth-service/app/config"
	"auth-service/app/utils/logger"
)

func TestNewConnection(t *testing.T) {
	tests := []struct {
		name      string
		config    *config.Config
		wantError bool
		skipTest  bool // Skip if database is not available
	}{
		{
			name: "valid connection config",
			config: &config.Config{
				DatabaseHost:     "localhost",
				DatabasePort:     "5432",
				DatabaseName:     "test_auth_db",
				DatabaseUser:     "test_user",
				DatabasePassword: "test_password",
				DatabaseSSLMode:  "disable",
			},
			wantError: false,
			skipTest:  true, // Skip by default as we don't have test DB in CI
		},
		{
			name: "invalid host",
			config: &config.Config{
				DatabaseHost:     "invalid-host",
				DatabasePort:     "5432",
				DatabaseName:     "test_auth_db",
				DatabaseUser:     "test_user",
				DatabasePassword: "test_password",
				DatabaseSSLMode:  "disable",
			},
			wantError: true,
			skipTest:  true, // Skip as this would take time to timeout
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skipTest {
				t.Skip("Skipping database integration test (requires real database)")
			}

			var buf bytes.Buffer
			logger, err := logger.NewWithWriter("info", &buf)
			require.NoError(t, err)

			db, err := NewConnection(tt.config, logger)

			if tt.wantError {
				assert.Error(t, err)
				assert.Nil(t, db)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, db)
				assert.NotNil(t, db.Pool())

				// Clean up
				if db != nil {
					db.Close()
				}
			}
		})
	}
}

func TestDB_Pool(t *testing.T) {
	// This test verifies the Pool() method returns the expected pool
	// We'll use a mock approach since we don't have a real database in tests

	db := &DB{
		pool: nil, // In real scenario this would be initialized
	}

	pool := db.Pool()
	// Pool should return the internal pool (even if nil in this test)
	assert.Equal(t, db.pool, pool)
}

func TestDB_Close(t *testing.T) {
	// Test that Close method doesn't panic when called
	// This is a basic test since we can't easily test the actual closing without a real connection

	var buf bytes.Buffer
	logger, err := logger.NewWithWriter("info", &buf)
	require.NoError(t, err)

	db := &DB{
		logger: logger,
		pool:   nil, // In real scenario this would be initialized
	}

	// Should not panic even with nil pool
	assert.NotPanics(t, func() {
		db.Close()
	})
}

// TestConnectionString tests the DSN construction logic
func TestConnectionString(t *testing.T) {
	cfg := &config.Config{
		DatabaseHost:     "localhost",
		DatabasePort:     "5432",
		DatabaseName:     "auth_db",
		DatabaseUser:     "auth_user",
		DatabasePassword: "password123",
		DatabaseSSLMode:  "require",
	}

	expected := "postgres://auth_user:password123@localhost:5432/auth_db?sslmode=require"

	// Test DSN construction using the exported function
	dsn := buildDSN(cfg)
	assert.Equal(t, expected, dsn)
}

// TestPoolConfiguration tests that pool configuration is set correctly
func TestPoolConfiguration(t *testing.T) {
	// Test pool configuration constants
	assert.Equal(t, int32(25), maxConns)
	assert.Equal(t, int32(5), minConns)
	assert.Equal(t, time.Hour, maxConnLifetime)
	assert.Equal(t, 30*time.Minute, maxConnIdleTime)
}

func TestDB_HealthCheck(t *testing.T) {
	var buf bytes.Buffer
	logger, err := logger.NewWithWriter("info", &buf)
	require.NoError(t, err)

	t.Run("health check with nil pool", func(t *testing.T) {
		db := &DB{
			logger: logger,
			pool:   nil,
		}

		ctx := context.Background()
		err := db.HealthCheck(ctx)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "database connection is not initialized")
	})
}
