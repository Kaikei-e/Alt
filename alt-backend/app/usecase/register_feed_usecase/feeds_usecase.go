package register_feed_usecase

import (
	"alt/domain"
	"alt/port/event_publisher_port"
	"alt/port/feed_link_availability_port"
	"alt/port/register_feed_port"
	"alt/port/subscription_port"
	"alt/port/validate_fetch_rss_port"
	"alt/utils/logger"
	"context"
	"errors"

	"github.com/google/uuid"
)

// FeedLinkIDResolver resolves a feed URL to its feed_link_id.
type FeedLinkIDResolver interface {
	FetchFeedLinkIDByURL(ctx context.Context, feedURL string) (*string, error)
}

// FeedPageInvalidator invalidates cached feed pages.
type FeedPageInvalidator interface {
	InvalidateFeedPage(ctx context.Context, feedLinkID uuid.UUID) error
}

// RegisterFeedsOpts holds optional dependencies for RegisterFeedsUsecase.
type RegisterFeedsOpts struct {
	FeedLinkIDResolver   FeedLinkIDResolver
	FeedLinkAvailability feed_link_availability_port.FeedLinkAvailabilityPort
	FeedPageInvalidator  FeedPageInvalidator
	SubscriptionPort     subscription_port.SubscriptionPort
	EventPublisher       event_publisher_port.EventPublisherPort
}

type RegisterFeedsUsecase struct {
	validateAndFetchPort validate_fetch_rss_port.ValidateAndFetchRSSPort
	registerFeedLinkPort register_feed_port.RegisterFeedLinkPort
	registerFeedsGateway register_feed_port.RegisterFeedsPort
	feedLinkIDResolver   FeedLinkIDResolver
	subscriptionPort     subscription_port.SubscriptionPort
	availabilityPort     feed_link_availability_port.FeedLinkAvailabilityPort
	eventPublisher       event_publisher_port.EventPublisherPort
	feedPageInvalidator  FeedPageInvalidator
}

func NewRegisterFeedsUsecase(
	validateAndFetchPort validate_fetch_rss_port.ValidateAndFetchRSSPort,
	registerFeedLinkPort register_feed_port.RegisterFeedLinkPort,
	registerFeedsGateway register_feed_port.RegisterFeedsPort,
	opts *RegisterFeedsOpts,
) *RegisterFeedsUsecase {
	uc := &RegisterFeedsUsecase{
		validateAndFetchPort: validateAndFetchPort,
		registerFeedLinkPort: registerFeedLinkPort,
		registerFeedsGateway: registerFeedsGateway,
	}
	if opts != nil {
		uc.feedLinkIDResolver = opts.FeedLinkIDResolver
		uc.availabilityPort = opts.FeedLinkAvailability
		uc.feedPageInvalidator = opts.FeedPageInvalidator
		uc.subscriptionPort = opts.SubscriptionPort
		uc.eventPublisher = opts.EventPublisher
	}
	return uc
}

func (r *RegisterFeedsUsecase) Execute(ctx context.Context, link string) error {
	// 1. Validate URL + fetch RSS (single external HTTP call)
	parsedFeed, err := r.validateAndFetchPort.ValidateAndFetch(ctx, link)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "Failed to validate and fetch RSS feed", "error", err)
		return errors.New("failed to register RSS feed link")
	}

	// 2. Register feed_link in DB (DB-only operation)
	err = r.registerFeedLinkPort.RegisterFeedLink(ctx, parsedFeed.FeedLink)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "Failed to register feed link", "error", err)
		return errors.New("failed to register RSS feed link")
	}

	// 3. Look up feed_link_id for this URL to associate feeds with their source
	var feedLinkID *string
	if r.feedLinkIDResolver != nil {
		feedLinkID, _ = r.feedLinkIDResolver.FetchFeedLinkIDByURL(ctx, parsedFeed.FeedLink)
	}

	// 4. Set FeedLinkID on all items
	for _, item := range parsedFeed.Items {
		item.FeedLinkID = feedLinkID
	}

	if feedLinkID != nil && r.feedPageInvalidator != nil {
		if parsedID, parseErr := uuid.Parse(*feedLinkID); parseErr == nil {
			_ = r.feedPageInvalidator.InvalidateFeedPage(ctx, parsedID)
		}
	}

	logger.Logger.InfoContext(ctx, "Feed items", "count", len(parsedFeed.Items))

	// 5. Register feed items in DB
	ids, err := r.registerFeedsGateway.RegisterFeeds(ctx, parsedFeed.Items)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "Failed to register feeds", "error", err)
		return errors.New("failed to register feeds")
	}

	logger.Logger.InfoContext(ctx, "Feed items registered", "count", len(parsedFeed.Items))

	// 6. Initialize feed availability state from the successful registration fetch.
	if r.availabilityPort != nil {
		if err := r.availabilityPort.ResetFeedLinkFailures(ctx, parsedFeed.FeedLink); err != nil {
			logger.Logger.ErrorContext(ctx, "Failed to initialize feed link availability", "feed_link", parsedFeed.FeedLink, "error", err)
			return errors.New("failed to initialize feed link availability")
		}
	}

	// 7. Fire-and-forget: event publishing + auto-subscribe (truly async)
	// Use context.WithoutCancel so goroutines survive after HTTP response is sent.
	// Both operations already log errors without propagating them.
	bgCtx := context.WithoutCancel(ctx)
	go r.publishFeedEvents(bgCtx, ids, parsedFeed.Items, feedLinkID)
	go r.autoSubscribeUser(bgCtx, feedLinkID)

	return nil
}

// publishFeedEvents publishes ArticleCreated events for each registered feed item.
// This is fire-and-forget: failures are logged but do not affect the main operation.
func (r *RegisterFeedsUsecase) publishFeedEvents(ctx context.Context, results []register_feed_port.RegisterFeedResult, feedItems []*domain.FeedItem, feedLinkID *string) {
	if r.eventPublisher == nil || !r.eventPublisher.IsEnabled() {
		return
	}
	feedID := ""
	if feedLinkID != nil {
		feedID = *feedLinkID
	}
	for i, result := range results {
		if i >= len(feedItems) {
			break
		}
		item := feedItems[i]

		if result.Created {
			if err := r.eventPublisher.PublishArticleCreated(ctx, event_publisher_port.ArticleCreatedEvent{
				ArticleID:   result.ArticleID,
				FeedID:      feedID,
				Title:       item.Title,
				URL:         item.Link,
				Content:     item.Description,
				PublishedAt: item.PublishedParsed,
			}); err != nil {
				logger.Logger.WarnContext(ctx, "failed to publish ArticleCreated event (non-fatal)",
					"article_id", result.ArticleID, "error", err)
			}
			continue
		}

		if err := r.eventPublisher.PublishArticleUpdated(ctx, event_publisher_port.ArticleUpdatedEvent{
			ArticleID:   result.ArticleID,
			FeedID:      feedID,
			Title:       item.Title,
			URL:         item.Link,
			Content:     item.Description,
			PublishedAt: item.PublishedParsed,
		}); err != nil {
			logger.Logger.WarnContext(ctx, "failed to publish ArticleUpdated event (non-fatal)",
				"article_id", result.ArticleID, "error", err)
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
