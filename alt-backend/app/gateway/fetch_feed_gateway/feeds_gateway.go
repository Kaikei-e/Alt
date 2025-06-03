package fetch_feed_gateway

import (
	"alt/domain"
	"alt/driver/alt_db"
	"context"

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
		return nil, err
	}

	var feedItems []*domain.FeedItem
	for _, item := range feed.Items {
		feedItems = append(feedItems, &domain.FeedItem{
			Title:           item.Title,
			Description:     item.Description,
			Link:            item.Link,
			Published:       item.Published,
			PublishedParsed: *item.PublishedParsed,
			Author: domain.Author{
				Name: item.Author.Name,
			},
			Authors: []domain.Author{
				{
					Name: item.Author.Name,
				},
			},
			Links: item.Links,
		})
	}

	return feedItems, nil
}
