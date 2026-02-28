package register_feed_gateway

import (
	"alt/domain"
	"alt/driver/alt_db"
	"alt/utils/constants"
	"alt/utils/errors"
	"alt/utils/logger"
	"alt/utils/metrics"
	"alt/utils/proxy"
	"alt/utils/resilience"
	"alt/utils/security"
	"context"
	"crypto/tls"
	stderrors "errors"
	"fmt"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mmcdole/gofeed"
)

// ProxyConfig holds proxy configuration
type ProxyConfig struct {
	ProxyURL string
	Enabled  bool
}

// getProxyConfigFromEnv retrieves proxy configuration from environment variables
func getProxyConfigFromEnv() *ProxyConfig {
	proxyURL := os.Getenv("HTTP_PROXY")
	if proxyURL == "" {
		proxyURL = "http://nginx-external.alt-ingress.svc.cluster.local:8888"
	}

	proxyEnabled := os.Getenv("PROXY_ENABLED")
	enabled := proxyEnabled == "true"

	return &ProxyConfig{
		ProxyURL: proxyURL,
		Enabled:  enabled,
	}
}

// EnvoyProxyConfig holds Envoy proxy configuration
type EnvoyProxyConfig struct {
	EnvoyURL string
	Enabled  bool
}

// getEnvoyProxyConfigFromEnv retrieves proxy configuration from environment variables
// REFACTORED: Now uses flexible proxy strategy pattern
func getEnvoyProxyConfigFromEnv() *EnvoyProxyConfig {
	strategy := proxy.GetStrategy()

	// Convert strategy to legacy EnvoyProxyConfig for backward compatibility
	if !strategy.Enabled {
		return &EnvoyProxyConfig{
			EnvoyURL: "",
			Enabled:  false,
		}
	}

	return &EnvoyProxyConfig{
		EnvoyURL: strategy.BaseURL,
		Enabled:  strategy.Enabled,
	}
}

// RSSFeedFetcher interface for mocking RSS feed fetching
type RSSFeedFetcher interface {
	FetchRSSFeed(ctx context.Context, link string) (*gofeed.Feed, error)
}

// DefaultRSSFeedFetcher implements RSSFeedFetcher with actual HTTP requests
type DefaultRSSFeedFetcher struct {
	proxyConfig      *ProxyConfig
	envoyProxyConfig *EnvoyProxyConfig
	proxyStrategy    *proxy.Strategy
}

// NewDefaultRSSFeedFetcher creates a new DefaultRSSFeedFetcher with proxy configuration
func NewDefaultRSSFeedFetcher() *DefaultRSSFeedFetcher {
	strategy := proxy.GetStrategy()
	return &DefaultRSSFeedFetcher{
		proxyConfig:      getProxyConfigFromEnv(),
		envoyProxyConfig: getEnvoyProxyConfigFromEnv(),
		proxyStrategy:    strategy,
	}
}

// createHTTPClient creates an HTTP client with HTTPS direct connection (ROOT FIX)
func (f *DefaultRSSFeedFetcher) createHTTPClient(ctx context.Context) *http.Client {
	transport := &http.Transport{
		// ROOT FIX: 企業環境のHTTPSアクセス最適化
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: false,
			MinVersion:         tls.VersionTLS12,
		},
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:        100,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 30 * time.Second,
	}

	// ULTRATHINK ROOT FIX: nginx-external proxyがCONNECTメソッド未サポートのため直接HTTPS接続使用
	if f.proxyConfig != nil && f.proxyConfig.Enabled && f.proxyConfig.ProxyURL != "" {
		logger.SafeInfoContext(ctx, "Using direct HTTPS connection due to proxy CONNECT method limitation",
			"proxy_enabled", f.proxyConfig.Enabled,
			"proxy_url", f.proxyConfig.ProxyURL,
			"reason", "nginx-external does not support CONNECT method for HTTPS tunneling")
		// NOTE: transport.Proxy = http.ProxyURL(proxyURL) を一時的に無効化
		// HTTPS URLs取得時にCONNECT method失敗（400 Bad Request）を回避
	}

	// Wrap transport with Envoy proxy Host header fixer using shared proxy package
	roundTripper := proxy.WrapTransportForProxy(transport, f.proxyStrategy)

	return &http.Client{
		Timeout:   60 * time.Second, // タイムアウト延長
		Transport: roundTripper,
	}
}

