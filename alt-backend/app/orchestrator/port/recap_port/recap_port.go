package recap_port

import (
	"context"

	"alt/domain"
)

type RecapPort interface {
	GetSevenDayRecap(ctx context.Context) (*domain.RecapSummary, error)
	GetThreeDayRecap(ctx context.Context) (*domain.RecapSummary, error)
	GetThreeDayRecapCards(ctx context.Context) (*domain.RecapCardsResponse, error)
	GetEveningPulse(ctx context.Context, date string) (*domain.EveningPulse, error)
}
