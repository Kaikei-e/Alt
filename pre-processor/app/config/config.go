// ABOUTME: This file implements configuration management with environment variable support
// ABOUTME: Provides validation, defaults, and dynamic configuration updates for production use
package config

import (
	"fmt"
	"log/slog"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Config struct {
	Server      ServerConfig      `json:"server"`
	HTTP        HTTPConfig        `json:"http"`
	Retry       RetryConfig       `json:"retry"`
	RateLimit   RateLimitConfig   `json:"rate_limit"`
	DLQ         DLQConfig         `json:"dlq"`
	Metrics     MetricsConfig     `json:"metrics"`
	NewsCreator NewsCreatorConfig `json:"news_creator"`
}

type ServerConfig struct {
	Port            int           `json:"port" env:"SERVER_PORT" default:"9200"`
	ShutdownTimeout time.Duration `json:"shutdown_timeout" env:"SERVER_SHUTDOWN_TIMEOUT" default:"30s"`
	ReadTimeout     time.Duration `json:"read_timeout" env:"SERVER_READ_TIMEOUT" default:"10s"`
	WriteTimeout    time.Duration `json:"write_timeout" env:"SERVER_WRITE_TIMEOUT" default:"300s"` // Extended to allow LLM processing
}

type HTTPConfig struct {
	Timeout               time.Duration `json:"timeout" env:"HTTP_TIMEOUT" default:"30s"`
	MaxIdleConns          int           `json:"max_idle_conns" env:"HTTP_MAX_IDLE_CONNS" default:"10"`
	MaxIdleConnsPerHost   int           `json:"max_idle_conns_per_host" env:"HTTP_MAX_IDLE_CONNS_PER_HOST" default:"2"`
	IdleConnTimeout       time.Duration `json:"idle_conn_timeout" env:"HTTP_IDLE_CONN_TIMEOUT" default:"90s"`
	TLSHandshakeTimeout   time.Duration `json:"tls_handshake_timeout" env:"HTTP_TLS_HANDSHAKE_TIMEOUT" default:"10s"`
	ExpectContinueTimeout time.Duration `json:"expect_continue_timeout" env:"HTTP_EXPECT_CONTINUE_TIMEOUT" default:"1s"`
	UserAgent             string        `json:"user_agent" env:"HTTP_USER_AGENT" default:"Mozilla/5.0 (compatible; AltBot/1.0; +https://alt.example.com/bot)"`
	// User-Agent rotation configuration
	UserAgentRotation bool     `json:"user_agent_rotation" env:"HTTP_USER_AGENT_ROTATION" default:"true"`
	UserAgents        []string `json:"user_agents" env:"HTTP_USER_AGENTS"`
	// Request headers configuration
	EnableBrowserHeaders bool `json:"enable_browser_headers" env:"HTTP_ENABLE_BROWSER_HEADERS" default:"true"`
	// HTTP error handling configuration
	SkipErrorResponses bool `json:"skip_error_responses" env:"HTTP_SKIP_ERROR_RESPONSES" default:"true"`
	MinContentLength   int  `json:"min_content_length" env:"HTTP_MIN_CONTENT_LENGTH" default:"500"`
	// Redirect handling configuration
	MaxRedirects    int  `json:"max_redirects" env:"HTTP_MAX_REDIRECTS" default:"5"`
	FollowRedirects bool `json:"follow_redirects" env:"HTTP_FOLLOW_REDIRECTS" default:"true"`
	// Envoy Proxy Configuration
	UseEnvoyProxy  bool          `json:"use_envoy_proxy" env:"USE_ENVOY_PROXY" default:"false"`
	EnvoyProxyURL  string        `json:"envoy_proxy_url" env:"ENVOY_PROXY_URL" default:"http://envoy-proxy.alt-apps.svc.cluster.local:8080"`
	EnvoyProxyPath string        `json:"envoy_proxy_path" env:"ENVOY_PROXY_PATH" default:"/proxy/https://"`
	EnvoyTimeout   time.Duration `json:"envoy_timeout" env:"ENVOY_TIMEOUT" default:"300s"`
}

type RetryConfig struct {
	MaxAttempts   int           `json:"max_attempts" env:"RETRY_MAX_ATTEMPTS" default:"3"`
	BaseDelay     time.Duration `json:"base_delay" env:"RETRY_BASE_DELAY" default:"1s"`
	MaxDelay      time.Duration `json:"max_delay" env:"RETRY_MAX_DELAY" default:"30s"`
	BackoffFactor float64       `json:"backoff_factor" env:"RETRY_BACKOFF_FACTOR" default:"2.0"`
	JitterFactor  float64       `json:"jitter_factor" env:"RETRY_JITTER_FACTOR" default:"0.1"`
}

type RateLimitConfig struct {
	DefaultInterval time.Duration            `json:"default_interval" env:"RATE_LIMIT_DEFAULT_INTERVAL" default:"5s"`
	DomainIntervals map[string]time.Duration `json:"domain_intervals" env:"RATE_LIMIT_DOMAIN_INTERVALS"`
	BurstSize       int                      `json:"burst_size" env:"RATE_LIMIT_BURST_SIZE" default:"1"`
	EnableAdaptive  bool                     `json:"enable_adaptive" env:"RATE_LIMIT_ENABLE_ADAPTIVE" default:"false"`
}

type DLQConfig struct {
	QueueName    string        `json:"queue_name" env:"DLQ_QUEUE_NAME" default:"failed-articles"`
	Timeout      time.Duration `json:"timeout" env:"DLQ_TIMEOUT" default:"10s"`
	RetryEnabled bool          `json:"retry_enabled" env:"DLQ_RETRY_ENABLED" default:"true"`
}

type MetricsConfig struct {
	Enabled           bool          `json:"enabled" env:"METRICS_ENABLED" default:"true"`
	Port              int           `json:"port" env:"METRICS_PORT" default:"9201"`
	Path              string        `json:"path" env:"METRICS_PATH" default:"/metrics"`
	UpdateInterval    time.Duration `json:"update_interval" env:"METRICS_UPDATE_INTERVAL" default:"10s"`
	ReadHeaderTimeout time.Duration `json:"read_header_timeout" env:"METRICS_READ_HEADER_TIMEOUT" default:"10s"`
	ReadTimeout       time.Duration `json:"read_timeout" env:"METRICS_READ_TIMEOUT" default:"30s"`
	WriteTimeout      time.Duration `json:"write_timeout" env:"METRICS_WRITE_TIMEOUT" default:"30s"`
	IdleTimeout       time.Duration `json:"idle_timeout" env:"METRICS_IDLE_TIMEOUT" default:"120s"`
}

type NewsCreatorConfig struct {
	Host    string        `json:"host" env:"NEWS_CREATOR_HOST" default:"http://news-creator:11434"`
	APIPath string        `json:"api_path" env:"NEWS_CREATOR_API_PATH" default:"/api/v1/summarize"`
	Model   string        `json:"model" env:"NEWS_CREATOR_MODEL" default:"gemma3:4b"`
	Timeout time.Duration `json:"timeout" env:"NEWS_CREATOR_TIMEOUT" default:"240s"` // Extended for LLM processing (16-19s typical, 240s for safety)
}

func LoadConfig() (*Config, error) {
	config := &Config{}

	// 環境変数から設定を読み込み
	if err := loadFromEnv(config); err != nil {
		return nil, fmt.Errorf("failed to load from environment: %w", err)
	}

	// 設定の検証
	if err := validateConfig(config); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return config, nil
}

func loadFromEnv(config *Config) error {
	// Server config
	if port := os.Getenv("SERVER_PORT"); port != "" {
		if p, err := strconv.Atoi(port); err == nil {
			config.Server.Port = p
		} else {
			return fmt.Errorf("invalid SERVER_PORT: %s", port)
		}
	} else {
		config.Server.Port = 9200
	}

	if timeout := os.Getenv("SERVER_SHUTDOWN_TIMEOUT"); timeout != "" {
		if t, err := time.ParseDuration(timeout); err == nil {
			config.Server.ShutdownTimeout = t
		} else {
			return fmt.Errorf("invalid SERVER_SHUTDOWN_TIMEOUT: %s", timeout)
		}
	} else {
		config.Server.ShutdownTimeout = 30 * time.Second
	}

	if timeout := os.Getenv("SERVER_READ_TIMEOUT"); timeout != "" {
		if t, err := time.ParseDuration(timeout); err == nil {
			config.Server.ReadTimeout = t
		} else {
			return fmt.Errorf("invalid SERVER_READ_TIMEOUT: %s", timeout)
		}
	} else {
		config.Server.ReadTimeout = 10 * time.Second
	}

	if timeout := os.Getenv("SERVER_WRITE_TIMEOUT"); timeout != "" {
		if t, err := time.ParseDuration(timeout); err == nil {
			config.Server.WriteTimeout = t
		} else {
			return fmt.Errorf("invalid SERVER_WRITE_TIMEOUT: %s", timeout)
		}
	} else {
		config.Server.WriteTimeout = 300 * time.Second // Extended to allow LLM processing
	}

	// HTTP config
	if timeout := os.Getenv("HTTP_TIMEOUT"); timeout != "" {
		if t, err := time.ParseDuration(timeout); err == nil {
			config.HTTP.Timeout = t
		} else {
			return fmt.Errorf("invalid HTTP_TIMEOUT: %s", timeout)
		}
	} else {
		config.HTTP.Timeout = 30 * time.Second
	}

	if conns := os.Getenv("HTTP_MAX_IDLE_CONNS"); conns != "" {
		if c, err := strconv.Atoi(conns); err == nil {
			config.HTTP.MaxIdleConns = c
		} else {
			return fmt.Errorf("invalid HTTP_MAX_IDLE_CONNS: %s", conns)
		}
	} else {
		config.HTTP.MaxIdleConns = 10
	}

	if conns := os.Getenv("HTTP_MAX_IDLE_CONNS_PER_HOST"); conns != "" {
		if c, err := strconv.Atoi(conns); err == nil {
			config.HTTP.MaxIdleConnsPerHost = c
		} else {
			return fmt.Errorf("invalid HTTP_MAX_IDLE_CONNS_PER_HOST: %s", conns)
		}
	} else {
		config.HTTP.MaxIdleConnsPerHost = 2
	}

	if timeout := os.Getenv("HTTP_IDLE_CONN_TIMEOUT"); timeout != "" {
		if t, err := time.ParseDuration(timeout); err == nil {
			config.HTTP.IdleConnTimeout = t
		} else {
			return fmt.Errorf("invalid HTTP_IDLE_CONN_TIMEOUT: %s", timeout)
		}
	} else {
		config.HTTP.IdleConnTimeout = 90 * time.Second
	}

	if timeout := os.Getenv("HTTP_TLS_HANDSHAKE_TIMEOUT"); timeout != "" {
		if t, err := time.ParseDuration(timeout); err == nil {
			config.HTTP.TLSHandshakeTimeout = t
		} else {
			return fmt.Errorf("invalid HTTP_TLS_HANDSHAKE_TIMEOUT: %s", timeout)
		}
	} else {
		config.HTTP.TLSHandshakeTimeout = 10 * time.Second
	}

	if timeout := os.Getenv("HTTP_EXPECT_CONTINUE_TIMEOUT"); timeout != "" {
		if t, err := time.ParseDuration(timeout); err == nil {
			config.HTTP.ExpectContinueTimeout = t
		} else {
			return fmt.Errorf("invalid HTTP_EXPECT_CONTINUE_TIMEOUT: %s", timeout)
		}
	} else {
		config.HTTP.ExpectContinueTimeout = 1 * time.Second
	}

	if agent := os.Getenv("HTTP_USER_AGENT"); agent != "" {
		config.HTTP.UserAgent = agent
	} else {
		config.HTTP.UserAgent = "Mozilla/5.0 (compatible; AltBot/1.0; +https://alt.example.com/bot)"
	}

	// User-Agent rotation config
	if rotation := os.Getenv("HTTP_USER_AGENT_ROTATION"); rotation != "" {
		if r, err := strconv.ParseBool(rotation); err == nil {
			config.HTTP.UserAgentRotation = r
		} else {
			return fmt.Errorf("invalid HTTP_USER_AGENT_ROTATION: %s", rotation)
		}
	} else {
		config.HTTP.UserAgentRotation = true
	}

	// User-Agents list from environment (comma-separated)
	if agents := os.Getenv("HTTP_USER_AGENTS"); agents != "" {
		config.HTTP.UserAgents = strings.Split(agents, ",")
		// Trim whitespace from each agent string
		for i, agent := range config.HTTP.UserAgents {
			config.HTTP.UserAgents[i] = strings.TrimSpace(agent)
		}
	} else {
		// Default User-Agent list for rotation
		config.HTTP.UserAgents = []string{
			"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
			"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
			"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
			"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:109.0) Gecko/20100101 Firefox/121.0",
			"Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:109.0) Gecko/20100101 Firefox/121.0",
			"Mozilla/5.0 (compatible; AltBot/1.0; +https://alt.example.com/bot)",
		}
	}

	// Browser headers config
	if browserHeaders := os.Getenv("HTTP_ENABLE_BROWSER_HEADERS"); browserHeaders != "" {
		if b, err := strconv.ParseBool(browserHeaders); err == nil {
			config.HTTP.EnableBrowserHeaders = b
		} else {
			return fmt.Errorf("invalid HTTP_ENABLE_BROWSER_HEADERS: %s", browserHeaders)
		}
	} else {
		config.HTTP.EnableBrowserHeaders = true
	}

	// Error response handling config
	if skipErrors := os.Getenv("HTTP_SKIP_ERROR_RESPONSES"); skipErrors != "" {
		if s, err := strconv.ParseBool(skipErrors); err == nil {
			config.HTTP.SkipErrorResponses = s
		} else {
			return fmt.Errorf("invalid HTTP_SKIP_ERROR_RESPONSES: %s", skipErrors)
		}
	} else {
		config.HTTP.SkipErrorResponses = true
	}

	if minLength := os.Getenv("HTTP_MIN_CONTENT_LENGTH"); minLength != "" {
		if m, err := strconv.Atoi(minLength); err == nil {
			config.HTTP.MinContentLength = m
		} else {
			return fmt.Errorf("invalid HTTP_MIN_CONTENT_LENGTH: %s", minLength)
		}
	} else {
		config.HTTP.MinContentLength = 500
	}

	// Redirect handling config
	if maxRedirects := os.Getenv("HTTP_MAX_REDIRECTS"); maxRedirects != "" {
		if m, err := strconv.Atoi(maxRedirects); err == nil {
			config.HTTP.MaxRedirects = m
		} else {
			return fmt.Errorf("invalid HTTP_MAX_REDIRECTS: %s", maxRedirects)
		}
	} else {
		config.HTTP.MaxRedirects = 5
	}

	if followRedirects := os.Getenv("HTTP_FOLLOW_REDIRECTS"); followRedirects != "" {
		if f, err := strconv.ParseBool(followRedirects); err == nil {
			config.HTTP.FollowRedirects = f
		} else {
			return fmt.Errorf("invalid HTTP_FOLLOW_REDIRECTS: %s", followRedirects)
		}
	} else {
		config.HTTP.FollowRedirects = true
	}

	// Envoy Proxy config
	if useProxy := os.Getenv("USE_ENVOY_PROXY"); useProxy != "" {
		if use, err := strconv.ParseBool(useProxy); err == nil {
			config.HTTP.UseEnvoyProxy = use
		} else {
			return fmt.Errorf("invalid USE_ENVOY_PROXY: %s", useProxy)
		}
	} else {
		config.HTTP.UseEnvoyProxy = false
	}

	if proxyURL := os.Getenv("ENVOY_PROXY_URL"); proxyURL != "" {
		config.HTTP.EnvoyProxyURL = proxyURL
	} else {
		config.HTTP.EnvoyProxyURL = "http://envoy-proxy.alt-apps.svc.cluster.local:8080"
	}

	if proxyPath := os.Getenv("ENVOY_PROXY_PATH"); proxyPath != "" {
		config.HTTP.EnvoyProxyPath = proxyPath
	} else {
		config.HTTP.EnvoyProxyPath = "/proxy/https://"
	}

	if timeout := os.Getenv("ENVOY_TIMEOUT"); timeout != "" {
		if t, err := time.ParseDuration(timeout); err == nil {
			config.HTTP.EnvoyTimeout = t
		} else {
			return fmt.Errorf("invalid ENVOY_TIMEOUT: %s", timeout)
		}
	} else {
		config.HTTP.EnvoyTimeout = 300 * time.Second
	}

	// Retry config
	if attempts := os.Getenv("RETRY_MAX_ATTEMPTS"); attempts != "" {
		if a, err := strconv.Atoi(attempts); err == nil {
			config.Retry.MaxAttempts = a
		} else {
			return fmt.Errorf("invalid RETRY_MAX_ATTEMPTS: %s", attempts)
		}
	} else {
		config.Retry.MaxAttempts = 3
	}

	if delay := os.Getenv("RETRY_BASE_DELAY"); delay != "" {
		if d, err := time.ParseDuration(delay); err == nil {
			config.Retry.BaseDelay = d
		} else {
			return fmt.Errorf("invalid RETRY_BASE_DELAY: %s", delay)
		}
	} else {
		config.Retry.BaseDelay = 1 * time.Second
	}

	if delay := os.Getenv("RETRY_MAX_DELAY"); delay != "" {
		if d, err := time.ParseDuration(delay); err == nil {
			config.Retry.MaxDelay = d
		} else {
			return fmt.Errorf("invalid RETRY_MAX_DELAY: %s", delay)
		}
	} else {
		config.Retry.MaxDelay = 30 * time.Second
	}

	if factor := os.Getenv("RETRY_BACKOFF_FACTOR"); factor != "" {
		if f, err := strconv.ParseFloat(factor, 64); err == nil {
			config.Retry.BackoffFactor = f
		} else {
			return fmt.Errorf("invalid RETRY_BACKOFF_FACTOR: %s", factor)
		}
	} else {
		config.Retry.BackoffFactor = 2.0
	}

	if factor := os.Getenv("RETRY_JITTER_FACTOR"); factor != "" {
		if f, err := strconv.ParseFloat(factor, 64); err == nil {
			config.Retry.JitterFactor = f
		} else {
			return fmt.Errorf("invalid RETRY_JITTER_FACTOR: %s", factor)
		}
	} else {
		config.Retry.JitterFactor = 0.1
	}

	// Rate limit config
	if interval := os.Getenv("RATE_LIMIT_DEFAULT_INTERVAL"); interval != "" {
		if i, err := time.ParseDuration(interval); err == nil {
			config.RateLimit.DefaultInterval = i
		} else {
			return fmt.Errorf("invalid RATE_LIMIT_DEFAULT_INTERVAL: %s", interval)
		}
	} else {
		config.RateLimit.DefaultInterval = 5 * time.Second
	}

	if size := os.Getenv("RATE_LIMIT_BURST_SIZE"); size != "" {
		if s, err := strconv.Atoi(size); err == nil {
			config.RateLimit.BurstSize = s
		} else {
			return fmt.Errorf("invalid RATE_LIMIT_BURST_SIZE: %s", size)
		}
	} else {
		config.RateLimit.BurstSize = 1
	}

	if adaptive := os.Getenv("RATE_LIMIT_ENABLE_ADAPTIVE"); adaptive != "" {
		if a, err := strconv.ParseBool(adaptive); err == nil {
			config.RateLimit.EnableAdaptive = a
		} else {
			return fmt.Errorf("invalid RATE_LIMIT_ENABLE_ADAPTIVE: %s", adaptive)
		}
	} else {
		config.RateLimit.EnableAdaptive = false
	}

	// DLQ config
	if name := os.Getenv("DLQ_QUEUE_NAME"); name != "" {
		config.DLQ.QueueName = name
	} else {
		config.DLQ.QueueName = "failed-articles"
	}

	if timeout := os.Getenv("DLQ_TIMEOUT"); timeout != "" {
		if t, err := time.ParseDuration(timeout); err == nil {
			config.DLQ.Timeout = t
		} else {
			return fmt.Errorf("invalid DLQ_TIMEOUT: %s", timeout)
		}
	} else {
		config.DLQ.Timeout = 10 * time.Second
	}

	if enabled := os.Getenv("DLQ_RETRY_ENABLED"); enabled != "" {
		if e, err := strconv.ParseBool(enabled); err == nil {
			config.DLQ.RetryEnabled = e
		} else {
			return fmt.Errorf("invalid DLQ_RETRY_ENABLED: %s", enabled)
		}
	} else {
		config.DLQ.RetryEnabled = true
	}

	// Metrics config
	if enabled := os.Getenv("METRICS_ENABLED"); enabled != "" {
		if e, err := strconv.ParseBool(enabled); err == nil {
			config.Metrics.Enabled = e
		} else {
			return fmt.Errorf("invalid METRICS_ENABLED: %s", enabled)
		}
	} else {
		config.Metrics.Enabled = true
	}

	if port := os.Getenv("METRICS_PORT"); port != "" {
		if p, err := strconv.Atoi(port); err == nil {
			config.Metrics.Port = p
		} else {
			return fmt.Errorf("invalid METRICS_PORT: %s", port)
		}
	} else {
		config.Metrics.Port = 9201
	}

	if path := os.Getenv("METRICS_PATH"); path != "" {
		config.Metrics.Path = path
	} else {
		config.Metrics.Path = "/metrics"
	}

	if interval := os.Getenv("METRICS_UPDATE_INTERVAL"); interval != "" {
		if i, err := time.ParseDuration(interval); err == nil {
			config.Metrics.UpdateInterval = i
		} else {
			return fmt.Errorf("invalid METRICS_UPDATE_INTERVAL: %s", interval)
		}
	} else {
		config.Metrics.UpdateInterval = 10 * time.Second
	}

	// NewsCreator config
	if host := os.Getenv("NEWS_CREATOR_HOST"); host != "" {
		config.NewsCreator.Host = host
	} else {
		config.NewsCreator.Host = "http://news-creator:11434"
	}

	if apiPath := os.Getenv("NEWS_CREATOR_API_PATH"); apiPath != "" {
		config.NewsCreator.APIPath = apiPath
	} else {
		config.NewsCreator.APIPath = "/api/v1/summarize"
	}

	if model := os.Getenv("NEWS_CREATOR_MODEL"); model != "" {
		config.NewsCreator.Model = model
	} else {
		config.NewsCreator.Model = "gemma3:4b"
	}

	if timeout := os.Getenv("NEWS_CREATOR_TIMEOUT"); timeout != "" {
		if t, err := time.ParseDuration(timeout); err == nil {
			config.NewsCreator.Timeout = t
		} else {
			return fmt.Errorf("invalid NEWS_CREATOR_TIMEOUT: %s", timeout)
		}
	} else {
		config.NewsCreator.Timeout = 300 * time.Second // Extended for LLM processing with 1000 tokens (num_predict) and continuation generation
	}

	return nil
}

func validateConfig(config *Config) error {
	if config.Server.Port <= 0 || config.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d", config.Server.Port)
	}

	if config.HTTP.Timeout <= 0 {
		return fmt.Errorf("HTTP timeout must be positive: %v", config.HTTP.Timeout)
	}

	if config.Retry.MaxAttempts <= 0 {
		return fmt.Errorf("retry max attempts must be positive: %d", config.Retry.MaxAttempts)
	}

	if config.Retry.BackoffFactor <= 1.0 {
		return fmt.Errorf("backoff factor must be greater than 1.0: %f", config.Retry.BackoffFactor)
	}

	if config.RateLimit.DefaultInterval <= 0 {
		return fmt.Errorf("rate limit default interval must be positive: %v", config.RateLimit.DefaultInterval)
	}

	if config.Metrics.Port <= 0 || config.Metrics.Port > 65535 {
		return fmt.Errorf("invalid metrics port: %d", config.Metrics.Port)
	}

	if config.NewsCreator.Host == "" {
		return fmt.Errorf("news creator host cannot be empty")
	}

	if config.NewsCreator.Timeout <= 0 {
		return fmt.Errorf("news creator timeout must be positive: %v", config.NewsCreator.Timeout)
	}

	// HTTP configuration validation
	if config.HTTP.MinContentLength < 0 {
		return fmt.Errorf("min content length must be non-negative: %d", config.HTTP.MinContentLength)
	}

	if config.HTTP.MaxRedirects < 0 {
		return fmt.Errorf("max redirects must be non-negative: %d", config.HTTP.MaxRedirects)
	}

	if config.HTTP.UserAgentRotation && len(config.HTTP.UserAgents) == 0 {
		return fmt.Errorf("user agent rotation enabled but no user agents configured")
	}

	// Validate User-Agent strings are not empty
	for i, agent := range config.HTTP.UserAgents {
		if strings.TrimSpace(agent) == "" {
			return fmt.Errorf("user agent at index %d cannot be empty", i)
		}
	}

	// Envoy Proxy validation
	if config.HTTP.UseEnvoyProxy {
		if config.HTTP.EnvoyProxyURL == "" {
			return fmt.Errorf("envoy proxy URL cannot be empty when USE_ENVOY_PROXY is true")
		}
		if config.HTTP.EnvoyProxyPath == "" {
			return fmt.Errorf("envoy proxy path cannot be empty when USE_ENVOY_PROXY is true")
		}
		if config.HTTP.EnvoyTimeout <= 0 {
			return fmt.Errorf("envoy timeout must be positive: %v", config.HTTP.EnvoyTimeout)
		}
	}

	return nil
}

