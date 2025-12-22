package config

import (
	"strings"
	"time"
)

// Config aggregates all service configuration blocks.
type Config struct {
	Server         ServerConfig         `json:"server"`
	HTTP           HTTPConfig           `json:"http"`
	Retry          RetryConfig          `json:"retry"`
	RateLimit      RateLimitConfig      `json:"rate_limit"`
	DLQ            DLQConfig            `json:"dlq"`
	Metrics        MetricsConfig        `json:"metrics"`
	NewsCreator    NewsCreatorConfig    `json:"news_creator"`
	SummarizeQueue SummarizeQueueConfig `json:"summarize_queue"`
	AltService     AltServiceConfig     `json:"alt_service"`
}

type AltServiceConfig struct {
	Host    string        `json:"host" env:"ALT_BACKEND_HOST" default:"http://alt-backend:8080"`
	Timeout time.Duration `json:"timeout" env:"ALT_BACKEND_TIMEOUT" default:"10s"`
}

type ServerConfig struct {
	Port            int           `json:"port" env:"SERVER_PORT" default:"9200"`
	ShutdownTimeout time.Duration `json:"shutdown_timeout" env:"SERVER_SHUTDOWN_TIMEOUT" default:"30s"`
	ReadTimeout     time.Duration `json:"read_timeout" env:"SERVER_READ_TIMEOUT" default:"10s"`
	WriteTimeout    time.Duration `json:"write_timeout" env:"SERVER_WRITE_TIMEOUT" default:"300s"`
}

type HTTPConfig struct {
	Timeout               time.Duration `json:"timeout" env:"HTTP_TIMEOUT" default:"30s"`
	MaxIdleConns          int           `json:"max_idle_conns" env:"HTTP_MAX_IDLE_CONNS" default:"10"`
	MaxIdleConnsPerHost   int           `json:"max_idle_conns_per_host" env:"HTTP_MAX_IDLE_CONNS_PER_HOST" default:"2"`
	IdleConnTimeout       time.Duration `json:"idle_conn_timeout" env:"HTTP_IDLE_CONN_TIMEOUT" default:"90s"`
	TLSHandshakeTimeout   time.Duration `json:"tls_handshake_timeout" env:"HTTP_TLS_HANDSHAKE_TIMEOUT" default:"10s"`
	ExpectContinueTimeout time.Duration `json:"expect_continue_timeout" env:"HTTP_EXPECT_CONTINUE_TIMEOUT" default:"1s"`
	UserAgent             string        `json:"user_agent" env:"HTTP_USER_AGENT" default:"Mozilla/5.0 (compatible; AltBot/1.0; +https://alt.example.com/bot)"`
	UserAgentRotation     bool          `json:"user_agent_rotation" env:"HTTP_USER_AGENT_ROTATION" default:"true"`
	UserAgents            []string      `json:"user_agents" env:"HTTP_USER_AGENTS"`
	EnableBrowserHeaders  bool          `json:"enable_browser_headers" env:"HTTP_ENABLE_BROWSER_HEADERS" default:"true"`
	SkipErrorResponses    bool          `json:"skip_error_responses" env:"HTTP_SKIP_ERROR_RESPONSES" default:"true"`
	MinContentLength      int           `json:"min_content_length" env:"HTTP_MIN_CONTENT_LENGTH" default:"500"`
	MaxRedirects          int           `json:"max_redirects" env:"HTTP_MAX_REDIRECTS" default:"5"`
	FollowRedirects       bool          `json:"follow_redirects" env:"HTTP_FOLLOW_REDIRECTS" default:"true"`
	UseEnvoyProxy         bool          `json:"use_envoy_proxy" env:"USE_ENVOY_PROXY" default:"false"`
	EnvoyProxyURL         string        `json:"envoy_proxy_url" env:"ENVOY_PROXY_URL" default:"http://envoy-proxy.alt-apps.svc.cluster.local:8080"`
	EnvoyProxyPath        string        `json:"envoy_proxy_path" env:"ENVOY_PROXY_PATH" default:"/proxy/https://"`
	EnvoyTimeout          time.Duration `json:"envoy_timeout" env:"ENVOY_TIMEOUT" default:"300s"`
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
	Timeout time.Duration `json:"timeout" env:"NEWS_CREATOR_TIMEOUT" default:"300s"`
}

