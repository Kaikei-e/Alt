package scraping_domain_usecase

import (
	"alt/domain"
	"alt/port/feed_link_domain_port"
	"alt/port/robots_txt_port"
	"alt/port/scraping_domain_port"
	"alt/utils/logger"
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// ScrapingDomainUsecase handles scraping domain business logic
type ScrapingDomainUsecase struct {
	scrapingDomainPort   scraping_domain_port.ScrapingDomainPort
	robotsTxtPort        robots_txt_port.RobotsTxtPort
	feedLinkDomainPort   feed_link_domain_port.FeedLinkDomainPort
}

// NewScrapingDomainUsecase creates a new ScrapingDomainUsecase
func NewScrapingDomainUsecase(scrapingDomainPort scraping_domain_port.ScrapingDomainPort) *ScrapingDomainUsecase {
	return &ScrapingDomainUsecase{
		scrapingDomainPort: scrapingDomainPort,
	}
}

// NewScrapingDomainUsecaseWithRobotsTxt creates a new ScrapingDomainUsecase with robots.txt port
func NewScrapingDomainUsecaseWithRobotsTxt(scrapingDomainPort scraping_domain_port.ScrapingDomainPort, robotsTxtPort robots_txt_port.RobotsTxtPort) *ScrapingDomainUsecase {
	return &ScrapingDomainUsecase{
		scrapingDomainPort: scrapingDomainPort,
		robotsTxtPort:      robotsTxtPort,
	}
}

// NewScrapingDomainUsecaseWithFeedLinkDomain creates a new ScrapingDomainUsecase with feed link domain port
func NewScrapingDomainUsecaseWithFeedLinkDomain(scrapingDomainPort scraping_domain_port.ScrapingDomainPort, robotsTxtPort robots_txt_port.RobotsTxtPort, feedLinkDomainPort feed_link_domain_port.FeedLinkDomainPort) *ScrapingDomainUsecase {
	return &ScrapingDomainUsecase{
		scrapingDomainPort: scrapingDomainPort,
		robotsTxtPort:      robotsTxtPort,
		feedLinkDomainPort: feedLinkDomainPort,
	}
}

// ListScrapingDomains lists scraping domains with pagination
func (u *ScrapingDomainUsecase) ListScrapingDomains(ctx context.Context, offset, limit int) ([]*domain.ScrapingDomain, error) {
	return u.scrapingDomainPort.List(ctx, offset, limit)
}

// GetScrapingDomain retrieves a scraping domain by ID
func (u *ScrapingDomainUsecase) GetScrapingDomain(ctx context.Context, id uuid.UUID) (*domain.ScrapingDomain, error) {
	return u.scrapingDomainPort.GetByID(ctx, id)
}

// UpdateScrapingDomainPolicy updates the policy fields of a scraping domain
func (u *ScrapingDomainUsecase) UpdateScrapingDomainPolicy(ctx context.Context, id uuid.UUID, update *domain.ScrapingPolicyUpdate) error {
	return u.scrapingDomainPort.UpdatePolicy(ctx, id, update)
}

// RefreshRobotsTxt fetches and updates robots.txt for a scraping domain
func (u *ScrapingDomainUsecase) RefreshRobotsTxt(ctx context.Context, id uuid.UUID) error {
	if u.robotsTxtPort == nil {
		return fmt.Errorf("robots.txt port not available")
	}

	// Get existing domain
	scrapingDomain, err := u.scrapingDomainPort.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to get scraping domain: %w", err)
	}
	if scrapingDomain == nil {
		return fmt.Errorf("scraping domain not found")
	}

	// Fetch robots.txt
	robotsTxt, err := u.robotsTxtPort.FetchRobotsTxt(ctx, scrapingDomain.Domain, scrapingDomain.Scheme)
	if err != nil {
		return fmt.Errorf("failed to fetch robots.txt: %w", err)
	}

	// Update domain with robots.txt data
	robotsTxtURL := fmt.Sprintf("%s://%s/robots.txt", scrapingDomain.Scheme, scrapingDomain.Domain)
	scrapingDomain.RobotsTxtURL = &robotsTxtURL
	// Copy Content to avoid dangling pointer
	content := robotsTxt.Content
	scrapingDomain.RobotsTxtContent = &content
	now := time.Now()
	scrapingDomain.RobotsTxtFetchedAt = &now
	statusCode := robotsTxt.StatusCode
	scrapingDomain.RobotsTxtLastStatus = &statusCode
	// Copy CrawlDelay to avoid dangling pointer
	crawlDelay := robotsTxt.CrawlDelay
	scrapingDomain.RobotsCrawlDelaySec = &crawlDelay
	scrapingDomain.RobotsDisallowPaths = robotsTxt.DisallowPaths
	scrapingDomain.UpdatedAt = now

	// Save updated domain
	if err := u.scrapingDomainPort.Save(ctx, scrapingDomain); err != nil {
		return fmt.Errorf("failed to save scraping domain: %w", err)
	}

	return nil
}