func (f *DefaultRSSFeedFetcher) FetchRSSFeed(ctx context.Context, link string) (*gofeed.Feed, error) {
	// ISSUE_RESOLVE_PLAN.md ROOT SOLUTION: Use proxy-sidecar exclusively
	// This eliminates upstream="10.96.32.212:8080" and achieves upstream="zenn.dev:443"

	// Safety check to prevent panic in tests with incomplete initialization
	if f.proxyStrategy == nil {
		logger.SafeWarnContext(ctx, "Proxy strategy not initialized, using direct connection")
		return f.fetchRSSFeedWithRetry(ctx, link)
	}

	logger.SafeInfoContext(ctx, "DEBUG: Proxy strategy configuration check",
		"strategy_mode", string(f.proxyStrategy.Mode),
		"strategy_enabled", f.proxyStrategy.Enabled,
		"strategy_base_url", f.proxyStrategy.BaseURL,
		"strategy_path_template", f.proxyStrategy.PathTemplate)

	// ROOT SOLUTION: Use strategic proxy configuration based on environment
	if f.proxyStrategy.Enabled {
		proxyURL := proxy.ConvertToProxyURLWithContext(ctx, link, f.proxyStrategy)

		// Extract expected upstream from original URL (without port 443 for HTTPS)
		u, _ := url.Parse(link)
		expectedUpstream := u.Host

		logger.SafeInfoContext(ctx, "Using strategic proxy for RSS fetching",
			"strategy_mode", string(f.proxyStrategy.Mode),
			"original_url", link,
			"proxy_url", proxyURL,
			"expected_upstream", expectedUpstream)

		return f.fetchRSSFeedWithRetry(ctx, proxyURL)
	}

	// Fallback: Direct connection when no proxy is configured
	logger.SafeInfoContext(ctx, "Using direct RSS feed connection (no proxy configured)",
		"original_url", link)

	return f.fetchRSSFeedWithRetry(ctx, link)
}

// isRetryableError determines if an error should trigger a retry
func isRetryableError(err error) bool {
	errStr := err.Error()
	return strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "deadline exceeded") ||
		strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "no such host") ||
		strings.Contains(errStr, "502") ||
		strings.Contains(errStr, "503") ||
		strings.Contains(errStr, "504")
}

// fetchRSSFeedWithRetry performs RSS feed fetching with exponential backoff retry
func (f *DefaultRSSFeedFetcher) fetchRSSFeedWithRetry(ctx context.Context, link string) (*gofeed.Feed, error) {
	// Create HTTP client with proxy configuration if enabled
	httpClient := f.createHTTPClient(ctx)

	fp := gofeed.NewParser()
	fp.Client = httpClient
	fp.UserAgent = "Alt-RSS-Reader/1.0 (+https://alt.example.com)"

	// Use context with extended 60 second timeout for additional protection
	feedCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	var lastErr error
	for attempt := 0; attempt < constants.DefaultMaxRetries; attempt++ {
		if attempt > 0 {
			// Calculate exponential backoff delay
			delay := time.Duration(float64(constants.DefaultInitialDelay) * math.Pow(2, float64(attempt-1)))
			if delay > constants.DefaultMaxDelay {
				delay = constants.DefaultMaxDelay
			}

			logger.SafeInfoContext(feedCtx, "Retrying RSS feed fetch",
				"url", link,
				"attempt", attempt+1,
				"delay_seconds", delay.Seconds())

			select {
			case <-time.After(delay):
			case <-feedCtx.Done():
				return nil, feedCtx.Err()
			}
		}

		feed, err := fp.ParseURLWithContext(link, feedCtx)
		if err == nil {
			if attempt > 0 {
				logger.SafeInfoContext(feedCtx, "RSS feed fetch succeeded after retry",
					"url", link,
					"attempts", attempt+1)
			}
			return feed, nil
		}

		lastErr = err
		if !isRetryableError(err) {
			logger.SafeWarnContext(feedCtx, "Non-retryable error, not retrying",
				"url", link,
				"error", err.Error())
			break
		}

		logger.SafeWarnContext(feedCtx, "RSS feed fetch failed, will retry",
			"url", link,
			"attempt", attempt+1,
			"error", err.Error())
	}

	return nil, lastErr
}

