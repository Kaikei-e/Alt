package backend_api

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"net/url"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/types/known/timestamppb"

	"pre-processor/domain"
	"pre-processor/driver"
	datahubv1 "pre-processor/gen/proto/services/datahub/v1"
	"pre-processor/utils"
)

// ArticleRepository implements repository.ArticleRepository using the backend API.
type ArticleRepository struct {
	client *Client
	dbPool *pgxpool.Pool
}

// NewArticleRepository creates a new API-backed article repository.
// dbPool is used for operations that require direct DB access (e.g. FetchInoreaderArticles).
func NewArticleRepository(client *Client, dbPool *pgxpool.Pool) *ArticleRepository {
	return &ArticleRepository{client: client, dbPool: dbPool}
}

// Create creates a new article via the backend API.
func (r *ArticleRepository) Create(ctx context.Context, article *domain.Article) error {
	// First resolve feed_id from feed_url if needed
	feedID := article.FeedID
	if feedID == "" && article.FeedURL != "" {
		id, err := r.getFeedID(ctx, article.FeedURL)
		if err != nil {
			return fmt.Errorf("failed to get feed ID: %w", err)
		}
		if id == "" {
			return fmt.Errorf("feed not found for URL: %s", article.FeedURL)
		}
		feedID = id
	}
	if feedID == "" && article.URL != "" {
		id, err := r.getFeedID(ctx, article.URL)
		if err != nil {
			return fmt.Errorf("failed to get feed ID: %w", err)
		}
		feedID = id
	}

	language := article.Language
	if language == "" {
		language = utils.DetectLanguage(article.Title + "\n" + article.Content)
	}

	protoReq := &datahubv1.CreateArticleRequest{
		Title:    article.Title,
		Url:      article.URL,
		Content:  article.Content,
		FeedId:   feedID,
		UserId:   article.UserID,
		Language: language,
	}
	if !article.PublishedAt.IsZero() {
		protoReq.PublishedAt = timestamppb.New(article.PublishedAt)
	}

	req := connect.NewRequest(protoReq)
	r.client.addAuth(req)

	resp, err := r.client.client.CreateArticle(ctx, req)
	if err != nil {
		return fmt.Errorf("CreateArticle: %w", err)
	}

	article.ID = resp.Msg.ArticleId
	return nil
}

// CheckExists checks if articles exist for the given URLs.
func (r *ArticleRepository) CheckExists(ctx context.Context, urls []string) (bool, error) {
	// For the API, we need a feed_id. Get it from the URL domain.
	// Since we don't have feed_id here, we check each URL individually.
	for _, u := range urls {
		parsedURL, err := url.Parse(u)
		if err != nil {
			continue
		}

		// Try to get feed ID from the URL. A not-found feed means the
		// article can't exist under it — skip. Any other error (network,
		// auth, backend outage) must propagate so callers don't treat a
		// transient failure as "article doesn't exist" and create a duplicate.
		feedID, err := r.getFeedID(ctx, parsedURL.String())
		if err != nil {
			if isNotFoundError(err) {
				continue
			}
			return false, fmt.Errorf("getFeedID for %s: %w", u, err)
		}
		if feedID == "" {
			continue
		}

		protoReq := &datahubv1.CheckArticleExistsRequest{
			Url:    u,
			FeedId: feedID,
		}
		req := connect.NewRequest(protoReq)
		r.client.addAuth(req)

		resp, err := r.client.client.CheckArticleExists(ctx, req)
		if err != nil {
			return false, fmt.Errorf("CheckArticleExists for %s: %w", u, err)
		}
		if resp.Msg.Exists {
			return true, nil
		}
	}
	return false, nil
}