// 設定の動的更新対応
type ConfigManager struct {
	config *Config
	mu     sync.RWMutex
	logger *slog.Logger
}

func NewConfigManager(config *Config, logger *slog.Logger) *ConfigManager {
	return &ConfigManager{
		config: config,
		logger: logger,
	}
}

func (cm *ConfigManager) GetConfig() *Config {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	// ディープコピーを返す
	configCopy := *cm.config
	return &configCopy
}

func (cm *ConfigManager) UpdateConfig(newConfig *Config) error {
	if err := validateConfig(newConfig); err != nil {
		return fmt.Errorf("new config validation failed: %w", err)
	}

	cm.mu.Lock()
	defer cm.mu.Unlock()

	oldConfig := cm.config
	cm.config = newConfig

	if cm.logger != nil {
		cm.logger.Info("configuration updated",
			"old_http_timeout", oldConfig.HTTP.Timeout,
			"new_http_timeout", newConfig.HTTP.Timeout,
			"old_retry_attempts", oldConfig.Retry.MaxAttempts,
			"new_retry_attempts", newConfig.Retry.MaxAttempts)
	}

	return nil
}

// UserAgentRotator handles User-Agent rotation with thread safety
type UserAgentRotator struct {
	config *HTTPConfig
	index  int
	mu     sync.Mutex
}

