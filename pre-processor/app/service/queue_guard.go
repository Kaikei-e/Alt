package service

import (
	"context"
	"log/slog"
	"time"

	"pre-processor/repository"
)

const recentSuccessfulJobWindow = 24 * time.Hour

// ShouldQueueSummarizeJob applies idempotency checks before creating a new job.
func ShouldQueueSummarizeJob(
	ctx context.Context,
	articleID string,
	summaryRepo repository.SummaryRepository,
	jobRepo repository.SummarizeJobRepository,
	logger *slog.Logger,
) (bool, string, error) {
	if summaryRepo != nil {
		exists, err := summaryRepo.Exists(ctx, articleID)
		if err != nil {
			return false, "", err
		}
		if exists {
			if logger != nil {
				logger.InfoContext(ctx, "skipping summarize job creation: summary already exists", "article_id", articleID)
			}
			return false, "summary_exists", nil
		}
	}

	if jobRepo != nil {
		hasRecentSuccess, err := jobRepo.HasRecentSuccessfulJob(ctx, articleID, time.Now().Add(-recentSuccessfulJobWindow))
		if err != nil {
			return false, "", err
		}
		if hasRecentSuccess {
			if logger != nil {
				logger.InfoContext(ctx, "skipping summarize job creation: recent successful job exists", "article_id", articleID)
			}
			return false, "recent_success", nil
		}
	}

	return true, "", nil
}