// CheckExistsWithFeedID reports whether any of urls already exists under the
// given feed.
//
// Unlike CheckExists it takes a feed_id the caller has already resolved. That
// is the only form alt-data-hub can answer for an article URL: GetFeedID
// matches feed_links.url, so looking a feed up from an article URL always
// comes back NotFound and the check degrades to a silent "nothing exists".
func (r *ArticleRepository) CheckExistsWithFeedID(ctx context.Context, feedID string, urls []string) (bool, error) {
	if feedID == "" {
		return false, fmt.Errorf("CheckExistsWithFeedID: feedID is required")
	}

	for _, u := range urls {
		protoReq := &datahubv1.CheckArticleExistsRequest{
			Url:    u,
			FeedId: feedID,
		}
		req := connect.NewRequest(protoReq)
		r.client.addAuth(req)

		resp, err := r.client.client.CheckArticleExists(ctx, req)
		if err != nil {
			return false, fmt.Errorf("CheckArticleExists for %s: %w", u, err)
		}
		if resp.Msg.Exists {
			return true, nil
		}
	}
	return false, nil
}

// FindForSummarization finds articles that need summarization via the backend API.
func (r *ArticleRepository) FindForSummarization(ctx context.Context, cursor *domain.Cursor, limit int) ([]*domain.Article, *domain.Cursor, error) {
	protoReq := &datahubv1.ListUnsummarizedArticlesRequest{
		Limit: int32(min(limit, math.MaxInt32)), // #nosec G115 -- clamped to int32 range
	}
	if cursor != nil {
		if cursor.LastCreatedAt != nil {
			protoReq.LastCreatedAt = timestamppb.New(*cursor.LastCreatedAt)
		}
		protoReq.LastId = cursor.LastID
	}

	req := connect.NewRequest(protoReq)
	r.client.addAuth(req)

	resp, err := r.client.client.ListUnsummarizedArticles(ctx, req)
	if err != nil {
		return nil, nil, fmt.Errorf("ListUnsummarizedArticles: %w", err)
	}

	articles := make([]*domain.Article, len(resp.Msg.Articles))
	for i, a := range resp.Msg.Articles {
		articles[i] = &domain.Article{
			ID:      a.Id,
			Title:   a.Title,
			Content: a.Content,
			URL:     a.Url,
			UserID:  a.UserId,
		}
		if a.CreatedAt != nil {
			articles[i].CreatedAt = a.CreatedAt.AsTime()
		}
	}

	var nextCursor *domain.Cursor
	if resp.Msg.NextId != "" {
		nextCursor = &domain.Cursor{
			LastID: resp.Msg.NextId,
		}
		if resp.Msg.NextCreatedAt != nil {
			t := resp.Msg.NextCreatedAt.AsTime()
			nextCursor.LastCreatedAt = &t
		}
	}

	return articles, nextCursor, nil
}

// HasUnsummarizedArticles checks if there are articles without summaries via the backend API.
func (r *ArticleRepository) HasUnsummarizedArticles(ctx context.Context) (bool, error) {
	protoReq := &datahubv1.HasUnsummarizedArticlesRequest{}
	req := connect.NewRequest(protoReq)
	r.client.addAuth(req)

	resp, err := r.client.client.HasUnsummarizedArticles(ctx, req)
	if err != nil {
		return false, fmt.Errorf("HasUnsummarizedArticles: %w", err)
	}

	return resp.Msg.HasUnsummarized, nil
}

// FindByID finds an article by its ID.
func (r *ArticleRepository) FindByID(ctx context.Context, articleID string) (*domain.Article, error) {
	protoReq := &datahubv1.GetArticleContentRequest{ArticleId: articleID}
	req := connect.NewRequest(protoReq)
	r.client.addAuth(req)

	resp, err := r.client.client.GetArticleContent(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("GetArticleContent: %w", err)
	}

	return &domain.Article{
		ID:      resp.Msg.ArticleId,
		Title:   resp.Msg.Title,
		Content: resp.Msg.Content,
		URL:     resp.Msg.Url,
		UserID:  resp.Msg.UserId,
	}, nil
}

// FetchInoreaderArticles fetches articles from the pre-processor's own inoreader_articles table.
// This requires direct DB access since the data lives in the sidecar DB, not the backend API.
func (r *ArticleRepository) FetchInoreaderArticles(ctx context.Context, since time.Time) ([]*domain.Article, error) {
	return driver.GetInoreaderArticles(ctx, r.dbPool, since)
}

