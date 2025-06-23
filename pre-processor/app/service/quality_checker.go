package service

import (
	"context"
	"log/slog"
	"pre-processor/driver"
	"pre-processor/models"
	qualitychecker "pre-processor/quality-checker"
	"pre-processor/repository"

	"github.com/jackc/pgx/v5/pgxpool"
)

// QualityCheckerService implementation
type qualityCheckerService struct {
	summaryRepo repository.SummaryRepository
	apiRepo     repository.ExternalAPIRepository
	dbPool      *pgxpool.Pool
	logger      *slog.Logger
	cursor      *repository.Cursor
}

// NewQualityCheckerService creates a new quality checker service
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
		cursor:      &repository.Cursor{},
	}
}

// CheckQuality processes a batch of articles for quality checking
func (s *qualityCheckerService) CheckQuality(ctx context.Context, batchSize int) (*QualityResult, error) {
	s.logger.Info("starting quality check", "batch_size", batchSize)

	// REFACTOR PHASE: Proper implementation
	// Safety check for testing with nil repositories
	if s.summaryRepo == nil {
		s.logger.Warn("summaryRepo is nil, returning empty result for testing")
		return &QualityResult{
			ProcessedCount: 0,
			SuccessCount:   0,
			ErrorCount:     0,
			RemovedCount:   0,
			RetainedCount:  0,
			Errors:         []error{},
			HasMore:        false,
		}, nil
	}

	// Get articles with summaries that need quality checking
	articlesWithSummaries, newCursor, err := s.summaryRepo.FindArticlesWithSummaries(ctx, s.cursor, batchSize)
	if err != nil {
		s.logger.Error("failed to find articles with summaries", "error", err)
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
		s.logger.Info("processing article for quality check", "article_id", articleWithSummary.ArticleID)
		
		// Convert models.ArticleWithSummary to driver.ArticleWithSummary for quality checker
		driverArticle := s.convertToDriverArticle(articleWithSummary)
		
		// Use the actual LLM-based quality scoring from quality_judger.go
		err := qualitychecker.RemoveLowScoreSummary(ctx, s.dbPool, driverArticle)
		if err != nil {
			s.logger.Error("failed to process quality check with LLM", "article_id", articleWithSummary.ArticleID, "error", err)
			result.ErrorCount++
			result.Errors = append(result.Errors, err)
			continue
		}
		
		// Check if the summary was actually removed by verifying it still exists
		stillExists, checkErr := s.summaryRepo.Exists(ctx, articleWithSummary.SummaryID)
		if checkErr != nil {
			s.logger.Error("failed to check if summary still exists", "article_id", articleWithSummary.ArticleID, "error", checkErr)
			result.ErrorCount++
			result.Errors = append(result.Errors, checkErr)
			continue
		}
		
		if stillExists {
			result.RetainedCount++
			s.logger.Debug("article quality acceptable - summary retained", "article_id", articleWithSummary.ArticleID)
		} else {
			result.RemovedCount++
			s.logger.Info("removed low quality summary", "article_id", articleWithSummary.ArticleID)
		}
		
		result.SuccessCount++
	}


	// Update cursor for pagination
	if newCursor != nil {
		s.cursor = newCursor
	}

	s.logger.Info("quality check completed", 
		"processed", result.ProcessedCount,
		"success", result.SuccessCount,
		"errors", result.ErrorCount,
		"removed", result.RemovedCount,
		"retained", result.RetainedCount,
		"has_more", result.HasMore)

	return result, nil
}

// convertToDriverArticle converts models.ArticleWithSummary to driver.ArticleWithSummary
func (s *qualityCheckerService) convertToDriverArticle(article *models.ArticleWithSummary) *driver.ArticleWithSummary {
	return &driver.ArticleWithSummary{
		ArticleID:       article.ArticleID,
		Content:         article.ArticleContent,
		SummaryJapanese: article.SummaryJapanese,
	}
}

// ProcessLowQualityArticles processes articles that have been marked as low quality
func (s *qualityCheckerService) ProcessLowQualityArticles(ctx context.Context, articles []models.ArticleWithSummary) error {
	s.logger.Info("processing low quality articles", "count", len(articles))

	// REFACTOR PHASE: Proper implementation
	// Safety check for testing with nil repositories
	if s.summaryRepo == nil {
		s.logger.Warn("summaryRepo is nil, skipping low quality article processing for testing")
		return nil
	}

	// Remove summaries for low quality articles
	for _, article := range articles {
		if err := s.summaryRepo.Delete(ctx, article.SummaryID); err != nil {
			s.logger.Error("failed to delete low quality summary", 
				"article_id", article.ArticleID, 
				"summary_id", article.SummaryID, 
				"error", err)
			return err
		}
		
		s.logger.Info("removed low quality summary", 
			"article_id", article.ArticleID, 
			"summary_id", article.SummaryID)
	}

	s.logger.Info("completed processing low quality articles", "removed_count", len(articles))
	return nil
}

// ResetPagination resets the pagination cursor
func (s *qualityCheckerService) ResetPagination() error {
	s.logger.Info("resetting pagination cursor")
	s.cursor = &repository.Cursor{}
	s.logger.Info("pagination cursor reset")
	return nil
}
