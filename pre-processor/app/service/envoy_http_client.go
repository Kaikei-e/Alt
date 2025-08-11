// TDD Phase 2 - GREEN: EnvoyHTTPClient Implementation
// ABOUTME: This file implements HTTP client that routes requests through Envoy proxy
// ABOUTME: Handles DNS resolution and required headers for Envoy's /proxy/https:// endpoint

package service

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"log/slog"

	"pre-processor/config"
)

// EnvoyHTTPClient implements HTTPClient interface for Envoy proxy routing
type EnvoyHTTPClient struct {
	config     *config.HTTPConfig
	logger     *slog.Logger
	httpClient *http.Client
}

// NewEnvoyHTTPClient creates a new Envoy-enabled HTTP client
func NewEnvoyHTTPClient(cfg *config.HTTPConfig, logger *slog.Logger) HTTPClient {
	if cfg == nil {
		logger.Error("EnvoyHTTPClient: config cannot be nil")
		return &errorHTTPClient{err: fmt.Errorf("config cannot be nil")}
	}

	if !cfg.UseEnvoyProxy {
		logger.Warn("EnvoyHTTPClient: proxy disabled but client created")
		return &errorHTTPClient{err: fmt.Errorf("EnvoyHTTPClient requires UseEnvoyProxy=true")}
	}

	if cfg.EnvoyProxyURL == "" {
		logger.Error("EnvoyHTTPClient: proxy URL cannot be empty")
		return &errorHTTPClient{err: fmt.Errorf("EnvoyProxyURL cannot be empty")}
	}

	// Create HTTP client with Envoy-specific settings
	httpClient := &http.Client{
		Timeout: cfg.EnvoyTimeout,
		Transport: &http.Transport{
			MaxIdleConns:          cfg.MaxIdleConns,
			MaxIdleConnsPerHost:   cfg.MaxIdleConnsPerHost,
			IdleConnTimeout:       cfg.IdleConnTimeout,
			TLSHandshakeTimeout:   cfg.TLSHandshakeTimeout,
			ExpectContinueTimeout: cfg.ExpectContinueTimeout,
		},
	}

	return &EnvoyHTTPClient{
		config:     cfg,
		logger:     logger,
		httpClient: httpClient,
	}
}

