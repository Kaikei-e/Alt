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

// syncStartupLookback bounds the first article-sync run of a process. The sync
// watermark lives in memory, so after a restart the job re-scans this much
// history instead of replaying the whole inoreader_articles table; every later
// run starts strictly above what it already synced.
const syncStartupLookback = 24 * time.Hour

// maxRefusalHold bounds how far behind the scan front a refused row may pin the
// watermark. A feed subscribed in Inoreader but never registered in alt-db is a
// stable state, not a transient one — nothing here calls RegisterFeedLink — so
// an unbounded hold keeps the scan window open forever and re-upserts every
// already-synced article inside it on every run. Past this distance the row is
// abandoned with a WARN naming the feed, which is what the fixed 24h window did
// anyway, only silently.
const maxRefusalHold = 24 * time.Hour

// SyncArticleRepository is the article port article-sync needs: everything in
// repository.ArticleRepository plus an upsert that reports what it refused to
// write. Plain UpsertArticles returns nil even when the write path drops rows
// for reasons external to them — a subscription row the sidecar has not
// written yet, a feed alt-db has not registered yet — and those rows are the
// ones the watermark must not step over, since nothing bumps their fetched_at
// when the missing piece finally arrives.
type SyncArticleRepository interface {
	repository.ArticleRepository

	// UpsertArticlesReportingSkipped upserts the batch and returns the
	// articles it refused to write. A real failure (network, auth) is an
	// error and aborts the batch as before.
	UpsertArticlesReportingSkipped(ctx context.Context, articles []*domain.Article) ([]*domain.Article, error)
}

// ArticleSyncService implementation.
type articleSyncService struct {
	articleRepo     SyncArticleRepository
	externalAPIRepo repository.ExternalAPIRepository
	sanitizer       *utils.Sanitizer
	logger          *slog.Logger

	// mu guards userID, lastSyncedFetchedAt and lastBackfillFetchedAt, which
	// are read and written by SyncArticles and BackfillEmptyFeeds — two
	// independent JobRunner goroutines (article-sync, article-backfill)
	// sharing this instance.
	mu                    sync.Mutex
	userID                string    // Cached system UserID
	lastSyncedFetchedAt   time.Time // Watermark for sync progress (advances once a batch is upserted)
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

// syncCursor returns the fetched_at watermark of the newest article-sync batch
// that made it into alt-db.
func (s *articleSyncService) syncCursor() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastSyncedFetchedAt
}

// advanceSyncCursor moves the sync watermark forward, never backward.
func (s *articleSyncService) advanceSyncCursor(t time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t.After(s.lastSyncedFetchedAt) {
		s.lastSyncedFetchedAt = t
	}
}

