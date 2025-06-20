package search_feed_usecase

import (
	"alt/domain"
	"alt/driver/models"
	"alt/port/feed_search_port"
	"alt/port/feed_url_link_port"
	"context"
	"log/slog"
)

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

	// Extract article IDs from search hits
	articleIDs := make([]string, len(hits))
	for i, hit := range hits {
		articleIDs[i] = hit.ID
	}

	u.logger.Info("extracted article IDs", "article_ids", articleIDs)

	// Get feed URLs for the article IDs
	var feedAndArticles []models.FeedAndArticle
	if len(articleIDs) > 0 {
		feedAndArticles, err = u.urlPort.GetFeedURLsByArticleIDs(ctx, articleIDs)
		if err != nil {
			u.logger.Error("failed to get feed URLs",
				"error", err,
				"article_count", len(articleIDs))
			return nil, err
		}
		u.logger.Info("retrieved feed URLs",
			"requested_count", len(articleIDs),
			"found_count", len(feedAndArticles))
	} else {
		feedAndArticles = make([]models.FeedAndArticle, 0)
	}

	// Create a map for quick URL lookup by article ID
	urlMap := make(map[string]string)
	for _, fa := range feedAndArticles {
		urlMap[fa.ArticleID] = fa.URL
	}

	// Map search hits to feed items with URLs
	feedItems := make([]*domain.FeedItem, len(hits))
	for i, hit := range hits {
		feedItems[i] = &domain.FeedItem{
			Title:       hit.Title,
			Description: hit.Content,
			Link:        urlMap[hit.ID], // Use map lookup, defaults to empty string if not found
		}
	}

	return feedItems, nil
}
