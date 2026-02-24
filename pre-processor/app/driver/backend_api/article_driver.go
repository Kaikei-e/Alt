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
	backendv1 "pre-processor/gen/proto/clients/preprocessor-backend/v1"
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

	protoReq := &backendv1.CreateArticleRequest{
		Title:   article.Title,
		Url:     article.URL,
		Content: article.Content,
		FeedId:  feedID,
		UserId:  article.UserID,
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

		// Try to get feed ID from the URL
		feedID, err := r.getFeedID(ctx, parsedURL.String())
		if err != nil || feedID == "" {
			continue
		}

		protoReq := &backendv1.CheckArticleExistsRequest{
			Url:    u,
			FeedId: feedID,
		}
		req := connect.NewRequest(protoReq)
		r.client.addAuth(req)

		resp, err := r.client.client.CheckArticleExists(ctx, req)
		if err != nil {
			continue
		}
		if resp.Msg.Exists {
			return true, nil
		}
	}
	return false, nil
}

// FindForSummarization finds articles that need summarization via the backend API.
func (r *ArticleRepository) FindForSummarization(ctx context.Context, cursor *domain.Cursor, limit int) ([]*domain.Article, *domain.Cursor, error) {
	protoReq := &backendv1.ListUnsummarizedArticlesRequest{
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
	protoReq := &backendv1.HasUnsummarizedArticlesRequest{}
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
	protoReq := &backendv1.GetArticleContentRequest{ArticleId: articleID}
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
	}, nil
}

// FetchInoreaderArticles fetches articles from the pre-processor's own inoreader_articles table.
// This requires direct DB access since the data lives in the sidecar DB, not the backend API.
func (r *ArticleRepository) FetchInoreaderArticles(ctx context.Context, since time.Time) ([]*domain.Article, error) {
	return driver.GetInoreaderArticles(ctx, r.dbPool, since)
}

// UpsertArticles batch upserts articles.
// Articles with unresolvable feeds are skipped (matching legacy DB behavior),
// while real errors (network, auth) abort the batch immediately.
func (r *ArticleRepository) UpsertArticles(ctx context.Context, articles []*domain.Article) error {
	if len(articles) == 0 {
		return nil
	}

	var created int
	for _, article := range articles {
		// Skip articles with empty FeedURL and no FeedID
		if article.FeedURL == "" && article.FeedID == "" {
			slog.WarnContext(ctx, "skipping article with empty FeedURL", "url", article.URL)
			continue
		}

		// Pre-resolve FeedID if not set
		if article.FeedID == "" {
			id, err := r.getFeedID(ctx, article.FeedURL)
			if err != nil {
				// Feed not found in backend — skip gracefully (matches legacy behavior)
				slog.WarnContext(ctx, "feed not found, skipping article",
					"feedURL", article.FeedURL, "url", article.URL)
				continue
			}
			if id == "" {
				slog.WarnContext(ctx, "feed not found, skipping article",
					"feedURL", article.FeedURL, "url", article.URL)
				continue
			}
			article.FeedID = id
		}

		// Create article — real errors (network, auth, etc.) abort the batch
		if err := r.Create(ctx, article); err != nil {
			return fmt.Errorf("upsert article %s: %w", article.URL, err)
		}
		created++
	}

	slog.InfoContext(ctx, "articles upserted via API", "created", created, "total", len(articles))
	return nil
}

func (r *ArticleRepository) getFeedID(ctx context.Context, feedURL string) (string, error) {
	protoReq := &backendv1.GetFeedIDRequest{FeedUrl: feedURL}
	req := connect.NewRequest(protoReq)
	r.client.addAuth(req)

	resp, err := r.client.client.GetFeedID(ctx, req)
	if err != nil {
		return "", err
	}
	return resp.Msg.FeedId, nil
}
