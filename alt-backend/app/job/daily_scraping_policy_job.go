package job

import (
	"alt/usecase/scraping_domain_usecase"
	"alt/utils/logger"
	"context"
	"time"
)

const (
	// ScrapingPolicyRefreshInterval is the interval for refreshing scraping policies and robots.txt
	// Default: 24 hours
	ScrapingPolicyRefreshInterval = 24 * time.Hour
)

// DailyScrapingPolicyJobRunner runs a job that refreshes robots.txt and scraping policies
// for all domains every 24 hours
func DailyScrapingPolicyJobRunner(ctx context.Context, usecase *scraping_domain_usecase.ScrapingDomainUsecase) {
	ticker := time.NewTicker(ScrapingPolicyRefreshInterval)
	defer ticker.Stop()

	// Run immediately on startup
	// First, ensure domains from feed_links exist in scraping_domains
	logger.Logger.InfoContext(ctx, "Ensuring domains from feed_links exist in scraping_domains")
	if err := usecase.EnsureDomainsFromFeedLinks(ctx); err != nil {
		logger.Logger.ErrorContext(ctx, "Error ensuring domains from feed_links", "error", err)
	} else {
		logger.Logger.InfoContext(ctx, "Domains from feed_links ensured")
	}

	// Then refresh robots.txt for all domains
	logger.Logger.InfoContext(ctx, "Starting initial scraping policy refresh")
	if err := usecase.RefreshAllRobotsTxt(ctx); err != nil {
		logger.Logger.ErrorContext(ctx, "Error refreshing scraping policies on startup", "error", err)
	} else {
		logger.Logger.InfoContext(ctx, "Initial scraping policy refresh completed")
	}

	// Then run every 24 hours
	for {
		select {
		case <-ctx.Done():
			logger.Logger.InfoContext(ctx, "Stopping daily scraping policy job")
			return
		case <-ticker.C:
			logger.Logger.InfoContext(ctx, "Starting scheduled scraping policy refresh")
			if err := usecase.RefreshAllRobotsTxt(ctx); err != nil {
				logger.Logger.ErrorContext(ctx, "Error refreshing scraping policies", "error", err)
			} else {
				logger.Logger.InfoContext(ctx, "Scheduled scraping policy refresh completed")
			}
		}
	}
}
