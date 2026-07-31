package knowledge_backfill_gateway

import (
	"alt/domain"
	"context"
	"time"

	"github.com/google/uuid"
)

// backfillSource is catalog §2.N, served by alt-data-hub since ADR-000954
// Wave 3 batch 2. Only the alt_db reads cross that boundary: the sovereign
// append and the progress bookkeeping stay in the usecases above.
type backfillSource interface {
	CountBackfillArticles(ctx context.Context) (int, error)
	ListBackfillArticles(ctx context.Context, lastCreatedAt *time.Time, lastArticleID *uuid.UUID, limit int) ([]domain.KnowledgeBackfillArticle, error)
	CountBackfillSummaryTitles(ctx context.Context) (int, error)
	ListBackfillSummaryTitles(ctx context.Context, lastGeneratedAt *time.Time, lastSummaryVersionID *uuid.UUID, limit int) ([]domain.KnowledgeBackfillSummaryTitle, error)
}

// Gateway implements the backfill port interfaces.
type Gateway struct {
	repo backfillSource
}

// NewGateway panics on a nil source.
//
// The nil check it replaces returned an error per call, which the backfill
// jobs logged and carried on from — a run that appended nothing and reported
// success. Boot is the right place to find out (CLAUDE.md rule 8).
func NewGateway(repo backfillSource) *Gateway {
	if repo == nil {
		panic("knowledge_backfill_gateway: a backfill source is required — historic article reads moved to alt-data-hub in ADR-000954 Wave 3")
	}
	return &Gateway{repo: repo}
}

// ListBackfillArticles implements knowledge_backfill_port.ListBackfillArticlesPort.
func (g *Gateway) ListBackfillArticles(ctx context.Context, lastCreatedAt *time.Time, lastArticleID *uuid.UUID, limit int) ([]domain.KnowledgeBackfillArticle, error) {
	return g.repo.ListBackfillArticles(ctx, lastCreatedAt, lastArticleID, limit)
}

// CountBackfillArticles implements knowledge_backfill_port.CountBackfillArticlesPort.
func (g *Gateway) CountBackfillArticles(ctx context.Context) (int, error) {
	return g.repo.CountBackfillArticles(ctx)
}

// ListBackfillSummaryTitles implements
// knowledge_backfill_port.ListBackfillSummaryTitlesPort. ADR-000846.
func (g *Gateway) ListBackfillSummaryTitles(ctx context.Context, lastGeneratedAt *time.Time, lastSummaryVersionID *uuid.UUID, limit int) ([]domain.KnowledgeBackfillSummaryTitle, error) {
	return g.repo.ListBackfillSummaryTitles(ctx, lastGeneratedAt, lastSummaryVersionID, limit)
}

// CountBackfillSummaryTitles implements
// knowledge_backfill_port.CountBackfillSummaryTitlesPort. ADR-000846.
func (g *Gateway) CountBackfillSummaryTitles(ctx context.Context) (int, error) {
	return g.repo.CountBackfillSummaryTitles(ctx)
}
