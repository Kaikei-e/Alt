package register_feed_gateway

import (
	"alt/driver/alt_db"
	"alt/utils"
	"alt/utils/constants"
	"alt/utils/logger"
	"alt/utils/proxy"
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
func getEnvoyProxyConfigFromEnv() *EnvoyProxyConfig {
	strategy := proxy.GetStrategy()

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
	httpClient       *http.Client // shared HTTP client with connection pooling
}

// NewDefaultRSSFeedFetcher creates a new DefaultRSSFeedFetcher with proxy configuration
func NewDefaultRSSFeedFetcher() *DefaultRSSFeedFetcher {
	strategy := proxy.GetStrategy()
	f := &DefaultRSSFeedFetcher{
		proxyConfig:      getProxyConfigFromEnv(),
		envoyProxyConfig: getEnvoyProxyConfigFromEnv(),
		proxyStrategy:    strategy,
	}

	// Create shared HTTP client with connection pooling (goroutine-safe)
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: false,
			MinVersion:         tls.VersionTLS12,
		},
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:        200,
		MaxIdleConnsPerHost: 50,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 30 * time.Second,
	}

	roundTripper := proxy.WrapTransportForProxy(transport, f.proxyStrategy)
	f.httpClient = &http.Client{
		Timeout:   60 * time.Second,
		Transport: roundTripper,
	}

	return f
}

func (f *DefaultRSSFeedFetcher) FetchRSSFeed(ctx context.Context, link string) (*gofeed.Feed, error) {
	if f.proxyStrategy == nil {
		logger.SafeWarnContext(ctx, "Proxy strategy not initialized, using direct connection")
		return f.fetchRSSFeedWithRetry(ctx, link)
	}

	logger.SafeInfoContext(ctx, "DEBUG: Proxy strategy configuration check",
		"strategy_mode", string(f.proxyStrategy.Mode),
		"strategy_enabled", f.proxyStrategy.Enabled,
		"strategy_base_url", f.proxyStrategy.BaseURL,
		"strategy_path_template", f.proxyStrategy.PathTemplate)

	if f.proxyStrategy.Enabled {
		proxyURL := proxy.ConvertToProxyURLWithContext(ctx, link, f.proxyStrategy)

		u, _ := url.Parse(link)
		expectedUpstream := u.Host

		logger.SafeInfoContext(ctx, "Using strategic proxy for RSS fetching",
			"strategy_mode", string(f.proxyStrategy.Mode),
			"original_url", link,
			"proxy_url", proxyURL,
			"expected_upstream", expectedUpstream)

		return f.fetchRSSFeedWithRetry(ctx, proxyURL)
	}

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
	fp := gofeed.NewParser()
	fp.Client = f.httpClient
	fp.UserAgent = "Alt-RSS-Reader/1.0 (+https://alt.example.com)"

	feedCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	var lastErr error
	for attempt := 0; attempt < constants.DefaultMaxRetries; attempt++ {
		if attempt > 0 {
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

// RegisterFeedGateway handles DB-only feed link registration.
type RegisterFeedGateway struct {
	alt_db *alt_db.AltDBRepository
}

func NewRegisterFeedLinkGateway(pool *pgxpool.Pool) *RegisterFeedGateway {
	return &RegisterFeedGateway{
		alt_db: alt_db.NewAltDBRepositoryWithPool(pool),
	}
}

// RegisterFeedLink inserts a feed link URL into the database.
// This is a DB-only operation — no external HTTP fetching.
// Tracking parameters (utm_source, fbclid, etc.) are stripped before registration.
func (g *RegisterFeedGateway) RegisterFeedLink(ctx context.Context, link string) error {
	if g.alt_db == nil {
		return stderrors.New("database connection not available")
	}

	sanitized, sanitizeErr := utils.StripTrackingParams(link)
	if sanitizeErr != nil {
		logger.SafeWarnContext(ctx, "Failed to strip tracking params, using original", "error", sanitizeErr)
		sanitized = link
	}

	err := g.alt_db.RegisterRSSFeedLink(ctx, sanitized)
	if err != nil {
		if stderrors.Is(err, pgx.ErrTxClosed) {
			logger.SafeErrorContext(ctx, "Failed to register RSS feed link", "error", err)
			return stderrors.New("failed to register RSS feed link")
		}
		logger.SafeErrorContext(ctx, "Error registering RSS feed link", "error", err)
		return stderrors.New("failed to register RSS feed link")
	}
	logger.SafeInfoContext(ctx, "RSS feed link registered", "link", sanitized)
	return nil
}

// extractSuggestedURLFromCertError extracts a suggested URL from TLS certificate error message
func extractSuggestedURLFromCertError(errStr, originalURL string) string {
	re := regexp.MustCompile(`certificate is valid for\s+([^,]+(?:,\s*[^,]+)*)`)
	matches := re.FindStringSubmatch(errStr)
	if len(matches) < 2 {
		return ""
	}

	validDomains := strings.Split(matches[1], ",")
	if len(validDomains) == 0 {
		return ""
	}

	firstValidDomain := strings.TrimSpace(validDomains[0])
	if firstValidDomain == "" {
		return ""
	}

	parsedURL, err := url.Parse(originalURL)
	if err != nil {
		return fmt.Sprintf("https://%s", firstValidDomain)
	}

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
