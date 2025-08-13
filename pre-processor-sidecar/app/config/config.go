// ABOUTME: This file handles configuration management for pre-processor-sidecar
// ABOUTME: Loads environment variables and validates configuration for Inoreader API integration

package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds all configuration for the pre-processor-sidecar service
type Config struct {
	// Service configuration
	ServiceName    string
	ServiceVersion string
	Environment    string
	LogLevel       string

	// HTTP Server configuration
	HTTPPort    int
	ReadTimeout time.Duration

	// Database configuration
	Database DatabaseConfig

	// Inoreader API configuration
	Inoreader InoreaderConfig

	// Proxy configuration
	Proxy ProxyConfig

	// Rate limiting configuration
	RateLimit RateLimitConfig

	// OAuth2 configuration
	OAuth2 OAuth2Config
	
	// OAuth2 Secret configuration (for auth-token-manager integration)
	OAuth2SecretName string

	// Token storage configuration
	TokenStoragePath string

	// TDD Phase 3 - REFACTOR: Enhanced Configuration Management
	// HTTP Client configuration
	HTTPClient HTTPClientConfig

	// Retry configuration
	Retry RetryConfig

	// Circuit Breaker configuration
	CircuitBreaker CircuitBreakerConfig

	// Monitoring configuration
	Monitoring MonitoringConfig

	// Feature Flags
	EnableScheduleMode bool
	EnableDebugMode    bool
	EnableHealthCheck  bool
	
	// Phase 5: Rotation processing configuration
	Rotation RotationConfig
	
	// Phase 5: Content processing configuration 
	Content ContentConfig
}

// DatabaseConfig holds database connection settings
type DatabaseConfig struct {
	Host     string
	Port     string
	Name     string
	User     string
	Password string
	SSLMode  string
}

// InoreaderConfig holds Inoreader API settings
type InoreaderConfig struct {
	BaseURL               string
	ClientID              string
	ClientSecret          string
	RefreshToken          string
	MaxArticlesPerRequest int
	TokenRefreshBuffer    time.Duration
}

// ProxyConfig holds proxy settings for Envoy integration
type ProxyConfig struct {
	HTTPSProxy string
	NoProxy    string
}

// RateLimitConfig holds rate limiting settings
type RateLimitConfig struct {
	DailyLimit   int
	SyncInterval time.Duration
}

// OAuth2Config holds OAuth2 token management settings
type OAuth2Config struct {
	ClientID      string
	ClientSecret  string
	RefreshToken  string
	RefreshBuffer time.Duration
}

// TDD Phase 3 - REFACTOR: Enhanced Configuration Structures
// HTTPClientConfig holds HTTP client configuration
type HTTPClientConfig struct {
	Timeout               time.Duration
	TLSHandshakeTimeout   time.Duration
	ResponseHeaderTimeout time.Duration
	IdleConnTimeout       time.Duration
	MaxIdleConns          int
	MaxIdleConnsPerHost   int
}

// RetryConfig holds retry mechanism configuration
type RetryConfig struct {
	MaxRetries   int
	InitialDelay time.Duration
	MaxDelay     time.Duration
	Multiplier   float64
}

// CircuitBreakerConfig holds circuit breaker configuration
type CircuitBreakerConfig struct {
	FailureThreshold int
	SuccessThreshold int
	Timeout          time.Duration
	MaxRequests      int
}

// MonitoringConfig holds monitoring system configuration
type MonitoringConfig struct {
	EnableMetrics     bool
	EnableTracing     bool
	MetricsBatchSize  int
	FlushInterval     time.Duration
	RetentionDuration time.Duration
}

// Phase 5: RotationConfig holds rotation processing configuration
type RotationConfig struct {
	Enabled                bool          // Enable rotation mode
	IntervalMinutes        int           // Processing interval in minutes (default: 20)
	MaxSubscriptionsPerDay int           // Maximum subscriptions to process daily (default: 40)
	APIBudget              int           // API requests budget for rotation (default: 40)
	ShuffleDailyOrder      bool          // Shuffle subscription order daily (default: true)
	RetryFailedSubscriptions bool        // Retry failed subscriptions (default: true)
}

// Phase 5: ContentConfig holds article content processing configuration
type ContentConfig struct {
	ExtractionEnabled    bool          // Enable content extraction from summary.content
	MaxContentLength     int           // Maximum content length in bytes (default: 50KB)
	ContentTypeDetection bool          // Enable content type detection (HTML/RTL)
	TruncationEnabled    bool          // Enable content truncation for large articles
	CompressionEnabled   bool          // Enable content compression (future feature)
}

