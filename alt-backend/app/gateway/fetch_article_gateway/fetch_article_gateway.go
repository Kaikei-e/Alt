package fetch_article_gateway

import (
	"alt/utils/rate_limiter"
	"alt/utils/security"
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/net/html/charset"
	"golang.org/x/text/transform"
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
			return nil, fmt.Errorf("rate limit wait failed for %q: %w", articleURL, err)
		}
	}

	// Parse and validate URL to guard against SSRF
	parsedURL, err := url.Parse(articleURL)
	if err != nil {
		return nil, fmt.Errorf("parse url failed for %q: %w", articleURL, err)
	}
	// SSRF Protection: Comprehensive multi-layer validation performed here
	// - Validates URL scheme (HTTP/HTTPS only)
	// - Blocks cloud metadata endpoints (AWS/GCP/Azure/etc)
	// - Blocks private IP ranges (RFC1918: 10.x, 172.16-31.x, 192.168.x)
	// - Blocks loopback and link-local addresses
	// - Validates DNS resolution to prevent DNS rebinding attacks
	// - Blocks internal domain suffixes (.local, .internal, .cluster.local, etc)
	// - Validates ports (only 80, 443, 8080, 8443 allowed)
	// - Prevents path traversal and Unicode/Punycode bypass attacks
	if err := g.ssrfValidator.ValidateURL(ctx, parsedURL); err != nil {
		return nil, fmt.Errorf("ssrf validation failed for %q: %w", parsedURL.String(), err)
	}

	// Build request with context and safe client
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsedURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create request failed for %q: %w", parsedURL.String(), err)
	}
	req.Header.Set("User-Agent", "Alt-Article-Fetcher/1.0")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	// Do NOT set Accept-Encoding manually to allow Go transport to auto-decompress

	// SSRF Protection: The httpClient used here is a secure client created by SSRFValidator
	// that performs connection-time IP validation (second layer defense against DNS rebinding)
	// and blocks all HTTP redirects to prevent redirect-based SSRF attacks.
	// See: SSRFValidator.CreateSecureHTTPClient() for implementation details.
	// codeql[go/request-forgery] - False positive: URL is validated by SSRFValidator before this call
	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request failed for %q: %w", parsedURL.String(), err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			// Log but don't fail - response has been processed
			_ = closeErr
		}
	}()

	// Validate HTTP status
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("unexpected status code %d for %q", resp.StatusCode, parsedURL.String())
	}

	// Decode body to UTF-8 to prevent mojibake
	reader := bufio.NewReader(resp.Body)
	// Try to peek up to 1024 bytes; EOF can be acceptable when the body is shorter
	peek, err := reader.Peek(1024)
	if err != nil && err != io.EOF && err != bufio.ErrBufferFull {
		return nil, fmt.Errorf("peek response body failed for %q: %w", parsedURL.String(), err)
	}
	// DetermineEncoding never returns an error; second return is 'certain bool'
	// If we couldn't peek any bytes, pass empty slice which falls back to UTF-8
	enc, _, _ := charset.DetermineEncoding(peek, resp.Header.Get("Content-Type"))
	utf8Reader := transform.NewReader(reader, enc.NewDecoder())

	body, err := io.ReadAll(utf8Reader)
	if err != nil {
		return nil, fmt.Errorf("read response body failed for %q: %w", parsedURL.String(), err)
	}
	content := string(body)
	return &content, nil
}
