package validate_fetch_rss_gateway

import (
	"alt/domain"
	"alt/utils/logger"
	"alt/utils/metrics"
	"alt/utils/rate_limiter"
	"alt/utils/resilience"
	"alt/utils/security"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"alt/orchestrator/gateway/register_feed_gateway"

	"golang.org/x/sync/singleflight"
)

// ValidateAndFetchRSSGateway validates an RSS URL and fetches the feed in a single
// operation. This is the external HTTP boundary for feed registration.
type ValidateAndFetchRSSGateway struct {
	feedFetcher      register_feed_gateway.RSSFeedFetcher
	urlValidator     *security.URLSecurityValidator
	circuitBreaker   *resilience.SimpleCircuitBreaker
	metricsCollector *metrics.BasicMetricsCollector
	fetchSem         chan struct{}
	sfGroup          singleflight.Group // deduplicates concurrent fetches for the same URL
	// rateLimiter throttles the external feed fetch per host (CLAUDE.md rule 2).
	// Injected in production; nil only in the WithFetcher test constructor, whose
	// fetcher is a mock that performs no real external call.
	rateLimiter *rate_limiter.HostRateLimiter
}

// NewValidateAndFetchRSSGateway builds the production gateway. rateLimiter is the
// process-wide host limiter (shared with the harvester's feed collector) so
// feed-registration fetches obey the same per-host 5s floor as scheduled polls.
func NewValidateAndFetchRSSGateway(rateLimiter *rate_limiter.HostRateLimiter) *ValidateAndFetchRSSGateway {
	if rateLimiter == nil {
		panic("validate_fetch_rss_gateway: HostRateLimiter is required (CLAUDE.md rule 2) — wire infra.RateLimiter at composition root")
	}
	semSize := 50
	if v := os.Getenv("FEED_FETCH_CONCURRENCY"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			semSize = n
		}
	}
	return &ValidateAndFetchRSSGateway{
		feedFetcher:      register_feed_gateway.NewDefaultRSSFeedFetcher(),
		urlValidator:     security.NewURLSecurityValidator(),
		circuitBreaker:   resilience.NewSimpleCircuitBreaker(resilience.DefaultCircuitBreakerConfig()),
		metricsCollector: metrics.NewBasicMetricsCollector(),
		fetchSem:         make(chan struct{}, semSize),
		rateLimiter:      rateLimiter,
	}
}

// NewValidateAndFetchRSSGatewayWithFetcher creates a gateway with a custom fetcher (for testing).
// The rate limiter is left nil: the injected fetcher is a mock, so there is no
// real external call to throttle.
func NewValidateAndFetchRSSGatewayWithFetcher(fetcher register_feed_gateway.RSSFeedFetcher) *ValidateAndFetchRSSGateway {
	return &ValidateAndFetchRSSGateway{
		feedFetcher:      fetcher,
		urlValidator:     security.NewURLSecurityValidator(),
		circuitBreaker:   resilience.NewSimpleCircuitBreaker(resilience.DefaultCircuitBreakerConfig()),
		metricsCollector: metrics.NewBasicMetricsCollector(),
		fetchSem:         make(chan struct{}, 50),
	}
}