// Get implements HTTPClient.Get through Envoy proxy
func (c *EnvoyHTTPClient) Get(targetURL string) (*http.Response, error) {
	start := time.Now()

	c.logger.Info("EnvoyHTTPClient: starting request", 
		"target_url", targetURL,
		"proxy_url", c.config.EnvoyProxyURL)

	// Get global metrics instance for tracking
	metrics := GetGlobalProxyMetrics(c.logger)

	// Parse target URL
	parsedURL, err := url.Parse(targetURL)
	if err != nil {
		metrics.RecordError(ProxyErrorConfig)
		c.logger.Error("EnvoyHTTPClient: invalid target URL", 
			"target_url", targetURL, 
			"error", err)
		metrics.RecordEnvoyRequest(time.Since(start), false, 0)
		return nil, fmt.Errorf("invalid target URL: %w", err)
	}

	// Only support HTTPS for RSS feeds
	if parsedURL.Scheme != "https" {
		metrics.RecordError(ProxyErrorConfig)
		c.logger.Error("EnvoyHTTPClient: only HTTPS URLs supported", 
			"target_url", targetURL,
			"scheme", parsedURL.Scheme)
		metrics.RecordEnvoyRequest(time.Since(start), false, 0)
		return nil, fmt.Errorf("only HTTPS URLs supported, got: %s", parsedURL.Scheme)
	}

	// Resolve target domain to IP (with timing)
	dnsStart := time.Now()
	resolvedIP, err := c.ResolveDomain(parsedURL.Hostname())
	dnsResolutionTime := time.Since(dnsStart)
	
	if err != nil {
		metrics.RecordError(ProxyErrorDNS)
		c.logger.Error("EnvoyHTTPClient: DNS resolution failed", 
			"hostname", parsedURL.Hostname(), 
			"error", err,
			"dns_duration_ms", dnsResolutionTime.Milliseconds())
		metrics.RecordEnvoyRequest(time.Since(start), false, dnsResolutionTime)
		return nil, fmt.Errorf("DNS resolution failed for %s: %w", parsedURL.Hostname(), err)
	}

	// Construct Envoy proxy URL
	// Format: http://envoy:8080/proxy/https://target-domain.com/path
	proxyPath := strings.TrimSuffix(c.config.EnvoyProxyPath, "/") + "/" + parsedURL.Host + parsedURL.Path
	if parsedURL.RawQuery != "" {
		proxyPath += "?" + parsedURL.RawQuery
	}

	proxyURL, err := url.Parse(c.config.EnvoyProxyURL)
	if err != nil {
		metrics.RecordError(ProxyErrorConfig)
		c.logger.Error("EnvoyHTTPClient: invalid proxy URL", 
			"proxy_url", c.config.EnvoyProxyURL, 
			"error", err)
		metrics.RecordEnvoyRequest(time.Since(start), false, dnsResolutionTime)
		return nil, fmt.Errorf("invalid proxy URL: %w", err)
	}

	proxyURL.Path = proxyPath

	// Create HTTP request
	req, err := http.NewRequest("GET", proxyURL.String(), nil)
	if err != nil {
		metrics.RecordError(ProxyErrorConfig)
		c.logger.Error("EnvoyHTTPClient: failed to create request", 
			"proxy_url", proxyURL.String(), 
			"error", err)
		metrics.RecordEnvoyRequest(time.Since(start), false, dnsResolutionTime)
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set required headers for Envoy
	req.Header.Set("X-Target-Domain", parsedURL.Hostname())
	req.Header.Set("X-Resolved-IP", resolvedIP)
	req.Header.Set("User-Agent", c.config.UserAgent)

	c.logger.Debug("EnvoyHTTPClient: sending request", 
		"proxy_url", proxyURL.String(),
		"target_domain", parsedURL.Hostname(),
		"resolved_ip", resolvedIP,
		"headers", req.Header)

	// Execute request
	resp, err := c.httpClient.Do(req)
	duration := time.Since(start)

	if err != nil {
		// Categorize error type for metrics
		if strings.Contains(err.Error(), "timeout") || strings.Contains(err.Error(), "deadline exceeded") {
			metrics.RecordError(ProxyErrorTimeout)
		} else if strings.Contains(err.Error(), "connection refused") || strings.Contains(err.Error(), "connection reset") {
			metrics.RecordError(ProxyErrorConnection)
		} else {
			metrics.RecordError(ProxyErrorConnection) // Default to connection error
		}
		
		c.logger.Error("EnvoyHTTPClient: request failed", 
			"target_url", targetURL,
			"proxy_url", proxyURL.String(),
			"duration_ms", duration.Milliseconds(),
			"dns_duration_ms", dnsResolutionTime.Milliseconds(),
			"error", err)
		
		metrics.RecordEnvoyRequest(duration, false, dnsResolutionTime)
		return nil, fmt.Errorf("Envoy proxy request failed: %w", err)
	}

	// Record successful request
	success := resp.StatusCode >= 200 && resp.StatusCode < 400
	metrics.RecordEnvoyRequest(duration, success, dnsResolutionTime)

	c.logger.Info("EnvoyHTTPClient: request completed", 
		"target_url", targetURL,
		"proxy_url", proxyURL.String(),
		"status_code", resp.StatusCode,
		"duration_ms", duration.Milliseconds(),
		"dns_duration_ms", dnsResolutionTime.Milliseconds(),
		"content_length", resp.ContentLength)

	return resp, nil
}

// ResolveDomain resolves a domain name to IP address for X-Resolved-IP header
func (c *EnvoyHTTPClient) ResolveDomain(hostname string) (string, error) {
	c.logger.Debug("EnvoyHTTPClient: resolving domain", "hostname", hostname)

	// Use Go's default DNS resolver
	ips, err := net.LookupIP(hostname)
	if err != nil {
		c.logger.Error("EnvoyHTTPClient: DNS lookup failed", 
			"hostname", hostname, 
			"error", err)
		return "", fmt.Errorf("DNS lookup failed: %w", err)
	}

	if len(ips) == 0 {
		c.logger.Error("EnvoyHTTPClient: no IPs found for hostname", "hostname", hostname)
		return "", fmt.Errorf("no IP addresses found for %s", hostname)
	}

	// Return first IPv4 address
	for _, ip := range ips {
		if ip.To4() != nil {
			resolvedIP := ip.String()
			c.logger.Debug("EnvoyHTTPClient: domain resolved", 
				"hostname", hostname, 
				"resolved_ip", resolvedIP)
			return resolvedIP, nil
		}
	}

	// Fallback to first IP (might be IPv6)
	resolvedIP := ips[0].String()
	c.logger.Debug("EnvoyHTTPClient: domain resolved (IPv6)", 
		"hostname", hostname, 
		"resolved_ip", resolvedIP)
	return resolvedIP, nil
}

// errorHTTPClient is a HTTPClient implementation that always returns an error
// Used for configuration validation failures
type errorHTTPClient struct {
	err error
}

func (c *errorHTTPClient) Get(url string) (*http.Response, error) {
	return nil, c.err
}