package fetch_article_gateway

import (
	"alt/utils/html_parser"
	"alt/utils/rate_limiter"
	"alt/utils/security"
	"context"
	"io"
	"net/http"
	"net/url"
	"time"
)

type FetchArticleGateway struct {
	rateLimiter   *rate_limiter.HostRateLimiter
	httpClient    *http.Client
	ssrfValidator *security.SSRFValidator
}

func NewFetchArticleGateway(rateLimiter *rate_limiter.HostRateLimiter, httpClient *http.Client) *FetchArticleGateway {
	// Create SSRF validator and wrap the provided client with a secure transport
	validator := security.NewSSRFValidator()
	var timeout time.Duration
	if httpClient != nil && httpClient.Timeout > 0 {
		timeout = httpClient.Timeout
	} else {
		timeout = 30 * time.Second
	}
	secureClient := validator.CreateSecureHTTPClient(timeout)

	return &FetchArticleGateway{
		rateLimiter:   rateLimiter,
		httpClient:    secureClient,
		ssrfValidator: validator,
	}
}

// NewFetchArticleGatewayWithDeps allows dependency injection for testing and advanced configurations.
// If ssrfValidator is nil, a default one is created. If httpClient is nil, a secure client is created.
func NewFetchArticleGatewayWithDeps(rateLimiter *rate_limiter.HostRateLimiter, httpClient *http.Client, ssrfValidator *security.SSRFValidator) *FetchArticleGateway {
	validator := ssrfValidator
	if validator == nil {
		validator = security.NewSSRFValidator()
	}

	client := httpClient
	if client == nil {
		client = validator.CreateSecureHTTPClient(30 * time.Second)
	}

	return &FetchArticleGateway{
		rateLimiter:   rateLimiter,
		httpClient:    client,
		ssrfValidator: validator,
	}
}

func (g *FetchArticleGateway) FetchArticleContents(ctx context.Context, articleURL string) (*string, error) {
	// Rate limit per host
	if g.rateLimiter != nil {
		if err := g.rateLimiter.WaitForHost(ctx, articleURL); err != nil {
			return nil, err
		}
	}

	// Parse and validate URL to guard against SSRF
	parsedURL, err := url.Parse(articleURL)
	if err != nil {
		return nil, err
	}
	if err := g.ssrfValidator.ValidateURL(ctx, parsedURL); err != nil {
		return nil, err
	}

	// Build request with context and safe client
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsedURL.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Alt-Article-Fetcher/1.0")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Encoding", "gzip, deflate")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	content := string(body)
	content = html_parser.StripTags(content)

	return &content, nil
}
