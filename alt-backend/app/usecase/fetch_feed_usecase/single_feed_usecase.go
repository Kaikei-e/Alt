package fetch_feed_usecase

import (
	"alt/domain"
	"alt/port/fetch_feed_port"
	"context"
)

type FetchSingleFeedUsecase struct {
	fetchSingleFeedPort fetch_feed_port.FetchSingleFeedPort
}

func NewFetchSingleFeedUsecase(fetchSingleFeedPort fetch_feed_port.FetchSingleFeedPort) *FetchSingleFeedUsecase {
	return &FetchSingleFeedUsecase{fetchSingleFeedPort: fetchSingleFeedPort}
}

func (u *FetchSingleFeedUsecase) Execute(ctx context.Context) (*domain.RSSFeed, error) {
	gateway := u.fetchSingleFeedPort
	feed, err := gateway.FetchSingleFeed(ctx)
	if err != nil {
		return nil, err
	}
	return feed, nil
}
