package internal_article_gateway

import (
	"context"
	"fmt"
	"time"

	"alt/dataplane/port/internal_tag_port"
	"alt/shared/driver/alt_db"
)

// TagCatalogGateway implements internal tag API ports using AltDBRepository.
type TagCatalogGateway struct {
	repo *alt_db.AltDBRepository
}

// NewTagCatalogGateway creates a new tag catalog gateway.
func NewTagCatalogGateway(repo *alt_db.AltDBRepository) *TagCatalogGateway {
	return &TagCatalogGateway{repo: repo}
}

// ── Tag operations ──

// UpsertArticleTags implements UpsertArticleTagsPort.
func (g *TagCatalogGateway) UpsertArticleTags(ctx context.Context, articleID string, feedID string, tags []internal_tag_port.TagItem) (int32, error) {
	count, err := g.repo.UpsertArticleTags(ctx, articleID, feedID, toDriverTagUpsertItems(tags))
	if err != nil {
		return 0, fmt.Errorf("UpsertArticleTags: %w", err)
	}
	return count, nil
}

// BatchUpsertArticleTags implements BatchUpsertArticleTagsPort.
func (g *TagCatalogGateway) BatchUpsertArticleTags(ctx context.Context, items []internal_tag_port.BatchUpsertItem) (int32, error) {
	total, err := g.repo.BatchUpsertArticleTags(ctx, toDriverBatchUpsertTagItems(items))
	if err != nil {
		return 0, fmt.Errorf("BatchUpsertArticleTags: %w", err)
	}
	return total, nil
}

// BatchGetTagsByArticleIDs implements BatchGetTagsByArticleIDsPort.
// It joins the driver rows (one row per (article, tag) pair) into
// the port-level grouped shape expected by the Connect-RPC handler.
func (g *TagCatalogGateway) BatchGetTagsByArticleIDs(ctx context.Context, articleIDs []string) ([]internal_tag_port.ArticleTagsByID, error) {
	if len(articleIDs) == 0 {
		return nil, nil
	}

	rows, err := g.repo.BatchGetTagsByArticleIDs(ctx, articleIDs)
	if err != nil {
		return nil, fmt.Errorf("BatchGetTagsByArticleIDs: %w", err)
	}

	return groupTagsByArticleIDs(rows, articleIDs), nil
}

// ListUntaggedArticles implements ListUntaggedArticlesPort.
func (g *TagCatalogGateway) ListUntaggedArticles(ctx context.Context, lastCreatedAt *time.Time, lastID string, limit int) ([]internal_tag_port.UntaggedArticle, *time.Time, string, int32, error) {
	driverArticles, nextCreatedAt, nextID, totalCount, err := g.repo.ListUntaggedArticles(ctx, lastCreatedAt, lastID, limit)
	if err != nil {
		return nil, nil, "", 0, fmt.Errorf("ListUntaggedArticles: %w", err)
	}

	return toPortUntaggedArticles(driverArticles), nextCreatedAt, nextID, totalCount, nil
}
