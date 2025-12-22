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

	var err error

	if config.Server.Port, err = parseIntEnv("SERVER_PORT", config.Server.Port); err != nil {
		return err
	}

	if config.Server.ShutdownTimeout, err = parseDurationEnv("SERVER_SHUTDOWN_TIMEOUT", config.Server.ShutdownTimeout); err != nil {
		return err
	}

	if config.Server.ReadTimeout, err = parseDurationEnv("SERVER_READ_TIMEOUT", config.Server.ReadTimeout); err != nil {
		return err
	}

	if config.Server.WriteTimeout, err = parseDurationEnv("SERVER_WRITE_TIMEOUT", config.Server.WriteTimeout); err != nil {
		return err
	}

	if config.HTTP.Timeout, err = parseDurationEnv("HTTP_TIMEOUT", config.HTTP.Timeout); err != nil {
		return err
	}

	if config.HTTP.MaxIdleConns, err = parseIntEnv("HTTP_MAX_IDLE_CONNS", config.HTTP.MaxIdleConns); err != nil {
		return err
	}

	if config.HTTP.MaxIdleConnsPerHost, err = parseIntEnv("HTTP_MAX_IDLE_CONNS_PER_HOST", config.HTTP.MaxIdleConnsPerHost); err != nil {
		return err
	}

	if config.HTTP.IdleConnTimeout, err = parseDurationEnv("HTTP_IDLE_CONN_TIMEOUT", config.HTTP.IdleConnTimeout); err != nil {
		return err
	}

	if config.HTTP.TLSHandshakeTimeout, err = parseDurationEnv("HTTP_TLS_HANDSHAKE_TIMEOUT", config.HTTP.TLSHandshakeTimeout); err != nil {
		return err
	}

	if config.HTTP.ExpectContinueTimeout, err = parseDurationEnv("HTTP_EXPECT_CONTINUE_TIMEOUT", config.HTTP.ExpectContinueTimeout); err != nil {
		return err
	}

	if agent := os.Getenv("HTTP_USER_AGENT"); agent != "" {
		config.HTTP.UserAgent = agent
	}

	if config.HTTP.UserAgentRotation, err = parseBoolEnv("HTTP_USER_AGENT_ROTATION", config.HTTP.UserAgentRotation); err != nil {
		return err
	}

	if agents := os.Getenv("HTTP_USER_AGENTS"); agents != "" {
		config.HTTP.UserAgents = splitUserAgents(agents)
	}

	if config.HTTP.EnableBrowserHeaders, err = parseBoolEnv("HTTP_ENABLE_BROWSER_HEADERS", config.HTTP.EnableBrowserHeaders); err != nil {
		return err
	}

	if config.HTTP.SkipErrorResponses, err = parseBoolEnv("HTTP_SKIP_ERROR_RESPONSES", config.HTTP.SkipErrorResponses); err != nil {
		return err
	}

	if config.HTTP.MinContentLength, err = parseIntEnv("HTTP_MIN_CONTENT_LENGTH", config.HTTP.MinContentLength); err != nil {
		return err
	}

	if config.HTTP.MaxRedirects, err = parseIntEnv("HTTP_MAX_REDIRECTS", config.HTTP.MaxRedirects); err != nil {
		return err
	}

	if config.HTTP.FollowRedirects, err = parseBoolEnv("HTTP_FOLLOW_REDIRECTS", config.HTTP.FollowRedirects); err != nil {
		return err
	}

	if config.HTTP.UseEnvoyProxy, err = parseBoolEnv("USE_ENVOY_PROXY", config.HTTP.UseEnvoyProxy); err != nil {
		return err
	}

	if proxy := os.Getenv("ENVOY_PROXY_URL"); proxy != "" {
		config.HTTP.EnvoyProxyURL = proxy
	}

	if path := os.Getenv("ENVOY_PROXY_PATH"); path != "" {
		config.HTTP.EnvoyProxyPath = path
	}

	if config.HTTP.EnvoyTimeout, err = parseDurationEnv("ENVOY_TIMEOUT", config.HTTP.EnvoyTimeout); err != nil {
		return err
	}

	if config.Retry.MaxAttempts, err = parseIntEnv("RETRY_MAX_ATTEMPTS", config.Retry.MaxAttempts); err != nil {
		return err
	}

	if config.Retry.BaseDelay, err = parseDurationEnv("RETRY_BASE_DELAY", config.Retry.BaseDelay); err != nil {
		return err
	}

	if config.Retry.MaxDelay, err = parseDurationEnv("RETRY_MAX_DELAY", config.Retry.MaxDelay); err != nil {
		return err
	}

	if config.Retry.BackoffFactor, err = parseFloatEnv("RETRY_BACKOFF_FACTOR", config.Retry.BackoffFactor); err != nil {
		return err
	}

	if config.Retry.JitterFactor, err = parseFloatEnv("RETRY_JITTER_FACTOR", config.Retry.JitterFactor); err != nil {
		return err
	}

	if config.RateLimit.DefaultInterval, err = parseDurationEnv("RATE_LIMIT_DEFAULT_INTERVAL", config.RateLimit.DefaultInterval); err != nil {
		return err
	}

	if config.RateLimit.BurstSize, err = parseIntEnv("RATE_LIMIT_BURST_SIZE", config.RateLimit.BurstSize); err != nil {
		return err
	}

	if config.RateLimit.EnableAdaptive, err = parseBoolEnv("RATE_LIMIT_ENABLE_ADAPTIVE", config.RateLimit.EnableAdaptive); err != nil {
		return err
	}

	if name := os.Getenv("DLQ_QUEUE_NAME"); name != "" {
		config.DLQ.QueueName = name
	}

	if config.DLQ.Timeout, err = parseDurationEnv("DLQ_TIMEOUT", config.DLQ.Timeout); err != nil {
		return err
	}

	if config.DLQ.RetryEnabled, err = parseBoolEnv("DLQ_RETRY_ENABLED", config.DLQ.RetryEnabled); err != nil {
		return err
	}

	if config.Metrics.Enabled, err = parseBoolEnv("METRICS_ENABLED", config.Metrics.Enabled); err != nil {
		return err
	}

	if config.Metrics.Port, err = parseIntEnv("METRICS_PORT", config.Metrics.Port); err != nil {
		return err
	}

	if path := os.Getenv("METRICS_PATH"); path != "" {
		config.Metrics.Path = path
	}

	if config.Metrics.UpdateInterval, err = parseDurationEnv("METRICS_UPDATE_INTERVAL", config.Metrics.UpdateInterval); err != nil {
		return err
	}

	if host := os.Getenv("NEWS_CREATOR_HOST"); host != "" {
		config.NewsCreator.Host = host
	}

	if apiPath := os.Getenv("NEWS_CREATOR_API_PATH"); apiPath != "" {
		config.NewsCreator.APIPath = apiPath
	}

	if host := os.Getenv("ALT_BACKEND_HOST"); host != "" {
		config.AltService.Host = host
	}

	if config.AltService.Timeout, err = parseDurationEnv("ALT_BACKEND_TIMEOUT", config.AltService.Timeout); err != nil {
		return err
	}

	if model := os.Getenv("NEWS_CREATOR_MODEL"); model != "" {
		config.NewsCreator.Model = model
	}

	if config.NewsCreator.Timeout, err = parseDurationEnv("NEWS_CREATOR_TIMEOUT", config.NewsCreator.Timeout); err != nil {
		return err
	}

	if config.SummarizeQueue.WorkerInterval, err = parseDurationEnv("SUMMARIZE_QUEUE_WORKER_INTERVAL", config.SummarizeQueue.WorkerInterval); err != nil {
		return err
	}

	if config.SummarizeQueue.MaxRetries, err = parseIntEnv("SUMMARIZE_QUEUE_MAX_RETRIES", config.SummarizeQueue.MaxRetries); err != nil {
		return err
	}

	if config.SummarizeQueue.PollingInterval, err = parseDurationEnv("SUMMARIZE_QUEUE_POLLING_INTERVAL", config.SummarizeQueue.PollingInterval); err != nil {
		return err
	}

	if config.Metrics.ReadHeaderTimeout, err = parseDurationEnv("METRICS_READ_HEADER_TIMEOUT", config.Metrics.ReadHeaderTimeout); err != nil {
		return err
	}

	if config.Metrics.ReadTimeout, err = parseDurationEnv("METRICS_READ_TIMEOUT", config.Metrics.ReadTimeout); err != nil {
		return err
	}

	if config.Metrics.WriteTimeout, err = parseDurationEnv("METRICS_WRITE_TIMEOUT", config.Metrics.WriteTimeout); err != nil {
		return err
	}

	if config.Metrics.IdleTimeout, err = parseDurationEnv("METRICS_IDLE_TIMEOUT", config.Metrics.IdleTimeout); err != nil {
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
