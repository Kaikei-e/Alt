// Package og_resolve_gateway fetches a feed's page to read its og:image, at the
// moment a reader brings that card into view.
//
// It is deliberately not the article fetcher. That gateway serves the reader
// opening an article and identifies itself accordingly; this one exists to make
// a different bargain with publishers, and the bargain is the whole design:
//
//   - the request happens because a person is looking at the card, so it is one
//     request for one page rather than a scheduled sweep of a work list;
//   - it carries a Referer, so the origin can see what it actually is;
//   - it says who it is honestly rather than impersonating a browser;
//   - it asks robots.txt first, once per host, and takes no for an answer.
//
// The batch job this replaces did none of those things and was refused by every
// publisher it tried.
package og_resolve_gateway

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"alt/domain"
	"alt/utils/html_parser"
	"alt/utils/rate_limiter"

	"github.com/temoto/robotstxt"
)

// userAgent identifies Alt to publishers. It is a truthful token, and the
// robots.txt group it matches, so a site that wants to exclude us can.
const userAgent = "AltBot/1.0 (RSS reader; on-demand link preview)"

// maxPageBytes bounds what one preview can pull from an origin. og:image lives
// in <head>, so anything past this is markup we would parse and discard.
const maxPageBytes = 2 << 20 // 2 MiB

// robotsTTL bounds how long a cached robots.txt stands. Long enough that a grid
// of one publisher's articles costs one robots fetch, short enough that a site
// changing its mind is honoured the same day.
const robotsTTL = 6 * time.Hour

// urlValidator is the SSRF gate. It is an interface so the tests can drive the
// gateway against a loopback httptest server, which a real validator correctly
// refuses.
type urlValidator interface {
	CanonicalRequestURL(ctx context.Context, u *url.URL) (string, error)
}

type robotsEntry struct {
	group   *robotstxt.Group
	fetched time.Time
}

// Gateway resolves one feed's og:image from its page.
type Gateway struct {
	httpClient  *http.Client
	rateLimiter *rate_limiter.HostRateLimiter
	validator   urlValidator

	mu     sync.Mutex
	robots map[string]robotsEntry
}

func NewGateway(rateLimiter *rate_limiter.HostRateLimiter, httpClient *http.Client, validator urlValidator) *Gateway {
	if httpClient == nil {
		panic("og_resolve_gateway: an HTTP client is required (see .claude/rules/di-wiring.md)")
	}
	if validator == nil {
		panic("og_resolve_gateway: an SSRF validator is required — resolving reads URLs that arrived from third-party RSS")
	}
	return &Gateway{
		httpClient:  httpClient,
		rateLimiter: rateLimiter,
		validator:   validator,
		robots:      make(map[string]robotsEntry),
	}
}

// FetchOgImage returns the page's og:image URL, or the refusal to record.
//
// A refusal is not an error: "they said no" and "the page has no image" are
// outcomes worth storing precisely so the next reader scrolling past the same
// card does not cause the same request. An error is returned only when we could
// not form a well-defined question — a malformed URL, or an SSRF rejection.
func (g *Gateway) FetchOgImage(ctx context.Context, pageURL string) (string, domain.OgImageRefusal, error) {
	parsed, err := url.Parse(pageURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", "", fmt.Errorf("og resolve: malformed page url %q", pageURL)
	}

	allowed, err := g.robotsAllows(ctx, parsed)
	if err != nil {
		// robots.txt being unreachable is not permission to ignore it, but it
		// is also not a refusal by the site. Treat it as a transient fetch
		// problem so it is retried rather than recorded as a standing no.
		return "", domain.OgImageFetchError, nil
	}
	if !allowed {
		return "", domain.OgImageRefusedByRobots, nil
	}

	body, refusal, err := g.get(ctx, parsed, pageURL)
	if err != nil || refusal != "" {
		return "", refusal, err
	}

	image := html_parser.ExtractOgImageURL(body, pageURL)
	if image == "" {
		return "", domain.OgImageNoTag, nil
	}
	return image, "", nil
}

// robotsAllows consults the host's robots.txt, fetching it at most once per TTL
// per host. A grid of one publisher's articles must not cost that publisher a
// robots.txt request per card.
func (g *Gateway) robotsAllows(ctx context.Context, target *url.URL) (bool, error) {
	group, err := g.robotsGroup(ctx, target)
	if err != nil {
		return false, err
	}
	if group == nil {
		// No robots.txt, or one the host does not serve: nothing to obey.
		return true, nil
	}
	return group.Test(target.Path), nil
}

func (g *Gateway) robotsGroup(ctx context.Context, target *url.URL) (*robotstxt.Group, error) {
	host := target.Scheme + "://" + target.Host

	g.mu.Lock()
	entry, ok := g.robots[host]
	g.mu.Unlock()
	if ok && time.Since(entry.fetched) < robotsTTL {
		return entry.group, nil
	}

	robotsURL, err := url.Parse(host + "/robots.txt")
	if err != nil {
		return nil, fmt.Errorf("og resolve: robots url for %s: %w", host, err)
	}

	body, status, err := g.rawGet(ctx, robotsURL)
	if err != nil {
		return nil, err
	}

	var group *robotstxt.Group
	// 4xx means "no robots.txt" and is permission to proceed; 5xx is the host
	// failing rather than answering, and rawGet already surfaced that as an
	// error. Anything else we parse.
	if status == http.StatusOK {
		parsedRobots, parseErr := robotstxt.FromBytes([]byte(body))
		if parseErr == nil {
			group = parsedRobots.FindGroup(userAgent)
		}
	}

	g.mu.Lock()
	g.robots[host] = robotsEntry{group: group, fetched: time.Now()}
	g.mu.Unlock()

	return group, nil
}

// get fetches the page itself and maps the origin's answer onto a refusal.
func (g *Gateway) get(ctx context.Context, target *url.URL, referer string) (string, domain.OgImageRefusal, error) {
	body, status, err := g.rawGetWithReferer(ctx, target, referer)
	if err != nil {
		return "", domain.OgImageFetchError, nil
	}

	switch status {
	case http.StatusOK:
		return body, "", nil
	case http.StatusForbidden, http.StatusUnauthorized:
		return "", domain.OgImageRefusedForbidden, nil
	case http.StatusNotFound, http.StatusGone:
		return "", domain.OgImageRefusedNotFound, nil
	default:
		return "", domain.OgImageFetchError, nil
	}
}

func (g *Gateway) rawGet(ctx context.Context, target *url.URL) (string, int, error) {
	return g.rawGetWithReferer(ctx, target, "")
}

func (g *Gateway) rawGetWithReferer(ctx context.Context, target *url.URL, referer string) (string, int, error) {
	safeURL, err := g.validator.CanonicalRequestURL(ctx, target)
	if err != nil {
		return "", 0, fmt.Errorf("og resolve: ssrf validation failed for %q: %w", target.Redacted(), err)
	}

	if g.rateLimiter != nil {
		if err := g.rateLimiter.WaitForHost(ctx, safeURL); err != nil {
			return "", 0, fmt.Errorf("og resolve: host slot for %s: %w", target.Host, err)
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, safeURL, nil)
	if err != nil {
		return "", 0, fmt.Errorf("og resolve: build request: %w", err)
	}

	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	if referer != "" {
		// The origin is entitled to know this request exists because someone is
		// looking at that page.
		req.Header.Set("Referer", referer)
	}

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("og resolve: get %s: %w", target.Redacted(), err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxPageBytes))
	if err != nil {
		return "", resp.StatusCode, fmt.Errorf("og resolve: read body: %w", err)
	}
	return string(body), resp.StatusCode, nil
}
