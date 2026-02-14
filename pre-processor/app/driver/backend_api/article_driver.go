package backend_api

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	backendv1 "pre-processor/gen/proto/clients/preprocessor-backend/v1"

	"pre-processor/domain"
)

// ArticleRepository implements repository.ArticleRepository using the backend API.
type ArticleRepository struct {
	client *Client
}

// NewArticleRepository creates a new API-backed article repository.
func NewArticleRepository(client *Client) *ArticleRepository {
	return &ArticleRepository{client: client}
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

// FindForSummarization finds articles that need summarization.
// This operation requires direct DB access as it involves complex joins.
// For API mode, we return empty results; summarization is triggered via events.
func (r *ArticleRepository) FindForSummarization(ctx context.Context, cursor *domain.Cursor, limit int) ([]*domain.Article, *domain.Cursor, error) {
	// Not available via API - summarization is event-driven in API mode
	return nil, nil, nil
}

// HasUnsummarizedArticles checks if there are articles without summaries.
// In API mode, returns false as summarization is event-driven.
func (r *ArticleRepository) HasUnsummarizedArticles(ctx context.Context) (bool, error) {
	return false, nil
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

// FetchInoreaderArticles fetches articles from Inoreader source.
// This uses the sidecar's own tables and requires direct DB access.
func (r *ArticleRepository) FetchInoreaderArticles(ctx context.Context, since time.Time) ([]*domain.Article, error) {
	// Category B: requires sidecar DB, not available via backend API
	return nil, fmt.Errorf("FetchInoreaderArticles requires direct database access (category B)")
}

// UpsertArticles batch upserts articles.
func (r *ArticleRepository) UpsertArticles(ctx context.Context, articles []*domain.Article) error {
	for _, article := range articles {
		if err := r.Create(ctx, article); err != nil {
			return fmt.Errorf("upsert article %s: %w", article.URL, err)
		}
	}
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
