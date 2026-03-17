package recall_candidate_gateway

import (
	"alt/domain"
	"alt/driver/alt_db"
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type Gateway struct {
	repo *alt_db.AltDBRepository
}

func NewGateway(repo *alt_db.AltDBRepository) *Gateway {
	return &Gateway{repo: repo}
}

func (g *Gateway) GetRecallCandidates(ctx context.Context, userID uuid.UUID, limit int) ([]domain.RecallCandidate, error) {
	if g.repo == nil {
		return nil, fmt.Errorf("GetRecallCandidates: database connection not available")
	}
	return g.repo.GetRecallCandidates(ctx, userID, limit)
}

func (g *Gateway) UpsertRecallCandidate(ctx context.Context, candidate domain.RecallCandidate) error {
	if g.repo == nil {
		return fmt.Errorf("UpsertRecallCandidate: database connection not available")
	}
	return g.repo.UpsertRecallCandidate(ctx, candidate)
}

func (g *Gateway) SnoozeRecallCandidate(ctx context.Context, userID uuid.UUID, itemKey string, until time.Time) error {
	if g.repo == nil {
		return fmt.Errorf("SnoozeRecallCandidate: database connection not available")
	}
	return g.repo.SnoozeRecallCandidate(ctx, userID, itemKey, until)
}

func (g *Gateway) DismissRecallCandidate(ctx context.Context, userID uuid.UUID, itemKey string) error {
	if g.repo == nil {
		return fmt.Errorf("DismissRecallCandidate: database connection not available")
	}
	return g.repo.DismissRecallCandidate(ctx, userID, itemKey)
}