// LoadConfig loads configuration from environment variables
func LoadConfig() (*Config, error) {
	cfg := &Config{
		ServiceName:    getEnvOrDefault("SERVICE_NAME", "pre-processor-sidecar"),
		ServiceVersion: getEnvOrDefault("SERVICE_VERSION", "1.0.0"),
		Environment:    getEnvOrDefault("ENVIRONMENT", "development"),
		LogLevel:       getEnvOrDefault("LOG_LEVEL", "info"),

		// TDD Phase 3 - REFACTOR: HTTP Server Configuration
		HTTPPort:    getEnvOrDefaultInt("HTTP_PORT", 8080),
		ReadTimeout: getEnvOrDefaultDuration("READ_TIMEOUT", 30*time.Second),

		Database: DatabaseConfig{
			Host:     getEnvOrDefault("DB_HOST", "postgres.alt-database.svc.cluster.local"),
			Port:     getEnvOrDefault("DB_PORT", "5432"),
			Name:     getEnvOrDefault("DB_NAME", "alt"),
			User:     getEnvOrDefault("PRE_PROCESSOR_SIDECAR_DB_USER", "pre_processor_sidecar_user"), // FIXED: Correct default user
			Password: os.Getenv("PRE_PROCESSOR_SIDECAR_DB_PASSWORD"),                                 // Required from secret
			SSLMode:  getEnvOrDefault("DB_SSL_MODE", "disable"),                                      // FIXED: Default to disable for Linkerd mTLS
		},

		Inoreader: InoreaderConfig{
			BaseURL:      getEnvOrDefault("INOREADER_BASE_URL", "https://www.inoreader.com/reader/api/0"),
			ClientID:     os.Getenv("INOREADER_CLIENT_ID"),     // Required from secret
			ClientSecret: os.Getenv("INOREADER_CLIENT_SECRET"), // Required from secret
			RefreshToken: os.Getenv("INOREADER_REFRESH_TOKEN"), // Required from secret
		},

		Proxy: ProxyConfig{
			HTTPSProxy: getEnvOrDefault("HTTPS_PROXY", "http://envoy-proxy.alt-apps.svc.cluster.local:8081"),
			NoProxy:    getEnvOrDefault("NO_PROXY", "localhost,127.0.0.1,.svc.cluster.local"),
		},

		RateLimit: RateLimitConfig{
			DailyLimit: 100, // Zone 1 limit
		},

		OAuth2: OAuth2Config{
			ClientID:     os.Getenv("INOREADER_CLIENT_ID"),     // Required from secret
			ClientSecret: os.Getenv("INOREADER_CLIENT_SECRET"), // Required from secret
			RefreshToken: os.Getenv("INOREADER_REFRESH_TOKEN"), // Optional - managed by auth-token-manager
		},

		OAuth2SecretName: getEnvOrDefault("OAUTH2_TOKEN_SECRET_NAME", "pre-processor-sidecar-oauth2-token"),
		TokenStoragePath: getEnvOrDefault("TOKEN_STORAGE_PATH", "/tmp/oauth2_token.env"),

		// TDD Phase 3 - REFACTOR: Enhanced Configuration Management
		HTTPClient: HTTPClientConfig{
			Timeout:               getEnvOrDefaultDuration("HTTP_CLIENT_TIMEOUT", 60*time.Second),
			TLSHandshakeTimeout:   getEnvOrDefaultDuration("HTTP_CLIENT_TLS_HANDSHAKE_TIMEOUT", 10*time.Second),
			ResponseHeaderTimeout: getEnvOrDefaultDuration("HTTP_CLIENT_RESPONSE_HEADER_TIMEOUT", 30*time.Second),
			IdleConnTimeout:       getEnvOrDefaultDuration("HTTP_CLIENT_IDLE_CONN_TIMEOUT", 90*time.Second),
			MaxIdleConns:          getEnvOrDefaultInt("HTTP_CLIENT_MAX_IDLE_CONNS", 10),
			MaxIdleConnsPerHost:   getEnvOrDefaultInt("HTTP_CLIENT_MAX_IDLE_CONNS_PER_HOST", 2),
		},

		Retry: RetryConfig{
			MaxRetries:   getEnvOrDefaultInt("RETRY_MAX_RETRIES", 3),
			InitialDelay: getEnvOrDefaultDuration("RETRY_INITIAL_DELAY", 5*time.Second),
			MaxDelay:     getEnvOrDefaultDuration("RETRY_MAX_DELAY", 30*time.Second),
			Multiplier:   getEnvOrDefaultFloat("RETRY_MULTIPLIER", 2.0),
		},

		CircuitBreaker: CircuitBreakerConfig{
			FailureThreshold: getEnvOrDefaultInt("CIRCUIT_BREAKER_FAILURE_THRESHOLD", 3),
			SuccessThreshold: getEnvOrDefaultInt("CIRCUIT_BREAKER_SUCCESS_THRESHOLD", 2),
			Timeout:          getEnvOrDefaultDuration("CIRCUIT_BREAKER_TIMEOUT", 60*time.Second),
			MaxRequests:      getEnvOrDefaultInt("CIRCUIT_BREAKER_MAX_REQUESTS", 1),
		},

		Monitoring: MonitoringConfig{
			EnableMetrics:     getEnvOrDefaultBool("MONITORING_ENABLE_METRICS", true),
			EnableTracing:     getEnvOrDefaultBool("MONITORING_ENABLE_TRACING", true),
			MetricsBatchSize:  getEnvOrDefaultInt("MONITORING_METRICS_BATCH_SIZE", 100),
			FlushInterval:     getEnvOrDefaultDuration("MONITORING_FLUSH_INTERVAL", 30*time.Second),
			RetentionDuration: getEnvOrDefaultDuration("MONITORING_RETENTION_DURATION", 24*time.Hour),
		},

		// Feature Flags
		EnableScheduleMode: getEnvOrDefaultBool("ENABLE_SCHEDULE_MODE", false),
		EnableDebugMode:    getEnvOrDefaultBool("ENABLE_DEBUG_MODE", false),
		EnableHealthCheck:  getEnvOrDefaultBool("ENABLE_HEALTH_CHECK", true),
	}

	// Parse integer configurations
	if maxArticles := os.Getenv("MAX_ARTICLES_PER_REQUEST"); maxArticles != "" {
		if val, err := strconv.Atoi(maxArticles); err == nil {
			cfg.Inoreader.MaxArticlesPerRequest = val
		} else {
			cfg.Inoreader.MaxArticlesPerRequest = 100 // Default
		}
	} else {
		cfg.Inoreader.MaxArticlesPerRequest = 100
	}

	// Parse duration configurations
	if syncInterval := os.Getenv("SYNC_INTERVAL"); syncInterval != "" {
		if duration, err := time.ParseDuration(syncInterval); err == nil {
			cfg.RateLimit.SyncInterval = duration
		} else {
			cfg.RateLimit.SyncInterval = 30 * time.Minute // Default
		}
	} else {
		cfg.RateLimit.SyncInterval = 30 * time.Minute
	}

	// Parse token refresh buffer for both Inoreader and OAuth2
	if buffer := os.Getenv("OAUTH2_TOKEN_REFRESH_BUFFER"); buffer != "" {
		if bufferSeconds, err := strconv.Atoi(buffer); err == nil {
			bufferDuration := time.Duration(bufferSeconds) * time.Second
			cfg.Inoreader.TokenRefreshBuffer = bufferDuration
			cfg.OAuth2.RefreshBuffer = bufferDuration
		} else {
			cfg.Inoreader.TokenRefreshBuffer = 5 * time.Minute // Default
			cfg.OAuth2.RefreshBuffer = 5 * time.Minute         // Default
		}
	} else {
		cfg.Inoreader.TokenRefreshBuffer = 5 * time.Minute
		cfg.OAuth2.RefreshBuffer = 5 * time.Minute
	}

	// Phase 5: Load rotation processing configuration
	cfg.Rotation = RotationConfig{
		Enabled:                getEnvOrDefaultBool("ROTATION_ENABLED", false),
		IntervalMinutes:        getEnvOrDefaultInt("ROTATION_INTERVAL_MINUTES", 20),
		MaxSubscriptionsPerDay: getEnvOrDefaultInt("SUBSCRIPTIONS_PER_DAY", 40),
		APIBudget:              getEnvOrDefaultInt("ROTATION_API_BUDGET", 40),
		ShuffleDailyOrder:      getEnvOrDefaultBool("ROTATION_SHUFFLE_DAILY", true),
		RetryFailedSubscriptions: getEnvOrDefaultBool("RETRY_FAILED_SUBSCRIPTIONS", true),
	}

	// Phase 5: Load content processing configuration
	cfg.Content = ContentConfig{
		ExtractionEnabled:    getEnvOrDefaultBool("CONTENT_EXTRACTION_ENABLED", false),
		MaxContentLength:     getEnvOrDefaultInt("MAX_CONTENT_LENGTH", 50000),
		ContentTypeDetection: getEnvOrDefaultBool("CONTENT_TYPE_DETECTION", true),
		TruncationEnabled:    getEnvOrDefaultBool("CONTENT_TRUNCATION_ENABLED", true),
		CompressionEnabled:   getEnvOrDefaultBool("CONTENT_COMPRESSION_ENABLED", false),
	}

	// Validate required configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return cfg, nil
}

