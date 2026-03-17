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

// CreateBackfillJob implements knowledge_backfill_port.CreateBackfillJobPort.
func (g *Gateway) CreateBackfillJob(ctx context.Context, job domain.KnowledgeBackfillJob) error {
	if g.repo == nil {
		return fmt.Errorf("CreateBackfillJob: database connection not available")
	}
	return g.repo.CreateBackfillJob(ctx, job)
}

// GetBackfillJob implements knowledge_backfill_port.GetBackfillJobPort.
func (g *Gateway) GetBackfillJob(ctx context.Context, jobID uuid.UUID) (*domain.KnowledgeBackfillJob, error) {
	if g.repo == nil {
		return nil, fmt.Errorf("GetBackfillJob: database connection not available")
	}
	return g.repo.GetBackfillJob(ctx, jobID)
}

// UpdateBackfillJob implements knowledge_backfill_port.UpdateBackfillJobPort.
func (g *Gateway) UpdateBackfillJob(ctx context.Context, job domain.KnowledgeBackfillJob) error {
	if g.repo == nil {
		return fmt.Errorf("UpdateBackfillJob: database connection not available")
	}
	return g.repo.UpdateBackfillJob(ctx, job)
}

// ListBackfillJobs implements knowledge_backfill_port.ListBackfillJobsPort.
func (g *Gateway) ListBackfillJobs(ctx context.Context) ([]domain.KnowledgeBackfillJob, error) {
	if g.repo == nil {
		return nil, fmt.Errorf("ListBackfillJobs: database connection not available")
	}
	return g.repo.ListBackfillJobs(ctx)
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
