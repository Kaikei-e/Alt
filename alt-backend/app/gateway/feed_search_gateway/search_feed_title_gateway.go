package feed_search_gateway

import (
	"context"

	"alt/domain"
	"alt/driver/alt_db"

	"github.com/jackc/pgx/v5/pgxpool"
)

type SearchByTitleGateway struct {
	alt_db *alt_db.AltDBRepository
}

func NewSearchByTitleGateway(pool *pgxpool.Pool) *SearchByTitleGateway {
	return &SearchByTitleGateway{alt_db: alt_db.NewAltDBRepository(pool)}
}

func (g *SearchByTitleGateway) SearchByTitle(ctx context.Context, query string) ([]*domain.FeedItem, error) {
	feeds, err := g.alt_db.SearchByTitle(ctx, query)
	if err != nil {
		return nil, err
	}

	items := make([]*domain.FeedItem, 0)
	for _, feed := range feeds {
		items = append(items, &domain.FeedItem{
			Title:           feed.Title,
			Link:            feed.Link,
			Description:     feed.Description,
			Published:       feed.Published,
			PublishedParsed: feed.PublishedParsed,
		})
	}

	return items, nil
}
