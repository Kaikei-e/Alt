// Package internal_article_gateway provides gateway implementations for internal article API.
package internal_article_gateway

import (
	"context"
	"fmt"
	"time"

	"alt/driver/alt_db"
	"alt/port/internal_article_port"
	"alt/port/internal_feed_port"
	"alt/port/internal_tag_port"
)

// Gateway implements internal article API ports using AltDBRepository.
type Gateway struct {
	repo *alt_db.AltDBRepository
}

// NewGateway creates a new internal article gateway.
func NewGateway(repo *alt_db.AltDBRepository) *Gateway {
	return &Gateway{repo: repo}
}

// ListArticlesWithTags implements ListArticlesWithTagsPort.
func (g *Gateway) ListArticlesWithTags(ctx context.Context, lastCreatedAt *time.Time, lastID string, limit int) ([]*internal_article_port.ArticleWithTags, *time.Time, string, error) {
	driverArticles, nextCreatedAt, nextID, err := g.repo.ListArticlesWithTags(ctx, lastCreatedAt, lastID, limit)
	if err != nil {
		return nil, nil, "", fmt.Errorf("ListArticlesWithTags: %w", err)
	}

	articles := make([]*internal_article_port.ArticleWithTags, len(driverArticles))
	for i, da := range driverArticles {
		articles[i] = toPortArticle(da)
	}

	return articles, nextCreatedAt, nextID, nil
}

// ListArticlesWithTagsForward implements ListArticlesWithTagsForwardPort.
func (g *Gateway) ListArticlesWithTagsForward(ctx context.Context, incrementalMark *time.Time, lastCreatedAt *time.Time, lastID string, limit int) ([]*internal_article_port.ArticleWithTags, *time.Time, string, error) {
	driverArticles, nextCreatedAt, nextID, err := g.repo.ListArticlesWithTagsForward(ctx, incrementalMark, lastCreatedAt, lastID, limit)
	if err != nil {
		return nil, nil, "", fmt.Errorf("ListArticlesWithTagsForward: %w", err)
	}

	articles := make([]*internal_article_port.ArticleWithTags, len(driverArticles))
	for i, da := range driverArticles {
		articles[i] = toPortArticle(da)
	}

	return articles, nextCreatedAt, nextID, nil
}

// ListDeletedArticles implements ListDeletedArticlesPort.
func (g *Gateway) ListDeletedArticles(ctx context.Context, lastDeletedAt *time.Time, limit int) ([]*internal_article_port.DeletedArticle, *time.Time, error) {
	driverArticles, nextDeletedAt, err := g.repo.ListDeletedArticles(ctx, lastDeletedAt, limit)
	if err != nil {
		return nil, nil, fmt.Errorf("ListDeletedArticles: %w", err)
	}

	articles := make([]*internal_article_port.DeletedArticle, len(driverArticles))
	for i, da := range driverArticles {
		articles[i] = &internal_article_port.DeletedArticle{
			ID:        da.ID,
			DeletedAt: da.DeletedAt,
		}
	}

	return articles, nextDeletedAt, nil
}

// GetLatestArticleTimestamp implements GetLatestArticleTimestampPort.
func (g *Gateway) GetLatestArticleTimestamp(ctx context.Context) (*time.Time, error) {
	ts, err := g.repo.GetLatestArticleTimestamp(ctx)
	if err != nil {
		return nil, fmt.Errorf("GetLatestArticleTimestamp: %w", err)
	}
	return ts, nil
}

// GetArticleByID implements GetArticleByIDPort.
func (g *Gateway) GetArticleByID(ctx context.Context, articleID string) (*internal_article_port.ArticleWithTags, error) {
	da, err := g.repo.GetArticleWithTagsByID(ctx, articleID)
	if err != nil {
		return nil, fmt.Errorf("GetArticleByID: %w", err)
	}
	return toPortArticle(da), nil
}

// ── Phase 2: Article write operations ──

// CheckArticleExists implements CheckArticleExistsPort.
func (g *Gateway) CheckArticleExists(ctx context.Context, url string, feedID string) (bool, string, error) {
	exists, articleID, err := g.repo.CheckArticleExistsByURL(ctx, url, feedID)
	if err != nil {
		return false, "", fmt.Errorf("CheckArticleExists: %w", err)
	}
	return exists, articleID, nil
}

