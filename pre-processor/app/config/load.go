package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// LoadConfig builds the configuration from defaults and overrides provided via environment variables.
func LoadConfig() (*Config, error) {
	config := defaultConfig()

	if err := loadFromEnv(config); err != nil {
		return nil, fmt.Errorf("failed to load from environment: %w", err)
	}

	if err := validateConfig(config); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return config, nil
}

func loadFromEnv(config *Config) error {
	*config = *defaultConfig()

	// Load each configuration section
	if err := loadServerConfig(&config.Server); err != nil {
		return fmt.Errorf("failed to load server config: %w", err)
	}

	if err := loadHTTPConfig(&config.HTTP); err != nil {
		return fmt.Errorf("failed to load HTTP config: %w", err)
	}

	if err := loadRetryConfig(&config.Retry); err != nil {
		return fmt.Errorf("failed to load retry config: %w", err)
	}

	if err := loadRateLimitConfig(&config.RateLimit); err != nil {
		return fmt.Errorf("failed to load rate limit config: %w", err)
	}

	if err := loadDLQConfig(&config.DLQ); err != nil {
		return fmt.Errorf("failed to load DLQ config: %w", err)
	}

	if err := loadMetricsConfig(&config.Metrics); err != nil {
		return fmt.Errorf("failed to load metrics config: %w", err)
	}

	if err := loadNewsCreatorConfig(&config.NewsCreator); err != nil {
		return fmt.Errorf("failed to load news creator config: %w", err)
	}

	if err := loadAltServiceConfig(&config.AltService); err != nil {
		return fmt.Errorf("failed to load alt service config: %w", err)
	}

	if err := loadSummarizeQueueConfig(&config.SummarizeQueue); err != nil {
		return fmt.Errorf("failed to load summarize queue config: %w", err)
	}

	return nil
}

// loadServerConfig loads server configuration from environment variables
func loadServerConfig(cfg *ServerConfig) error {
	var err error

	if cfg.Port, err = parseIntEnv("SERVER_PORT", cfg.Port); err != nil {
		return err
	}

	if cfg.ShutdownTimeout, err = parseDurationEnv("SERVER_SHUTDOWN_TIMEOUT", cfg.ShutdownTimeout); err != nil {
		return err
	}

	if cfg.ReadTimeout, err = parseDurationEnv("SERVER_READ_TIMEOUT", cfg.ReadTimeout); err != nil {
		return err
	}

	if cfg.WriteTimeout, err = parseDurationEnv("SERVER_WRITE_TIMEOUT", cfg.WriteTimeout); err != nil {
		return err
	}

	return nil
}

