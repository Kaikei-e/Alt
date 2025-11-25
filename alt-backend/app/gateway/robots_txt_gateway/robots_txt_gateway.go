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
	defer resp.Body.Close()

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