// FetchInoreaderArticlesForEmptyFeeds fetches inoreader articles for backfill.
// In API mode (split-DB), queries inoreader tables from pre-processor-db and
// resolves empty feedIDs via backend API using push-down anti-join.
// Only articles for feeds with zero existing articles are returned.
func (r *ArticleRepository) FetchInoreaderArticlesForEmptyFeeds(ctx context.Context, fetchedAfter time.Time, limit int) ([]*domain.Article, time.Time, error) {
	// Get inoreader articles with feed_urls (pre-processor-db only)
	candidates, err := driver.GetInoreaderArticlesForBackfill(ctx, r.dbPool, fetchedAfter, limit)
	if err != nil {
		return nil, time.Time{}, err
	}

	if len(candidates) == 0 {
		return nil, time.Time{}, nil
	}

	// Candidates come back ordered by fetched_at ASC, so the last one marks how
	// far this scan got — including the rows the empty-feed filter below drops.
	scannedThrough := candidates[len(candidates)-1].CreatedAt

	// feed_url → feedID (empty feed) cache
	emptyFeedCache := make(map[string]string) // feedURL → feedID ("" = no empty feed)

	var result []*domain.Article
	for _, article := range candidates {
		if article.FeedURL == "" {
			continue
		}

		feedID, cached := emptyFeedCache[article.FeedURL]
		if !cached {
			feedID, err = r.getEmptyFeedID(ctx, article.FeedURL)
			if err != nil {
				slog.WarnContext(ctx, "failed to get empty feed ID, skipping",
					"feedURL", article.FeedURL, "error", err)
				emptyFeedCache[article.FeedURL] = ""
				continue
			}
			emptyFeedCache[article.FeedURL] = feedID
		}
		if feedID == "" {
			continue // no empty feed → skip
		}

		article.FeedID = feedID
		result = append(result, article)
	}

	return result, scannedThrough, nil
}

// UpsertArticles batch upserts articles, discarding the report of what it
// refused. Callers that track a fetched_at watermark must use
// UpsertArticlesReportingSkipped instead.
func (r *ArticleRepository) UpsertArticles(ctx context.Context, articles []*domain.Article) error {
	_, err := r.UpsertArticlesReportingSkipped(ctx, articles)
	return err
}

// UpsertArticlesReportingSkipped batch upserts articles and returns the ones it
// refused to write. Articles with unresolvable feeds are skipped (matching
// legacy DB behavior), while real errors (network, auth) abort the batch
// immediately.
//
// The refusals are reported rather than swallowed because none of them is a
// property of the article: a missing inoreader_subscriptions row and a feed
// alt-db has not registered yet both get fixed later without ever touching
// inoreader_articles.fetched_at. A caller that advanced a fetched_at watermark
// over such a row would sink it below its own window forever.
//
// A per-batch FeedURL → FeedID cache coalesces GetFeedID calls so a fan-out of
// articles for the same feed only hits alt-backend once (Pillar 4, 2026-05-26).
// The cache also remembers misses ("" sentinel) so a single unregistered feed
// no longer produces N "feed not found" log lines — one warn per feed per
// batch is enough to drive operator action.
func (r *ArticleRepository) UpsertArticlesReportingSkipped(ctx context.Context, articles []*domain.Article) ([]*domain.Article, error) {
	if len(articles) == 0 {
		return nil, nil
	}

	feedIDCache := make(map[string]string) // FeedURL → FeedID; "" means already-resolved miss.

	var created int
	var skippedFeedNotFound int
	var skipped []*domain.Article
	for _, article := range articles {
		// Skip articles with empty FeedURL and no FeedID
		if article.FeedURL == "" && article.FeedID == "" {
			slog.WarnContext(ctx, "skipping article with empty FeedURL", "url", article.URL)
			skipped = append(skipped, article)
			continue
		}

		// Pre-resolve FeedID if not set
		if article.FeedID == "" {
			cached, hit := feedIDCache[article.FeedURL]
			if !hit {
				id, err := r.getFeedID(ctx, article.FeedURL)
				if err != nil {
					if !isNotFoundError(err) {
						// Real errors (network, auth, backend outage) abort the
						// batch immediately instead of being silently treated as
						// a missing feed — see doc comment above.
						return nil, fmt.Errorf("getFeedID for %s: %w", article.FeedURL, err)
					}
					// Feed not found in backend — log once per feed, cache the miss,
					// and skip the rest of the batch's articles for the same URL.
					slog.WarnContext(ctx, "feed not found, skipping articles for feed",
						"feedURL", article.FeedURL, "first_url", article.URL)
					feedIDCache[article.FeedURL] = ""
					cached = ""
				} else {
					if id == "" {
						slog.WarnContext(ctx, "feed not found, skipping articles for feed",
							"feedURL", article.FeedURL, "first_url", article.URL)
					}
					feedIDCache[article.FeedURL] = id
					cached = id
				}
			}
			if cached == "" {
				skippedFeedNotFound++
				skipped = append(skipped, article)
				continue
			}
			article.FeedID = cached
		}

		// Create article — real errors (network, auth, etc.) abort the batch
		if err := r.Create(ctx, article); err != nil {
			return nil, fmt.Errorf("upsert article %s: %w", article.URL, err)
		}
		created++
	}

	slog.InfoContext(ctx, "articles upserted via API",
		"created", created,
		"total", len(articles),
		"skipped_feed_not_found", skippedFeedNotFound)
	return skipped, nil
}

