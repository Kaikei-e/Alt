// ABOUTME: This file implements HTTPClient factory for Envoy proxy integration
// ABOUTME: Provides clean switch between direct HTTP and Envoy proxy based on configuration

package service

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"pre-processor/config"
	"pre-processor/utils"
)

// HTTPClientFactory creates HTTPClient implementations based on configuration
type HTTPClientFactory struct {
	config *config.Config
	logger *slog.Logger
}

// NewHTTPClientFactory creates a new HTTP client factory
func NewHTTPClientFactory(cfg *config.Config, logger *slog.Logger) *HTTPClientFactory {
	return &HTTPClientFactory{
		config: cfg,
		logger: logger,
	}
}

// CreateClient creates an HTTPClient based on configuration
func (f *HTTPClientFactory) CreateClient() HTTPClient {
	if f.config == nil {
		f.logger.Error("HTTPClientFactory: config is nil")
		return &errorHTTPClient{err: fmt.Errorf("config cannot be nil")}
	}

	if f.config.HTTP.UseEnvoyProxy {
		f.logger.Info("HTTPClientFactory: creating Envoy proxy client",
			"proxy_url", f.config.HTTP.EnvoyProxyURL,
			"proxy_path", f.config.HTTP.EnvoyProxyPath)
		return NewEnvoyHTTPClient(&f.config.HTTP, f.logger)
	}

	f.logger.Info("HTTPClientFactory: creating direct HTTP client")
	return f.createDirectClient()
}

// CreateArticleFetcherClient creates HTTPClient optimized for article fetching
func (f *HTTPClientFactory) CreateArticleFetcherClient() HTTPClient {
	if f.config == nil {
		f.logger.Error("HTTPClientFactory: config is nil")
		return &errorHTTPClient{err: fmt.Errorf("config cannot be nil")}
	}

	if f.config.HTTP.UseEnvoyProxy {
		f.logger.Info("HTTPClientFactory: creating Envoy client for article fetching",
			"proxy_url", f.config.HTTP.EnvoyProxyURL,
			"timeout", f.config.HTTP.EnvoyTimeout)

		// Use longer timeout for article fetching through Envoy
		envoyConfig := f.config.HTTP
		if envoyConfig.EnvoyTimeout < 60*time.Second {
			envoyConfig.EnvoyTimeout = 60 * time.Second
		}

		return NewEnvoyHTTPClient(&envoyConfig, f.logger)
	}

	f.logger.Info("HTTPClientFactory: creating optimized direct client for article fetching")
	return f.createOptimizedDirectClient()
}

// CreateHealthCheckClient creates HTTPClient optimized for health checks
func (f *HTTPClientFactory) CreateHealthCheckClient() HTTPClient {
	if f.config == nil {
		f.logger.Error("HTTPClientFactory: config is nil")
		return &errorHTTPClient{err: fmt.Errorf("config cannot be nil")}
	}

	if f.config.HTTP.UseEnvoyProxy {
		f.logger.Info("HTTPClientFactory: creating Envoy client for health checks")

		// Use shorter timeout for health checks
		envoyConfig := f.config.HTTP
		envoyConfig.EnvoyTimeout = 30 * time.Second

		return NewEnvoyHTTPClient(&envoyConfig, f.logger)
	}

	f.logger.Info("HTTPClientFactory: creating direct client for health checks")
	return f.createHealthCheckClient()
}

// createDirectClient creates standard direct HTTP client
func (f *HTTPClientFactory) createDirectClient() HTTPClient {
	clientManager := utils.NewHTTPClientManager()
	return &HTTPClientWrapper{
		Client:    clientManager.GetFeedClient(),
		UserAgent: "", // Use config default
		Config:    &f.config.HTTP,
	}
}

// createOptimizedDirectClient creates optimized direct HTTP client for article fetching
func (f *HTTPClientFactory) createOptimizedDirectClient() HTTPClient {
	transport := &http.Transport{
		MaxIdleConns:          f.config.HTTP.MaxIdleConns,
		MaxIdleConnsPerHost:   f.config.HTTP.MaxIdleConnsPerHost,
		IdleConnTimeout:       f.config.HTTP.IdleConnTimeout,
		TLSHandshakeTimeout:   f.config.HTTP.TLSHandshakeTimeout,
		ExpectContinueTimeout: f.config.HTTP.ExpectContinueTimeout,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   f.config.HTTP.Timeout,
	}

	// Initialize User-Agent rotator
	userAgentRotator := config.NewUserAgentRotator(&f.config.HTTP)

	return &OptimizedHTTPClientWrapper{
		Client:           client,
		UserAgent:        f.config.HTTP.UserAgent,
		Logger:           f.logger,
		Config:           &f.config.HTTP,
		UserAgentRotator: userAgentRotator,
	}
}

// createHealthCheckClient creates HTTP client optimized for health checks
func (f *HTTPClientFactory) createHealthCheckClient() HTTPClient {
	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        5,
			IdleConnTimeout:     30 * time.Second,
			TLSHandshakeTimeout: 10 * time.Second,
		},
	}

	// Initialize User-Agent rotator
	userAgentRotator := config.NewUserAgentRotator(&f.config.HTTP)

	return &OptimizedHTTPClientWrapper{
		Client:           client,
		UserAgent:        f.config.HTTP.UserAgent,
		Logger:           f.logger,
		Config:           &f.config.HTTP,
		UserAgentRotator: userAgentRotator,
	}
}

