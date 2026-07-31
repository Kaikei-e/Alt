package fetch_feed_tags_gateway

import (
	"alt/domain"
	"alt/utils/logger"
	"context"
	"fmt"
	"time"
)

// feedTagReader is the alt-data-hub read (capability catalog §2.J W3-J2).
type feedTagReader interface {
	FetchFeedTags(ctx context.Context, feedID string, cursor *time.Time, limit int) ([]*domain.FeedTag, error)
}

type FetchFeedTagsGateway struct {
	tags feedTagReader
}

func NewFetchFeedTagsGateway(tags feedTagReader) *FetchFeedTagsGateway {
	if tags == nil {
		panic("fetch_feed_tags_gateway: a tag reader is required — a nil one would render every feed as having no topics (see .claude/rules/di-wiring.md)")
	}
	return &FetchFeedTagsGateway{tags: tags}
}

func (g *FetchFeedTagsGateway) FetchFeedTags(ctx context.Context, feedID string, cursor *time.Time, limit int) ([]*domain.FeedTag, error) {
	logger.Logger.InfoContext(ctx, "fetching feed tags", "feedID", feedID, "cursor", cursor, "limit", limit)

	tags, err := g.tags.FetchFeedTags(ctx, feedID, cursor, limit)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "failed to fetch feed tags", "error", err, "feedID", feedID)
		return nil, fmt.Errorf("fetch feed tags for %s: %w", feedID, err)
	}

	logger.Logger.InfoContext(ctx, "successfully fetched feed tags", "count", len(tags))
	return tags, nil
}