type SummarizeQueueConfig struct {
	WorkerInterval  time.Duration `json:"worker_interval" env:"SUMMARIZE_QUEUE_WORKER_INTERVAL" default:"10s"`
	MaxRetries      int           `json:"max_retries" env:"SUMMARIZE_QUEUE_MAX_RETRIES" default:"3"`
	PollingInterval time.Duration `json:"polling_interval" env:"SUMMARIZE_QUEUE_POLLING_INTERVAL" default:"5s"`
}

func defaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Port:            9200,
			ShutdownTimeout: 30 * time.Second,
			ReadTimeout:     10 * time.Second,
			WriteTimeout:    300 * time.Second,
		},
		HTTP: HTTPConfig{
			Timeout:               30 * time.Second,
			MaxIdleConns:          10,
			MaxIdleConnsPerHost:   2,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			UserAgent:             "Mozilla/5.0 (compatible; AltBot/1.0; +https://alt.example.com/bot)",
			UserAgentRotation:     true,
			UserAgents:            defaultUserAgents(),
			EnableBrowserHeaders:  true,
			SkipErrorResponses:    true,
			MinContentLength:      500,
			MaxRedirects:          5,
			FollowRedirects:       true,
			UseEnvoyProxy:         false,
			EnvoyProxyURL:         "http://envoy-proxy.alt-apps.svc.cluster.local:8080",
			EnvoyProxyPath:        "/proxy/https://",
			EnvoyTimeout:          300 * time.Second,
		},
		Retry: RetryConfig{
			MaxAttempts:   3,
			BaseDelay:     1 * time.Second,
			MaxDelay:      30 * time.Second,
			BackoffFactor: 2.0,
			JitterFactor:  0.1,
		},
		RateLimit: RateLimitConfig{
			DefaultInterval: 5 * time.Second,
			BurstSize:       1,
			EnableAdaptive:  false,
		},
		DLQ: DLQConfig{
			QueueName:    "failed-articles",
			Timeout:      10 * time.Second,
			RetryEnabled: true,
		},
		Metrics: MetricsConfig{
			Enabled:           true,
			Port:              9201,
			Path:              "/metrics",
			UpdateInterval:    10 * time.Second,
			ReadHeaderTimeout: 10 * time.Second,
			ReadTimeout:       30 * time.Second,
			WriteTimeout:      30 * time.Second,
			IdleTimeout:       120 * time.Second,
		},
		NewsCreator: NewsCreatorConfig{
			Host:    "http://news-creator:11434",
			APIPath: "/api/v1/summarize",
			Model:   "gemma3:4b",
			Timeout: 300 * time.Second,
		},
		SummarizeQueue: SummarizeQueueConfig{
			WorkerInterval:  10 * time.Second,
			MaxRetries:      3,
			PollingInterval: 5 * time.Second,
		},
		AltService: AltServiceConfig{
			Host:    "http://alt-backend:8080",
			Timeout: 10 * time.Second,
		},
	}
}

func defaultUserAgents() []string {
	return []string{
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:109.0) Gecko/20100101 Firefox/121.0",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:109.0) Gecko/20100101 Firefox/121.0",
		"Mozilla/5.0 (compatible; AltBot/1.0; +https://alt.example.com/bot)",
	}
}

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

	if strings.Contains(userAgent, "Chrome") {
		headers["sec-ch-ua"] = `"Not_A Brand";v="8", "Chromium";v="120", "Google Chrome";v="120"`
		headers["sec-ch-ua-mobile"] = "?0"
		headers["sec-ch-ua-platform"] = `"Windows"`
	} else if strings.Contains(userAgent, "Firefox") {
		headers["Cache-Control"] = "max-age=0"
	}

	return headers
}
