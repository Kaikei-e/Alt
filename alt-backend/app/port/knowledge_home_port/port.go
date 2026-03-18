package knowledge_home_port

import (
	"alt/domain"
	"context"
	"time"

	"github.com/google/uuid"
)

// GetKnowledgeHomeItemsPort reads items from the knowledge home projection.
type GetKnowledgeHomeItemsPort interface {
	GetKnowledgeHomeItems(ctx context.Context, userID uuid.UUID, cursor string, limit int, filter *domain.KnowledgeHomeLensFilter) ([]domain.KnowledgeHomeItem, string, bool, error)
}

// UpsertKnowledgeHomeItemPort writes items to the knowledge home projection.
type UpsertKnowledgeHomeItemPort interface {
	UpsertKnowledgeHomeItem(ctx context.Context, item domain.KnowledgeHomeItem) error
}

// DismissKnowledgeHomeItemPort marks an item as dismissed so it no longer appears in Home.
type DismissKnowledgeHomeItemPort interface {
	DismissKnowledgeHomeItem(ctx context.Context, userID uuid.UUID, itemKey string, dismissedAt time.Time) error
}

// ClearSupersedeStatePort clears supersede state for an item after user acknowledgement (e.g. open).
type ClearSupersedeStatePort interface {
	ClearSupersedeState(ctx context.Context, userID uuid.UUID, itemKey string) error
}