type RegisterFeedGateway struct {
	alt_db           *alt_db.AltDBRepository
	feedFetcher      RSSFeedFetcher
	urlValidator     *security.URLSecurityValidator
	circuitBreaker   *resilience.SimpleCircuitBreaker
	metricsCollector *metrics.BasicMetricsCollector
}

func NewRegisterFeedLinkGateway(pool *pgxpool.Pool) *RegisterFeedGateway {
	return &RegisterFeedGateway{
		alt_db:           alt_db.NewAltDBRepositoryWithPool(pool),
		feedFetcher:      NewDefaultRSSFeedFetcher(),
		urlValidator:     security.NewURLSecurityValidator(),
		circuitBreaker:   resilience.NewSimpleCircuitBreaker(resilience.DefaultCircuitBreakerConfig()),
		metricsCollector: metrics.NewBasicMetricsCollector(),
	}
}

// NewRegisterFeedLinkGatewayWithFetcher creates a gateway with a custom RSS feed fetcher (for testing)
func NewRegisterFeedLinkGatewayWithFetcher(pool *pgxpool.Pool, fetcher RSSFeedFetcher) *RegisterFeedGateway {
	return &RegisterFeedGateway{
		alt_db:           alt_db.NewAltDBRepositoryWithPool(pool),
		feedFetcher:      fetcher,
		urlValidator:     security.NewURLSecurityValidator(),
		circuitBreaker:   resilience.NewSimpleCircuitBreaker(resilience.DefaultCircuitBreakerConfig()),
		metricsCollector: metrics.NewBasicMetricsCollector(),
	}
}

