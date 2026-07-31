package register_feed_gateway

import (
	"alt/domain"
	"alt/orchestrator/driver/models"
	register_feed_port "alt/orchestrator/port/register_feed_port"
	"alt/utils"
	"alt/utils/logger"
	"context"
	"strings"
	"time"
)

// FeedWriteStore is the batch upsert alt-data-hub serves (capability catalog
// §2.H W3-H1). One method, because the batch is one transaction there: a
// caller that could register half a poll would leave the next poll unable to
// tell which half it already has.
type FeedWriteStore interface {
	RegisterMultipleFeedsWithState(ctx context.Context, feeds []models.Feed) ([]domain.FeedRegistrationResult, error)
}

type RegisterFeedsGateway struct {
	store FeedWriteStore
}

func NewRegisterFeedsGateway(store FeedWriteStore) *RegisterFeedsGateway {
	if store == nil {
		panic("register_feed_gateway: FeedWriteStore is required (see .claude/rules/di-wiring.md)")
	}
	return &RegisterFeedsGateway{store: store}
}

func buildFeedModels(ctx context.Context, feeds []*domain.FeedItem) []models.Feed {
	var items []models.Feed
	for _, feedItem := range feeds {
		// Additional validation: Skip feeds with empty titles as a safety net
		if strings.TrimSpace(feedItem.Title) == "" {
			logger.Logger.WarnContext(ctx, "Skipping feed registration with empty title",
				"link", feedItem.Link,
				"description", feedItem.Description)
			continue
		}

		// Zero-trust: Normalize URL to remove tracking parameters (UTM, etc.)
		normalizedLink, err := utils.NormalizeURL(feedItem.Link)
		if err != nil {
			logger.Logger.WarnContext(ctx, "Failed to normalize feed link, using original",
				"link", feedItem.Link,
				"error", err)
			normalizedLink = feedItem.Link
		}

		feedModel := &models.Feed{
			Title:       strings.TrimSpace(feedItem.Title),
			Description: feedItem.Description,
			WebsiteURL:  normalizedLink,
			PubDate:     feedItem.PublishedParsed,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
			FeedLinkID:  feedItem.FeedLinkID,
		}
		if feedItem.OgImageURL != "" {
			feedModel.OgImageURL = &feedItem.OgImageURL
		}

		logger.SafeInfoContext(ctx, "Feed model link", "feedModel", feedModel.WebsiteURL)
		items = append(items, *feedModel)
	}
	return items
}

func (g *RegisterFeedsGateway) RegisterFeeds(ctx context.Context, feeds []*domain.FeedItem) ([]register_feed_port.RegisterFeedResult, error) {
	items := buildFeedModels(ctx, feeds)

	results, err := g.store.RegisterMultipleFeedsWithState(ctx, items)
	if err != nil {
		logger.SafeErrorContext(ctx, "Error registering multiple feeds", "error", err)
		return nil, err
	}

	logger.SafeInfoContext(ctx, "Feeds registered", "number of feeds", len(items))
	mapped := make([]register_feed_port.RegisterFeedResult, 0, len(results))
	for _, result := range results {
		mapped = append(mapped, register_feed_port.RegisterFeedResult{
			FeedID:  result.FeedID,
			Created: result.Created,
		})
	}

	return mapped, nil
}
