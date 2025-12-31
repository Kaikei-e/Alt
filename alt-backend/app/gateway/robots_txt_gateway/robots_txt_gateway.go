package robots_txt_gateway

import (
	"alt/domain"
	"alt/utils/security"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/temoto/robotstxt"
)

// RobotsTxtGateway handles fetching and parsing robots.txt files
// Implements robots_txt_port.RobotsTxtPort
type RobotsTxtGateway struct {
	httpClient    *http.Client
	ssrfValidator *security.SSRFValidator
}

// NewRobotsTxtGateway creates a new RobotsTxtGateway
func NewRobotsTxtGateway(httpClient *http.Client) *RobotsTxtGateway {
	ssrfValidator := security.NewSSRFValidator()
	var secureClient *http.Client
	if httpClient != nil {
		timeout := httpClient.Timeout
		if timeout == 0 {
			timeout = 10 * time.Second
		}
		secureClient = ssrfValidator.CreateSecureHTTPClient(timeout)
		// Preserve custom Transport if provided (for testing)
		if httpClient.Transport != nil {
			secureClient.Transport = httpClient.Transport
		}
	} else {
		secureClient = ssrfValidator.CreateSecureHTTPClient(10 * time.Second)
	}

	return &RobotsTxtGateway{
		httpClient:    secureClient,
		ssrfValidator: ssrfValidator,
	}
}

// NewRobotsTxtGatewayWithDeps creates a new RobotsTxtGateway with explicit dependencies (for testing)
func NewRobotsTxtGatewayWithDeps(httpClient *http.Client, ssrfValidator *security.SSRFValidator) *RobotsTxtGateway {
	return &RobotsTxtGateway{
		httpClient:    httpClient,
		ssrfValidator: ssrfValidator,
	}
}

// FetchRobotsTxt fetches and parses robots.txt for a given domain
func (g *RobotsTxtGateway) FetchRobotsTxt(ctx context.Context, domainName, scheme string) (*domain.RobotsTxt, error) {
	robotsURL := fmt.Sprintf("%s://%s/robots.txt", scheme, domainName)
	parsedURL, err := url.Parse(robotsURL)
	if err != nil {
		return nil, fmt.Errorf("invalid robots.txt URL: %w", err)
	}

	// SSRF protection
	if err := g.ssrfValidator.ValidateURL(ctx, parsedURL); err != nil {
		return nil, fmt.Errorf("ssrf validation failed: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsedURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "Alt-RSS-Reader/1.0 (+https://alt.example.com)")
	req.Header.Set("Accept", "text/plain")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch robots.txt: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			// Log but don't fail - response has been processed
			_ = closeErr
		}
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read robots.txt body: %w", err)
	}

	content := string(body)
	robotsTxt := &domain.RobotsTxt{
		URL:           robotsURL,
		Content:       content,
		FetchedAt:     time.Now(),
		StatusCode:    resp.StatusCode,
		DisallowPaths: []string{},
	}

	// Parse robots.txt content
	if resp.StatusCode == 200 {
		parsed := g.parseRobotsTxt(content)
		robotsTxt.CrawlDelay = parsed.CrawlDelay
		robotsTxt.DisallowPaths = parsed.DisallowPaths
	}

	return robotsTxt, nil
}

// parseResult holds parsed robots.txt information
type parseResult struct {
	CrawlDelay    int
	DisallowPaths []string
}

// parseRobotsTxt parses robots.txt content and extracts relevant information
func (g *RobotsTxtGateway) parseRobotsTxt(content string) *parseResult {
	result := &parseResult{
		CrawlDelay:    0,
		DisallowPaths: []string{},
	}

	lines := strings.Split(content, "\n")
	var currentUserAgent string
	var inUserAgentBlock bool
	var maxCrawlDelay int

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse User-agent directive
		if strings.HasPrefix(strings.ToLower(line), "user-agent:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				currentUserAgent = strings.TrimSpace(parts[1])
				// Match wildcard or our user agent
				inUserAgentBlock = currentUserAgent == "*" ||
					strings.Contains(strings.ToLower(currentUserAgent), "alt") ||
					strings.Contains(strings.ToLower(currentUserAgent), "rss")
			} else {
				inUserAgentBlock = false
			}
			continue
		}

		// Only process directives for our user agent or wildcard
		if !inUserAgentBlock {
			continue
		}

		// Parse Crawl-delay directive
		if strings.HasPrefix(strings.ToLower(line), "crawl-delay:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				var delay int
				if _, err := fmt.Sscanf(strings.TrimSpace(parts[1]), "%d", &delay); err == nil {
					if delay > maxCrawlDelay {
						maxCrawlDelay = delay
					}
				}
			}
			continue
		}

		// Parse Disallow directive
		if strings.HasPrefix(strings.ToLower(line), "disallow:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				path := strings.TrimSpace(parts[1])
				if path != "" {
					result.DisallowPaths = append(result.DisallowPaths, path)
				}
			}
			continue
		}

		// Reset user agent block on Allow directive or new section
		if strings.HasPrefix(strings.ToLower(line), "allow:") {
			// Continue processing but don't reset
			continue
		}
	}

	result.CrawlDelay = maxCrawlDelay
	return result
}

// IsPathAllowed checks if a specific path is allowed for a given user agent
func (g *RobotsTxtGateway) IsPathAllowed(ctx context.Context, targetURL *url.URL, userAgent string) (bool, error) {
	robots, err := g.FetchRobotsTxt(ctx, targetURL.Hostname(), targetURL.Scheme)
	if err != nil {
		// If we can't fetch robots.txt, we should default to ALLOW (standard convention) or DISALLOW based on strictness.
		// For this implementation, we'll log the error and allow, unless it's a specific error that warrants blocking.
		// However, failing to fetch could mean the site is down or blocks robots.txt.
		// Let's assume ALLOW effectively if robots.txt is missing/erroring for now,
		// but typically we might want to be careful.
		// The requirements imply we MUST check. If check fails, what then?
		// Temoto's library handles parsing. If we use it directly, we can parse the content we fetched.

		// Note: The caching/logic in FetchRobotsTxt returns a domain.RobotsTxt struct.
		// We need to parse that content with temoto/robotstxt.

		// If fetch fails (e.g. 404), it usually means allowed.
		// If 5xx, it might mean temporary issues.
		// Let's rely on the fact that if FetchRobotsTxt returns error, it's likely a fetch failure.
		// If robots.txt doesn't exist (404), FetchRobotsTxt currently returns struct with 404 status.
		// We need to check that status.
		return true, nil
	}

	if robots.StatusCode >= 400 && robots.StatusCode < 500 {
		// 4xx implies no robots.txt, so everything is allowed
		return true, nil
	}

	if robots.StatusCode >= 500 {
		// 5xx implies server error, usually full allowance is assumed after retries,
		// but strictly speaking validation fails. Here we allow to avoid blocking on server fluff.
		return true, nil
	}

	// Use temoto/robotstxt to parse
	data, err := robotstxt.FromBytes([]byte(robots.Content))
	if err != nil {
		// If parsing fails, maybe content is malformed. Assume Allowed.
		return true, nil
	}

	return data.TestAgent(targetURL.Path, userAgent), nil
}
