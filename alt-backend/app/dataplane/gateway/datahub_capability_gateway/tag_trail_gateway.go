package datahub_capability_gateway

import (
	"context"
	"fmt"
	"time"

	"alt/domain"
	"alt/shared/driver/alt_db"
)

// ---------------------------------------------------------------------------
// §2.J Tag Trail paging (ADR-000954 Wave 3 batch 6)
// ---------------------------------------------------------------------------

type tagTrailDriver interface {
	FetchArticlesByTag(ctx context.Context, tagID string, cursor *time.Time, limit int) ([]*domain.TagTrailArticle, error)
	FetchArticlesByTagName(ctx context.Context, tagName string, cursor *time.Time, limit int) ([]*domain.TagTrailArticle, error)
}

// TagTrailGateway implements datahub_capability_port.TagTrailPort.
type TagTrailGateway struct {
	db tagTrailDriver
}

func NewTagTrailGateway(db *alt_db.AltDBRepository) *TagTrailGateway {
	return &TagTrailGateway{db: db}
}

func (g *TagTrailGateway) ArticlesByTagID(ctx context.Context, tagID string, cursor *time.Time, limit int) ([]*domain.TagTrailArticle, error) {
	articles, err := g.db.FetchArticlesByTag(ctx, tagID, cursor, limit)
	if err != nil {
		return nil, fmt.Errorf("list articles by tag id %s: %w", tagID, err)
	}
	return articles, nil
}

func (g *TagTrailGateway) ArticlesByTagName(ctx context.Context, tagName string, cursor *time.Time, limit int) ([]*domain.TagTrailArticle, error) {
	articles, err := g.db.FetchArticlesByTagName(ctx, tagName, cursor, limit)
	if err != nil {
		return nil, fmt.Errorf("list articles by tag name %q: %w", tagName, err)
	}
	return articles, nil
}

// ---------------------------------------------------------------------------
// §2.C Article reference for the recall rail (ADR-000954 Wave 3 batch 6)
// ---------------------------------------------------------------------------

type articleRefDriver interface {
	GetArticleTitleAndURL(ctx context.Context, articleID string) (*domain.ArticleRef, error)
}

// ArticleRefGateway implements datahub_capability_port.ArticleRefPort.
type ArticleRefGateway struct {
	db articleRefDriver
}

func NewArticleRefGateway(db *alt_db.AltDBRepository) *ArticleRefGateway {
	return &ArticleRefGateway{db: db}
}

// ArticleRef passes the driver's nil straight through. A missing article is
// not wrapped into an error here for the same reason the driver does not raise
// one: the caller's question is "is this still renderable", and "no" is an
// answer rather than a fault.
func (g *ArticleRefGateway) ArticleRef(ctx context.Context, articleID string) (*domain.ArticleRef, error) {
	ref, err := g.db.GetArticleTitleAndURL(ctx, articleID)
	if err != nil {
		return nil, fmt.Errorf("get article ref %s: %w", articleID, err)
	}
	return ref, nil
}
