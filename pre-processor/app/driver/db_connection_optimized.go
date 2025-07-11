package driver

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	logger "pre-processor/utils/logger"

	"github.com/jackc/pgx/v5/pgxpool"
)

// OptimizedPoolConfig holds optimized database connection pool configuration
type OptimizedPoolConfig struct {
	*pgxpool.Config
}

// PoolStats represents connection pool statistics
type PoolStats struct {
	TotalConns    int32 `json:"total_conns"`
	IdleConns     int32 `json:"idle_conns"`
	AcquiredConns int32 `json:"acquired_conns"`
	MaxConns      int32 `json:"max_conns"`
	MinConns      int32 `json:"min_conns"`
}

// ConnectionPoolManager manages optimized database connection pools
type ConnectionPoolManager struct {
	pool           *pgxpool.Pool
	config         *OptimizedPoolConfig
	metricsEnabled bool
	metricsStop    chan struct{}
	metricsMutex   sync.RWMutex
}

// NewOptimizedPoolConfig creates an optimized database connection pool configuration
func NewOptimizedPoolConfig() *OptimizedPoolConfig {
	// Build optimized connection string
	connString := buildOptimizedConnectionString()

	// Parse the connection string to create pool config
	config, err := pgxpool.ParseConfig(connString)
	if err != nil {
		logger.Logger.Error("Failed to parse optimized database config", "error", err)
		// Fall back to basic config with minimal settings
		basicConnString := fmt.Sprintf(
			"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
			getEnvOrDefault("DB_HOST", "localhost"),
			getEnvOrDefault("DB_PORT", "5432"),
			getEnvOrDefault("PRE_PROCESSOR_DB_USER", "postgres"),
			getEnvOrDefault("PRE_PROCESSOR_DB_PASSWORD", "postgres"),
			getEnvOrDefault("DB_NAME", "pre_processor"),
		)

		config, err = pgxpool.ParseConfig(basicConnString)
		if err != nil {
			logger.Logger.Error("Failed to parse basic database config", "error", err)
			return nil
		}
	}

	// Apply optimized settings with environment variable overrides
	applyOptimizedSettings(config)

	return &OptimizedPoolConfig{Config: config}
}

// buildOptimizedConnectionString builds an optimized connection string
func buildOptimizedConnectionString() string {
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable "+
			"pool_max_conns=%s pool_min_conns=%s "+
			"pool_max_conn_lifetime=%s pool_max_conn_idle_time=%s "+
			"pool_health_check_period=%s",
		getEnvOrDefault("DB_HOST", "localhost"),
		getEnvOrDefault("DB_PORT", "5432"),
		getEnvOrDefault("PRE_PROCESSOR_DB_USER", "postgres"),
		getEnvOrDefault("PRE_PROCESSOR_DB_PASSWORD", "postgres"),
		getEnvOrDefault("DB_NAME", "pre_processor"),
		getEnvOrDefault("DB_POOL_MAX_CONNS", "30"),
		getEnvOrDefault("DB_POOL_MIN_CONNS", "10"),
		getEnvOrDefault("DB_POOL_MAX_CONN_LIFETIME", "2h"),
		getEnvOrDefault("DB_POOL_MAX_CONN_IDLE_TIME", "15m"),
		getEnvOrDefault("DB_POOL_HEALTH_CHECK_PERIOD", "45s"),
	)
}

// applyOptimizedSettings applies optimized configuration settings
func applyOptimizedSettings(config *pgxpool.Config) {
	// Connection pool optimization
	config.MaxConns = parseEnvInt32("DB_POOL_MAX_CONNS", 30)
	config.MinConns = parseEnvInt32("DB_POOL_MIN_CONNS", 10)

	// Ensure min_conns <= max_conns
	if config.MinConns > config.MaxConns {
		logger.Logger.Warn("MinConns greater than MaxConns, adjusting",
			"min_conns", config.MinConns,
			"max_conns", config.MaxConns)
		config.MinConns = config.MaxConns / 3
	}

	// Connection lifetime optimization
	config.MaxConnLifetime = parseEnvDuration("DB_POOL_MAX_CONN_LIFETIME", 2*time.Hour)
	config.MaxConnIdleTime = parseEnvDuration("DB_POOL_MAX_CONN_IDLE_TIME", 15*time.Minute)
	config.HealthCheckPeriod = parseEnvDuration("DB_POOL_HEALTH_CHECK_PERIOD", 45*time.Second)

	// Connection timeout optimization
	config.ConnConfig.ConnectTimeout = 30 * time.Second

	// Add query tracer for monitoring
	config.ConnConfig.Tracer = &QueryTracer{}

	logger.Logger.Info("Applied optimized database configuration",
		"max_conns", config.MaxConns,
		"min_conns", config.MinConns,
		"max_conn_lifetime", config.MaxConnLifetime,
		"max_conn_idle_time", config.MaxConnIdleTime,
		"health_check_period", config.HealthCheckPeriod,
		"connect_timeout", config.ConnConfig.ConnectTimeout)
}

