package search_feed_usecase

import (
	"context"

	"alt/domain"
	"alt/port/feed_search_port"
)

type SearchFeedTitleUsecase struct {
	searchFeedTitlePort feed_search_port.SearchByTitlePort
}

func NewSearchFeedTitleUsecase(searchFeedTitlePort feed_search_port.SearchByTitlePort) *SearchFeedTitleUsecase {
	return &SearchFeedTitleUsecase{searchFeedTitlePort: searchFeedTitlePort}
}

func (u *SearchFeedTitleUsecase) Execute(ctx context.Context, query string) ([]*domain.FeedItem, error) {
	return u.searchFeedTitlePort.SearchByTitle(ctx, query)
}
