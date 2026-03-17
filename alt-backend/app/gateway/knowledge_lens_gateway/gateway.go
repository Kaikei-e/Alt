package knowledge_lens_gateway

import (
	"alt/domain"
	"alt/driver/alt_db"
	"context"
	"fmt"

	"github.com/google/uuid"
)

type Gateway struct {
	repo *alt_db.AltDBRepository
}

func NewGateway(repo *alt_db.AltDBRepository) *Gateway {
	return &Gateway{repo: repo}
}

func (g *Gateway) CreateLens(ctx context.Context, lens domain.KnowledgeLens) error {
	if g.repo == nil {
		return fmt.Errorf("CreateLens: database connection not available")
	}
	return g.repo.CreateLens(ctx, lens)
}

func (g *Gateway) CreateLensVersion(ctx context.Context, version domain.KnowledgeLensVersion) error {
	if g.repo == nil {
		return fmt.Errorf("CreateLensVersion: database connection not available")
	}
	return g.repo.CreateLensVersion(ctx, version)
}

func (g *Gateway) ListLenses(ctx context.Context, userID uuid.UUID) ([]domain.KnowledgeLens, error) {
	if g.repo == nil {
		return nil, fmt.Errorf("ListLenses: database connection not available")
	}
	return g.repo.ListLenses(ctx, userID)
}

func (g *Gateway) GetLens(ctx context.Context, lensID uuid.UUID) (*domain.KnowledgeLens, error) {
	if g.repo == nil {
		return nil, fmt.Errorf("GetLens: database connection not available")
	}
	return g.repo.GetLens(ctx, lensID)
}

func (g *Gateway) GetCurrentLensVersion(ctx context.Context, lensID uuid.UUID) (*domain.KnowledgeLensVersion, error) {
	if g.repo == nil {
		return nil, fmt.Errorf("GetCurrentLensVersion: database connection not available")
	}
	return g.repo.GetCurrentLensVersion(ctx, lensID)
}

func (g *Gateway) SelectCurrentLens(ctx context.Context, current domain.KnowledgeCurrentLens) error {
	if g.repo == nil {
		return fmt.Errorf("SelectCurrentLens: database connection not available")
	}
	return g.repo.SelectCurrentLens(ctx, current)
}

func (g *Gateway) ArchiveLens(ctx context.Context, lensID uuid.UUID) error {
	if g.repo == nil {
		return fmt.Errorf("ArchiveLens: database connection not available")
	}
	return g.repo.ArchiveLens(ctx, lensID)
}
