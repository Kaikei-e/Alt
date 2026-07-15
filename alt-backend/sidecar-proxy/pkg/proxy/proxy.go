// Package proxy implements the core HTTP proxy functionality for the lightweight sidecar
// This package contains the critical upstream resolution logic described in ISSUE_RESOLVE_PLAN.md
// to transform upstream="10.96.32.212:8080" into upstream="zenn.dev:443"
package proxy

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/alt-rss/alt-backend/sidecar-proxy/pkg/autolearn"
	"github.com/alt-rss/alt-backend/sidecar-proxy/pkg/config"
	"github.com/alt-rss/alt-backend/sidecar-proxy/pkg/dns"
	"github.com/alt-rss/alt-backend/sidecar-proxy/pkg/metrics"
)

// LightweightProxy represents the main proxy sidecar implementation
// This struct contains all necessary components for the upstream resolution solution
type LightweightProxy struct {
	config     *config.ProxyConfig
	httpClient *http.Client
	// connectHTTPClient is a separate, long-lived client for CONNECT /
	// persistent-tunnel forwarding (forwardToEnvoyConnect): it needs a much
	// longer timeout (model downloads can run for minutes) than httpClient's
	// cfg.RequestTimeout, so it cannot share that client's Transport.
	connectHTTPClient *http.Client
	dnsResolver       *dns.ExternalDNSResolver
	logger            *log.Logger
	server            *http.Server
	metrics           *metrics.Collector

	// 自動学習機能
	autoLearner *autolearn.AutoLearner

	// オンメモリDNS管理: 動的ドメイン解決システム
	dynamicDNS *dns.DynamicResolver

	// Request processing state
	shutdownChan chan struct{}
	// ready is read from the /ready health handler and written from
	// Start/Stop on different goroutines, so it must be accessed atomically.
	ready atomic.Bool
	// reqSlots bounds in-flight requests to cfg.MaxConcurrentReqs: a slot is
	// acquired at request entry and released when the handler returns.
	reqSlots chan struct{}
}

// RequestContext holds context information for each proxy request
// This enables detailed logging and tracing as specified in the plan
type RequestContext struct {
	TraceID       string    `json:"trace_id"`
	StartTime     time.Time `json:"start_time"`
	TargetURL     *url.URL  `json:"target_url"`
	ResolvedIP    net.IP    `json:"resolved_ip"`
	Method        string    `json:"method"`
	UserAgent     string    `json:"user_agent"`
	ContentLength int64     `json:"content_length"`
}

// ProxyResponse contains the response details for logging and metrics
type ProxyResponse struct {
	StatusCode    int           `json:"status_code"`
	ContentLength int64         `json:"content_length"`
	Duration      time.Duration `json:"duration"`
	DNSTime       time.Duration `json:"dns_time"`
	ProxyTime     time.Duration `json:"proxy_time"`
	UpstreamHost  string        `json:"upstream_host"` // This is the key field for solving the upstream problem
}

// NewLightweightProxy creates a new proxy instance with optimized configuration
// This constructor implements the architecture described in ISSUE_RESOLVE_PLAN.md
func NewLightweightProxy(cfg *config.ProxyConfig) (*LightweightProxy, error) {
	// Create optimized HTTP transport for Envoy communication
	transport := &http.Transport{
		MaxIdleConns:          cfg.EnvoyMaxConns,
		MaxConnsPerHost:       cfg.EnvoyMaxConns,
		MaxIdleConnsPerHost:   cfg.EnvoyMaxIdleConns,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   30 * time.Second,
		ExpectContinueTimeout: 10 * time.Second,
		DisableKeepAlives:     false,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: false,
		},
	}

	// Create HTTP client with timeouts
	httpClient := &http.Client{
		Transport: transport,
		Timeout:   cfg.RequestTimeout,
	}

	// Long-lived client for CONNECT/persistent-tunnel forwarding, built once
	// and reused across requests so its Transport actually keeps connections
	// alive instead of being discarded per-request.
	connectHTTPClient := &http.Client{
		Timeout: 10 * time.Minute, // Extended timeout for model downloads
		Transport: &http.Transport{
			DisableKeepAlives:     false,
			MaxIdleConns:          10,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   30 * time.Second,
			ExpectContinueTimeout: 10 * time.Second,
		},
	}

	// Initialize logger
	logger := log.New(os.Stdout, "[PROXY] ", log.LstdFlags|log.Lshortfile)

	// Initialize external DNS resolver
	dnsResolver := dns.NewExternalDNSResolver(
		cfg.DNSServers,
		cfg.DNSTimeout,
		int(cfg.DNSCacheTimeout.Seconds()),
	)

	// Initialize metrics collector
	metricsCollector := metrics.NewCollector("sidecar-proxy")

	// Initialize auto-learner via its constructor so domains/config/logger/
	// validator/rateLimiter are never nil (a zero-value &autolearn.AutoLearner{}
	// used to panic in LearnDomain and made admin/metrics endpoints
	// indistinguishable from "disabled").
	//
	// LearningEnabled is intentionally false: SSRF allowlists must stay
	// static/reviewed-only (see .claude/rules/security-boundaries.md) — no
	// destination is ever added to the allowlist through auto-learning.
	autoLearnConfig := &autolearn.Config{
		MaxDomains:       100,
		LearningEnabled:  false,
		SecurityLevel:    "strict",
		RateLimitPerHour: 10,
		CooldownMinutes:  60,
	}
	autoLearner, err := autolearn.NewAutoLearner(autoLearnConfig, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize auto-learner: %w", err)
	}
	if autoLearnConfig.LearningEnabled {
		logger.Printf("autolearn_enabled: dynamic domain learning is ACTIVE (max_domains=%d, rate_limit_per_hour=%d)",
			autoLearnConfig.MaxDomains, autoLearnConfig.RateLimitPerHour)
	} else {
		logger.Printf("autolearn_disabled: dynamic domain learning is OFF by design; allowlist is static/reviewed-only")
	}

	// Initialize dynamic DNS resolver (オンメモリDNS管理)
	dynamicDNS := dns.NewDynamicResolver(
		cfg.AllowedDomains,
		cfg.DNSServers,
		cfg.DNSCacheTimeout,
		cfg.DNSMaxCacheEntries,
	)

	return &LightweightProxy{
		config:            cfg,
		httpClient:        httpClient,
		connectHTTPClient: connectHTTPClient,
		dnsResolver:       dnsResolver,
		logger:            logger,
		metrics:           metricsCollector,
		autoLearner:       autoLearner,
		dynamicDNS:        dynamicDNS,
		shutdownChan:      make(chan struct{}),
		reqSlots:          make(chan struct{}, cfg.MaxConcurrentReqs),
	}, nil
}