// loadHTTPConfig loads HTTP configuration from environment variables
func loadHTTPConfig(cfg *HTTPConfig) error {
	var err error

	if cfg.Timeout, err = parseDurationEnv("HTTP_TIMEOUT", cfg.Timeout); err != nil {
		return err
	}

	if cfg.MaxIdleConns, err = parseIntEnv("HTTP_MAX_IDLE_CONNS", cfg.MaxIdleConns); err != nil {
		return err
	}

	if cfg.MaxIdleConnsPerHost, err = parseIntEnv("HTTP_MAX_IDLE_CONNS_PER_HOST", cfg.MaxIdleConnsPerHost); err != nil {
		return err
	}

	if cfg.IdleConnTimeout, err = parseDurationEnv("HTTP_IDLE_CONN_TIMEOUT", cfg.IdleConnTimeout); err != nil {
		return err
	}

	if cfg.TLSHandshakeTimeout, err = parseDurationEnv("HTTP_TLS_HANDSHAKE_TIMEOUT", cfg.TLSHandshakeTimeout); err != nil {
		return err
	}

	if cfg.ExpectContinueTimeout, err = parseDurationEnv("HTTP_EXPECT_CONTINUE_TIMEOUT", cfg.ExpectContinueTimeout); err != nil {
		return err
	}

	if agent := os.Getenv("HTTP_USER_AGENT"); agent != "" {
		cfg.UserAgent = agent
	}

	if cfg.UserAgentRotation, err = parseBoolEnv("HTTP_USER_AGENT_ROTATION", cfg.UserAgentRotation); err != nil {
		return err
	}

	if agents := os.Getenv("HTTP_USER_AGENTS"); agents != "" {
		cfg.UserAgents = splitUserAgents(agents)
	}

	if cfg.EnableBrowserHeaders, err = parseBoolEnv("HTTP_ENABLE_BROWSER_HEADERS", cfg.EnableBrowserHeaders); err != nil {
		return err
	}

	if cfg.SkipErrorResponses, err = parseBoolEnv("HTTP_SKIP_ERROR_RESPONSES", cfg.SkipErrorResponses); err != nil {
		return err
	}

	if cfg.MinContentLength, err = parseIntEnv("HTTP_MIN_CONTENT_LENGTH", cfg.MinContentLength); err != nil {
		return err
	}

	if cfg.MaxRedirects, err = parseIntEnv("HTTP_MAX_REDIRECTS", cfg.MaxRedirects); err != nil {
		return err
	}

	if cfg.FollowRedirects, err = parseBoolEnv("HTTP_FOLLOW_REDIRECTS", cfg.FollowRedirects); err != nil {
		return err
	}

	if cfg.UseEnvoyProxy, err = parseBoolEnv("USE_ENVOY_PROXY", cfg.UseEnvoyProxy); err != nil {
		return err
	}

	if proxy := os.Getenv("ENVOY_PROXY_URL"); proxy != "" {
		cfg.EnvoyProxyURL = proxy
	}

	if path := os.Getenv("ENVOY_PROXY_PATH"); path != "" {
		cfg.EnvoyProxyPath = path
	}

	if cfg.EnvoyTimeout, err = parseDurationEnv("ENVOY_TIMEOUT", cfg.EnvoyTimeout); err != nil {
		return err
	}

	return nil
}

// loadRetryConfig loads retry configuration from environment variables
func loadRetryConfig(cfg *RetryConfig) error {
	var err error

	if cfg.MaxAttempts, err = parseIntEnv("RETRY_MAX_ATTEMPTS", cfg.MaxAttempts); err != nil {
		return err
	}

	if cfg.BaseDelay, err = parseDurationEnv("RETRY_BASE_DELAY", cfg.BaseDelay); err != nil {
		return err
	}

	if cfg.MaxDelay, err = parseDurationEnv("RETRY_MAX_DELAY", cfg.MaxDelay); err != nil {
		return err
	}

	if cfg.BackoffFactor, err = parseFloatEnv("RETRY_BACKOFF_FACTOR", cfg.BackoffFactor); err != nil {
		return err
	}

	if cfg.JitterFactor, err = parseFloatEnv("RETRY_JITTER_FACTOR", cfg.JitterFactor); err != nil {
		return err
	}

	return nil
}

// loadRateLimitConfig loads rate limit configuration from environment variables
func loadRateLimitConfig(cfg *RateLimitConfig) error {
	var err error

	if cfg.DefaultInterval, err = parseDurationEnv("RATE_LIMIT_DEFAULT_INTERVAL", cfg.DefaultInterval); err != nil {
		return err
	}

	if cfg.BurstSize, err = parseIntEnv("RATE_LIMIT_BURST_SIZE", cfg.BurstSize); err != nil {
		return err
	}

	if cfg.EnableAdaptive, err = parseBoolEnv("RATE_LIMIT_ENABLE_ADAPTIVE", cfg.EnableAdaptive); err != nil {
		return err
	}

	return nil
}

// loadDLQConfig loads DLQ configuration from environment variables
func loadDLQConfig(cfg *DLQConfig) error {
	var err error

	if name := os.Getenv("DLQ_QUEUE_NAME"); name != "" {
		cfg.QueueName = name
	}

	if cfg.Timeout, err = parseDurationEnv("DLQ_TIMEOUT", cfg.Timeout); err != nil {
		return err
	}

	if cfg.RetryEnabled, err = parseBoolEnv("DLQ_RETRY_ENABLED", cfg.RetryEnabled); err != nil {
		return err
	}

	return nil
}

