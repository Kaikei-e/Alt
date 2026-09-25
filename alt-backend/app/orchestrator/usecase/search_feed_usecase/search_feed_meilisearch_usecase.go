package search_feed_usecase

import (
	"alt/domain"
	"alt/orchestrator/port/feed_search_port"
	"alt/orchestrator/port/feed_url_link_port"
	"context"
	"errors"
	"log/slog"
)

// convertHitsToFeedItems maps search article hits to domain FeedItems using the resolved URL map.
func convertHitsToFeedItems(hits []domain.SearchArticleHit, urlMap map[string]string) []*domain.FeedItem {
	feedItems := make([]*domain.FeedItem, len(hits))
	for i, hit := range hits {
		feedItems[i] = &domain.FeedItem{
			Title:       hit.Title,
			Description: hit.Content,
			Link:        urlMap[hit.ID],
			ArticleID:   hit.ID,
		}
	}
	return feedItems
}

const maxSearchResults = 200

// hasMoreSearchResults determines if additional search results exist within the maximum page budget.
func hasMoreSearchResults(returnedCount, limit, offset int) bool {
	return returnedCount >= limit && offset+returnedCount < maxSearchResults
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

func (u *SearchFeedMeilisearchUsecase) resolveFeedItems(ctx context.Context, searchHits []domain.SearchArticleHit) ([]*domain.FeedItem, error) {
	articleIDs := make([]string, len(searchHits))
	for i, hit := range searchHits {
		articleIDs[i] = hit.ID
	}

	feedURLs, err := u.urlPort.GetFeedURLsByArticleIDs(ctx, articleIDs)
	if err != nil {
		u.logger.Error("failed to get feed URLs", "error", err, "article_ids", articleIDs)
		return nil, err
	}

	urlMap := make(map[string]string, len(feedURLs))
	for _, feedURL := range feedURLs {
		urlMap[feedURL.ArticleID] = feedURL.URL
	}

	return convertHitsToFeedItems(searchHits, urlMap), nil
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

	feedItems, err := u.resolveFeedItems(ctx, searchHits)
	if err != nil {
		return nil, err
	}

	u.logger.Info("feed search via meilisearch completed",
		"query", query,
		"results_count", len(feedItems))

	return feedItems, nil
}

func (u *SearchFeedMeilisearchUsecase) ExecuteWithPagination(ctx context.Context, query string, offset int, limit int) ([]*domain.FeedItem, bool, error) {
	// Validate limit
	if limit <= 0 {
		u.logger.Error("invalid limit: must be greater than 0", "limit", limit)
		return nil, false, errors.New("limit must be greater than 0")
	}
	if limit > 100 {
		u.logger.Error("invalid limit: cannot exceed 100", "limit", limit)
		return nil, false, errors.New("limit cannot exceed 100")
	}

	// Validate offset
	if offset < 0 {
		u.logger.Error("invalid offset: must be non-negative", "offset", offset)
		return nil, false, errors.New("offset must be non-negative")
	}

	u.logger.Info("executing feed search via meilisearch with pagination",
		"query", query,
		"offset", offset,
		"limit", limit)

	// Search for articles using Meilisearch with pagination
	searchHits, totalCount, err := u.searchPort.SearchFeedsWithPagination(ctx, query, offset, limit)
	if err != nil {
		u.logger.Error("failed to search feeds via meilisearch", "error", err, "query", query)
		return nil, false, err
	}

	if len(searchHits) == 0 {
		u.logger.Info("no search results found", "query", query)
		return []*domain.FeedItem{}, false, nil
	}

	feedItems, err := u.resolveFeedItems(ctx, searchHits)
	if err != nil {
		return nil, false, err
	}

	// Determine if there are more results.
	// Do NOT rely on totalCount (Meilisearch's estimatedTotalHits) — it is capped at 1000
	// and is an unreliable estimate in offset/limit mode. Instead, use two deterministic checks:
	// 1. If fewer results than requested were returned, there are no more results.
	// 2. Cap total searchable results to avoid infinite scrolling.
	hasMore := hasMoreSearchResults(len(feedItems), limit, offset)

	u.logger.Info("feed search via meilisearch with pagination completed",
		"query", query,
		"offset", offset,
		"limit", limit,
		"results_count", len(feedItems),
		"total_count", totalCount,
		"has_more", hasMore)

	return feedItems, hasMore, nil
}
