// Package config provides configuration management for the lightweight proxy sidecar
// This package handles environment variable loading, validation, and default values
// following the ISSUE_RESOLVE_PLAN.md specifications for configuration management.
package config

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ProxyConfig manages the complete configuration for the proxy sidecar
// It includes all necessary settings for HTTP server, DNS resolution, Envoy integration,
// and operational parameters as specified in ISSUE_RESOLVE_PLAN.md
type ProxyConfig struct {
	// Server Configuration
	ListenPort      string        `json:"listen_port"`
	RequestTimeout  time.Duration `json:"request_timeout"`
	ReadTimeout     time.Duration `json:"read_timeout"`
	WriteTimeout    time.Duration `json:"write_timeout"`
	IdleTimeout     time.Duration `json:"idle_timeout"`
	MaxRetries      int           `json:"max_retries"`

	// Envoy Integration
	EnvoyUpstream     string        `json:"envoy_upstream"`
	EnvoyTimeout      time.Duration `json:"envoy_timeout"`
	EnvoyMaxConns     int           `json:"envoy_max_conns"`
	EnvoyMaxIdleConns int           `json:"envoy_max_idle_conns"`

	// DNS Configuration  
	DNSServers         []string      `json:"dns_servers"`
	DNSTimeout         time.Duration `json:"dns_timeout"`
	DNSCacheTimeout    time.Duration `json:"dns_cache_timeout"`
	DNSMaxCacheEntries int           `json:"dns_max_cache_entries"`

	// Security Configuration
	AllowedDomains     []*regexp.Regexp `json:"-"` // Not serializable due to regexp
	AllowedDomainsRaw  []string         `json:"allowed_domains_raw"`
	MaxRequestSize     int64            `json:"max_request_size"`
	HeaderTimeoutSec   int              `json:"header_timeout_sec"`

	// Monitoring Configuration
	MetricsEnabled  bool   `json:"metrics_enabled"`
	MetricsPort     string `json:"metrics_port"`
	HealthPort      string `json:"health_port"`
	LogLevel        string `json:"log_level"`
	LogFormat       string `json:"log_format"`
	StructuredLogs  bool   `json:"structured_logs"`

	// Performance Configuration  
	WorkerPoolSize     int `json:"worker_pool_size"`
	BufferSize         int `json:"buffer_size"`
	MaxConcurrentReqs  int `json:"max_concurrent_reqs"`

	// Development/Debug Configuration
	DebugMode       bool   `json:"debug_mode"`
	DryRunMode      bool   `json:"dry_run_mode"`
	VerboseLogging  bool   `json:"verbose_logging"`
	TraceHeaders    bool   `json:"trace_headers"`
}

// LoadConfig loads configuration from environment variables with sensible defaults
// This function implements the configuration loading strategy outlined in ISSUE_RESOLVE_PLAN.md
func LoadConfig() (*ProxyConfig, error) {
	config := &ProxyConfig{
		// Server defaults (optimized for sidecar performance)
		ListenPort:     getEnvOrDefault("LISTEN_PORT", "8080"),
		RequestTimeout: getDurationOrDefault("REQUEST_TIMEOUT", 30*time.Second),
		ReadTimeout:    getDurationOrDefault("READ_TIMEOUT", 30*time.Second),
		WriteTimeout:   getDurationOrDefault("WRITE_TIMEOUT", 30*time.Second),
		IdleTimeout:    getDurationOrDefault("IDLE_TIMEOUT", 120*time.Second),
		MaxRetries:     getIntOrDefault("MAX_RETRIES", 3),

		// Envoy integration defaults (localhost communication within Pod)
		EnvoyUpstream:     getEnvOrDefault("ENVOY_UPSTREAM", "localhost:10000"),
		EnvoyTimeout:      getDurationOrDefault("ENVOY_TIMEOUT", 60*time.Second),
		EnvoyMaxConns:     getIntOrDefault("ENVOY_MAX_CONNS", 20),
		EnvoyMaxIdleConns: getIntOrDefault("ENVOY_MAX_IDLE_CONNS", 10),

		// DNS resolution defaults (external DNS bypass as per plan)
		DNSServers:         parseDNSServers(getEnvOrDefault("DNS_SERVERS", "8.8.8.8:53,1.1.1.1:53,208.67.222.222:53")),
		DNSTimeout:         getDurationOrDefault("DNS_TIMEOUT", 5*time.Second),
		DNSCacheTimeout:    getDurationOrDefault("DNS_CACHE_TIMEOUT", 300*time.Second),
		DNSMaxCacheEntries: getIntOrDefault("DNS_MAX_CACHE_ENTRIES", 1000),

		// Security defaults
		MaxRequestSize:   getInt64OrDefault("MAX_REQUEST_SIZE", 10*1024*1024), // 10MB
		HeaderTimeoutSec: getIntOrDefault("HEADER_TIMEOUT_SEC", 10),

		// Monitoring defaults  
		MetricsEnabled: getBoolOrDefault("METRICS_ENABLED", true),
		MetricsPort:    getEnvOrDefault("METRICS_PORT", "9090"),
		HealthPort:     getEnvOrDefault("HEALTH_PORT", "8081"),
		LogLevel:       getEnvOrDefault("LOG_LEVEL", "info"),
		LogFormat:      getEnvOrDefault("LOG_FORMAT", "json"),
		StructuredLogs: getBoolOrDefault("STRUCTURED_LOGS", true),

		// Performance defaults (lightweight sidecar optimization)
		WorkerPoolSize:    getIntOrDefault("WORKER_POOL_SIZE", 10),
		BufferSize:        getIntOrDefault("BUFFER_SIZE", 4096),
		MaxConcurrentReqs: getIntOrDefault("MAX_CONCURRENT_REQS", 100),

		// Development defaults
		DebugMode:      getBoolOrDefault("DEBUG_MODE", false),
		DryRunMode:     getBoolOrDefault("DRY_RUN_MODE", false),
		VerboseLogging: getBoolOrDefault("VERBOSE_LOGGING", false),
		TraceHeaders:   getBoolOrDefault("TRACE_HEADERS", true),
	}

	// Parse allowed domains with proper regex compilation
	if err := config.parseAllowedDomains(); err != nil {
		return nil, fmt.Errorf("failed to parse allowed domains: %w", err)
	}

	// Validate the complete configuration
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return config, nil
}

