package knowledge_user_event_gateway

import (
	"alt/domain"
	"alt/driver/alt_db"
	"context"
	"fmt"
)

// Gateway implements knowledge user event port interfaces using AltDBRepository.
type Gateway struct {
	repo *alt_db.AltDBRepository
}

// NewGateway creates a new knowledge user event gateway.
func NewGateway(repo *alt_db.AltDBRepository) *Gateway {
	return &Gateway{repo: repo}
}

// AppendKnowledgeUserEvent implements knowledge_user_event_port.AppendKnowledgeUserEventPort.
func (g *Gateway) AppendKnowledgeUserEvent(ctx context.Context, event domain.KnowledgeUserEvent) error {
	if g.repo == nil {
		return fmt.Errorf("AppendKnowledgeUserEvent: database connection not available")
	}
	return g.repo.AppendKnowledgeUserEvent(ctx, event)
}
