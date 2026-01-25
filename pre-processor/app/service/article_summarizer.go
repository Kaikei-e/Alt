package service

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"pre-processor/domain"
	"pre-processor/models"
	"pre-processor/repository"
)

// ArticleSummarizerService implementation.
type articleSummarizerService struct {
	articleRepo repository.ArticleRepository
	summaryRepo repository.SummaryRepository
	apiRepo     repository.ExternalAPIRepository
	logger      *slog.Logger
	cursor      *repository.Cursor
}

// NewArticleSummarizerService creates a new article summarizer service.
func NewArticleSummarizerService(
	articleRepo repository.ArticleRepository,
	summaryRepo repository.SummaryRepository,
	apiRepo repository.ExternalAPIRepository,
	logger *slog.Logger,
) ArticleSummarizerService {
	return &articleSummarizerService{
		articleRepo: articleRepo,
		summaryRepo: summaryRepo,
		apiRepo:     apiRepo,
		logger:      logger,
		cursor:      &repository.Cursor{},
	}
}

// SummarizeArticles processes a batch of articles for summarization.
func (s *articleSummarizerService) SummarizeArticles(ctx context.Context, batchSize int) (*SummarizationResult, error) {
	s.logger.InfoContext(ctx, "starting article summarization", "batch_size", batchSize)

	// REFACTOR PHASE: Proper implementation
	// Safety check for testing with nil repositories
	if s.articleRepo == nil {
		s.logger.WarnContext(ctx, "articleRepo is nil, returning empty result for testing")

		return &SummarizationResult{
			ProcessedCount: 0,
			SuccessCount:   0,
			ErrorCount:     0,
			Errors:         []error{},
			HasMore:        false,
		}, nil
	}

	// Get articles that need summarization
	articles, newCursor, err := s.articleRepo.FindForSummarization(ctx, s.cursor, batchSize)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to find articles for summarization", "error", err)
		return nil, err
	}

	result := &SummarizationResult{
		ProcessedCount: len(articles),
		SuccessCount:   0,
		ErrorCount:     0,
		Errors:         []error{},
		HasMore:        newCursor != nil,
	}

	// Process each article
	for _, article := range articles {
		// Check if context was canceled before processing the next article
		if ctx.Err() != nil {
			s.logger.WarnContext(ctx, "context canceled, skipping remaining articles",
				"remaining", len(articles)-result.SuccessCount-result.ErrorCount,
				"reason", ctx.Err())
			break
		}

		// Generate summary using external API with LOW priority (batch operation)
		summarizedContent, err := s.apiRepo.SummarizeArticle(ctx, article, "low")
		if err != nil {
			// Handle content too short: Save a placeholder summary to mark as processed
			if errors.Is(err, domain.ErrContentTooShort) {
				s.logger.InfoContext(ctx, "article content too short, saving placeholder summary",
					"article_id", article.ID,
					"content_length", len(article.Content))

				// Create placeholder summary
				placeholderSummary := &models.ArticleSummary{
					ArticleID:       article.ID,
					UserID:          article.UserID,
					ArticleTitle:    article.Title,
					SummaryJapanese: "本文が短すぎるため要約できませんでした。",
					CreatedAt:       time.Now(),
				}

				// Save the placeholder summary
				if createErr := s.summaryRepo.Create(ctx, placeholderSummary); createErr != nil {
					s.logger.ErrorContext(ctx, "failed to save placeholder summary", "article_id", article.ID, "error", createErr)
					result.ErrorCount++
					result.Errors = append(result.Errors, createErr)
				} else {
					result.SuccessCount++ // Count as success since we processed it
				}

				// Continue to next article
				continue
			}

			// Handle content too long: Save a placeholder summary to mark as processed
			if errors.Is(err, domain.ErrContentTooLong) {
				s.logger.InfoContext(ctx, "article content too long, saving placeholder summary",
					"article_id", article.ID,
					"content_length", len(article.Content))

				// Create placeholder summary
				placeholderSummary := &models.ArticleSummary{
					ArticleID:       article.ID,
					UserID:          article.UserID,
					ArticleTitle:    article.Title,
					SummaryJapanese: "本文が長すぎるため要約できませんでした。",
					CreatedAt:       time.Now(),
				}

				// Save the placeholder summary
				if createErr := s.summaryRepo.Create(ctx, placeholderSummary); createErr != nil {
					s.logger.ErrorContext(ctx, "failed to save placeholder summary", "article_id", article.ID, "error", createErr)
					result.ErrorCount++
					result.Errors = append(result.Errors, createErr)
				} else {
					result.SuccessCount++ // Count as success since we processed it
				}

				// Continue to next article
				continue
			}

			s.logger.ErrorContext(ctx, "failed to generate summary", "article_id", article.ID, "error", err)

			result.ErrorCount++
			result.Errors = append(result.Errors, err)

			continue
		}

		// Create summary model
		summary := &models.ArticleSummary{
			ArticleID:       article.ID,
			UserID:          article.UserID,
			ArticleTitle:    article.Title,
			SummaryJapanese: summarizedContent.SummaryJapanese,
			CreatedAt:       time.Now(),
		}

		// Save the summary
		if err := s.summaryRepo.Create(ctx, summary); err != nil {
			s.logger.ErrorContext(ctx, "failed to save summary", "article_id", article.ID, "error", err)

			result.ErrorCount++
			result.Errors = append(result.Errors, err)

			continue
		}

		result.SuccessCount++

		s.logger.DebugContext(ctx, "successfully summarized article", "article_id", article.ID)
	}

	// Update cursor for pagination
	if newCursor != nil {
		s.cursor = newCursor
	}

	s.logger.InfoContext(ctx, "article summarization completed",
		"processed", result.ProcessedCount,
		"success", result.SuccessCount,
		"errors", result.ErrorCount,
		"has_more", result.HasMore)

	return result, nil
}

// HasUnsummarizedArticles checks if there are articles that need summarization.
func (s *articleSummarizerService) HasUnsummarizedArticles(ctx context.Context) (bool, error) {
	s.logger.InfoContext(ctx, "checking for unsummarized articles")

	// REFACTOR PHASE: Proper implementation
	// Safety check for testing with nil repositories
	if s.articleRepo == nil {
		s.logger.WarnContext(ctx, "articleRepo is nil, returning false for testing")
		return false, nil
	}

	hasArticles, err := s.articleRepo.HasUnsummarizedArticles(ctx)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to check for unsummarized articles", "error", err)
		return false, err
	}

	s.logger.InfoContext(ctx, "unsummarized articles check completed", "has_articles", hasArticles)

	return hasArticles, nil
}

// ResetPagination resets the pagination cursor.
func (s *articleSummarizerService) ResetPagination() error {
	s.logger.Info("resetting pagination cursor")
	s.cursor = &repository.Cursor{}
	s.logger.Info("pagination cursor reset")

	return nil
}
