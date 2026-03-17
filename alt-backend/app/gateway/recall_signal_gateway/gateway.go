package recall_signal_gateway

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

func (g *Gateway) AppendRecallSignal(ctx context.Context, signal domain.RecallSignal) error {
	if g.repo == nil {
		return fmt.Errorf("AppendRecallSignal: database connection not available")
	}
	return g.repo.AppendRecallSignal(ctx, signal)
}

func (g *Gateway) ListRecallSignalsByUser(ctx context.Context, userID uuid.UUID, sinceDays int) ([]domain.RecallSignal, error) {
	if g.repo == nil {
		return nil, fmt.Errorf("ListRecallSignalsByUser: database connection not available")
	}
	return g.repo.ListRecallSignalsByUser(ctx, userID, sinceDays)
}