// parseAllowedDomains processes the ALLOWED_DOMAINS environment variable
// and compiles regex patterns for domain matching as specified in the plan
func (c *ProxyConfig) parseAllowedDomains() error {
	allowedDomainsStr := getEnvOrDefault("ALLOWED_DOMAINS", 
		"feeds\\.bbci\\.co\\.uk,zenn\\.dev,github\\.com,feeds\\.feedburner\\.com,rss\\.cnn\\.com,qiita\\.com,feeds\\.reuters\\.com,httpbin\\.org")
	
	c.AllowedDomainsRaw = strings.Split(allowedDomainsStr, ",")
	c.AllowedDomains = make([]*regexp.Regexp, 0, len(c.AllowedDomainsRaw))

	for _, domain := range c.AllowedDomainsRaw {
		domain = strings.TrimSpace(domain)
		if domain == "" {
			continue
		}

		// Compile regex pattern for domain matching
		// Support exact matches and simple wildcard patterns
		pattern := domain
		if !strings.Contains(pattern, "\\") {
			// If not already escaped, escape dots and add anchors
			pattern = "^" + strings.ReplaceAll(pattern, ".", "\\.") + "$"
		}

		compiled, err := regexp.Compile(pattern)
		if err != nil {
			return fmt.Errorf("invalid domain pattern '%s': %w", domain, err)
		}

		c.AllowedDomains = append(c.AllowedDomains, compiled)
	}

	return nil
}

// Validate performs comprehensive configuration validation
// This ensures all settings are within acceptable ranges and compatible
func (c *ProxyConfig) Validate() error {
	// Port validation
	if port, err := strconv.Atoi(c.ListenPort); err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("invalid listen port: %s", c.ListenPort)
	}

	if port, err := strconv.Atoi(c.MetricsPort); err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("invalid metrics port: %s", c.MetricsPort)
	}

	if port, err := strconv.Atoi(c.HealthPort); err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("invalid health port: %s", c.HealthPort)
	}

	// Timeout validation
	if c.RequestTimeout <= 0 {
		return fmt.Errorf("request timeout must be positive: %v", c.RequestTimeout)
	}

	if c.DNSTimeout <= 0 {
		return fmt.Errorf("DNS timeout must be positive: %v", c.DNSTimeout)
	}

	// DNS servers validation
	if len(c.DNSServers) == 0 {
		return fmt.Errorf("at least one DNS server must be configured")
	}

	// Allowed domains validation
	if len(c.AllowedDomains) == 0 {
		return fmt.Errorf("at least one allowed domain must be configured")
	}

	// Performance limits validation
	if c.MaxConcurrentReqs <= 0 {
		return fmt.Errorf("max concurrent requests must be positive: %d", c.MaxConcurrentReqs)
	}

	if c.WorkerPoolSize <= 0 {
		return fmt.Errorf("worker pool size must be positive: %d", c.WorkerPoolSize)
	}

	return nil
}

// IsDomainAllowed checks if a domain matches any of the allowed patterns
// This is the core security function for domain validation
func (c *ProxyConfig) IsDomainAllowed(domain string) bool {
	for _, pattern := range c.AllowedDomains {
		if pattern.MatchString(domain) {
			return true
		}
	}
	return false
}

// Helper functions for environment variable parsing with defaults

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getIntOrDefault(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}

func getInt64OrDefault(key string, defaultValue int64) int64 {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.ParseInt(value, 10, 64); err == nil {
			return intVal
		}
	}
	return defaultValue
}

func getBoolOrDefault(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolVal, err := strconv.ParseBool(value); err == nil {
			return boolVal
		}
	}
	return defaultValue
}

func getDurationOrDefault(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}

func parseDNSServers(serversStr string) []string {
	servers := strings.Split(serversStr, ",")
	result := make([]string, 0, len(servers))
	
	for _, server := range servers {
		server = strings.TrimSpace(server)
		if server != "" {
			// Ensure port is specified for DNS servers
			if !strings.Contains(server, ":") {
				server += ":53"
			}
			result = append(result, server)
		}
	}
	
	return result
}