package fetch_feed_usecase

import (
	"alt/domain"
	"alt/port/fetch_feed_port"
)

type FetchSingleFeedUsecase struct {
	fetchSingleFeedPort fetch_feed_port.FetchSingleFeedPort
}

func NewFetchSingleFeedUsecase(fetchSingleFeedPort fetch_feed_port.FetchSingleFeedPort) *FetchSingleFeedUsecase {
	return &FetchSingleFeedUsecase{fetchSingleFeedPort: fetchSingleFeedPort}
}

func (u *FetchSingleFeedUsecase) Execute() (*domain.RSSFeed, error) {
	return u.fetchSingleFeedPort.FetchSingleFeed()
}