func (g *RegisterFeedGateway) RegisterRSSFeedLink(ctx context.Context, link string) error {
	start := time.Now()

	// SECURITY INTEGRATION: Comprehensive URL validation using URLSecurityValidator
	if g.urlValidator != nil {
		if err := g.urlValidator.ValidateRSSURL(link); err != nil {
			g.metricsCollector.RecordFailure()
			g.metricsCollector.RecordResponseTime(time.Since(start))
			logger.SafeWarnContext(ctx, "URL security validation failed", "url", link, "error", err.Error())
			return err
		}

		// Additional RSS-specific validation
		if err := g.urlValidator.ValidateForRSSFeed(link); err != nil {
			g.metricsCollector.RecordFailure()
			g.metricsCollector.RecordResponseTime(time.Since(start))
			logger.SafeWarnContext(ctx, "RSS-specific validation failed", "url", link, "error", err.Error())
			return err
		}
	}

	// CIRCUIT BREAKER INTEGRATION: Protect against service failures
	err := g.circuitBreaker.Execute(ctx, func() error {
		// Parse and validate the URL (basic validation)
		parsedURL, err := url.Parse(link)
		if err != nil {
			return stderrors.New("invalid URL format")
		}

		// Ensure the URL has a scheme
		if parsedURL.Scheme == "" {
			return stderrors.New("URL must include a scheme (http or https)")
		}

		// Try to fetch and parse the RSS feed with retry mechanism
		feed, err := g.feedFetcher.FetchRSSFeed(ctx, link)
		if err != nil {
			errStr := err.Error()
			// Check for connection errors
			if strings.Contains(errStr, "no such host") || strings.Contains(errStr, "connection refused") {
				logger.SafeErrorContext(ctx, "RSS feed connection error", "url", link, "error", errStr, "error_type", "connection_error")
				return stderrors.New("could not reach the RSS feed URL")
			}
			// Check for timeout errors
			if strings.Contains(errStr, "timeout") || strings.Contains(errStr, "deadline exceeded") {
				logger.SafeErrorContext(ctx, "RSS feed timeout error", "url", link, "error", errStr, "error_type", "timeout")
				return stderrors.New("RSS feed fetch timeout - server took too long to respond")
			}
			// Check for 404 Not Found errors
			if strings.Contains(errStr, "404") || strings.Contains(errStr, "Not Found") || strings.Contains(errStr, "not found") {
				logger.SafeErrorContext(ctx, "RSS feed not found (404)", "url", link, "error", errStr, "error_type", "http_404", "http_status_code", 404)
				return stderrors.New("RSS feed not found (404)")
			}
			// Check for 403 Forbidden errors
			if strings.Contains(errStr, "403") || strings.Contains(errStr, "Forbidden") || strings.Contains(errStr, "forbidden") {
				logger.SafeErrorContext(ctx, "RSS feed access forbidden (403)", "url", link, "error", errStr, "error_type", "http_403", "http_status_code", 403)
				return stderrors.New("RSS feed access forbidden (403)")
			}
			// Check for 400 Bad Request errors
			if strings.Contains(errStr, "400") || strings.Contains(errStr, "Bad request") || strings.Contains(errStr, "bad request") {
				logger.SafeErrorContext(ctx, "RSS feed bad request (400)", "url", link, "error", errStr, "error_type", "http_400", "http_status_code", 400)
				return stderrors.New("RSS feed URL returned bad request (400)")
			}
			// Check for redirect loop errors
			if strings.Contains(errStr, "stopped after") && strings.Contains(errStr, "redirects") {
				logger.SafeErrorContext(ctx, "RSS feed redirect loop", "url", link, "error", errStr, "error_type", "redirect_loop")
				return stderrors.New("RSS feed URL redirects too many times")
			}
			// Check for TLS certificate errors
			if strings.Contains(errStr, "tls: failed to verify certificate") || strings.Contains(errStr, "x509: certificate is valid for") {
				suggestedURL := extractSuggestedURLFromCertError(errStr, link)
				context := map[string]interface{}{
					"original_url": link,
					"error_type":   "tls_certificate",
				}
				if suggestedURL != "" {
					context["suggested_url"] = suggestedURL
				}
				return errors.NewTLSCertificateContextError(
					buildTLSErrorMessage(suggestedURL),
					"gateway",
					"RegisterFeedGateway",
					"RegisterRSSFeedLink",
					err,
					context,
				)
			}
			// Default error for unrecognized errors
			logger.SafeErrorContext(ctx, "RSS feed format error", "url", link, "error", errStr, "error_type", "format_error")
			return stderrors.New("invalid RSS feed format")
		}

		if feed.Link == "" {
			logger.SafeWarnContext(ctx, "RSS feed link is empty, using the link from the RSS feed", "link", link)
			feed.Link = link
		}

		if feed.FeedLink == "" {
			logger.SafeWarnContext(ctx, "RSS feed feed link is empty, using the link from the RSS feed", "link", feed.Link)
			feed.FeedLink = link
		}

		// Check database connection only after RSS feed validation
		if g.alt_db == nil {
			return stderrors.New("database connection not available")
		}

		err = g.alt_db.RegisterRSSFeedLink(ctx, feed.FeedLink)
		if err != nil {
			if stderrors.Is(err, pgx.ErrTxClosed) {
				logger.SafeErrorContext(ctx, "Failed to register RSS feed link", "error", err)
				return stderrors.New("failed to register RSS feed link")
			}
			logger.SafeErrorContext(ctx, "Error registering RSS feed link", "error", err)
			return stderrors.New("failed to register RSS feed link")
		}
		logger.SafeInfoContext(ctx, "RSS feed link registered", "link", link)

		// Auto-subscribe the authenticated user to the newly registered feed link.
		// This is best-effort: failures are logged but do not fail the registration.
		g.autoSubscribeUser(ctx, feed.FeedLink)

		return nil
	})

	// METRICS INTEGRATION: Record operation results and response time
	responseTime := time.Since(start)
	g.metricsCollector.RecordResponseTime(responseTime)

	if err != nil {
		g.metricsCollector.RecordFailure()
		logger.SafeErrorContext(ctx, "RSS feed registration failed", "url", link, "error", err.Error(), "response_time", responseTime)
		return err
	}

	g.metricsCollector.RecordSuccess()
	logger.SafeInfoContext(ctx, "RSS feed registration successful", "url", link, "response_time", responseTime)
	return nil
}

