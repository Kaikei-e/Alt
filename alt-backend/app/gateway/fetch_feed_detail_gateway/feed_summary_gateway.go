package fetch_feed_detail_gateway

import (
	"alt/domain"
	"alt/driver/alt_db"
	"context"
	"net/url"

	"github.com/jackc/pgx/v5/pgxpool"
)

type FeedSummaryGateway struct {
	alt_db *alt_db.AltDBRepository
}

func NewFeedSummaryGateway(pool *pgxpool.Pool) *FeedSummaryGateway {
	return &FeedSummaryGateway{alt_db: alt_db.NewAltDBRepository(pool)}
}

func (g *FeedSummaryGateway) FetchFeedDetails(ctx context.Context, feedURL *url.URL) (*domain.FeedSummary, error) {
	summary, err := g.alt_db.FetchFeedSummary(ctx, feedURL)
	if err != nil {
		return nil, err
	}
	return summary, nil
}
