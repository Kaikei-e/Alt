package knowledge_backfill_gateway

import (
	"alt/domain"
	"alt/driver/alt_db"
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Gateway implements backfill port interfaces using AltDBRepository.
type Gateway struct {
	repo *alt_db.AltDBRepository
}

// NewGateway creates a new knowledge backfill gateway.
func NewGateway(repo *alt_db.AltDBRepository) *Gateway {
	return &Gateway{repo: repo}
}

// ListBackfillArticles implements knowledge_backfill_port.ListBackfillArticlesPort.
func (g *Gateway) ListBackfillArticles(ctx context.Context, lastCreatedAt *time.Time, lastArticleID *uuid.UUID, limit int) ([]domain.KnowledgeBackfillArticle, error) {
	if g.repo == nil {
		return nil, fmt.Errorf("ListBackfillArticles: database connection not available")
	}
	return g.repo.ListBackfillArticles(ctx, lastCreatedAt, lastArticleID, limit)
}

// CountBackfillArticles implements knowledge_backfill_port.CountBackfillArticlesPort.
func (g *Gateway) CountBackfillArticles(ctx context.Context) (int, error) {
	if g.repo == nil {
		return 0, fmt.Errorf("CountBackfillArticles: database connection not available")
	}
	return g.repo.CountBackfillArticles(ctx)
}

// ListBackfillSummaryTitles implements
// knowledge_backfill_port.ListBackfillSummaryTitlesPort. ADR-000846.
func (g *Gateway) ListBackfillSummaryTitles(ctx context.Context, lastGeneratedAt *time.Time, lastSummaryVersionID *uuid.UUID, limit int) ([]domain.KnowledgeBackfillSummaryTitle, error) {
	if g.repo == nil {
		return nil, fmt.Errorf("ListBackfillSummaryTitles: database connection not available")
	}
	return g.repo.ListBackfillSummaryTitles(ctx, lastGeneratedAt, lastSummaryVersionID, limit)
}

// CountBackfillSummaryTitles implements
// knowledge_backfill_port.CountBackfillSummaryTitlesPort. ADR-000846.
func (g *Gateway) CountBackfillSummaryTitles(ctx context.Context) (int, error) {
	if g.repo == nil {
		return 0, fmt.Errorf("CountBackfillSummaryTitles: database connection not available")
	}
	return g.repo.CountBackfillSummaryTitles(ctx)
}