// Validate validates the configuration
func (c *Config) Validate() error {
	// Validate required service configuration
	if c.ServiceName == "" {
		return fmt.Errorf("SERVICE_NAME is required")
	}
	if c.ServiceVersion == "" {
		return fmt.Errorf("SERVICE_VERSION is required")
	}

	// Validate database configuration
	if c.Database.Password == "" {
		return fmt.Errorf("PRE_PROCESSOR_SIDECAR_DB_PASSWORD is required")
	}

	// Validate Inoreader configuration
	if c.Inoreader.ClientID == "" {
		return fmt.Errorf("INOREADER_CLIENT_ID is required")
	}
	if c.Inoreader.ClientSecret == "" {
		return fmt.Errorf("INOREADER_CLIENT_SECRET is required")
	}
	// OAuth2 refresh token now optional - managed by auth-token-manager via OAuth2 Secret
	// if c.Inoreader.RefreshToken == "" {
	//     return fmt.Errorf("INOREADER_REFRESH_TOKEN is required")
	// }

	// Validate proxy configuration
	if c.Proxy.HTTPSProxy == "" {
		return fmt.Errorf("HTTPS_PROXY is required for Envoy integration")
	}

	// TDD Phase 3 - REFACTOR: Enhanced Validation
	// Validate HTTP configuration
	if c.HTTPPort <= 0 || c.HTTPPort > 65535 {
		return fmt.Errorf("HTTP_PORT must be between 1 and 65535")
	}
	if c.HTTPClient.Timeout <= 0 {
		return fmt.Errorf("HTTP_CLIENT_TIMEOUT must be positive")
	}

	// Validate Circuit Breaker configuration
	if c.CircuitBreaker.FailureThreshold <= 0 {
		return fmt.Errorf("CIRCUIT_BREAKER_FAILURE_THRESHOLD must be positive")
	}
	if c.CircuitBreaker.SuccessThreshold <= 0 {
		return fmt.Errorf("CIRCUIT_BREAKER_SUCCESS_THRESHOLD must be positive")
	}
	if c.CircuitBreaker.Timeout <= 0 {
		return fmt.Errorf("CIRCUIT_BREAKER_TIMEOUT must be positive")
	}

	// Validate Retry configuration
	if c.Retry.MaxRetries < 0 {
		return fmt.Errorf("RETRY_MAX_RETRIES must be non-negative")
	}
	if c.Retry.InitialDelay <= 0 {
		return fmt.Errorf("RETRY_INITIAL_DELAY must be positive")
	}
	if c.Retry.MaxDelay <= 0 {
		return fmt.Errorf("RETRY_MAX_DELAY must be positive")
	}
	if c.Retry.InitialDelay > c.Retry.MaxDelay {
		return fmt.Errorf("RETRY_INITIAL_DELAY must be less than or equal to RETRY_MAX_DELAY")
	}
	if c.Retry.Multiplier <= 1.0 {
		return fmt.Errorf("RETRY_MULTIPLIER must be greater than 1.0")
	}

	return nil
}

