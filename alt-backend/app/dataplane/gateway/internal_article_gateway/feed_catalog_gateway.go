package internal_article_gateway

import (
	"context"
	"fmt"

	"alt/dataplane/port/internal_feed_port"
	"alt/shared/driver/alt_db"
)

// FeedCatalogGateway implements internal feed API ports using AltDBRepository.
type FeedCatalogGateway struct {
	repo *alt_db.AltDBRepository
}

// NewFeedCatalogGateway creates a new feed catalog gateway.
func NewFeedCatalogGateway(repo *alt_db.AltDBRepository) *FeedCatalogGateway {
	return &FeedCatalogGateway{repo: repo}
}

// GetFeedID implements GetFeedIDPort.
func (g *FeedCatalogGateway) GetFeedID(ctx context.Context, feedURL string) (string, error) {
	feedID, err := g.repo.GetFeedIDByURL(ctx, feedURL)
	if err != nil {
		return "", fmt.Errorf("GetFeedID: %w", err)
	}
	return feedID, nil
}

// ListFeedURLs implements ListFeedURLsPort.
func (g *FeedCatalogGateway) ListFeedURLs(ctx context.Context, cursor string, limit int) ([]internal_feed_port.FeedURL, string, bool, error) {
	driverFeeds, nextCursor, hasMore, err := g.repo.ListFeedURLs(ctx, cursor, limit)
	if err != nil {
		return nil, "", false, fmt.Errorf("ListFeedURLs: %w", err)
	}

	return toPortFeedURLs(driverFeeds), nextCursor, hasMore, nil
}

// GetEmptyFeedID implements GetEmptyFeedIDPort.
func (g *FeedCatalogGateway) GetEmptyFeedID(ctx context.Context, feedURL string) (string, error) {
	feedID, err := g.repo.GetEmptyFeedID(ctx, feedURL)
	if err != nil {
		return "", fmt.Errorf("GetEmptyFeedID: %w", err)
	}
	return feedID, nil
}
