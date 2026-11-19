package recap_port

import (
	"context"

	"alt/domain"
)

type RecapPort interface {
	GetThreeDayRecap(ctx context.Context) (*domain.RecapSummary, error)
	GetTopicCards(ctx context.Context) (*domain.RecapCardsResponse, error)
}
