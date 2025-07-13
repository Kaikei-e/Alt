// ABOUTME: This file implements configuration management with environment variable support
// ABOUTME: Provides validation, defaults, and dynamic configuration updates for production use
package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"sync"
	"time"
)

type Config struct {
	Server    ServerConfig    `json:"server"`
	HTTP      HTTPConfig      `json:"http"`
	Retry     RetryConfig     `json:"retry"`
	RateLimit RateLimitConfig `json:"rate_limit"`
	DLQ       DLQConfig       `json:"dlq"`
	Metrics   MetricsConfig   `json:"metrics"`
}

type ServerConfig struct {
	Port            int           `json:"port" env:"SERVER_PORT" default:"9200"`
	ShutdownTimeout time.Duration `json:"shutdown_timeout" env:"SERVER_SHUTDOWN_TIMEOUT" default:"30s"`
	ReadTimeout     time.Duration `json:"read_timeout" env:"SERVER_READ_TIMEOUT" default:"10s"`
	WriteTimeout    time.Duration `json:"write_timeout" env:"SERVER_WRITE_TIMEOUT" default:"10s"`
}

type HTTPConfig struct {
	Timeout               time.Duration `json:"timeout" env:"HTTP_TIMEOUT" default:"30s"`
	MaxIdleConns          int           `json:"max_idle_conns" env:"HTTP_MAX_IDLE_CONNS" default:"10"`
	MaxIdleConnsPerHost   int           `json:"max_idle_conns_per_host" env:"HTTP_MAX_IDLE_CONNS_PER_HOST" default:"2"`
	IdleConnTimeout       time.Duration `json:"idle_conn_timeout" env:"HTTP_IDLE_CONN_TIMEOUT" default:"90s"`
	TLSHandshakeTimeout   time.Duration `json:"tls_handshake_timeout" env:"HTTP_TLS_HANDSHAKE_TIMEOUT" default:"10s"`
	ExpectContinueTimeout time.Duration `json:"expect_continue_timeout" env:"HTTP_EXPECT_CONTINUE_TIMEOUT" default:"1s"`
	UserAgent             string        `json:"user_agent" env:"HTTP_USER_AGENT" default:"pre-processor/1.0 (+https://alt.example.com/bot)"`
}

type RetryConfig struct {
	MaxAttempts   int           `json:"max_attempts" env:"RETRY_MAX_ATTEMPTS" default:"3"`
	BaseDelay     time.Duration `json:"base_delay" env:"RETRY_BASE_DELAY" default:"1s"`
	MaxDelay      time.Duration `json:"max_delay" env:"RETRY_MAX_DELAY" default:"30s"`
	BackoffFactor float64       `json:"backoff_factor" env:"RETRY_BACKOFF_FACTOR" default:"2.0"`
	JitterFactor  float64       `json:"jitter_factor" env:"RETRY_JITTER_FACTOR" default:"0.1"`
}

type RateLimitConfig struct {
	DefaultInterval    time.Duration            `json:"default_interval" env:"RATE_LIMIT_DEFAULT_INTERVAL" default:"5s"`
	DomainIntervals    map[string]time.Duration `json:"domain_intervals" env:"RATE_LIMIT_DOMAIN_INTERVALS"`
	BurstSize          int                      `json:"burst_size" env:"RATE_LIMIT_BURST_SIZE" default:"1"`
	EnableAdaptive     bool                     `json:"enable_adaptive" env:"RATE_LIMIT_ENABLE_ADAPTIVE" default:"false"`
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
		config.Server.WriteTimeout = 10 * time.Second
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
		config.HTTP.UserAgent = "pre-processor/1.0 (+https://alt.example.com/bot)"
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