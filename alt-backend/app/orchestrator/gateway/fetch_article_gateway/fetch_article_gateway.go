package fetch_article_gateway

import (
	"alt/domain"
	"alt/utils/rate_limiter"
	"alt/utils/security"
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"syscall"
	"time"

	"golang.org/x/net/html/charset"
	"golang.org/x/text/transform"
)

// upstreamErrnos are the connection failures that mean the publisher's server
// is the thing that did not work.
var upstreamErrnos = []error{
	syscall.ECONNREFUSED,
	syscall.ECONNRESET,
	syscall.EHOSTUNREACH,
	syscall.ENETUNREACH,
	syscall.ETIMEDOUT,
	syscall.EPIPE,
}

// isUpstreamUnreachable reports whether err means the request to the publisher
// never completed for reasons outside this process.
//
// It is a positive allowlist on purpose. The secure client's SSRF checks run
// inside the dialer, so a refused address also surfaces as a *net.OpError —
// matching on the transport error's shape rather than on its class would turn
// our own policy decisions into "the site is slow", and turn an unclassified
// fault into one too.
func isUpstreamUnreachable(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, context.Canceled) ||
		errors.Is(err, os.ErrDeadlineExceeded) {
		return true
	}

	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return true
	}

	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}

	for _, errno := range upstreamErrnos {
		if errors.Is(err, errno) {
			return true
		}
	}

	return false
}

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

	// slotWaitBudget bounds how long this gateway queues for its turn at a
	// host before giving the turn up. Zero means wait for as long as the
	// caller's deadline allows, which is what a background job wants: it has
	// nobody waiting on it and should keep the slot it queued for. It never
	// affects how often a host may be fetched — only how long we are willing
	// to wait for the same permission.
	slotWaitBudget time.Duration
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

// NewInteractiveFetchArticleGateway builds a gateway for requests a user is
// waiting on. It queues for its turn at a host for at most slotWaitBudget and
// then reports a transient refusal, rather than spending the whole request
// deadline behind a background job fetching the same publisher.
//
// The interval itself is untouched: giving the turn up early can only make a
// host see fewer requests, never more.
func NewInteractiveFetchArticleGateway(rateLimiter *rate_limiter.HostRateLimiter, httpClient *http.Client, slotWaitBudget time.Duration) *FetchArticleGateway {
	gw := NewFetchArticleGateway(rateLimiter, httpClient)
	gw.slotWaitBudget = slotWaitBudget
	return gw
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

// waitForHostSlot blocks until this caller may contact the host, and decides
// what failing to get there means.
//
// A failed wait is never the publisher's doing: not a packet was sent. It is
// this system's own politeness gate holding the turn for another request —
// ours or another user's — which makes it transient, retryable, and nobody's
// fault. Reporting it as an unavailable dependency would let our own gate,
// working exactly as designed, count against the publisher downstream.
func (g *FetchArticleGateway) waitForHostSlot(ctx context.Context, articleURL string) error {
	if g.rateLimiter == nil {
		return nil
	}

	waitCtx := ctx
	if g.slotWaitBudget > 0 {
		var cancel context.CancelFunc
		waitCtx, cancel = context.WithTimeout(ctx, g.slotWaitBudget)
		defer cancel()
	}

	err := g.rateLimiter.WaitForHost(waitCtx, articleURL)
	if err == nil {
		return nil
	}

	// The URL named no host to wait for. That is a caller fault, not a busy
	// gate, and it stays loud.
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return fmt.Errorf("rate limit wait failed for %q: %w", articleURL, err)
	}

	return &domain.RateLimitedError{
		Message:    "This site is busy with another request. Please try again shortly.",
		RetryAfter: g.rateLimiter.RetryAfterFor(articleURL),
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
	if err := g.waitForHostSlot(ctx, articleURL); err != nil {
		return nil, err
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
		if isUpstreamUnreachable(err) {
			return nil, &domain.UpstreamFetchError{URL: parsedURL.String(), Cause: err}
		}
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
		if isUpstreamUnreachable(err) {
			return nil, &domain.UpstreamFetchError{URL: parsedURL.String(), Cause: err}
		}
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
		if isUpstreamUnreachable(err) {
			return nil, &domain.UpstreamFetchError{URL: parsedURL.String(), Cause: err}
		}
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