// CreateArticle implements CreateArticlePort.
func (g *Gateway) CreateArticle(ctx context.Context, params internal_article_port.CreateArticleParams) (string, error) {
	articleID, err := g.repo.CreateArticleInternal(ctx, alt_db.CreateArticleParams{
		Title:       params.Title,
		URL:         params.URL,
		Content:     params.Content,
		FeedID:      params.FeedID,
		UserID:      params.UserID,
		PublishedAt: params.PublishedAt,
	})
	if err != nil {
		return "", fmt.Errorf("CreateArticle: %w", err)
	}
	return articleID, nil
}

// SaveArticleSummary implements SaveArticleSummaryPort.
func (g *Gateway) SaveArticleSummary(ctx context.Context, articleID string, userID string, summary string, language string) error {
	err := g.repo.SaveArticleSummary(ctx, articleID, userID, "", summary)
	if err != nil {
		return fmt.Errorf("SaveArticleSummary: %w", err)
	}
	return nil
}

// GetArticleContent implements GetArticleContentPort.
func (g *Gateway) GetArticleContent(ctx context.Context, articleID string) (*internal_article_port.ArticleContent, error) {
	ac, err := g.repo.GetArticleContent(ctx, articleID)
	if err != nil {
		return nil, fmt.Errorf("GetArticleContent: %w", err)
	}
	if ac == nil {
		return nil, nil
	}
	return &internal_article_port.ArticleContent{
		ID:      ac.ID,
		Title:   ac.Title,
		Content: ac.Content,
		URL:     ac.URL,
		UserID:  ac.UserID,
	}, nil
}

// GetFeedID implements GetFeedIDPort.
func (g *Gateway) GetFeedID(ctx context.Context, feedURL string) (string, error) {
	feedID, err := g.repo.GetFeedIDByURL(ctx, feedURL)
	if err != nil {
		return "", fmt.Errorf("GetFeedID: %w", err)
	}
	return feedID, nil
}

// ListFeedURLs implements ListFeedURLsPort.
func (g *Gateway) ListFeedURLs(ctx context.Context, cursor string, limit int) ([]internal_feed_port.FeedURL, string, bool, error) {
	driverFeeds, nextCursor, hasMore, err := g.repo.ListFeedURLs(ctx, cursor, limit)
	if err != nil {
		return nil, "", false, fmt.Errorf("ListFeedURLs: %w", err)
	}

	feeds := make([]internal_feed_port.FeedURL, len(driverFeeds))
	for i, df := range driverFeeds {
		feeds[i] = internal_feed_port.FeedURL{
			FeedID: df.FeedID,
			URL:    df.URL,
		}
	}

	return feeds, nextCursor, hasMore, nil
}

// ── Phase 3: Tag operations ──

// UpsertArticleTags implements UpsertArticleTagsPort.
func (g *Gateway) UpsertArticleTags(ctx context.Context, articleID string, feedID string, tags []internal_tag_port.TagItem) (int32, error) {
	driverTags := make([]alt_db.TagUpsertItem, len(tags))
	for i, t := range tags {
		driverTags[i] = alt_db.TagUpsertItem{Name: t.Name, Confidence: t.Confidence}
	}

	count, err := g.repo.UpsertArticleTags(ctx, articleID, feedID, driverTags)
	if err != nil {
		return 0, fmt.Errorf("UpsertArticleTags: %w", err)
	}
	return count, nil
}

// BatchUpsertArticleTags implements BatchUpsertArticleTagsPort.
func (g *Gateway) BatchUpsertArticleTags(ctx context.Context, items []internal_tag_port.BatchUpsertItem) (int32, error) {
	driverItems := make([]alt_db.BatchUpsertTagItem, len(items))
	for i, item := range items {
		driverTags := make([]alt_db.TagUpsertItem, len(item.Tags))
		for j, t := range item.Tags {
			driverTags[j] = alt_db.TagUpsertItem{Name: t.Name, Confidence: t.Confidence}
		}
		driverItems[i] = alt_db.BatchUpsertTagItem{
			ArticleID: item.ArticleID,
			FeedID:    item.FeedID,
			Tags:      driverTags,
		}
	}

	total, err := g.repo.BatchUpsertArticleTags(ctx, driverItems)
	if err != nil {
		return 0, fmt.Errorf("BatchUpsertArticleTags: %w", err)
	}
	return total, nil
}

