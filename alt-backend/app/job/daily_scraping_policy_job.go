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

// ScrapingPolicyJob returns a function suitable for the JobScheduler that
// ensures domains exist and refreshes robots.txt policies.
func ScrapingPolicyJob(usecase *scraping_domain_usecase.ScrapingDomainUsecase) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		logger.Logger.InfoContext(ctx, "Ensuring domains from feed_links exist in scraping_domains")
		if err := usecase.EnsureDomainsFromFeedLinks(ctx); err != nil {
			logger.Logger.ErrorContext(ctx, "Error ensuring domains from feed_links", "error", err)
		}

		logger.Logger.InfoContext(ctx, "Starting scraping policy refresh")
		if err := usecase.RefreshAllRobotsTxt(ctx); err != nil {
			return err
		}
		logger.Logger.InfoContext(ctx, "Scraping policy refresh completed")
		return nil
	}
}

// DailyScrapingPolicyJobRunner is kept for backward compatibility.
// Deprecated: Use ScrapingPolicyJob with JobScheduler instead.
func DailyScrapingPolicyJobRunner(ctx context.Context, usecase *scraping_domain_usecase.ScrapingDomainUsecase) {
	if err := ScrapingPolicyJob(usecase)(ctx); err != nil {
		logger.Logger.ErrorContext(ctx, "Error in scraping policy refresh", "error", err)
	}
}
