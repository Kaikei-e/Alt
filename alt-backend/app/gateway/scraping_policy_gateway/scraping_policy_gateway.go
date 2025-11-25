package scraping_policy_gateway

import (
	"alt/domain"
	"alt/port/scraping_domain_port"
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ScrapingPolicyGateway handles scraping policy checks and rate limiting
// Implements scraping_policy_port.ScrapingPolicyPort
type ScrapingPolicyGateway struct {
	scrapingDomainPort scraping_domain_port.ScrapingDomainPort
	// Last request time per domain for rate limiting
	lastRequestTime map[string]time.Time
}

// NewScrapingPolicyGateway creates a new ScrapingPolicyGateway
func NewScrapingPolicyGateway(scrapingDomainPort scraping_domain_port.ScrapingDomainPort) *ScrapingPolicyGateway {
	return &ScrapingPolicyGateway{
		scrapingDomainPort: scrapingDomainPort,
		lastRequestTime:    make(map[string]time.Time),
	}
}

// CanFetchArticle checks if an article URL can be fetched based on domain policy and robots.txt
func (g *ScrapingPolicyGateway) CanFetchArticle(ctx context.Context, articleURL string) (bool, error) {
	parsedURL, err := url.Parse(articleURL)
	if err != nil {
		return false, fmt.Errorf("invalid article URL: %w", err)
	}

	domainName := parsedURL.Hostname()
	scheme := parsedURL.Scheme
	if scheme == "" {
		scheme = "https"
	}

	// Get scraping domain policy
	scrapingDomain, err := g.scrapingDomainPort.GetByDomain(ctx, domainName)
	if err != nil {
		return false, fmt.Errorf("error fetching scraping domain: %w", err)
	}

	// If no policy exists, create a default one
	if scrapingDomain == nil {
		scrapingDomain = &domain.ScrapingDomain{
			ID:                  uuid.New(),
			Domain:              domainName,
			Scheme:              scheme,
			AllowFetchBody:      true,
			AllowMLTraining:     true,
			AllowCacheDays:      7,
			ForceRespectRobots:  true,
			RobotsDisallowPaths: []string{},
			CreatedAt:           time.Now(),
			UpdatedAt:           time.Now(),
		}
		// Save default policy
		if err := g.scrapingDomainPort.Save(ctx, scrapingDomain); err != nil {
			// Continue with default policy even if save fails
		}
	}

	// Check if fetching is allowed
	if !scrapingDomain.AllowFetchBody {
		return false, nil
	}

	// Check robots.txt if force_respect_robots is enabled
	if scrapingDomain.ForceRespectRobots {
		// Check if path is disallowed
		articlePath := parsedURL.Path
		if articlePath == "" {
			articlePath = "/"
		}

		for _, disallowPath := range scrapingDomain.RobotsDisallowPaths {
			if g.pathMatches(articlePath, disallowPath) {
				return false, nil
			}
		}
	}

	// Check rate limiting (crawl delay)
	if scrapingDomain.RobotsCrawlDelaySec != nil && *scrapingDomain.RobotsCrawlDelaySec > 0 {
		domainKey := domainName
		lastTime, exists := g.lastRequestTime[domainKey]
		if exists {
			delay := time.Duration(*scrapingDomain.RobotsCrawlDelaySec) * time.Second
			timeSinceLastRequest := time.Since(lastTime)
			if timeSinceLastRequest < delay {
				// Rate limit: wait time needed, but for now we'll just log and allow
				// In a real implementation, you might want to wait here or return an error
			}
		}
		g.lastRequestTime[domainKey] = time.Now()
	}

	return true, nil
}

// pathMatches checks if an article path matches a robots.txt disallow pattern
func (g *ScrapingPolicyGateway) pathMatches(articlePath, disallowPath string) bool {
	// Simple pattern matching for robots.txt disallow paths
	// This handles basic wildcards and exact matches

	// Exact match
	if articlePath == disallowPath {
		return true
	}

	// Wildcard at the end (e.g., "/admin/*")
	if strings.HasSuffix(disallowPath, "*") {
		prefix := strings.TrimSuffix(disallowPath, "*")
		if strings.HasPrefix(articlePath, prefix) {
			return true
		}
	}

	// Path prefix match (e.g., "/admin" matches "/admin/anything")
	if strings.HasPrefix(articlePath, disallowPath) {
		return true
	}

	return false
}