// ListUntaggedArticles implements ListUntaggedArticlesPort.
func (g *Gateway) ListUntaggedArticles(ctx context.Context, limit int, offset int) ([]internal_tag_port.UntaggedArticle, int32, error) {
	driverArticles, totalCount, err := g.repo.ListUntaggedArticles(ctx, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("ListUntaggedArticles: %w", err)
	}

	articles := make([]internal_tag_port.UntaggedArticle, len(driverArticles))
	for i, da := range driverArticles {
		articles[i] = internal_tag_port.UntaggedArticle{
			ID:      da.ID,
			Title:   da.Title,
			Content: da.Content,
			UserID:  da.UserID,
			FeedID:  da.FeedID,
		}
	}

	return articles, totalCount, nil
}

// ── Phase 4: Summary quality operations ──

// DeleteArticleSummary implements DeleteArticleSummaryPort.
func (g *Gateway) DeleteArticleSummary(ctx context.Context, articleID string) error {
	err := g.repo.DeleteArticleSummaryByArticleID(ctx, articleID)
	if err != nil {
		return fmt.Errorf("DeleteArticleSummary: %w", err)
	}
	return nil
}

// CheckArticleSummaryExists implements CheckArticleSummaryExistsPort.
func (g *Gateway) CheckArticleSummaryExists(ctx context.Context, articleID string) (bool, string, error) {
	exists, summaryID, err := g.repo.CheckArticleSummaryExists(ctx, articleID)
	if err != nil {
		return false, "", fmt.Errorf("CheckArticleSummaryExists: %w", err)
	}
	return exists, summaryID, nil
}

// FindArticlesWithSummaries implements FindArticlesWithSummariesPort.
func (g *Gateway) FindArticlesWithSummaries(ctx context.Context, lastCreatedAt *time.Time, lastID string, limit int) ([]*internal_article_port.ArticleWithSummaryResult, *time.Time, string, error) {
	driverResults, nextCreatedAt, nextID, err := g.repo.FindArticlesWithSummaries(ctx, lastCreatedAt, lastID, limit)
	if err != nil {
		return nil, nil, "", fmt.Errorf("FindArticlesWithSummaries: %w", err)
	}

	results := make([]*internal_article_port.ArticleWithSummaryResult, len(driverResults))
	for i, dr := range driverResults {
		results[i] = &internal_article_port.ArticleWithSummaryResult{
			ArticleID:       dr.ArticleID,
			ArticleContent:  dr.ArticleContent,
			ArticleURL:      dr.ArticleURL,
			SummaryID:       dr.SummaryID,
			SummaryJapanese: dr.SummaryJapanese,
			CreatedAt:       dr.CreatedAt,
		}
	}

	return results, nextCreatedAt, nextID, nil
}

// ── Summarization operations ──

// ListUnsummarizedArticles implements ListUnsummarizedArticlesPort.
func (g *Gateway) ListUnsummarizedArticles(ctx context.Context, lastCreatedAt *time.Time, lastID string, limit int) ([]*internal_article_port.UnsummarizedArticle, *time.Time, string, error) {
	driverArticles, nextCreatedAt, nextID, err := g.repo.ListUnsummarizedArticles(ctx, lastCreatedAt, lastID, limit)
	if err != nil {
		return nil, nil, "", fmt.Errorf("ListUnsummarizedArticles: %w", err)
	}

	articles := make([]*internal_article_port.UnsummarizedArticle, len(driverArticles))
	for i, da := range driverArticles {
		articles[i] = &internal_article_port.UnsummarizedArticle{
			ID:        da.ID,
			Title:     da.Title,
			Content:   da.Content,
			URL:       da.URL,
			CreatedAt: da.CreatedAt,
			UserID:    da.UserID,
		}
	}

	return articles, nextCreatedAt, nextID, nil
}

// HasUnsummarizedArticles implements HasUnsummarizedArticlesPort.
func (g *Gateway) HasUnsummarizedArticles(ctx context.Context) (bool, error) {
	has, err := g.repo.HasUnsummarizedArticles(ctx)
	if err != nil {
		return false, fmt.Errorf("HasUnsummarizedArticles: %w", err)
	}
	return has, nil
}

func toPortArticle(da *alt_db.InternalArticleWithTags) *internal_article_port.ArticleWithTags {
	return &internal_article_port.ArticleWithTags{
		ID:        da.ID,
		Title:     da.Title,
		Content:   da.Content,
		Tags:      da.Tags,
		CreatedAt: da.CreatedAt,
		UserID:    da.UserID,
	}
}
