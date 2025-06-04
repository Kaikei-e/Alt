package fetch_feed_gateway

import (
	"alt/domain"
	"alt/driver/alt_db"
	"alt/utils/logger"
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/mmcdole/gofeed"
)

type FetchFeedsGateway struct {
	alt_db *alt_db.AltDBRepository
}

func NewFetchFeedsGateway(db *pgx.Conn) *FetchFeedsGateway {
	return &FetchFeedsGateway{
		alt_db: alt_db.NewAltDBRepository(db),
	}
}

func (g *FetchFeedsGateway) FetchFeeds(ctx context.Context, link string) ([]*domain.FeedItem, error) {
	fp := gofeed.NewParser()
	feed, err := fp.ParseURL(link)
	if err != nil {
		logger.Logger.Error("Error parsing feed", "error", err)
		return nil, errors.New("error parsing feed")
	}

	var feedItems []*domain.FeedItem
	for _, item := range feed.Items {
		feedItem := &domain.FeedItem{
			Title:       item.Title,
			Description: item.Description,
			Link:        item.Link,
			Published:   item.Published,
			Links:       item.Links,
		}

		// Handle PublishedParsed with nil check
		if item.PublishedParsed != nil {
			feedItem.PublishedParsed = *item.PublishedParsed
		}

		// Handle Author with nil check
		if item.Author != nil {
			feedItem.Author = domain.Author{
				Name: item.Author.Name,
			}
			feedItem.Authors = []domain.Author{
				{
					Name: item.Author.Name,
				},
			}
		}

		feedItems = append(feedItems, feedItem)
	}

	return feedItems, nil
}

func (g *FetchFeedsGateway) FetchFeedsList(ctx context.Context) ([]*domain.FeedItem, error) {
	feeds, err := g.alt_db.FetchFeedsList(ctx)
	if err != nil {
		logger.Logger.Error("Error fetching feeds list", "error", err)
		return nil, errors.New("error fetching feeds list")
	}

	var feedItems []*domain.FeedItem
	for _, feed := range feeds {
		feedItems = append(feedItems, &domain.FeedItem{
			Title:           feed.Title,
			Description:     feed.Description,
			Link:            feed.Link,
			PublishedParsed: feed.PubDate,
		})
	}
	return feedItems, nil
}

func (g *FetchFeedsGateway) FetchFeedsListLimit(ctx context.Context, offset int) ([]*domain.FeedItem, error) {
	feeds, err := g.alt_db.FetchFeedsListLimit(ctx, offset)
	if err != nil {
		logger.Logger.Error("Error fetching feeds list offset", "error", err)
		return nil, errors.New("error fetching feeds list offset")
	}

	var feedItems []*domain.FeedItem
	for _, feed := range feeds {
		feedItems = append(feedItems, &domain.FeedItem{
			Title:           feed.Title,
			Description:     feed.Description,
			Link:            feed.Link,
			PublishedParsed: feed.PubDate,
		})
	}

	return feedItems, nil
}

func (g *FetchFeedsGateway) FetchFeedsListPage(ctx context.Context, page int) ([]*domain.FeedItem, error) {
	feeds, err := g.alt_db.FetchFeedsListPage(ctx, page)
	if err != nil {
		logger.Logger.Error("Error fetching feeds list page", "error", err)
		return nil, errors.New("error fetching feeds list page")
	}

	var feedItems []*domain.FeedItem
	for _, feed := range feeds {
		feedItems = append(feedItems, &domain.FeedItem{
			Title:           feed.Title,
			Description:     feed.Description,
			Link:            feed.Link,
			PublishedParsed: feed.PubDate,
		})
	}

	return feedItems, nil
}