// recordSyncProgress moves the watermark to the fetched_at of the last row the
// run scanned, which includes the rows validation dropped: a skipped row never
// becomes valid on its own, and if the sidecar later fixes it the upsert bumps
// fetched_at, so the repaired row reappears above the watermark anyway. Rows
// the write path refused are a different story and are excluded by the caller
// — see holdWatermarkBelow.
func (s *articleSyncService) recordSyncProgress(ctx context.Context, scannedThrough time.Time) {
	if !scannedThrough.After(s.syncCursor()) {
		return
	}

	s.advanceSyncCursor(scannedThrough)
	s.logger.InfoContext(ctx, "sync watermark advanced", "watermark", s.syncCursor())
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

// recordBackfillProgress moves the cursor to the watermark of the last row the
// query scanned, which is not the same as the last article it returned: the
// empty-feed filter runs after the LIMIT, so a window whose rows all belong to
// feeds that already have articles returns nothing and would otherwise be
// rescanned on every run forever.
func (s *articleSyncService) recordBackfillProgress(ctx context.Context, scannedThrough time.Time) {
	if !scannedThrough.After(s.backfillCursor()) {
		return
	}

	s.advanceBackfillCursor(scannedThrough)
	s.logger.InfoContext(ctx, "backfill cursor advanced", "cursor", s.backfillCursor())
}

// NewArticleSyncService creates a new article sync service.
func NewArticleSyncService(
	articleRepo SyncArticleRepository,
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

	// Take only what the sidecar wrote after the last batch we synced. The
	// query is fetched_at > since with no LIMIT, so a fixed 24h window would
	// re-upsert every article once per hourly run — 24 CreateArticle RPCs and
	// 24 ArticleUpdated events per article. The 24h window is the startup
	// re-scan only, until the first batch sets the watermark.
	since := s.syncCursor()
	if since.IsZero() {
		since = time.Now().Add(-syncStartupLookback)
	}

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

	// scannedThrough is the fetched_at watermark of this run. It is the max over
	// every row fetched rather than the last one returned, because the fetch
	// ordering is the driver's business, not this service's.
	var scannedThrough time.Time

	var validArticles []*domain.Article
	for _, article := range articles {
		if article.CreatedAt.After(scannedThrough) {
			scannedThrough = article.CreatedAt
		}

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
		refused, err := s.articleRepo.UpsertArticlesReportingSkipped(ctx, validArticles)
		if err != nil {
			s.logger.ErrorContext(ctx, "failed to upsert articles", "error", err)
			return fmt.Errorf("failed to upsert articles: %w", err)
		}
		s.logger.InfoContext(ctx, "successfully synced articles",
			"count", len(validArticles)-len(refused),
			"refused", len(refused))

		scannedThrough = s.holdWatermarkBelow(ctx, articles, refused, scannedThrough)
	} else {
		s.logger.InfoContext(ctx, "no valid articles to upsert after validation")
	}

	// Only now that the batch is durable in alt-db: a failed upsert returns
	// above, leaving the watermark where it was so the next run retries it.
	s.recordSyncProgress(ctx, scannedThrough)

	return nil
}

// holdWatermarkBelow pulls this run's watermark back under the oldest row the
// write path refused, so the next run re-offers it. Refusals are external to
// the row — the subscription or the feed shows up in alt-db hours later
// without inoreader_articles.fetched_at moving — so a watermark left at
// scannedThrough would sink those articles under `fetched_at > since` forever.
// The next query is strictly greater-than, hence the watermark stops at the
// newest row scanned *before* the refusal, not at the refusal itself.
func (s *articleSyncService) holdWatermarkBelow(ctx context.Context, scanned, refused []*domain.Article, scannedThrough time.Time) time.Time {
	holdFloor := scannedThrough.Add(-maxRefusalHold)

	var refusedFrom time.Time
	var abandoned []*domain.Article
	for _, article := range refused {
		// A row this far behind the scan front is not waiting on a registration
		// that is about to happen; holding for it costs every later run.
		if article.CreatedAt.Before(holdFloor) {
			abandoned = append(abandoned, article)
			continue
		}
		if refusedFrom.IsZero() || article.CreatedAt.Before(refusedFrom) {
			refusedFrom = article.CreatedAt
		}
	}

	if len(abandoned) > 0 {
		s.logger.WarnContext(ctx, "abandoning rows the upsert has refused for longer than the hold window",
			"abandoned", len(abandoned),
			"oldest_url", abandoned[0].URL,
			"oldest_feed_url", abandoned[0].FeedURL,
			"hold_window", maxRefusalHold)
	}

	if refusedFrom.IsZero() {
		return scannedThrough
	}

	var held time.Time
	for _, article := range scanned {
		if article.CreatedAt.Before(refusedFrom) && article.CreatedAt.After(held) {
			held = article.CreatedAt
		}
	}

	// recordSyncProgress never moves the cursor backwards, so the watermark that
	// actually takes effect is whichever of the two is newer. Logging `held`
	// alone reports 0001-01-01 whenever the oldest scanned row is the refused
	// one, which reads as "the cursor was reset to the epoch".
	effective := held
	if cursor := s.syncCursor(); cursor.After(effective) {
		effective = cursor
	}

	s.logger.WarnContext(ctx, "holding sync watermark below rows the upsert refused",
		"refused", len(refused),
		"refused_from", refusedFrom,
		"watermark", effective)

	return held
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
	articles, scannedThrough, err := s.articleRepo.FetchInoreaderArticlesForEmptyFeeds(ctx, s.backfillCursor(), 1000)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to fetch inoreader articles for empty feeds", "error", err)
		return fmt.Errorf("failed to fetch inoreader articles for empty feeds: %w", err)
	}

	if len(articles) == 0 {
		s.logger.InfoContext(ctx, "no articles to backfill")
		s.recordBackfillProgress(ctx, scannedThrough)
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

	s.recordBackfillProgress(ctx, scannedThrough)

	return nil
}
