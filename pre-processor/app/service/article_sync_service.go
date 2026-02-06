package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"pre-processor/domain"
	"pre-processor/repository"
	"pre-processor/utils"
)

// ArticleSyncService implementation.
type articleSyncService struct {
	articleRepo     repository.ArticleRepository
	externalAPIRepo repository.ExternalAPIRepository
	sanitizer       *utils.Sanitizer
	logger          *slog.Logger
	userID          string // Cached system UserID
}

// NewArticleSyncService creates a new article sync service.
func NewArticleSyncService(
	articleRepo repository.ArticleRepository,
	externalAPIRepo repository.ExternalAPIRepository,
	logger *slog.Logger,
) ArticleSyncService {
	return &articleSyncService{
		articleRepo:     articleRepo,
		externalAPIRepo: externalAPIRepo,
		sanitizer:       utils.NewSanitizer(),
		logger:          logger,
	}
}

// SyncArticles synchronizes articles from Inoreader source to articles table.
func (s *articleSyncService) SyncArticles(ctx context.Context) error {
	s.logger.InfoContext(ctx, "starting article synchronization")

	// Ensure system UserID is available
	if s.userID == "" {
		userID, err := s.externalAPIRepo.GetSystemUserID(ctx)
		if err != nil {
			s.logger.ErrorContext(ctx, "failed to get system user id", "error", err)
			return fmt.Errorf("failed to get system user id: %w", err)
		}
		s.userID = userID
		s.logger.InfoContext(ctx, "retrieved system user id", "user_id", s.userID)
	}

	// Fetch articles from the last 24 hours (or configurable)
	// For now, let's look back 24 hours to catch any lag
	since := time.Now().Add(-24 * time.Hour)

	articles, err := s.articleRepo.FetchInoreaderArticles(ctx, since)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to fetch Inoreader articles", "error", err)
		return fmt.Errorf("failed to fetch Inoreader articles: %w", err)
	}

	if len(articles) == 0 {
		s.logger.InfoContext(ctx, "no new articles to sync")
		return nil
	}

	s.logger.InfoContext(ctx, "processing articles for sync", "count", len(articles))

	var validArticles []*domain.Article
	for _, article := range articles {
		// 1. Sanitize content (Zero-Trust)
		sanitizedContent := s.sanitizer.SanitizeHTMLAndTrim(article.Content)

		// 2. Check validation (Empty content safeguard)
		if sanitizedContent == "" {
			s.logger.WarnContext(ctx, "skipping article with empty content after sanitization", "url", article.URL)
			continue
		}

		// Update article with sanitized content
		article.Content = sanitizedContent
		article.UserID = s.userID

		// Ensure other required fields if missing
		if article.Title == "" {
			s.logger.WarnContext(ctx, "skipping article with empty title", "url", article.URL)
			continue
		}

		validArticles = append(validArticles, article)
	}

	// 3. Upsert
	if len(validArticles) > 0 {
		if err := s.articleRepo.UpsertArticles(ctx, validArticles); err != nil {
			s.logger.ErrorContext(ctx, "failed to upsert articles", "error", err)
			return fmt.Errorf("failed to upsert articles: %w", err)
		}
		s.logger.InfoContext(ctx, "successfully synced articles", "count", len(validArticles))
	} else {
		s.logger.InfoContext(ctx, "no valid articles to upsert after validation")
	}

	return nil
}
