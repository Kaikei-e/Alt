package consumer

import (
	"context"
	"log/slog"

	"pre-processor/repository"
)

// SummarizeServiceAdapter adapts existing services to the SummarizeService interface.
type SummarizeServiceAdapter struct {
	jobRepo     repository.SummarizeJobRepository
	articleRepo repository.ArticleRepository
	logger      *slog.Logger
}

// NewSummarizeServiceAdapter creates a new SummarizeServiceAdapter.
func NewSummarizeServiceAdapter(
	jobRepo repository.SummarizeJobRepository,
	articleRepo repository.ArticleRepository,
	logger *slog.Logger,
) *SummarizeServiceAdapter {
	return &SummarizeServiceAdapter{
		jobRepo:     jobRepo,
		articleRepo: articleRepo,
		logger:      logger,
	}
}

// SummarizeArticle queues an article for summarization via the existing job system.
func (a *SummarizeServiceAdapter) SummarizeArticle(ctx context.Context, articleID, title string) error {
	a.logger.Info("queueing article for summarization via event",
		"article_id", articleID,
		"title", title,
	)

	_, err := a.jobRepo.CreateJob(ctx, articleID)
	if err != nil {
		a.logger.Error("failed to create summarization job",
			"article_id", articleID,
			"error", err,
		)
		return err
	}

	a.logger.Info("article queued for summarization",
		"article_id", articleID,
	)
	return nil
}
