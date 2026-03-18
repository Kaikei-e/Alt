package knowledge_event_gateway

import (
	"alt/domain"
	"alt/driver/alt_db"
	"context"
	"fmt"

	"github.com/google/uuid"
)

// Gateway implements knowledge event port interfaces using AltDBRepository.
type Gateway struct {
	repo *alt_db.AltDBRepository
}

// NewGateway creates a new knowledge event gateway.
func NewGateway(repo *alt_db.AltDBRepository) *Gateway {
	return &Gateway{repo: repo}
}

// AppendKnowledgeEvent implements knowledge_event_port.AppendKnowledgeEventPort.
func (g *Gateway) AppendKnowledgeEvent(ctx context.Context, event domain.KnowledgeEvent) error {
	if g.repo == nil {
		return fmt.Errorf("AppendKnowledgeEvent: database connection not available")
	}
	return g.repo.AppendKnowledgeEvent(ctx, event)
}

// ListKnowledgeEventsSince implements knowledge_event_port.ListKnowledgeEventsPort.
func (g *Gateway) ListKnowledgeEventsSince(ctx context.Context, afterSeq int64, limit int) ([]domain.KnowledgeEvent, error) {
	if g.repo == nil {
		return nil, fmt.Errorf("ListKnowledgeEventsSince: database connection not available")
	}
	return g.repo.ListKnowledgeEventsSince(ctx, afterSeq, limit)
}

// ListKnowledgeEventsSinceForUser implements knowledge_event_port.ListKnowledgeEventsForUserPort.
func (g *Gateway) ListKnowledgeEventsSinceForUser(ctx context.Context, userID uuid.UUID, afterSeq int64, limit int) ([]domain.KnowledgeEvent, error) {
	if g.repo == nil {
		return nil, fmt.Errorf("ListKnowledgeEventsSinceForUser: database connection not available")
	}
	return g.repo.ListKnowledgeEventsSinceForUser(ctx, userID, afterSeq, limit)
}
