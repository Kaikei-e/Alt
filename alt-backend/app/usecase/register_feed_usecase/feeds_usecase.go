package register_feed_usecase

import (
	"alt/domain"
	"alt/port/fetch_feed_port"
	"alt/port/register_feed_port"
	"alt/utils/logger"
	"context"
	"errors"
)

type RegisterFeedsUsecase struct {
	registerFeedLinkGateway register_feed_port.RegisterFeedLinkPort
	registerFeedsGateway    register_feed_port.RegisterFeedsPort
	fetchFeedGateway        fetch_feed_port.FetchFeedsPort
}

func NewRegisterFeedsUsecase(registerFeedLinkGateway register_feed_port.RegisterFeedLinkPort, registerFeedsGateway register_feed_port.RegisterFeedsPort, fetchFeedGateway fetch_feed_port.FetchFeedsPort) *RegisterFeedsUsecase {
	return &RegisterFeedsUsecase{
		registerFeedLinkGateway: registerFeedLinkGateway,
		registerFeedsGateway:    registerFeedsGateway,
		fetchFeedGateway:        fetchFeedGateway,
	}
}

func (r *RegisterFeedsUsecase) Execute(ctx context.Context, link string) error {
	err := r.registerFeedLinkGateway.RegisterRSSFeedLink(ctx, link)
	if err != nil {
		logger.Logger.Error("Failed to register RSS feed link", "error", err)
		if errors.Is(err, errors.New("RSS feed link cannot be empty")) {
			return errors.New("RSS feed link cannot be empty")
		}
		return errors.New("failed to register RSS feed link")
	}

	feeds, err := r.fetchFeedGateway.FetchFeeds(ctx, link)
	if err != nil {
		logger.Logger.Error("Failed to fetch feeds", "error", err)
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
			Links: feed.Links,
		})
	}

	logger.Logger.Info("Feed items", "count", len(feedItems))

	err = r.registerFeedsGateway.RegisterFeeds(ctx, feedItems)
	if err != nil {
		logger.Logger.Error("Failed to register feeds", "error", err)
		return errors.New("failed to register feeds")
	}

	logger.Logger.Info("Feed items", "count", len(feedItems))

	return nil
}
