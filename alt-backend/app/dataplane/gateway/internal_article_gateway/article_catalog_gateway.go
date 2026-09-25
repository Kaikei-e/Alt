package internal_article_gateway

import (
	"context"
	"fmt"
	"time"

	"alt/dataplane/port/internal_article_port"
	"alt/shared/driver/alt_db"
)

// ArticleCatalogGateway implements internal article API ports using AltDBRepository.
type ArticleCatalogGateway struct {
	repo *alt_db.AltDBRepository
}

// NewArticleCatalogGateway creates a new article catalog gateway.
func NewArticleCatalogGateway(repo *alt_db.AltDBRepository) *ArticleCatalogGateway {
	return &ArticleCatalogGateway{repo: repo}
}

// ListArticlesWithTags implements ListArticlesWithTagsPort.
func (g *ArticleCatalogGateway) ListArticlesWithTags(ctx context.Context, lastCreatedAt *time.Time, lastID string, limit int) ([]*internal_article_port.ArticleWithTags, *time.Time, string, error) {
	driverArticles, nextCreatedAt, nextID, err := g.repo.ListArticlesWithTags(ctx, lastCreatedAt, lastID, limit)
	if err != nil {
		return nil, nil, "", fmt.Errorf("ListArticlesWithTags: %w", err)
	}

	return toPortArticles(driverArticles), nextCreatedAt, nextID, nil
}

// ListArticlesWithTagsForward implements ListArticlesWithTagsForwardPort.
func (g *ArticleCatalogGateway) ListArticlesWithTagsForward(ctx context.Context, incrementalMark *time.Time, lastCreatedAt *time.Time, lastID string, limit int) ([]*internal_article_port.ArticleWithTags, *time.Time, string, error) {
	driverArticles, nextCreatedAt, nextID, err := g.repo.ListArticlesWithTagsForward(ctx, incrementalMark, lastCreatedAt, lastID, limit)
	if err != nil {
		return nil, nil, "", fmt.Errorf("ListArticlesWithTagsForward: %w", err)
	}

	return toPortArticles(driverArticles), nextCreatedAt, nextID, nil
}

// ListDeletedArticles implements ListDeletedArticlesPort.
func (g *ArticleCatalogGateway) ListDeletedArticles(ctx context.Context, lastDeletedAt *time.Time, limit int) ([]*internal_article_port.DeletedArticle, *time.Time, error) {
	driverArticles, nextDeletedAt, err := g.repo.ListDeletedArticles(ctx, lastDeletedAt, limit)
	if err != nil {
		return nil, nil, fmt.Errorf("ListDeletedArticles: %w", err)
	}

	return toPortDeletedArticles(driverArticles), nextDeletedAt, nil
}

// GetLatestArticleTimestamp implements GetLatestArticleTimestampPort.
func (g *ArticleCatalogGateway) GetLatestArticleTimestamp(ctx context.Context) (*time.Time, error) {
	ts, err := g.repo.GetLatestArticleTimestamp(ctx)
	if err != nil {
		return nil, fmt.Errorf("GetLatestArticleTimestamp: %w", err)
	}
	return ts, nil
}

// GetArticleByID implements GetArticleByIDPort.
func (g *ArticleCatalogGateway) GetArticleByID(ctx context.Context, articleID string) (*internal_article_port.ArticleWithTags, error) {
	da, err := g.repo.GetArticleWithTagsByID(ctx, articleID)
	if err != nil {
		return nil, fmt.Errorf("GetArticleByID: %w", err)
	}
	return toPortArticle(da), nil
}

// ── Article write operations ──

// CheckArticleExists implements CheckArticleExistsPort.
func (g *ArticleCatalogGateway) CheckArticleExists(ctx context.Context, url string, feedID string) (bool, string, error) {
	exists, articleID, err := g.repo.CheckArticleExistsByURL(ctx, url, feedID)
	if err != nil {
		return false, "", fmt.Errorf("CheckArticleExists: %w", err)
	}
	return exists, articleID, nil
}

// CreateArticle implements CreateArticlePort.
func (g *ArticleCatalogGateway) CreateArticle(ctx context.Context, params internal_article_port.CreateArticleParams) (string, bool, error) {
	articleID, created, err := g.repo.CreateArticleInternal(ctx, toDriverCreateArticleParams(params))
	if err != nil {
		return "", false, fmt.Errorf("CreateArticle: %w", err)
	}
	return articleID, created, nil
}

