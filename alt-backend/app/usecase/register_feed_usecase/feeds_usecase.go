package register_feed_usecase

import (
	"alt/domain"
	"alt/port/fetch_feed_port"
	"alt/port/register_feed_port"
	"alt/utils/logger"
	"context"
	"errors"
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

	err = r.registerFeedsGateway.RegisterFeeds(ctx, feedItems)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "Failed to register feeds", "error", err)
		return errors.New("failed to register feeds")
	}

	logger.Logger.InfoContext(ctx, "Feed items", "count", len(feedItems))

	return nil
}