// RefreshAllRobotsTxt refreshes robots.txt for all scraping domains
// It processes domains in batches and continues even if some domains fail
func (u *ScrapingDomainUsecase) RefreshAllRobotsTxt(ctx context.Context) error {
	if u.robotsTxtPort == nil {
		return fmt.Errorf("robots.txt port not available")
	}

	const batchSize = 50
	offset := 0
	totalProcessed := 0
	totalErrors := 0

	for {
		// Fetch a batch of domains
		domains, err := u.scrapingDomainPort.List(ctx, offset, batchSize)
		if err != nil {
			return fmt.Errorf("failed to list scraping domains: %w", err)
		}

		if len(domains) == 0 {
			break // No more domains to process
		}

		// Process each domain in the batch
		for _, domain := range domains {
			if err := u.RefreshRobotsTxt(ctx, domain.ID); err != nil {
				totalErrors++
				// Log error but continue processing other domains
				logger.Logger.ErrorContext(ctx, "Failed to refresh robots.txt for domain", "domain", domain.Domain, "error", err)
				continue
			}
			totalProcessed++
		}

		// If we got fewer domains than batchSize, we've reached the end
		if len(domains) < batchSize {
			break
		}

		offset += batchSize
	}

	// Return error only if all domains failed
	if totalProcessed == 0 && totalErrors > 0 {
		return fmt.Errorf("failed to refresh robots.txt for all domains (%d errors)", totalErrors)
	}

	return nil
}

// EnsureDomainsFromFeedLinks ensures that all domains from feed_links exist in scraping_domains
// It extracts unique domains from feed_links and creates missing entries in scraping_domains
func (u *ScrapingDomainUsecase) EnsureDomainsFromFeedLinks(ctx context.Context) error {
	if u.feedLinkDomainPort == nil {
		return fmt.Errorf("feedLinkDomainPort not available")
	}

	// Get unique domains from feed_links
	feedLinkDomains, err := u.feedLinkDomainPort.ListFeedLinkDomains(ctx)
	if err != nil {
		return fmt.Errorf("failed to list feed link domains: %w", err)
	}

	logger.Logger.InfoContext(ctx, "Found domains from feed_links", "count", len(feedLinkDomains))

	createdCount := 0
	existingCount := 0

	// For each domain, check if it exists in scraping_domains, create if not
	for _, feedLinkDomain := range feedLinkDomains {
		// Check if domain already exists
		existing, err := u.scrapingDomainPort.GetByDomain(ctx, feedLinkDomain.Domain)
		if err != nil {
			logger.Logger.ErrorContext(ctx, "Error checking existing domain", "domain", feedLinkDomain.Domain, "error", err)
			continue
		}

		if existing != nil {
			existingCount++
			continue // Domain already exists
		}

		// Create new scraping domain with default values
		newDomain := &domain.ScrapingDomain{
			ID:                  uuid.New(),
			Domain:              feedLinkDomain.Domain,
			Scheme:              feedLinkDomain.Scheme,
			AllowFetchBody:      true,
			AllowMLTraining:     true,
			AllowCacheDays:      7,
			ForceRespectRobots:  true,
			RobotsDisallowPaths: []string{},
			CreatedAt:           time.Now(),
			UpdatedAt:           time.Now(),
		}

		if err := u.scrapingDomainPort.Save(ctx, newDomain); err != nil {
			logger.Logger.ErrorContext(ctx, "Error creating scraping domain", "domain", feedLinkDomain.Domain, "error", err)
			continue
		}

		createdCount++
		logger.Logger.InfoContext(ctx, "Created new scraping domain from feed_links", "domain", feedLinkDomain.Domain, "scheme", feedLinkDomain.Scheme)
	}

	logger.Logger.InfoContext(ctx, "Ensured domains from feed_links", "total", len(feedLinkDomains), "created", createdCount, "existing", existingCount)
	return nil
}
