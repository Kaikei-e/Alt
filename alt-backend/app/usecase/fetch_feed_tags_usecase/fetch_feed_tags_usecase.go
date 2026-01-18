package fetch_feed_tags_usecase

import (
	"alt/domain"
	"alt/port/feed_url_to_id_port"
	"alt/port/fetch_feed_tags_port"
	"alt/utils/logger"
	"context"
	"errors"
	"strings"
	"time"
)

type FetchFeedTagsUsecase struct {
	feedURLToIDGateway   feed_url_to_id_port.FeedURLToIDPort
	fetchFeedTagsGateway fetch_feed_tags_port.FetchFeedTagsPort
}

func NewFetchFeedTagsUsecase(feedURLToIDGateway feed_url_to_id_port.FeedURLToIDPort, fetchFeedTagsGateway fetch_feed_tags_port.FetchFeedTagsPort) *FetchFeedTagsUsecase {
	return &FetchFeedTagsUsecase{
		feedURLToIDGateway:   feedURLToIDGateway,
		fetchFeedTagsGateway: fetchFeedTagsGateway,
	}
}

func (u *FetchFeedTagsUsecase) Execute(ctx context.Context, feedURL string, cursor *time.Time, limit int) ([]*domain.FeedTag, error) {
	// Business rule validation
	if strings.TrimSpace(feedURL) == "" {
		logger.Logger.ErrorContext(ctx, "invalid feed_url: must not be empty", "feedURL", feedURL)
		return nil, errors.New("feed_url must not be empty")
	}

	if limit <= 0 {
		logger.Logger.ErrorContext(ctx, "invalid limit: must be greater than 0", "limit", limit)
		return nil, errors.New("limit must be greater than 0")
	}

	if limit > 100 {
		logger.Logger.ErrorContext(ctx, "invalid limit: cannot exceed 100", "limit", limit)
		return nil, errors.New("limit cannot exceed 100")
	}

	logger.Logger.InfoContext(ctx, "fetching feed tags", "feedURL", feedURL, "cursor", cursor, "limit", limit)

	// Step 1: Get feed ID from feed URL
	feedID, err := u.feedURLToIDGateway.GetFeedIDByURL(ctx, feedURL)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "failed to get feed ID by URL", "error", err, "feedURL", feedURL)
		return nil, err
	}

	// Step 2: Fetch tags using feed ID
	tags, err := u.fetchFeedTagsGateway.FetchFeedTags(ctx, feedID, cursor, limit)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "failed to fetch feed tags", "error", err, "feedID", feedID, "limit", limit)
		return nil, err
	}

	logger.Logger.InfoContext(ctx, "successfully fetched feed tags", "count", len(tags))
	return tags, nil
}