// Start initializes and starts the proxy server
// This is the main entry point for the proxy service
func (p *LightweightProxy) Start() error {
	// Setup HTTP handlers
	mux := http.NewServeMux()

	// Main proxy routing - this is where the magic happens
	mux.HandleFunc("/", p.handleRawRequest)

	// Create HTTP server
	p.server = &http.Server{
		Addr:         fmt.Sprintf(":%s", p.config.ListenPort),
		Handler:      mux,
		ReadTimeout:  p.config.ReadTimeout,
		WriteTimeout: p.config.WriteTimeout,
		IdleTimeout:  p.config.IdleTimeout,
	}

	p.ready.Store(true)
	p.logger.Printf("🚀 Lightweight Proxy Sidecar started on port %s", p.config.ListenPort)
	p.logger.Printf("   Envoy upstream: %s", p.config.EnvoyUpstream)
	p.logger.Printf("   DNS servers: %v", p.config.DNSServers)

	// Start graceful shutdown handler in background
	go p.setupGracefulShutdown()

	// Start the server
	if err := p.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server failed to start: %w", err)
	}

	return nil
}

// Stop gracefully shuts down the proxy server
func (p *LightweightProxy) Stop() error {
	p.ready.Store(false)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	return p.server.Shutdown(ctx)
}

// handleRawRequest is the main routing handler - this is the core of the proxy
// All requests flow through this function for proper routing to specialized handlers
func (p *LightweightProxy) handleRawRequest(w http.ResponseWriter, r *http.Request) {
	if r.Body != nil && p.config.MaxRequestSize > 0 {
		r.Body = http.MaxBytesReader(w, r.Body, p.config.MaxRequestSize)
	}

	select {
	case p.reqSlots <- struct{}{}:
		defer func() { <-p.reqSlots }()
	default:
		http.Error(w, "Too Many Requests", http.StatusServiceUnavailable)
		return
	}

	// Route based on method and path
	switch r.Method {
	case http.MethodConnect:
		// CONNECT method for HTTPS tunneling (news-creator, ollama)
		p.HandleCONNECTRequest(w, r)

	case http.MethodGet, http.MethodPost:
		// Handle different path patterns
		switch {
		case strings.HasPrefix(r.URL.Path, "/proxy/"):
			// RSS feed proxy requests (main functionality)
			p.HandleProxyRequest(w, r)

		case strings.HasPrefix(r.URL.Path, "/connect/"):
			// Persistent tunnel requests
			p.HandlePersistentTunnelRequest(w, r)

		case r.URL.Path == "/health":
			p.HandleHealthCheck(w, r)
		case r.URL.Path == "/ready":
			p.HandleReadinessCheck(w, r)
		case r.URL.Path == "/metrics":
			p.HandleMetrics(w, r)
		case r.URL.Path == "/debug/dns":
			p.HandleDNSDebug(w, r)
		case r.URL.Path == "/debug/config":
			p.HandleConfigDebug(w, r)
		case r.URL.Path == "/admin/autolearn":
			p.HandleAutoLearnAdmin(w, r)
		case r.URL.Path == "/metrics/autolearn":
			p.HandleAutoLearnMetrics(w, r)

		default:
			http.Error(w, "Not Found", http.StatusNotFound)
		}

	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}
