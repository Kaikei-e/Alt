package driver

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestOptimizedConnectionPool(t *testing.T) {
	tests := []struct {
		name string
		test func(t *testing.T)
	}{
		{
			name: "should create optimized pool config with environment variables",
			test: func(t *testing.T) {
				// Set test environment variables
				originalVars := setTestEnvVars()
				defer restoreEnvVars(originalVars)

				config := NewOptimizedPoolConfig()
				assert.NotNil(t, config)

				// Verify optimized settings
				assert.Equal(t, int32(30), config.MaxConns, "Max connections should be optimized")
				assert.Equal(t, int32(10), config.MinConns, "Min connections should be optimized")
				assert.Equal(t, 2*time.Hour, config.MaxConnLifetime, "Connection lifetime should be extended")
				assert.Equal(t, 15*time.Minute, config.MaxConnIdleTime, "Idle time should be optimized")
				assert.Equal(t, 45*time.Second, config.HealthCheckPeriod, "Health check period should be set")
			},
		},
		{
			name: "should use environment variable overrides for pool settings",
			test: func(t *testing.T) {
				// Set custom environment variables
				originalVars := setTestEnvVars()
				defer restoreEnvVars(originalVars)

				// Override specific settings
				os.Setenv("DB_POOL_MAX_CONNS", "50")
				os.Setenv("DB_POOL_MIN_CONNS", "15")
				os.Setenv("DB_POOL_MAX_CONN_LIFETIME", "3h")
				os.Setenv("DB_POOL_MAX_CONN_IDLE_TIME", "20m")

				config := NewOptimizedPoolConfig()
				assert.NotNil(t, config)

				// Verify environment overrides are applied
				assert.Equal(t, int32(50), config.MaxConns)
				assert.Equal(t, int32(15), config.MinConns)
				assert.Equal(t, 3*time.Hour, config.MaxConnLifetime)
				assert.Equal(t, 20*time.Minute, config.MaxConnIdleTime)
			},
		},
		{
			name: "should use default values for invalid environment variables",
			test: func(t *testing.T) {
				originalVars := setTestEnvVars()
				defer restoreEnvVars(originalVars)

				// Set invalid values
				os.Setenv("DB_POOL_MAX_CONNS", "invalid")
				os.Setenv("DB_POOL_MIN_CONNS", "not_a_number")
				os.Setenv("DB_POOL_MAX_CONN_LIFETIME", "invalid_duration")

				config := NewOptimizedPoolConfig()
				assert.NotNil(t, config)

				// Should fall back to optimized defaults
				assert.Equal(t, int32(30), config.MaxConns)
				assert.Equal(t, int32(10), config.MinConns)
				assert.Equal(t, 2*time.Hour, config.MaxConnLifetime)
			},
		},
		{
			name: "should validate connection pool constraints",
			test: func(t *testing.T) {
				originalVars := setTestEnvVars()
				defer restoreEnvVars(originalVars)

				// Set min_conns > max_conns (invalid)
				os.Setenv("DB_POOL_MAX_CONNS", "5")
				os.Setenv("DB_POOL_MIN_CONNS", "10")

				config := NewOptimizedPoolConfig()
				assert.NotNil(t, config)

				// Should correct the invalid configuration
				assert.LessOrEqual(t, config.MinConns, config.MaxConns)
			},
		},
		{
			name: "should create optimized connection with proper settings",
			test: func(t *testing.T) {
				if testing.Short() {
					t.Skip("skipping connection test")
				}

				originalVars := setTestEnvVars()
				defer restoreEnvVars(originalVars)

				config := NewOptimizedPoolConfig()

				// We can't actually connect without a real database
				// But we can verify the configuration is properly structured
				assert.NotNil(t, config)
				assert.Greater(t, config.MaxConns, int32(0))
				assert.Greater(t, config.MinConns, int32(0))
				assert.Greater(t, config.MaxConnLifetime, time.Duration(0))
				assert.Greater(t, config.MaxConnIdleTime, time.Duration(0))
				assert.Greater(t, config.HealthCheckPeriod, time.Duration(0))

				// Verify connection string is properly built
				connString := buildOptimizedConnectionString()
				assert.Contains(t, connString, "host=localhost")
				assert.Contains(t, connString, "dbname=test_db")
				assert.Contains(t, connString, "sslmode=prefer")
			},
		},
		{
			name: "should handle connection timeout and retry settings",
			test: func(t *testing.T) {
				originalVars := setTestEnvVars()
				defer restoreEnvVars(originalVars)

				config := NewOptimizedPoolConfig()

				// Verify timeout settings are properly configured
				assert.Equal(t, 30*time.Second, config.ConnConfig.ConnectTimeout)
				assert.NotNil(t, config.ConnConfig.Config)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.test)
	}
}

func TestConnectionPoolMetrics(t *testing.T) {
	t.Run("should provide pool statistics", func(t *testing.T) {
		manager := NewConnectionPoolManager()
		assert.NotNil(t, manager)

		stats := manager.GetPoolStats()
		assert.NotNil(t, stats)

		// Stats should have the expected structure
		assert.GreaterOrEqual(t, stats.TotalConns, int32(0))
		assert.GreaterOrEqual(t, stats.IdleConns, int32(0))
		assert.GreaterOrEqual(t, stats.AcquiredConns, int32(0))
	})

	t.Run("should track connection metrics over time", func(t *testing.T) {
		manager := NewConnectionPoolManager()

		// Track metrics
		err := manager.StartMetricsCollection(context.Background())
		assert.NoError(t, err)

		// Stop metrics collection
		manager.StopMetricsCollection()
	})
}

// Helper functions for testing

func setTestEnvVars() map[string]string {
	originalVars := make(map[string]string)

	envVars := map[string]string{
		"DB_HOST":                   "localhost",
		"DB_PORT":                   "5432",
		"PRE_PROCESSOR_DB_USER":     "test_user",
		"PRE_PROCESSOR_DB_PASSWORD": "test_password",
		"DB_NAME":                   "test_db",
	}

	for key, value := range envVars {
		originalVars[key] = os.Getenv(key)
		os.Setenv(key, value)
	}

	return originalVars
}

func restoreEnvVars(originalVars map[string]string) {
	for key, value := range originalVars {
		if value == "" {
			os.Unsetenv(key)
		} else {
			os.Setenv(key, value)
		}
	}
}
