package service

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
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

	// mu guards userID and lastBackfillFetchedAt, which are read and written
	// by SyncArticles and BackfillEmptyFeeds — two independent JobRunner
	// goroutines (article-sync, article-backfill) sharing this instance.
	mu                    sync.Mutex
	userID                string    // Cached system UserID
	lastBackfillFetchedAt time.Time // Cursor for backfill progress (advances with each batch)
}

// systemUserID returns the cached system UserID, fetching and caching it via
// externalAPIRepo on first use. Concurrent callers may both miss the cache
// and fetch once each — GetSystemUserID is idempotent, so the only guarantee
// mu needs to provide is race-free access to the field itself.
func (s *articleSyncService) systemUserID(ctx context.Context) (string, error) {
	s.mu.Lock()
	cached := s.userID
	s.mu.Unlock()
	if cached != "" {
		return cached, nil
	}

	userID, err := s.externalAPIRepo.GetSystemUserID(ctx)
	if err != nil {
		return "", err
	}

	s.mu.Lock()
	s.userID = userID
	s.mu.Unlock()

	return userID, nil
}

// backfillCursor returns the current backfill progress cursor.
func (s *articleSyncService) backfillCursor() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastBackfillFetchedAt
}

// advanceBackfillCursor moves the backfill cursor forward, never backward.
func (s *articleSyncService) advanceBackfillCursor(t time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t.After(s.lastBackfillFetchedAt) {
		s.lastBackfillFetchedAt = t
	}
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
	userID, err := s.systemUserID(ctx)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to get system user id", "error", err)
		return fmt.Errorf("failed to get system user id: %w", err)
	}
	s.logger.InfoContext(ctx, "retrieved system user id", "user_id", userID)

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
		article.UserID = userID

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

// BackfillEmptyFeeds inserts Inoreader articles as core articles for feeds that have no articles.
func (s *articleSyncService) BackfillEmptyFeeds(ctx context.Context) error {
	s.logger.InfoContext(ctx, "starting backfill for empty feeds")

	// Ensure system UserID is available
	userID, err := s.systemUserID(ctx)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to get system user id", "error", err)
		return fmt.Errorf("failed to get system user id: %w", err)
	}

	// Fetch inoreader articles for feeds that have no articles
	// Uses lastBackfillFetchedAt as cursor to avoid re-processing in API mode
	articles, err := s.articleRepo.FetchInoreaderArticlesForEmptyFeeds(ctx, s.backfillCursor(), 1000)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to fetch inoreader articles for empty feeds", "error", err)
		return fmt.Errorf("failed to fetch inoreader articles for empty feeds: %w", err)
	}

	if len(articles) == 0 {
		s.logger.InfoContext(ctx, "no articles to backfill")
		return nil
	}

	s.logger.InfoContext(ctx, "processing articles for backfill", "count", len(articles))

	var validArticles []*domain.Article
	for _, article := range articles {
		// Sanitize content (Zero-Trust)
		sanitizedContent := s.sanitizer.SanitizeHTMLAndTrim(article.Content)
		if sanitizedContent == "" {
			s.logger.WarnContext(ctx, "skipping article with empty content after sanitization", "url", article.URL)
			continue
		}
		article.Content = sanitizedContent
		article.UserID = userID

		if article.Title == "" {
			s.logger.WarnContext(ctx, "skipping article with empty title", "url", article.URL)
			continue
		}

		validArticles = append(validArticles, article)
	}

	if len(validArticles) > 0 {
		if err := s.articleRepo.UpsertArticlesWithFeedID(ctx, validArticles); err != nil {
			s.logger.ErrorContext(ctx, "failed to upsert backfill articles", "error", err)
			return fmt.Errorf("failed to upsert backfill articles: %w", err)
		}
		s.logger.InfoContext(ctx, "backfill completed", "count", len(validArticles))
	} else {
		s.logger.InfoContext(ctx, "no valid articles to backfill after validation")
	}

	// Advance cursor to the last fetched_at to avoid re-processing
	if len(articles) > 0 {
		lastArticle := articles[len(articles)-1]
		if lastArticle.CreatedAt.After(s.backfillCursor()) {
			s.advanceBackfillCursor(lastArticle.CreatedAt)
			s.logger.InfoContext(ctx, "backfill cursor advanced", "cursor", s.backfillCursor())
		}
	}

	return nil
}
