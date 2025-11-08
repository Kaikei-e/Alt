package recap_port

import (
	"context"

	"alt/domain"
)

type RecapPort interface {
	GetSevenDayRecap(ctx context.Context) (*domain.RecapSummary, error)
}
