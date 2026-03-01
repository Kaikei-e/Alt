package register_feed_usecase

import (
	"alt/domain"
	"alt/port/event_publisher_port"
	"alt/port/fetch_feed_port"
	"alt/port/register_feed_port"
	"alt/port/subscription_port"
	"alt/utils/logger"
	"context"
	"errors"

	"github.com/google/uuid"
)

// FeedLinkIDResolver resolves a feed URL to its feed_link_id.
type FeedLinkIDResolver interface {
	FetchFeedLinkIDByURL(ctx context.Context, feedURL string) (*string, error)
}

type RegisterFeedsUsecase struct {
	registerFeedLinkGateway register_feed_port.RegisterFeedLinkPort
	registerFeedsGateway    register_feed_port.RegisterFeedsPort
	fetchFeedGateway        fetch_feed_port.FetchFeedsPort
	feedLinkIDResolver      FeedLinkIDResolver
	subscriptionPort        subscription_port.SubscriptionPort
	eventPublisher          event_publisher_port.EventPublisherPort
}

func NewRegisterFeedsUsecase(registerFeedLinkGateway register_feed_port.RegisterFeedLinkPort, registerFeedsGateway register_feed_port.RegisterFeedsPort, fetchFeedGateway fetch_feed_port.FetchFeedsPort) *RegisterFeedsUsecase {
	return &RegisterFeedsUsecase{
		registerFeedLinkGateway: registerFeedLinkGateway,
		registerFeedsGateway:    registerFeedsGateway,
		fetchFeedGateway:        fetchFeedGateway,
	}
}

// SetFeedLinkIDResolver sets the resolver for looking up feed_link_id by URL.
func (r *RegisterFeedsUsecase) SetFeedLinkIDResolver(resolver FeedLinkIDResolver) {
	r.feedLinkIDResolver = resolver
}

// SetSubscriptionPort sets the subscription port for auto-subscribing users.
func (r *RegisterFeedsUsecase) SetSubscriptionPort(port subscription_port.SubscriptionPort) {
	r.subscriptionPort = port
}

// SetEventPublisher sets the event publisher for domain event publishing.
func (r *RegisterFeedsUsecase) SetEventPublisher(port event_publisher_port.EventPublisherPort) {
	r.eventPublisher = port
}

func (r *RegisterFeedsUsecase) Execute(ctx context.Context, link string) error {
	err := r.registerFeedLinkGateway.RegisterRSSFeedLink(ctx, link)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "Failed to register RSS feed link", "error", err)
		if errors.Is(err, errors.New("RSS feed link cannot be empty")) {
			return errors.New("RSS feed link cannot be empty")
		}
		return errors.New("failed to register RSS feed link")
	}

	// Look up feed_link_id for this URL to associate feeds with their source
	var feedLinkID *string
	if r.feedLinkIDResolver != nil {
		feedLinkID, _ = r.feedLinkIDResolver.FetchFeedLinkIDByURL(ctx, link)
	}

	feeds, err := r.fetchFeedGateway.FetchFeeds(ctx, link)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "Failed to fetch feeds", "error", err)
		return errors.New("failed to fetch feeds")
	}

	var feedItems []*domain.FeedItem
	for _, feed := range feeds {
		feedItems = append(feedItems, &domain.FeedItem{
			Title:           feed.Title,
			Description:     feed.Description,
			Link:            feed.Link,
			Published:       feed.Published,
			PublishedParsed: feed.PublishedParsed,
			Author: domain.Author{
				Name: feed.Author.Name,
			},
			Authors: []domain.Author{
				{
					Name: feed.Author.Name,
				},
			},
			Links:      feed.Links,
			FeedLinkID: feedLinkID,
		})
	}

	logger.Logger.InfoContext(ctx, "Feed items", "count", len(feedItems))

	ids, err := r.registerFeedsGateway.RegisterFeeds(ctx, feedItems)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "Failed to register feeds", "error", err)
		return errors.New("failed to register feeds")
	}

	logger.Logger.InfoContext(ctx, "Feed items", "count", len(feedItems))

	r.publishFeedEvents(ctx, ids, feedItems, feedLinkID)
	r.autoSubscribeUser(ctx, feedLinkID)

	return nil
}

// publishFeedEvents publishes ArticleCreated events for each registered feed item.
// This is fire-and-forget: failures are logged but do not affect the main operation.
func (r *RegisterFeedsUsecase) publishFeedEvents(ctx context.Context, ids []string, feedItems []*domain.FeedItem, feedLinkID *string) {
	if r.eventPublisher == nil || !r.eventPublisher.IsEnabled() {
		return
	}
	feedID := ""
	if feedLinkID != nil {
		feedID = *feedLinkID
	}
	for i, id := range ids {
		if i >= len(feedItems) {
			break
		}
		item := feedItems[i]
		if err := r.eventPublisher.PublishArticleCreated(ctx, event_publisher_port.ArticleCreatedEvent{
			ArticleID:   id,
			FeedID:      feedID,
			Title:       item.Title,
			URL:         item.Link,
			Content:     item.Description,
			PublishedAt: item.PublishedParsed,
		}); err != nil {
			logger.Logger.WarnContext(ctx, "failed to publish ArticleCreated event (non-fatal)",
				"article_id", id, "error", err)
		}
	}
}

// autoSubscribeUser subscribes the authenticated user to the feed link.
// This is best-effort: if user context is unavailable or Subscribe fails,
// the error is logged but does not propagate.
func (r *RegisterFeedsUsecase) autoSubscribeUser(ctx context.Context, feedLinkID *string) {
	if r.subscriptionPort == nil || feedLinkID == nil {
		return
	}
	userCtx, err := domain.GetUserFromContext(ctx)
	if err != nil {
		return
	}
	parsedID, err := uuid.Parse(*feedLinkID)
	if err != nil {
		return
	}
	if err := r.subscriptionPort.Subscribe(ctx, userCtx.UserID, parsedID); err != nil {
		logger.Logger.WarnContext(ctx, "Auto-subscribe failed", "error", err)
	}
}