// loadMetricsConfig loads metrics configuration from environment variables
func loadMetricsConfig(cfg *MetricsConfig) error {
	var err error

	if cfg.Enabled, err = parseBoolEnv("METRICS_ENABLED", cfg.Enabled); err != nil {
		return err
	}

	if cfg.Port, err = parseIntEnv("METRICS_PORT", cfg.Port); err != nil {
		return err
	}

	if path := os.Getenv("METRICS_PATH"); path != "" {
		cfg.Path = path
	}

	if cfg.UpdateInterval, err = parseDurationEnv("METRICS_UPDATE_INTERVAL", cfg.UpdateInterval); err != nil {
		return err
	}

	if cfg.ReadHeaderTimeout, err = parseDurationEnv("METRICS_READ_HEADER_TIMEOUT", cfg.ReadHeaderTimeout); err != nil {
		return err
	}

	if cfg.ReadTimeout, err = parseDurationEnv("METRICS_READ_TIMEOUT", cfg.ReadTimeout); err != nil {
		return err
	}

	if cfg.WriteTimeout, err = parseDurationEnv("METRICS_WRITE_TIMEOUT", cfg.WriteTimeout); err != nil {
		return err
	}

	if cfg.IdleTimeout, err = parseDurationEnv("METRICS_IDLE_TIMEOUT", cfg.IdleTimeout); err != nil {
		return err
	}

	return nil
}

// loadNewsCreatorConfig loads news creator configuration from environment variables
func loadNewsCreatorConfig(cfg *NewsCreatorConfig) error {
	var err error

	if host := os.Getenv("NEWS_CREATOR_HOST"); host != "" {
		cfg.Host = host
	}

	if apiPath := os.Getenv("NEWS_CREATOR_API_PATH"); apiPath != "" {
		cfg.APIPath = apiPath
	}

	if model := os.Getenv("NEWS_CREATOR_MODEL"); model != "" {
		cfg.Model = model
	}

	if cfg.Timeout, err = parseDurationEnv("NEWS_CREATOR_TIMEOUT", cfg.Timeout); err != nil {
		return err
	}

	return nil
}

// loadAltServiceConfig loads alt service configuration from environment variables
func loadAltServiceConfig(cfg *AltServiceConfig) error {
	var err error

	if host := os.Getenv("ALT_BACKEND_HOST"); host != "" {
		cfg.Host = host
	}

	if cfg.Timeout, err = parseDurationEnv("ALT_BACKEND_TIMEOUT", cfg.Timeout); err != nil {
		return err
	}

	return nil
}

// loadSummarizeQueueConfig loads summarize queue configuration from environment variables
func loadSummarizeQueueConfig(cfg *SummarizeQueueConfig) error {
	var err error

	if cfg.WorkerInterval, err = parseDurationEnv("SUMMARIZE_QUEUE_WORKER_INTERVAL", cfg.WorkerInterval); err != nil {
		return err
	}

	if cfg.MaxRetries, err = parseIntEnv("SUMMARIZE_QUEUE_MAX_RETRIES", cfg.MaxRetries); err != nil {
		return err
	}

	if cfg.PollingInterval, err = parseDurationEnv("SUMMARIZE_QUEUE_POLLING_INTERVAL", cfg.PollingInterval); err != nil {
		return err
	}

	return nil
}

func splitUserAgents(value string) []string {
	parts := strings.Split(value, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

func parseDurationEnv(key string, defaultValue time.Duration) (time.Duration, error) {
	if value := os.Getenv(key); value != "" {
		d, err := time.ParseDuration(value)
		if err != nil {
			return 0, fmt.Errorf("invalid %s: %s", key, value)
		}
		return d, nil
	}
	return defaultValue, nil
}

func parseIntEnv(key string, defaultValue int) (int, error) {
	if value := os.Getenv(key); value != "" {
		i, err := strconv.Atoi(value)
		if err != nil {
			return 0, fmt.Errorf("invalid %s: %s", key, value)
		}
		return i, nil
	}
	return defaultValue, nil
}

func parseBoolEnv(key string, defaultValue bool) (bool, error) {
	if value := os.Getenv(key); value != "" {
		b, err := strconv.ParseBool(value)
		if err != nil {
			return false, fmt.Errorf("invalid %s: %s", key, value)
		}
		return b, nil
	}
	return defaultValue, nil
}

func parseFloatEnv(key string, defaultValue float64) (float64, error) {
	if value := os.Getenv(key); value != "" {
		f, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid %s: %s", key, value)
		}
		return f, nil
	}
	return defaultValue, nil
}
