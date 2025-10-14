package search_feed_usecase

import (
	"alt/domain"
	"alt/port/feed_search_port"
	"alt/port/feed_url_link_port"
	"context"
	"log/slog"
)

type SearchFeedByTitleUsecase struct {
	searchByTitlePort feed_search_port.SearchByTitlePort
	logger            *slog.Logger
}

func NewSearchFeedByTitleUsecase(searchByTitlePort feed_search_port.SearchByTitlePort) *SearchFeedByTitleUsecase {
	return &SearchFeedByTitleUsecase{
		searchByTitlePort: searchByTitlePort,
		logger:            slog.Default(),
	}
}

func (u *SearchFeedByTitleUsecase) Execute(ctx context.Context, query string) ([]*domain.FeedItem, error) {
	// contextからuser取得
	user, err := domain.GetUserFromContext(ctx)
	if err != nil {
		u.logger.Error("user context not found", "error", err)
		return nil, err
	}

	u.logger.Info("executing feed search by title",
		"query", query,
		"user_id", user.UserID)

	feeds, err := u.searchByTitlePort.SearchFeedsByTitle(ctx, query, user.UserID.String())
	if err != nil {
		u.logger.Error("failed to search feeds by title",
			"error", err,
			"query", query,
			"user_id", user.UserID)
		return nil, err
	}

	u.logger.Info("feed search by title completed",
		"query", query,
		"user_id", user.UserID,
		"results_count", len(feeds))

	return feeds, nil
}

type SearchFeedMeilisearchUsecase struct {
	searchPort feed_search_port.SearchFeedPort
	urlPort    feed_url_link_port.FeedURLLinkPort
	logger     *slog.Logger
}

func NewSearchFeedMeilisearchUsecase(searchPort feed_search_port.SearchFeedPort, urlPort feed_url_link_port.FeedURLLinkPort) *SearchFeedMeilisearchUsecase {
	return &SearchFeedMeilisearchUsecase{
		searchPort: searchPort,
		urlPort:    urlPort,
		logger:     slog.Default(),
	}
}

func (u *SearchFeedMeilisearchUsecase) Execute(ctx context.Context, query string) ([]*domain.FeedItem, error) {
	u.logger.Info("executing feed search via meilisearch", "query", query)

	// Search for articles using Meilisearch
	searchHits, err := u.searchPort.SearchFeeds(ctx, query)
	if err != nil {
		u.logger.Error("failed to search feeds via meilisearch", "error", err, "query", query)
		return nil, err
	}

	if len(searchHits) == 0 {
		u.logger.Info("no search results found", "query", query)
		return []*domain.FeedItem{}, nil
	}

	// Extract article IDs for URL lookup
	articleIDs := make([]string, len(searchHits))
	for i, hit := range searchHits {
		articleIDs[i] = hit.ID
	}

	// Get feed URLs for the articles
	feedURLs, err := u.urlPort.GetFeedURLsByArticleIDs(ctx, articleIDs)
	if err != nil {
		u.logger.Error("failed to get feed URLs", "error", err, "article_ids", articleIDs)
		return nil, err
	}

	// Create URL map for quick lookup
	urlMap := make(map[string]string)
	for _, feedURL := range feedURLs {
		urlMap[feedURL.ArticleID] = feedURL.URL
	}

	// Convert search hits to feed items
	feedItems := make([]*domain.FeedItem, len(searchHits))
	for i, hit := range searchHits {
		feedItems[i] = &domain.FeedItem{
			Title:       hit.Title,
			Description: hit.Content,
			Link:        urlMap[hit.ID], // Will be empty string if not found
		}
	}

	u.logger.Info("feed search via meilisearch completed",
		"query", query,
		"results_count", len(feedItems))

	return feedItems, nil
}
