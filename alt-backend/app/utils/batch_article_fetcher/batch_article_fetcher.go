package batch_article_fetcher

import (
	"alt/gateway/fetch_article_gateway"
	"alt/utils/html_parser"
	"alt/utils/rate_limiter"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

// FetchResult represents the result of fetching a single article
type FetchResult struct {
	Content string
	Title   string
	ID      string
	Error   error
}

// GatewayFactory is a function type for creating FetchArticleGateway instances
// This allows dependency injection for testing
type GatewayFactory func() *fetch_article_gateway.FetchArticleGateway

// BatchArticleFetcher handles batch fetching of articles with domain-based rate limiting
type BatchArticleFetcher struct {
	rateLimiter    *rate_limiter.HostRateLimiter
	httpClient     *http.Client
	gatewayFactory GatewayFactory
}

// NewBatchArticleFetcher creates a new BatchArticleFetcher
func NewBatchArticleFetcher(rateLimiter *rate_limiter.HostRateLimiter, httpClient *http.Client) *BatchArticleFetcher {
	return &BatchArticleFetcher{
		rateLimiter: rateLimiter,
		httpClient:  httpClient,
		gatewayFactory: func() *fetch_article_gateway.FetchArticleGateway {
			return fetch_article_gateway.NewFetchArticleGateway(rateLimiter, httpClient)
		},
	}
}

// NewBatchArticleFetcherWithFactory creates a new BatchArticleFetcher with a custom gateway factory
// This is useful for testing with custom SSRF validators
func NewBatchArticleFetcherWithFactory(rateLimiter *rate_limiter.HostRateLimiter, httpClient *http.Client, factory GatewayFactory) *BatchArticleFetcher {
	return &BatchArticleFetcher{
		rateLimiter:    rateLimiter,
		httpClient:     httpClient,
		gatewayFactory: factory,
	}
}

// FetchMultiple fetches multiple articles with domain-based rate limiting and queuing
// URLs are grouped by domain, and requests to the same domain are queued with 5-second intervals
// Different domains are processed in parallel
func (b *BatchArticleFetcher) FetchMultiple(ctx context.Context, urls []string) map[string]*FetchResult {
	// Group URLs by domain
	domainGroups := b.groupByDomain(urls)

	// Create result map
	results := make(map[string]*FetchResult, len(urls))
	var mu sync.Mutex

	// Process each domain group in parallel
	var wg sync.WaitGroup
	for domain, urlList := range domainGroups {
		wg.Add(1)
		go func(d string, urls []string) {
			defer wg.Done()
			b.processDomainGroup(ctx, d, urls, results, &mu)
		}(domain, urlList)
	}

	wg.Wait()
	return results
}

// groupByDomain groups URLs by their domain
func (b *BatchArticleFetcher) groupByDomain(urls []string) map[string][]string {
	groups := make(map[string][]string)

	for _, urlStr := range urls {
		parsedURL, err := url.Parse(urlStr)
		if err != nil {
			// Skip invalid URLs, they will be handled in FetchResult
			continue
		}

		domain := parsedURL.Host
		if domain == "" {
			continue
		}

		groups[domain] = append(groups[domain], urlStr)
	}

	return groups
}

// processDomainGroup processes all URLs for a single domain sequentially with rate limiting
func (b *BatchArticleFetcher) processDomainGroup(ctx context.Context, domain string, urls []string, results map[string]*FetchResult, mu *sync.Mutex) {
	// Create a gateway instance for fetching using the factory
	gateway := b.gatewayFactory()

	for _, urlStr := range urls {
		// Fetch article content (HTML)
		htmlContent, err := gateway.FetchArticleContents(ctx, urlStr)

		// Extract title and text from HTML (matching fetchArticleContent behavior)
		var title string
		var extractedText string
		var articleID string
		if err == nil && htmlContent != nil {
			htmlContentStr := *htmlContent
			// Extract title from HTML using html_parser
			title = html_parser.ExtractTitle(htmlContentStr)

			// Extract text content from HTML (save only text, not full HTML)
			extractedText = html_parser.ExtractArticleText(htmlContentStr)
			if extractedText == "" {
				// If extraction fails, set error
				err = fmt.Errorf("failed to extract article text from HTML")
			}

			articleID = b.generateArticleID(urlStr)
		}

		// Store result
		mu.Lock()
		results[urlStr] = &FetchResult{
			Content: extractedText,
			Title:   title,
			ID:      articleID,
			Error:   err,
		}
		mu.Unlock()
	}
}

// generateArticleID generates a simple article ID from URL
// This matches the behavior of generateArticleID in rest/utils.go
func (b *BatchArticleFetcher) generateArticleID(urlStr string) string {
	return fmt.Sprintf("article_%s", strings.ReplaceAll(urlStr, "/", "_"))
}