// NewUserAgentRotator creates a new User-Agent rotator
func NewUserAgentRotator(httpConfig *HTTPConfig) *UserAgentRotator {
	return &UserAgentRotator{
		config: httpConfig,
		index:  0,
	}
}

// GetUserAgent returns the current User-Agent or rotates if enabled
func (uar *UserAgentRotator) GetUserAgent() string {
	if !uar.config.UserAgentRotation || len(uar.config.UserAgents) == 0 {
		return uar.config.UserAgent
	}

	uar.mu.Lock()
	defer uar.mu.Unlock()

	userAgent := uar.config.UserAgents[uar.index]
	uar.index = (uar.index + 1) % len(uar.config.UserAgents)

	return userAgent
}

// GetRandomUserAgent returns a random User-Agent from the list
func (uar *UserAgentRotator) GetRandomUserAgent() string {
	if !uar.config.UserAgentRotation || len(uar.config.UserAgents) == 0 {
		return uar.config.UserAgent
	}

	// Random seed is automatically initialized in Go 1.20+

	uar.mu.Lock()
	defer uar.mu.Unlock()

	index := rand.Intn(len(uar.config.UserAgents))
	return uar.config.UserAgents[index]
}

// GetBrowserHeaders returns appropriate browser headers based on configuration
func (config *HTTPConfig) GetBrowserHeaders(userAgent string) map[string]string {
	if !config.EnableBrowserHeaders {
		return map[string]string{
			"User-Agent": userAgent,
		}
	}

	headers := map[string]string{
		"User-Agent":                userAgent,
		"Accept":                    "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8",
		"Accept-Language":           "en-US,en;q=0.9",
		"Accept-Encoding":           "gzip, deflate, br",
		"DNT":                       "1",
		"Connection":                "keep-alive",
		"Upgrade-Insecure-Requests": "1",
	}

	// Add browser-specific headers based on User-Agent
	if strings.Contains(userAgent, "Chrome") {
		headers["sec-ch-ua"] = `"Not_A Brand";v="8", "Chromium";v="120", "Google Chrome";v="120"`
		headers["sec-ch-ua-mobile"] = "?0"
		headers["sec-ch-ua-platform"] = `"Windows"`
	} else if strings.Contains(userAgent, "Firefox") {
		headers["Cache-Control"] = "max-age=0"
	}

	return headers
}