// OptimizedHTTPClientWrapper implements HTTPClient with enhanced logging and error handling
type OptimizedHTTPClientWrapper struct {
	Client           *http.Client
	UserAgent        string
	Logger           *slog.Logger
	Config           *config.HTTPConfig
	UserAgentRotator *config.UserAgentRotator
}

// Get implements HTTPClient.Get with enhanced logging
func (w *OptimizedHTTPClientWrapper) Get(url string) (*http.Response, error) {
	start := time.Now()

	// Get User-Agent (rotated if available, otherwise use default)
	userAgent := w.UserAgent
	if w.UserAgentRotator != nil {
		userAgent = w.UserAgentRotator.GetUserAgent()
	}

	w.Logger.Debug("OptimizedHTTPClient: starting direct request",
		"url", url,
		"user_agent", userAgent,
		"rotation_enabled", w.UserAgentRotator != nil)

	// Get global metrics instance for tracking
	metrics := GetGlobalProxyMetrics(w.Logger)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		w.Logger.Error("OptimizedHTTPClient: failed to create request",
			"url", url,
			"error", err)
		metrics.RecordDirectRequest(time.Since(start), false)
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers (including rotated User-Agent)
	if w.Config != nil && w.Config.EnableBrowserHeaders {
		headers := w.Config.GetBrowserHeaders(userAgent)
		for key, value := range headers {
			req.Header.Set(key, value)
		}
	} else {
		req.Header.Set("User-Agent", userAgent)
	}

	resp, err := w.Client.Do(req)
	duration := time.Since(start)

	if err != nil {
		// Determine error type for domain-specific tracking
		var errorType ProxyErrorType
		if strings.Contains(err.Error(), "timeout") || strings.Contains(err.Error(), "deadline exceeded") {
			errorType = ProxyErrorTimeout
		} else if strings.Contains(err.Error(), "connection refused") || strings.Contains(err.Error(), "connection reset") {
			errorType = ProxyErrorConnection
		} else {
			errorType = ProxyErrorConnection // Default to connection error
		}

		w.Logger.Error("OptimizedHTTPClient: request failed",
			"url", url,
			"duration_ms", duration.Milliseconds(),
			"error", err)

		// Record both general and domain-specific metrics
		metrics.RecordDirectRequest(duration, false)
		metrics.RecordDomainRequest(url, duration, false, errorType)

		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}

	// Check for HTTP errors if skip error responses is enabled
	if w.Config != nil && w.Config.SkipErrorResponses && resp.StatusCode >= 400 {
		w.Logger.Error("OptimizedHTTPClient: HTTP error response detected",
			"url", url,
			"status_code", resp.StatusCode,
			"duration_ms", duration.Milliseconds())

		// Record as failed request (both general and domain-specific)
		metrics.RecordDirectRequest(duration, false)

		// Record domain-specific metrics with bot detection logic
		errorType := ProxyErrorConnection // Default to connection error for HTTP errors
		if resp.StatusCode == 403 || resp.StatusCode == 429 {
			errorType = ProxyErrorConnection // Bot detection typically shows as connection issues
		}
		metrics.RecordDomainRequest(url, duration, false, errorType)

		// Close response body to prevent resource leak
		_ = resp.Body.Close()

		// Return error instead of the response to prevent saving error content
		return nil, fmt.Errorf("HTTP error response: %d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	}

	// Record successful request (both general and domain-specific)
	success := resp.StatusCode >= 200 && resp.StatusCode < 400
	metrics.RecordDirectRequest(duration, success)

	if success {
		metrics.RecordDomainRequest(url, duration, true, ProxyErrorConfig) // No error type for success
	}

	w.Logger.Debug("OptimizedHTTPClient: request completed",
		"url", url,
		"status_code", resp.StatusCode,
		"duration_ms", duration.Milliseconds(),
		"content_length", resp.ContentLength,
		"user_agent", userAgent)

	return resp, nil
}

// ClientStats provides statistics about client usage
type ClientStats struct {
	EnvoyEnabled   bool          `json:"envoy_enabled"`
	ClientType     string        `json:"client_type"`
	DefaultTimeout time.Duration `json:"default_timeout"`
	EnvoyProxyURL  string        `json:"envoy_proxy_url,omitempty"`
	TotalClients   int           `json:"total_clients"`
}

// GetClientStats returns current client factory statistics
func (f *HTTPClientFactory) GetClientStats() *ClientStats {
	if f.config == nil {
		return &ClientStats{
			EnvoyEnabled:   false,
			ClientType:     "error",
			DefaultTimeout: 0,
			TotalClients:   0,
		}
	}

	stats := &ClientStats{
		EnvoyEnabled:   f.config.HTTP.UseEnvoyProxy,
		DefaultTimeout: f.config.HTTP.Timeout,
		TotalClients:   1, // This factory creates one client at a time
	}

	if f.config.HTTP.UseEnvoyProxy {
		stats.ClientType = "envoy_proxy"
		stats.EnvoyProxyURL = f.config.HTTP.EnvoyProxyURL
	} else {
		stats.ClientType = "direct_http"
	}

	return stats
}
