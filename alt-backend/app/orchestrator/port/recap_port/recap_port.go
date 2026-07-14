package recap_port

import (
	"context"

	"alt/domain"
)

type RecapPort interface {
	GetSevenDayRecap(ctx context.Context) (*domain.RecapSummary, error)
	GetThreeDayRecap(ctx context.Context) (*domain.RecapSummary, error)
	GetEveningPulse(ctx context.Context, date string) (*domain.EveningPulse, error)
	SearchRecapsByTag(ctx context.Context, tagName string, limit int) ([]*domain.RecapSearchResult, error)
	SearchRecapsByQuery(ctx context.Context, query string, limit int) ([]*domain.RecapSearchResult, error)
}
