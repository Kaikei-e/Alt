package service

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"pre-processor/domain"
	"pre-processor/repository"
)

const (
	maxBatchFailures     = 3
	failureBlockDuration = 1 * time.Hour
)

// articleFailure tracks failure count and last failure time for a single article.
type articleFailure struct {
	count      int
	lastFailed time.Time
}

// ArticleSummarizerService implementation.
type articleSummarizerService struct {
	articleRepo    repository.ArticleRepository
	summaryRepo    repository.SummaryRepository
	apiRepo        repository.ExternalAPIRepository
	logger         *slog.Logger
	cursor         *domain.Cursor
	failureTracker map[string]*articleFailure
	failureMu      sync.Mutex
}

// NewArticleSummarizerService creates a new article summarizer service.
func NewArticleSummarizerService(
	articleRepo repository.ArticleRepository,
	summaryRepo repository.SummaryRepository,
	apiRepo repository.ExternalAPIRepository,
	logger *slog.Logger,
) ArticleSummarizerService {
	return &articleSummarizerService{
		articleRepo:    articleRepo,
		summaryRepo:    summaryRepo,
		apiRepo:        apiRepo,
		logger:         logger,
		cursor:         &domain.Cursor{},
		failureTracker: make(map[string]*articleFailure),
	}
}

// SummarizeArticles processes a batch of articles for summarization.
func (s *articleSummarizerService) SummarizeArticles(ctx context.Context, batchSize int) (*SummarizationResult, error) {
	s.logger.InfoContext(ctx, "starting article summarization", "batch_size", batchSize)

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

		// Skip articles that have repeatedly failed (e.g., timeout on large content)
		if s.isBlocked(article.ID) {
			s.logger.WarnContext(ctx, "skipping blocked article (repeated batch failures)",
				"article_id", article.ID,
				"max_failures", maxBatchFailures,
				"block_duration", failureBlockDuration)
			continue
		}

		// Generate summary using external API with LOW priority (batch operation)
		summarizedContent, err := s.apiRepo.SummarizeArticle(ctx, article, "low")
		if err != nil {
			// Handle content length errors: save a placeholder summary to mark as processed
			if msg, ok := placeholderMessage(err); ok {
				s.logger.InfoContext(ctx, "saving placeholder summary",
					"article_id", article.ID,
					"content_length", len(article.Content),
					"reason", err)
				if createErr := s.savePlaceholder(ctx, article, msg); createErr != nil {
					result.ErrorCount++
					result.Errors = append(result.Errors, createErr)
				} else {
					result.SuccessCount++
				}
				continue
			}

			s.logger.ErrorContext(ctx, "failed to generate summary", "article_id", article.ID, "error", err)
			s.recordFailure(article.ID)
			result.ErrorCount++
			result.Errors = append(result.Errors, err)
			continue
		}

		// Create summary model
		summary := &domain.ArticleSummary{
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
	s.cursor = &domain.Cursor{}
	s.logger.Info("pagination cursor reset")

	return nil
}

// isBlocked checks if an article is blocked due to repeated failures.
// Returns true if the article has failed maxBatchFailures times and the block duration hasn't elapsed.
func (s *articleSummarizerService) isBlocked(articleID string) bool {
	s.failureMu.Lock()
	defer s.failureMu.Unlock()

	f, ok := s.failureTracker[articleID]
	if !ok {
		return false
	}

	if f.count >= maxBatchFailures {
		if time.Since(f.lastFailed) < failureBlockDuration {
			return true
		}
		// Block duration elapsed, remove from tracker
		delete(s.failureTracker, articleID)
	}
	return false
}

// recordFailure records a failure for an article.
func (s *articleSummarizerService) recordFailure(articleID string) {
	s.failureMu.Lock()
	defer s.failureMu.Unlock()

	f, ok := s.failureTracker[articleID]
	if !ok {
		f = &articleFailure{}
		s.failureTracker[articleID] = f
	}
	f.count++
	f.lastFailed = time.Now()
}

// placeholderMessages maps content-length errors to Japanese placeholder messages.
var placeholderMessages = map[error]string{
	domain.ErrContentTooShort: "本文が短すぎるため要約できませんでした。",
	domain.ErrContentTooLong:  "本文が長すぎるため要約できませんでした。",
}

// placeholderMessage returns the placeholder message for a content-length error, or empty if not applicable.
func placeholderMessage(err error) (string, bool) {
	for target, msg := range placeholderMessages {
		if errors.Is(err, target) {
			return msg, true
		}
	}
	return "", false
}

// savePlaceholder creates and persists a placeholder summary for an article.
func (s *articleSummarizerService) savePlaceholder(ctx context.Context, article *domain.Article, msg string) error {
	summary := &domain.ArticleSummary{
		ArticleID:       article.ID,
		UserID:          article.UserID,
		ArticleTitle:    article.Title,
		SummaryJapanese: msg,
		CreatedAt:       time.Now(),
	}
	if err := s.summaryRepo.Create(ctx, summary); err != nil {
		s.logger.ErrorContext(ctx, "failed to save placeholder summary", "article_id", article.ID, "error", err)
		return err
	}
	return nil
}
