package search_feed_usecase

import (
	"alt/domain"
	"alt/port/feed_search_port"
	"context"
	"log/slog"
)

type SearchFeedMeilisearchUsecase struct {
	searchPort feed_search_port.SearchFeedPort
	logger     *slog.Logger
}

func NewSearchFeedMeilisearchUsecase(searchPort feed_search_port.SearchFeedPort) *SearchFeedMeilisearchUsecase {
	return &SearchFeedMeilisearchUsecase{
		searchPort: searchPort,
		logger:     slog.Default(),
	}
}

func (u *SearchFeedMeilisearchUsecase) Execute(ctx context.Context, query string) ([]*domain.FeedItem, error) {
	u.logger.Info("executing feed search",
		"query", query)

	hits, err := u.searchPort.SearchFeeds(ctx, query)
	if err != nil {
		u.logger.Error("failed to search feeds",
			"error", err,
			"query", query)
		return nil, err
	}

	u.logger.Info("search completed",
		"query", query,
		"results_count", len(hits))

	feedItems := make([]*domain.FeedItem, len(hits))
	for i, hit := range hits {
		feedItems[i] = &domain.FeedItem{
			Title:       hit.Title,
			Description: hit.Content,
			Link:        "", // Note: search-indexer doesn't include URLs in search results
		}
	}

	return feedItems, nil
}