// NewConnectionPoolManager creates a new connection pool manager
func NewConnectionPoolManager() *ConnectionPoolManager {
	return &ConnectionPoolManager{
		metricsStop: nil, // Will be created when metrics collection starts
	}
}

// InitOptimizedPool initializes an optimized database connection pool
func (m *ConnectionPoolManager) InitOptimizedPool(ctx context.Context) (*pgxpool.Pool, error) {
	config := NewOptimizedPoolConfig()
	m.config = config

	// Create the pool with optimized configuration
	pool, err := pgxpool.NewWithConfig(ctx, config.Config)
	if err != nil {
		logger.Logger.Error("Failed to create optimized database pool", "error", err)
		return nil, fmt.Errorf("failed to create optimized pool: %w", err)
	}

	// Test the connection
	err = pool.Ping(ctx)
	if err != nil {
		logger.Logger.Error("Failed to ping optimized database", "error", err)
		pool.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	m.pool = pool

	logger.Logger.Info("Initialized optimized database connection pool",
		"max_conns", config.MaxConns,
		"min_conns", config.MinConns)

	return pool, nil
}

// GetPoolStats returns current pool statistics
func (m *ConnectionPoolManager) GetPoolStats() *PoolStats {
	if m.pool == nil {
		return &PoolStats{}
	}

	stat := m.pool.Stat()

	maxConns := int32(0)
	minConns := int32(0)
	if m.config != nil {
		maxConns = m.config.MaxConns
		minConns = m.config.MinConns
	}

	return &PoolStats{
		TotalConns:    stat.TotalConns(),
		IdleConns:     stat.IdleConns(),
		AcquiredConns: stat.AcquiredConns(),
		MaxConns:      maxConns,
		MinConns:      minConns,
	}
}

// StartMetricsCollection starts collecting pool metrics
func (m *ConnectionPoolManager) StartMetricsCollection(ctx context.Context) error {
	m.metricsMutex.Lock()
	defer m.metricsMutex.Unlock()

	if m.metricsEnabled {
		return nil // Already started
	}

	m.metricsEnabled = true
	// Create a new stop channel for this metrics collection session
	m.metricsStop = make(chan struct{})
	stopChan := m.metricsStop // Copy the channel reference

	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-stopChan: // Use the local copy to avoid race condition
				return
			case <-ticker.C:
				// Check if metrics are still enabled before logging
				m.metricsMutex.RLock()
				enabled := m.metricsEnabled
				m.metricsMutex.RUnlock()

				if enabled && m.pool != nil {
					stats := m.GetPoolStats()
					logger.Logger.Info("Connection pool metrics",
						"total_conns", stats.TotalConns,
						"idle_conns", stats.IdleConns,
						"acquired_conns", stats.AcquiredConns,
						"max_conns", stats.MaxConns)
				}
			}
		}
	}()

	logger.Logger.Info("Started connection pool metrics collection")
	return nil
}

// StopMetricsCollection stops collecting pool metrics
func (m *ConnectionPoolManager) StopMetricsCollection() {
	m.metricsMutex.Lock()
	defer m.metricsMutex.Unlock()

	if !m.metricsEnabled {
		return
	}

	m.metricsEnabled = false
	// Close the current stop channel to signal goroutine to stop
	if m.metricsStop != nil {
		close(m.metricsStop)
		m.metricsStop = nil
	}

	logger.Logger.Info("Stopped connection pool metrics collection")
}

// Utility functions

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func parseEnvInt32(key string, defaultValue int32) int32 {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.ParseInt(value, 10, 32); err == nil && parsed > 0 {
			return int32(parsed)
		}
		logger.Logger.Warn("Invalid environment variable, using default",
			"key", key, "value", value, "default", defaultValue)
	}
	return defaultValue
}

func parseEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if parsed, err := time.ParseDuration(value); err == nil && parsed > 0 {
			return parsed
		}
		logger.Logger.Warn("Invalid duration environment variable, using default",
			"key", key, "value", value, "default", defaultValue)
	}
	return defaultValue
}
