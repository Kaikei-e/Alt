package service

import (
	"context"
	"log/slog"
	"time"

	"pre-processor/repository"
)

const (
	recentSuccessfulJobWindow = 24 * time.Hour
	// inFlightJobWindow bounds how long a pending/running row is treated as
	// in-flight. Beyond this, RecoverStuckJobs is expected to have reset the
	// row, so the guard should no longer block re-enqueue.
	inFlightJobWindow = 10 * time.Minute
)

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

		hasInFlight, err := jobRepo.HasInFlightJob(ctx, articleID, time.Now().Add(-inFlightJobWindow))
		if err != nil {
			return false, "", err
		}
		if hasInFlight {
			if logger != nil {
				logger.InfoContext(ctx, "skipping summarize job creation: in-flight job exists", "article_id", articleID, "reason", "in_flight")
			}
			return false, "in_flight", nil
		}
	}

	return true, "", nil
}