func (g *ValidateAndFetchRSSGateway) ValidateAndFetch(ctx context.Context, link string) (*domain.ParsedFeed, error) {
	start := time.Now()

	// URL security validation (SSRF check)
	if g.urlValidator != nil {
		if err := g.urlValidator.ValidateRSSURL(link); err != nil {
			g.metricsCollector.RecordFailure()
			g.metricsCollector.RecordResponseTime(time.Since(start))
			logger.SafeWarnContext(ctx, "URL security validation failed", "url", link, "error", err.Error())
			return nil, err
		}
		if err := g.urlValidator.ValidateForRSSFeed(link); err != nil {
			g.metricsCollector.RecordFailure()
			g.metricsCollector.RecordResponseTime(time.Since(start))
			logger.SafeWarnContext(ctx, "RSS-specific validation failed", "url", link, "error", err.Error())
			return nil, err
		}
	}

	// singleflight: deduplicate concurrent fetches for the same URL
	val, err, shared := g.sfGroup.Do(link, func() (interface{}, error) {
		// Semaphore: limit concurrent external fetches
		select {
		case g.fetchSem <- struct{}{}:
			defer func() { <-g.fetchSem }()
		case <-ctx.Done():
			return nil, ctx.Err()
		}

		var result *domain.ParsedFeed

		// Circuit breaker wrapping
		cbErr := g.circuitBreaker.Execute(ctx, func() error {
			parsedURL, err := url.Parse(link)
			if err != nil {
				return errors.New("invalid URL format")
			}
			if parsedURL.Scheme == "" {
				return errors.New("URL must include a scheme (http or https)")
			}

			// Rule 2: throttle the external fetch per host, shared with the
			// harvester's feed collector. Skipped for FEED_ALLOWED_HOSTS hosts —
			// operator-trusted internal/test sources are not third-party publishers
			// the politeness interval protects, the same escape hatch the SSRF guard
			// and CheckRedirect already honor. Also nil under the WithFetcher test
			// constructor (mock fetcher, no real external call).
			if g.rateLimiter != nil && !security.IsFeedHostAllowed(parsedURL.Hostname()) {
				if waitErr := g.rateLimiter.WaitForHost(ctx, link); waitErr != nil {
					return waitErr
				}
			}

			feed, err := g.feedFetcher.FetchRSSFeed(ctx, link)
			if err != nil {
				return classifyFetchError(ctx, link, err)
			}

			feedLink := feed.FeedLink
			if feedLink == "" {
				feedLink = link
			}

			// Convert gofeed items to domain FeedItems
			items := make([]*domain.FeedItem, 0, len(feed.Items))
			for _, item := range feed.Items {
				fi := &domain.FeedItem{
					Title:       item.Title,
					Description: item.Description,
					Link:        item.Link,
					Published:   item.Published,
					Links:       item.Links,
				}
				if item.PublishedParsed != nil {
					fi.PublishedParsed = *item.PublishedParsed
				}
				if item.Author != nil {
					fi.Author = domain.Author{Name: item.Author.Name}
					fi.Authors = []domain.Author{{Name: item.Author.Name}}
				}
				items = append(items, fi)
			}

			result = &domain.ParsedFeed{
				FeedLink: feedLink,
				Items:    items,
			}
			return nil
		})

		if cbErr != nil {
			return nil, cbErr
		}
		return result, nil
	})

	responseTime := time.Since(start)
	g.metricsCollector.RecordResponseTime(responseTime)

	if err != nil {
		g.metricsCollector.RecordFailure()
		logger.SafeErrorContext(ctx, "RSS feed validation+fetch failed", "url", link, "error", err.Error(), "response_time", responseTime)
		return nil, err
	}

	result := val.(*domain.ParsedFeed)
	if shared {
		logger.SafeInfoContext(ctx, "RSS feed fetch deduplicated via singleflight", "url", link, "response_time", responseTime)
	}
	g.metricsCollector.RecordSuccess()
	logger.SafeInfoContext(ctx, "RSS feed validation+fetch successful", "url", link, "items", len(result.Items), "response_time", responseTime)
	return result, nil
}

// classifyFetchError converts raw fetch errors into user-friendly messages.
func classifyFetchError(ctx context.Context, link string, err error) error {
	errStr := err.Error()

	if strings.Contains(errStr, "no such host") || strings.Contains(errStr, "connection refused") {
		logger.SafeErrorContext(ctx, "RSS feed connection error", "url", link, "error", errStr)
		return errors.New("could not reach the RSS feed URL")
	}
	if strings.Contains(errStr, "timeout") || strings.Contains(errStr, "deadline exceeded") {
		logger.SafeErrorContext(ctx, "RSS feed timeout error", "url", link, "error", errStr)
		return errors.New("RSS feed fetch timeout - server took too long to respond")
	}
	if strings.Contains(errStr, "404") || strings.Contains(errStr, "Not Found") || strings.Contains(errStr, "not found") {
		logger.SafeErrorContext(ctx, "RSS feed not found (404)", "url", link, "error", errStr)
		return errors.New("RSS feed not found (404)")
	}
	if strings.Contains(errStr, "403") || strings.Contains(errStr, "Forbidden") || strings.Contains(errStr, "forbidden") {
		logger.SafeErrorContext(ctx, "RSS feed access forbidden (403)", "url", link, "error", errStr)
		return errors.New("RSS feed access forbidden (403)")
	}
	if strings.Contains(errStr, "400") || strings.Contains(errStr, "Bad request") || strings.Contains(errStr, "bad request") {
		logger.SafeErrorContext(ctx, "RSS feed bad request (400)", "url", link, "error", errStr)
		return errors.New("RSS feed URL returned bad request (400)")
	}
	if strings.Contains(errStr, "stopped after") && strings.Contains(errStr, "redirects") {
		logger.SafeErrorContext(ctx, "RSS feed redirect loop", "url", link, "error", errStr)
		return errors.New("RSS feed URL redirects too many times")
	}
	if strings.Contains(errStr, "tls: failed to verify certificate") || strings.Contains(errStr, "x509: certificate is valid for") {
		logger.SafeErrorContext(ctx, "RSS feed TLS certificate error", "url", link, "error", errStr)
		return fmt.Errorf("このURLの証明書に問題があります。別のURLを試してください")
	}

	logger.SafeErrorContext(ctx, "RSS feed format error", "url", link, "error", errStr)
	return errors.New("invalid RSS feed format")
}

// GetMetrics returns the current metrics collector for monitoring and testing.
func (g *ValidateAndFetchRSSGateway) GetMetrics() *metrics.BasicMetricsCollector {
	return g.metricsCollector
}
