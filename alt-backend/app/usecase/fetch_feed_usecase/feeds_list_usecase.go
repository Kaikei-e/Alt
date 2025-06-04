package fetch_feed_usecase

import (
	"alt/domain"
	"alt/port/fetch_feed_port"
	"context"
)

type FetchFeedsListUsecase struct {
	fetchFeedsListGateway fetch_feed_port.FetchFeedsPort
}

func NewFetchFeedsListUsecase(fetchFeedsListGateway fetch_feed_port.FetchFeedsPort) *FetchFeedsListUsecase {
	return &FetchFeedsListUsecase{fetchFeedsListGateway: fetchFeedsListGateway}
}

func (u *FetchFeedsListUsecase) Execute(ctx context.Context) ([]*domain.FeedItem, error) {
	return u.fetchFeedsListGateway.FetchFeedsList(ctx)
}

func (u *FetchFeedsListUsecase) ExecuteLimit(ctx context.Context, limit int) ([]*domain.FeedItem, error) {
	return u.fetchFeedsListGateway.FetchFeedsListLimit(ctx, limit)
}