// getEnvOrDefault returns environment variable value or default if not set
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// TDD Phase 3 - REFACTOR: Enhanced Helper Functions
// getEnvOrDefaultInt returns environment variable as int or default if not set
func getEnvOrDefaultInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}

// getEnvOrDefaultDuration returns environment variable as duration or default if not set
func getEnvOrDefaultDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}

// getEnvOrDefaultBool returns environment variable as bool or default if not set
func getEnvOrDefaultBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolVal, err := strconv.ParseBool(value); err == nil {
			return boolVal
		}
	}
	return defaultValue
}

// getEnvOrDefaultFloat returns environment variable as float64 or default if not set
func getEnvOrDefaultFloat(key string, defaultValue float64) float64 {
	if value := os.Getenv(key); value != "" {
		if floatVal, err := strconv.ParseFloat(value, 64); err == nil {
			return floatVal
		}
	}
	return defaultValue
}

// IsProduction returns true if the environment is production
func (c *Config) IsProduction() bool {
	return c.Environment == "production"
}

// IsDevelopment returns true if the environment is development
func (c *Config) IsDevelopment() bool {
	return c.Environment == "development"
}

// GetDatabaseConnectionString returns the database connection string
func (c *Config) GetDatabaseConnectionString() string {
	return fmt.Sprintf(
		"host=%s port=%s dbname=%s user=%s password=%s sslmode=%s",
		c.Database.Host,
		c.Database.Port,
		c.Database.Name,
		c.Database.User,
		c.Database.Password,
		c.Database.SSLMode,
	)
}
