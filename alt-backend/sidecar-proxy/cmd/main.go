// Main entry point for the lightweight proxy sidecar
// This implements the core upstream resolution solution described in ISSUE_RESOLVE_PLAN.md
// to transform upstream="10.96.32.212:8080" into upstream="zenn.dev:443"
package main

import (
	"fmt"
	"log"
	"os"

	"github.com/alt-rss/alt-backend/sidecar-proxy/pkg/config"
	"github.com/alt-rss/alt-backend/sidecar-proxy/pkg/proxy"
)

const (
	Version = "1.0.0"
	Banner  = `
╔══════════════════════════════════════════════════════════════════════════════╗
║                     Lightweight Proxy Sidecar v%s                      ║
║                                                                              ║
║    🎯 Mission: Transform upstream="10.96.32.212:8080"                      ║
║                     → upstream="zenn.dev:443"                              ║
║                                                                              ║
║    🔧 Solution: External DNS resolution + Header control                    ║
║    📊 Monitoring: /metrics, /health, /ready endpoints                       ║
║    🛡️  Security: Domain allowlist + TLS verification                        ║
║                                                                              ║
║    Implementation following ISSUE_RESOLVE_PLAN.md specifications            ║
╚══════════════════════════════════════════════════════════════════════════════╝
`
)

func main() {
	// Display startup banner
	fmt.Printf(Banner, Version)
	
	// Initialize logger
	logger := log.New(os.Stdout, "[Main] ", log.LstdFlags|log.Lshortfile)
	
	logger.Println("🚀 Starting Lightweight Proxy Sidecar...")
	logger.Printf("📋 Version: %s", Version)
	logger.Printf("📁 Implementation: ISSUE_RESOLVE_PLAN.md compliant")
	
	// Load configuration from environment variables
	logger.Println("📖 Loading configuration...")
	cfg, err := config.LoadConfig()
	if err != nil {
		logger.Fatalf("❌ Failed to load configuration: %v", err)
	}
	
	logger.Printf("✅ Configuration loaded successfully")
	logger.Printf("   - Listen Port: %s", cfg.ListenPort)
	logger.Printf("   - Envoy Upstream: %s", cfg.EnvoyUpstream)
	logger.Printf("   - DNS Servers: %v", cfg.DNSServers)
	logger.Printf("   - Allowed Domains: %v", cfg.AllowedDomainsRaw)
	logger.Printf("   - DNS Cache TTL: %v", cfg.DNSCacheTimeout)
	logger.Printf("   - Request Timeout: %v", cfg.RequestTimeout)
	logger.Printf("   - Max Retries: %d", cfg.MaxRetries)
	logger.Printf("   - Metrics Enabled: %t", cfg.MetricsEnabled)
	logger.Printf("   - Debug Mode: %t", cfg.DebugMode)
	
	// Validate critical settings
	if err := validateCriticalSettings(cfg); err != nil {
		logger.Fatalf("❌ Critical configuration validation failed: %v", err)
	}
	
	// Create proxy instance
	logger.Println("🔧 Initializing proxy sidecar...")
	proxyInstance, err := proxy.NewLightweightProxy(cfg)
	if err != nil {
		logger.Fatalf("❌ Failed to create proxy instance: %v", err)
	}
	
	// Display operational information
	logger.Println("🎯 Proxy sidecar ready for upstream resolution!")
	logger.Printf("   📡 Proxy endpoint: http://localhost:%s/proxy/https://example.com/path", cfg.ListenPort)
	logger.Printf("   🏥 Health check: http://localhost:%s/health", cfg.ListenPort)
	logger.Printf("   ✅ Readiness check: http://localhost:%s/ready", cfg.ListenPort)
	if cfg.MetricsEnabled {
		logger.Printf("   📊 Metrics: http://localhost:%s/metrics", cfg.MetricsPort)
	}
	logger.Printf("   🔍 DNS debug: http://localhost:%s/debug/dns", cfg.ListenPort)
	logger.Printf("   ⚙️  Config debug: http://localhost:%s/debug/config", cfg.ListenPort)
	
	// Display expected upstream resolution behavior
	logger.Println("")
	logger.Println("🎯 Expected Upstream Resolution Behavior:")
	logger.Printf("   📥 Input Request: /proxy/https://zenn.dev/feed")
	logger.Printf("   🌐 DNS Resolution: zenn.dev → External IP (bypassing k8s DNS)")
	logger.Printf("   📤 Envoy Headers: Host=zenn.dev, X-Target-Domain=zenn.dev")
	logger.Printf("   📋 Expected Log: upstream=\"zenn.dev:443\" (instead of internal IP)")
	logger.Println("")
	
	// Final startup message
	logger.Printf("🌟 Starting HTTP server on port %s...", cfg.ListenPort)
	logger.Println("🔄 Proxy sidecar is now operational!")
	
	// Start the proxy server (this blocks until shutdown)
	if err := proxyInstance.Start(); err != nil {
		if err.Error() == "http: Server closed" {
			// Normal shutdown
			logger.Println("✅ Proxy sidecar shutdown completed successfully")
		} else {
			logger.Fatalf("❌ Proxy server error: %v", err)
		}
	}
}

// validateCriticalSettings ensures all critical configuration is valid
func validateCriticalSettings(cfg *config.ProxyConfig) error {
	// Ensure Envoy upstream is configured
	if cfg.EnvoyUpstream == "" {
		return fmt.Errorf("ENVOY_UPSTREAM must be configured")
	}
	
	// Ensure at least one DNS server is configured
	if len(cfg.DNSServers) == 0 {
		return fmt.Errorf("at least one DNS server must be configured")
	}
	
	// Ensure at least one domain is allowed
	if len(cfg.AllowedDomains) == 0 {
		return fmt.Errorf("at least one allowed domain must be configured")
	}
	
	// Validate that DNS servers include port numbers
	for _, server := range cfg.DNSServers {
		if !isValidDNSServer(server) {
			return fmt.Errorf("invalid DNS server format: %s (must include port, e.g., 8.8.8.8:53)", server)
		}
	}
	
	// Warn about debug mode in production
	if cfg.DebugMode {
		log.Printf("⚠️  WARNING: Debug mode is enabled - not recommended for production")
	}
	
	return nil
}

// isValidDNSServer checks if a DNS server string is in the correct format
func isValidDNSServer(server string) bool {
	// Simple check for host:port format
	if server == "" {
		return false
	}
	
	// Should contain a colon for port separation
	return len(server) > 0 && server != "" && (server[len(server)-3:] == ":53" || 
		server[len(server)-4:] == ":853" || server[len(server)-5:] == ":5053")
}

// init performs any necessary initialization before main()
func init() {
	// Set log flags for consistent output
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	
	// Environment variable validation
	if os.Getenv("DEBUG_STARTUP") == "true" {
		log.Println("🔍 DEBUG_STARTUP enabled - will show detailed startup information")
		
		// Display all relevant environment variables
		envVars := []string{
			"LISTEN_PORT",
			"ENVOY_UPSTREAM", 
			"DNS_SERVERS",
			"ALLOWED_DOMAINS",
			"DNS_CACHE_TIMEOUT",
			"REQUEST_TIMEOUT",
			"MAX_RETRIES",
			"METRICS_ENABLED",
			"DEBUG_MODE",
			"LOG_LEVEL",
		}
		
		log.Println("📋 Environment Variables:")
		for _, envVar := range envVars {
			value := os.Getenv(envVar)
			if value == "" {
				value = "<not set>"
			}
			log.Printf("   %s = %s", envVar, value)
		}
	}
}