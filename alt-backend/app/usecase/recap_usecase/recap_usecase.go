package recap_usecase

import (
	"context"

	"alt/domain"
	"alt/port/recap_port"
)

type RecapUsecase struct {
	recapPort recap_port.RecapPort
}

func NewRecapUsecase(recapPort recap_port.RecapPort) *RecapUsecase {
	return &RecapUsecase{
		recapPort: recapPort,
	}
}

func (u *RecapUsecase) GetSevenDayRecap(ctx context.Context) (*domain.RecapSummary, error) {
	return u.recapPort.GetSevenDayRecap(ctx)
}
