package fetch_article_gateway

import (
	"alt/domain"
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

// maxArticleBodyBytes bounds how much of an article body is read into memory.
// The transport transparently decompresses gzip (see the Accept-Encoding note in
// FetchArticleContents), so without a ceiling a few KB on the wire can expand into
// GBs resident — the same decompression-bomb guard image_fetch_gateway applies
// via io.LimitReader (ADR-000702).
const maxArticleBodyBytes = 10 * 1024 * 1024

type FetchArticleGateway struct {
	rateLimiter   *rate_limiter.HostRateLimiter
	httpClient    *http.Client
	ssrfValidator *security.SSRFValidator
	fetchSem      chan struct{}
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
		fetchSem:      make(chan struct{}, 3),
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
		fetchSem:      make(chan struct{}, 3),
	}
}

func (g *FetchArticleGateway) FetchArticleContents(ctx context.Context, articleURL string) (*string, error) {
	// Acquire global fetch semaphore to limit concurrent external fetches
	if g.fetchSem != nil {
		select {
		case g.fetchSem <- struct{}{}:
			defer func() { <-g.fetchSem }()
		case <-ctx.Done():
			return nil, fmt.Errorf("semaphore wait cancelled for %q: %w", articleURL, ctx.Err())
		}
	}

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
	// SSRF: validate + reconstruct request URL (scheme/host/path/query only).
	safeURL, err := g.ssrfValidator.CanonicalRequestURL(ctx, parsedURL)
	if err != nil {
		return nil, fmt.Errorf("ssrf validation failed for %q: %w", parsedURL.String(), err)
	}

	// Build request with context and safe client
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, safeURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request failed for %q: %w", safeURL, err)
	}
	req.Header.Set("User-Agent", "Alt-Article-Fetcher/1.0")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	// Do NOT set Accept-Encoding manually to allow Go transport to auto-decompress

	// SSRF: CanonicalRequestURL + CreateSecureHTTPClient (connection-time IP + redirect checks).
	// codeql[go/request-forgery] - URL reconstructed by SSRFValidator.CanonicalRequestURL
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
		return nil, &domain.ExternalHTTPError{StatusCode: resp.StatusCode, URL: parsedURL.String()}
	}

	// Reject bodies that already advertise more than the ceiling before reading them.
	if resp.ContentLength > maxArticleBodyBytes {
		return nil, fmt.Errorf("response body exceeds %d bytes for %q: content length %d", maxArticleBodyBytes, parsedURL.String(), resp.ContentLength)
	}

	// Bound the body before decoding: the transport hands us a decompressed stream,
	// so the ceiling has to apply here rather than to the bytes on the wire.
	// +1 byte of budget so an exhausted limit is distinguishable from an exact fit.
	limitedBody := &io.LimitedReader{R: resp.Body, N: maxArticleBodyBytes + 1}

	// Decode body to UTF-8 to prevent mojibake
	reader := bufio.NewReader(limitedBody)
	// Try to peek up to 1024 bytes; EOF can be acceptable when the body is shorter
	peek, err := reader.Peek(1024)
	if err != nil && err != io.EOF && err != bufio.ErrBufferFull {
		return nil, fmt.Errorf("peek response body failed for %q: %w", parsedURL.String(), err)
	}
	// DetermineEncoding never returns an error; second return is 'certain bool'
	// If we couldn't peek any bytes, pass empty slice which falls back to UTF-8
	enc, _, _ := charset.DetermineEncoding(peek, resp.Header.Get("Content-Type"))
	utf8Reader := transform.NewReader(reader, enc.NewDecoder())

	// Bound the decoded bytes too: an attacker-chosen charset expands past the
	// pre-decode ceiling (a Shift-JIS/EUC-JP/GB18030 single byte becomes a 3-byte
	// UTF-8 rune), so a body that fits under limitedBody can still triple here.
	limitedContent := &io.LimitedReader{R: utf8Reader, N: maxArticleBodyBytes + 1}

	body, err := io.ReadAll(limitedContent)
	if err != nil {
		return nil, fmt.Errorf("read response body failed for %q: %w", parsedURL.String(), err)
	}
	// A budget is exhausted only when the body was larger than the ceiling.
	if limitedBody.N <= 0 {
		return nil, fmt.Errorf("response body exceeds %d bytes for %q", maxArticleBodyBytes, parsedURL.String())
	}
	if limitedContent.N <= 0 {
		return nil, fmt.Errorf("decoded response body exceeds %d bytes for %q", maxArticleBodyBytes, parsedURL.String())
	}
	content := string(body)
	return &content, nil
}
