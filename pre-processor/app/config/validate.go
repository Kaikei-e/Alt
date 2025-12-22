package config

import (
	"fmt"
	"strings"
)

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

	if config.SummarizeQueue.WorkerInterval <= 0 {
		return fmt.Errorf("summarize queue worker interval must be positive: %v", config.SummarizeQueue.WorkerInterval)
	}

	if config.SummarizeQueue.MaxRetries < 0 {
		return fmt.Errorf("summarize queue max retries must be non-negative: %d", config.SummarizeQueue.MaxRetries)
	}

	if config.SummarizeQueue.PollingInterval <= 0 {
		return fmt.Errorf("summarize queue polling interval must be positive: %v", config.SummarizeQueue.PollingInterval)
	}

	if config.HTTP.MinContentLength < 0 {
		return fmt.Errorf("min content length must be non-negative: %d", config.HTTP.MinContentLength)
	}

	if config.HTTP.MaxRedirects < 0 {
		return fmt.Errorf("max redirects must be non-negative: %d", config.HTTP.MaxRedirects)
	}

	if config.HTTP.UserAgentRotation && len(config.HTTP.UserAgents) == 0 {
		return fmt.Errorf("user agent rotation enabled but no user agents configured")
	}

	for i, agent := range config.HTTP.UserAgents {
		if strings.TrimSpace(agent) == "" {
			return fmt.Errorf("user agent at index %d cannot be empty", i)
		}
	}

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