// UpsertArticlesWithFeedID batch inserts articles that already have FeedID resolved.
// Skips articles that already exist (DO NOTHING semantics) to avoid overwriting
// full-text articles with Inoreader RSS summaries.
func (r *ArticleRepository) UpsertArticlesWithFeedID(ctx context.Context, articles []*domain.Article) error {
	if len(articles) == 0 {
		return nil
	}

	var created, skipped int
	for _, article := range articles {
		if article.FeedID == "" || article.UserID == "" {
			slog.WarnContext(ctx, "skipping article with empty FeedID or UserID", "url", article.URL)
			continue
		}

		// Check existence first to avoid overwriting (ON CONFLICT DO NOTHING
		// semantics). The FeedID is already resolved here, so ask under it
		// directly — CheckExists would try to re-derive a feed from the
		// article URL and never find one.
		exists, err := r.CheckExistsWithFeedID(ctx, article.FeedID, []string{article.URL})
		if err != nil {
			slog.WarnContext(ctx, "failed to check article existence, skipping", "url", article.URL, "error", err)
			skipped++
			continue
		}
		if exists {
			skipped++
			continue
		}

		if err := r.Create(ctx, article); err != nil {
			return fmt.Errorf("insert backfill article %s: %w", article.URL, err)
		}
		created++
	}

	slog.InfoContext(ctx, "backfill articles inserted via API (skipped existing)", "created", created, "skipped", skipped, "total", len(articles))
	return nil
}

func (r *ArticleRepository) getEmptyFeedID(ctx context.Context, feedURL string) (string, error) {
	protoReq := &datahubv1.GetEmptyFeedIDRequest{FeedUrl: feedURL}
	req := connect.NewRequest(protoReq)
	r.client.addAuth(req)

	resp, err := r.client.client.GetEmptyFeedID(ctx, req)
	if err != nil {
		return "", err
	}
	return resp.Msg.FeedId, nil
}

func (r *ArticleRepository) getFeedID(ctx context.Context, feedURL string) (string, error) {
	protoReq := &datahubv1.GetFeedIDRequest{FeedUrl: feedURL}
	req := connect.NewRequest(protoReq)
	r.client.addAuth(req)

	resp, err := r.client.client.GetFeedID(ctx, req)
	if err != nil {
		return "", err
	}
	return resp.Msg.FeedId, nil
}

// isNotFoundError reports whether err is a Connect-RPC error carrying
// CodeNotFound, as opposed to a transport/auth/backend failure that must be
// treated as a real error rather than a missing-feed sentinel.
func isNotFoundError(err error) bool {
	return connect.CodeOf(err) == connect.CodeNotFound
}