// SaveArticleSummary implements SaveArticleSummaryPort.
//
// The title now travels instead of being hard-coded empty here. It was empty
// because the only caller — pre-processor — does not know it, and that is still
// what pre-processor sends; what changed is that alt-backend's summarise paths
// come through this procedure too since ADR-000954 summary version integration, and they do
// know it. An empty string still overwrites, which is correct: it is the
// writer's honest answer, not a missing argument.
//
// Language stops here. article_summaries has no language column, so this is the
// layer that knows the field has nowhere to go.
func (g *ArticleCatalogGateway) SaveArticleSummary(ctx context.Context, params internal_article_port.SaveArticleSummaryParams) error {
	err := g.repo.SaveArticleSummary(ctx, params.ArticleID, params.UserID, params.ArticleTitle, params.Summary)
	if err != nil {
		return fmt.Errorf("SaveArticleSummary: %w", err)
	}
	return nil
}

// GetArticleContent implements GetArticleContentPort.
func (g *ArticleCatalogGateway) GetArticleContent(ctx context.Context, articleID string) (*internal_article_port.ArticleContent, error) {
	ac, err := g.repo.GetArticleContent(ctx, articleID)
	if err != nil {
		return nil, fmt.Errorf("GetArticleContent: %w", err)
	}
	return toPortArticleContent(ac), nil
}

// ── Summary quality operations ──

// DeleteArticleSummary implements DeleteArticleSummaryPort.
func (g *ArticleCatalogGateway) DeleteArticleSummary(ctx context.Context, articleID string) error {
	err := g.repo.DeleteArticleSummaryByArticleID(ctx, articleID)
	if err != nil {
		return fmt.Errorf("DeleteArticleSummary: %w", err)
	}
	return nil
}

// CheckArticleSummaryExists implements CheckArticleSummaryExistsPort.
func (g *ArticleCatalogGateway) CheckArticleSummaryExists(ctx context.Context, articleID string) (bool, string, error) {
	exists, summaryID, err := g.repo.CheckArticleSummaryExists(ctx, articleID)
	if err != nil {
		return false, "", fmt.Errorf("CheckArticleSummaryExists: %w", err)
	}
	return exists, summaryID, nil
}

// FindArticlesWithSummaries implements FindArticlesWithSummariesPort.
func (g *ArticleCatalogGateway) FindArticlesWithSummaries(ctx context.Context, lastCreatedAt *time.Time, lastID string, limit int) ([]*internal_article_port.ArticleWithSummaryResult, *time.Time, string, error) {
	driverResults, nextCreatedAt, nextID, err := g.repo.FindArticlesWithSummaries(ctx, lastCreatedAt, lastID, limit)
	if err != nil {
		return nil, nil, "", fmt.Errorf("FindArticlesWithSummaries: %w", err)
	}

	return toPortArticlesWithSummaryResults(driverResults), nextCreatedAt, nextID, nil
}

// ── Summarization operations ──

// ListUnsummarizedArticles implements ListUnsummarizedArticlesPort.
func (g *ArticleCatalogGateway) ListUnsummarizedArticles(ctx context.Context, lastCreatedAt *time.Time, lastID string, limit int) ([]*internal_article_port.UnsummarizedArticle, *time.Time, string, error) {
	driverArticles, nextCreatedAt, nextID, err := g.repo.ListUnsummarizedArticles(ctx, lastCreatedAt, lastID, limit)
	if err != nil {
		return nil, nil, "", fmt.Errorf("ListUnsummarizedArticles: %w", err)
	}

	return toPortUnsummarizedArticles(driverArticles), nextCreatedAt, nextID, nil
}

// HasUnsummarizedArticles implements HasUnsummarizedArticlesPort.
func (g *ArticleCatalogGateway) HasUnsummarizedArticles(ctx context.Context) (bool, error) {
	has, err := g.repo.HasUnsummarizedArticles(ctx)
	if err != nil {
		return false, fmt.Errorf("HasUnsummarizedArticles: %w", err)
	}
	return has, nil
}
