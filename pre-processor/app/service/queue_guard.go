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
	// failedJobCooldownWindow bounds how long a retry-exhausted job (status
	// 'failed') blocks re-enqueue. UpdateJobStatus writes 'failed' for ANY
	// error once retries are exhausted, including transient infrastructure
	// failures (DB errors, network timeouts, upstream 5xxs) — unlike
	// dead_letter, which is reserved for the explicit
	// domain.ErrContentNotProcessable classification. A bounded cooldown
	// stops the EnqueueUnsummarizedBatch sweep from re-creating a job every
	// tick against a still-broken dependency, while still giving the article
	// a path back to retry once the outage clears — a permanent block would
	// not.
	failedJobCooldownWindow = 1 * time.Hour
)

// ShouldQueueSummarizeJob applies idempotency checks before creating a new job.
func ShouldQueueSummarizeJob(
	ctx context.Context,
	articleID string,
	summaryRepo repository.SummaryRepository,
	jobRepo repository.SummarizeJobRepository,
	logger *slog.Logger,
) (bool, string, error) {
	// summaryRepo/jobRepo are required dependencies — every production call
	// site (handler, connect handler, consumer adapter, queue worker) wires
	// them from constructor fields set at startup. A nil here means DI
	// wiring was forgotten, not "idempotency checks intentionally disabled";
	// silently skipping the checks would let duplicate summarize jobs and
	// LLM calls through indistinguishably from a deliberate config choice
	// (CLAUDE.md rule 8). Fail loudly instead.
	if summaryRepo == nil {
		panic("ShouldQueueSummarizeJob: summaryRepo is nil (idempotency guard unwired)")
	}
	if jobRepo == nil {
		panic("ShouldQueueSummarizeJob: jobRepo is nil (idempotency guard unwired)")
	}

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

	// A dead_letter job means the content is permanently unprocessable
	// (the explicit domain.ErrContentNotProcessable classification). Without
	// this check an article the model can never summarize satisfies none of
	// the checks above and gets re-enqueued every sweep, burning a
	// news-creator call each time.
	hasDeadLetter, err := jobRepo.HasDeadLetterJob(ctx, articleID)
	if err != nil {
		return false, "", err
	}
	if hasDeadLetter {
		if logger != nil {
			logger.InfoContext(ctx, "skipping summarize job creation: article has a dead-lettered job", "article_id", articleID, "reason", "dead_letter_exists")
		}
		return false, "dead_letter_exists", nil
	}

	// A recently retry-exhausted job ('failed') covers any error, including
	// transient ones, so it only blocks for failedJobCooldownWindow rather
	// than forever — see the constant's doc comment.
	hasRecentFailure, err := jobRepo.HasRecentFailedJob(ctx, articleID, time.Now().Add(-failedJobCooldownWindow))
	if err != nil {
		return false, "", err
	}
	if hasRecentFailure {
		if logger != nil {
			logger.InfoContext(ctx, "skipping summarize job creation: article recently exhausted retries", "article_id", articleID, "reason", "recent_failure_cooldown")
		}
		return false, "recent_failure_cooldown", nil
	}

	return true, "", nil
}