// autoSubscribeUser subscribes the authenticated user to a feed link.
// This is best-effort: if user context is unavailable or the DB call fails,
// the error is logged but does not propagate.
func (g *RegisterFeedGateway) autoSubscribeUser(ctx context.Context, feedLinkURL string) {
	userCtx, err := domain.GetUserFromContext(ctx)
	if err != nil {
		logger.SafeInfoContext(ctx, "Skipping auto-subscribe: no user context", "error", err)
		return
	}

	feedLinkIDStr, err := g.alt_db.FetchFeedLinkIDByURL(ctx, feedLinkURL)
	if err != nil || feedLinkIDStr == nil {
		logger.SafeWarnContext(ctx, "Skipping auto-subscribe: feed_link_id not found", "url", feedLinkURL, "error", err)
		return
	}

	feedLinkID, err := uuid.Parse(*feedLinkIDStr)
	if err != nil {
		logger.SafeWarnContext(ctx, "Skipping auto-subscribe: invalid feed_link_id", "feed_link_id", *feedLinkIDStr, "error", err)
		return
	}

	if err := g.alt_db.InsertSubscription(ctx, userCtx.UserID, feedLinkID); err != nil {
		logger.SafeWarnContext(ctx, "Auto-subscribe failed", "user_id", userCtx.UserID, "feed_link_id", feedLinkID, "error", err)
		return
	}

	logger.SafeInfoContext(ctx, "User auto-subscribed to feed link", "user_id", userCtx.UserID, "feed_link_id", feedLinkID, "url", feedLinkURL)
}

// GetMetrics returns the current metrics collector for monitoring and testing
func (g *RegisterFeedGateway) GetMetrics() *metrics.BasicMetricsCollector {
	return g.metricsCollector
}

// extractSuggestedURLFromCertError extracts a suggested URL from TLS certificate error message
// Example error: "x509: certificate is valid for aar.art-it.asia, www.art-it.asia, not art-it.asia"
// This function extracts the first valid domain and constructs a suggested URL
func extractSuggestedURLFromCertError(errStr, originalURL string) string {
	// Pattern to match: "certificate is valid for domain1, domain2, ..."
	re := regexp.MustCompile(`certificate is valid for\s+([^,]+(?:,\s*[^,]+)*)`)
	matches := re.FindStringSubmatch(errStr)
	if len(matches) < 2 {
		return ""
	}

	// Extract the first valid domain (remove leading/trailing spaces)
	validDomains := strings.Split(matches[1], ",")
	if len(validDomains) == 0 {
		return ""
	}

	firstValidDomain := strings.TrimSpace(validDomains[0])
	if firstValidDomain == "" {
		return ""
	}

	// Parse original URL to preserve scheme and path
	parsedURL, err := url.Parse(originalURL)
	if err != nil {
		// If parsing fails, construct a simple URL with the valid domain
		return fmt.Sprintf("https://%s", firstValidDomain)
	}

	// Construct suggested URL with the same scheme and path, but with valid domain
	suggestedURL := fmt.Sprintf("%s://%s%s", parsedURL.Scheme, firstValidDomain, parsedURL.Path)
	if parsedURL.RawQuery != "" {
		suggestedURL += "?" + parsedURL.RawQuery
	}

	return suggestedURL
}

// buildTLSErrorMessage builds a user-friendly error message for TLS certificate errors
func buildTLSErrorMessage(suggestedURL string) string {
	if suggestedURL != "" {
		return fmt.Sprintf("このURLの証明書に問題があります。%s を試してください", suggestedURL)
	}
	return "このURLの証明書に問題があります。別のURLを試してください"
}
