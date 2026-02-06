package service

import (
	"context"
	"log/slog"

	"pre-processor/domain"
	"pre-processor/driver"
	qualitychecker "pre-processor/quality-checker"
	"pre-processor/repository"

	"github.com/jackc/pgx/v5/pgxpool"
)

// QualityCheckerService implementation.
type qualityCheckerService struct {
	summaryRepo repository.SummaryRepository
	apiRepo     repository.ExternalAPIRepository
	dbPool      *pgxpool.Pool
	logger      *slog.Logger
	cursor      *domain.Cursor
}

// NewQualityCheckerService creates a new quality checker service.
func NewQualityCheckerService(
	summaryRepo repository.SummaryRepository,
	apiRepo repository.ExternalAPIRepository,
	dbPool *pgxpool.Pool,
	logger *slog.Logger,
) QualityCheckerService {
	return &qualityCheckerService{
		summaryRepo: summaryRepo,
		apiRepo:     apiRepo,
		dbPool:      dbPool,
		logger:      logger,
		cursor:      &domain.Cursor{},
	}
}

// CheckQuality processes a batch of articles for quality checking.
func (s *qualityCheckerService) CheckQuality(ctx context.Context, batchSize int) (*QualityResult, error) {
	s.logger.InfoContext(ctx, "starting quality check", "batch_size", batchSize)

	// Get articles with summaries that need quality checking
	articlesWithSummaries, newCursor, err := s.summaryRepo.FindArticlesWithSummaries(ctx, s.cursor, batchSize)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to find articles with summaries", "error", err)
		return nil, err
	}

	result := &QualityResult{
		ProcessedCount: len(articlesWithSummaries),
		SuccessCount:   0,
		ErrorCount:     0,
		RemovedCount:   0,
		RetainedCount:  0,
		Errors:         []error{},
		HasMore:        newCursor != nil,
	}

	// Process each article with summary for quality check using LLM scoring
	for _, articleWithSummary := range articlesWithSummaries {
		s.logger.InfoContext(ctx, "processing article for quality check", "article_id", articleWithSummary.ArticleID)

		// Convert domain.ArticleWithSummary to driver.ArticleWithSummary for quality checker
		driverArticle := s.convertToDriverArticle(articleWithSummary)

		// Use the actual LLM-based quality scoring from quality_judger.go
		// JudgeArticleQuality handles scoring and removal of low-quality summaries
		err := qualitychecker.JudgeArticleQuality(ctx, s.dbPool, driverArticle)
		if err != nil {
			s.logger.ErrorContext(ctx, "failed to process quality check with LLM", "article_id", articleWithSummary.ArticleID, "error", err)

			result.ErrorCount++
			result.Errors = append(result.Errors, err)

			continue
		}

		// Check if the summary was actually removed by verifying it still exists
		stillExists, checkErr := s.summaryRepo.Exists(ctx, articleWithSummary.SummaryID)
		if checkErr != nil {
			s.logger.ErrorContext(ctx, "failed to check if summary still exists", "article_id", articleWithSummary.ArticleID, "error", checkErr)

			result.ErrorCount++
			result.Errors = append(result.Errors, checkErr)

			continue
		}

		if stillExists {
			result.RetainedCount++

			s.logger.DebugContext(ctx, "article quality acceptable - summary retained", "article_id", articleWithSummary.ArticleID)
		} else {
			result.RemovedCount++

			s.logger.InfoContext(ctx, "removed low quality summary", "article_id", articleWithSummary.ArticleID)
		}

		result.SuccessCount++
	}

	// Update cursor for pagination
	if newCursor != nil {
		s.cursor = newCursor
	}

	s.logger.InfoContext(ctx, "quality check completed",
		"processed", result.ProcessedCount,
		"success", result.SuccessCount,
		"errors", result.ErrorCount,
		"removed", result.RemovedCount,
		"retained", result.RetainedCount,
		"has_more", result.HasMore)

	return result, nil
}

// convertToDriverArticle converts domain.ArticleWithSummary to driver.ArticleWithSummary.
func (s *qualityCheckerService) convertToDriverArticle(article *domain.ArticleWithSummary) *driver.ArticleWithSummary {
	return &driver.ArticleWithSummary{
		ArticleID:       article.ArticleID,
		Content:         article.ArticleContent,
		SummaryJapanese: article.SummaryJapanese,
	}
}

// ProcessLowQualityArticles processes articles that have been marked as low quality.
func (s *qualityCheckerService) ProcessLowQualityArticles(ctx context.Context, articles []domain.ArticleWithSummary) error {
	s.logger.InfoContext(ctx, "processing low quality articles", "count", len(articles))

	// Remove summaries for low quality articles
	for _, article := range articles {
		if err := s.summaryRepo.Delete(ctx, article.SummaryID); err != nil {
			s.logger.ErrorContext(ctx, "failed to delete low quality summary",
				"article_id", article.ArticleID,
				"summary_id", article.SummaryID,
				"error", err)

			return err
		}

		s.logger.InfoContext(ctx, "removed low quality summary",
			"article_id", article.ArticleID,
			"summary_id", article.SummaryID)
	}

	s.logger.InfoContext(ctx, "completed processing low quality articles", "removed_count", len(articles))

	return nil
}

// ResetPagination resets the pagination cursor.
func (s *qualityCheckerService) ResetPagination() error {
	s.logger.Info("resetting pagination cursor")
	s.cursor = &domain.Cursor{}
	s.logger.Info("pagination cursor reset")

	return nil
}
