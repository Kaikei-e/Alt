package knowledge_home_gateway

import (
	"alt/domain"
	"alt/driver/alt_db"
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Gateway implements knowledge home port interfaces using AltDBRepository.
type Gateway struct {
	repo *alt_db.AltDBRepository
}

// NewGateway creates a new knowledge home gateway.
func NewGateway(repo *alt_db.AltDBRepository) *Gateway {
	return &Gateway{repo: repo}
}

// GetKnowledgeHomeItems implements knowledge_home_port.GetKnowledgeHomeItemsPort.
func (g *Gateway) GetKnowledgeHomeItems(ctx context.Context, userID uuid.UUID, cursor string, limit int, filter *domain.KnowledgeHomeLensFilter) ([]domain.KnowledgeHomeItem, string, bool, error) {
	if g.repo == nil {
		return nil, "", false, fmt.Errorf("GetKnowledgeHomeItems: database connection not available")
	}
	return g.repo.GetKnowledgeHomeItems(ctx, userID, cursor, limit, filter)
}

// UpsertKnowledgeHomeItem implements knowledge_home_port.UpsertKnowledgeHomeItemPort.
func (g *Gateway) UpsertKnowledgeHomeItem(ctx context.Context, item domain.KnowledgeHomeItem) error {
	if g.repo == nil {
		return fmt.Errorf("UpsertKnowledgeHomeItem: database connection not available")
	}
	return g.repo.UpsertKnowledgeHomeItem(ctx, item)
}

// DismissKnowledgeHomeItem implements knowledge_home_port.DismissKnowledgeHomeItemPort.
func (g *Gateway) DismissKnowledgeHomeItem(ctx context.Context, userID uuid.UUID, itemKey string, dismissedAt time.Time) error {
	if g.repo == nil {
		return fmt.Errorf("DismissKnowledgeHomeItem: database connection not available")
	}
	return g.repo.DismissKnowledgeHomeItem(ctx, userID, itemKey, dismissedAt)
}

// ClearSupersedeState implements knowledge_home_port.ClearSupersedeStatePort.
func (g *Gateway) ClearSupersedeState(ctx context.Context, userID uuid.UUID, itemKey string) error {
	if g.repo == nil {
		return fmt.Errorf("ClearSupersedeState: database connection not available")
	}
	return g.repo.ClearSupersedeState(ctx, userID, itemKey)
}
