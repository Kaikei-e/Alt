package recap_usecase

import (
	"context"

	"alt/domain"
	"alt/orchestrator/port/recap_port"
	"alt/orchestrator/port/search_indexer_port"
)

type RecapUsecase struct {
	recapPort   recap_port.RecapPort
	recapSearch search_indexer_port.RecapSearchPort
}

func NewRecapUsecase(recapPort recap_port.RecapPort, recapSearch search_indexer_port.RecapSearchPort) *RecapUsecase {
	return &RecapUsecase{
		recapPort:   recapPort,
		recapSearch: recapSearch,
	}
}

func (u *RecapUsecase) GetSevenDayRecap(ctx context.Context) (*domain.RecapSummary, error) {
	return u.recapPort.GetSevenDayRecap(ctx)
}

func (u *RecapUsecase) GetThreeDayRecap(ctx context.Context) (*domain.RecapSummary, error) {
	return u.recapPort.GetThreeDayRecap(ctx)
}

func (u *RecapUsecase) GetThreeDayRecapCards(ctx context.Context) (*domain.RecapCardsResponse, error) {
	return u.recapPort.GetThreeDayRecapCards(ctx)
}

func (u *RecapUsecase) GetEveningPulse(ctx context.Context, date string) (*domain.EveningPulse, error) {
	return u.recapPort.GetEveningPulse(ctx, date)
}

func (u *RecapUsecase) SearchRecapsByTag(ctx context.Context, tagName string, limit int) ([]*domain.RecapSearchResult, error) {
	return u.recapSearch.SearchRecapsByTag(ctx, tagName, limit)
}

func (u *RecapUsecase) SearchRecapsByQuery(ctx context.Context, query string, limit int) ([]*domain.RecapSearchResult, error) {
	results, _, err := u.recapSearch.SearchRecapsByQuery(ctx, query, limit)
	return results, err
}
