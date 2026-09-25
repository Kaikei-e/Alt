package fetch_feed_gateway

import (
	"alt/domain"
	"alt/orchestrator/driver/models"
	"alt/utils/logger"
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
)

// FeedListStore is the set of feed reads this gateway renders (ADR-000954
// Wave 3 batch 3, capability catalog §2.H).
//
// It returns rows, not domain.FeedItem: the sanitising and the RFC3339
// formatting below are pure functions of the columns and stay on this side of
// the boundary (D4). The four cursor walks are separate methods rather than
// one with a scope argument because that is the shape the usecases above
// already call.
type FeedListStore interface {
	FetchFeedsList(ctx context.Context) ([]*models.Feed, error)
	FetchFeedsListLimit(ctx context.Context, limit int) ([]*models.Feed, error)
	FetchUnreadFeedsListPage(ctx context.Context, page int) ([]*models.Feed, error)
	FetchAllFeedsListCursor(ctx context.Context, cursor *time.Time, limit int, excludeFeedLinkIDs []uuid.UUID) ([]*models.Feed, error)
	FetchUnreadFeedsListCursor(ctx context.Context, cursor *time.Time, limit int, excludeFeedLinkIDs []uuid.UUID) ([]*models.Feed, error)
	FetchReadFeedsListCursor(ctx context.Context, cursor *time.Time, limit int) ([]*models.Feed, error)
	FetchFavoriteFeedsListCursor(ctx context.Context, cursor *time.Time, limit int) ([]*models.Feed, error)
}

func (g *FetchFeedsGateway) FetchFeedsList(ctx context.Context) ([]*domain.FeedItem, error) {
	feeds, err := g.store.FetchFeedsList(ctx)
	if err != nil {
		logger.SafeErrorContext(ctx, "Error fetching feeds list", "error", err)
		return nil, errors.New("error fetching feeds list")
	}

	feedItems := make([]*domain.FeedItem, 0, len(feeds))
	for _, feed := range feeds {
		feedItems = append(feedItems, mapFeedBasic(feed))
	}
	return feedItems, nil
}

func (g *FetchFeedsGateway) FetchFeedsListLimit(ctx context.Context, offset int) ([]*domain.FeedItem, error) {
	feeds, err := g.store.FetchFeedsListLimit(ctx, offset)
	if err != nil {
		logger.SafeErrorContext(ctx, "Error fetching feeds list offset", "error", err)
		return nil, errors.New("error fetching feeds list offset")
	}

	var feedItems []*domain.FeedItem
	for _, feed := range feeds {
		feedItems = append(feedItems, mapFeedBasic(feed))
	}

	return feedItems, nil
}

func (g *FetchFeedsGateway) FetchFeedsListPage(ctx context.Context, page int) ([]*domain.FeedItem, error) {
	// TDD Fix: No dangerous fallback! Only fetch unread feeds
	feeds, err := g.store.FetchUnreadFeedsListPage(ctx, page)
	if err != nil {
		logger.SafeErrorContext(ctx, "Error fetching unread feeds", "error", err)
		return nil, errors.New("error fetching unread feeds list page")
	}

	var feedItems []*domain.FeedItem
	for _, feed := range feeds {
		feedItems = append(feedItems, mapFeedBasic(feed))
	}

	return feedItems, nil
}

func (g *FetchFeedsGateway) FetchFeedsListCursor(ctx context.Context, cursor *time.Time, limit int, excludeFeedLinkIDs []uuid.UUID) ([]*domain.FeedItem, error) {
	ctx, span := otel.Tracer("alt-backend").Start(ctx, "gateway.FetchFeedsListCursor")
	defer span.End()

	feeds, err := g.store.FetchAllFeedsListCursor(ctx, cursor, limit, excludeFeedLinkIDs)
	if err != nil {
		logger.SafeErrorContext(ctx, "Error fetching all feeds with cursor", "error", err)
		return nil, errors.New("error fetching feeds with cursor")
	}

	var feedItems []*domain.FeedItem
	for _, feed := range feeds {
		feedItem, err := mapFeedCursor(feed, true)
		if err != nil {
			logger.SafeErrorContext(ctx, "Error reading feed id from cursor page", "error", err)
			return nil, errors.New("error fetching feeds with cursor")
		}
		feedItems = append(feedItems, feedItem)
	}

	return feedItems, nil
}

func (g *FetchFeedsGateway) FetchUnreadFeedsListCursor(ctx context.Context, cursor *time.Time, limit int, excludeFeedLinkIDs []uuid.UUID) ([]*domain.FeedItem, error) {
	ctx, span := otel.Tracer("alt-backend").Start(ctx, "gateway.FetchUnreadFeedsListCursor")
	defer span.End()

	feeds, err := g.store.FetchUnreadFeedsListCursor(ctx, cursor, limit, excludeFeedLinkIDs)
	if err != nil {
		logger.SafeErrorContext(ctx, "Error fetching unread feeds with cursor", "error", err)
		return nil, errors.New("error fetching unread feeds with cursor")
	}

	var feedItems []*domain.FeedItem
	// Use created_at only for cursor pagination
	// created_at is always populated (NOT NULL DEFAULT CURRENT_TIMESTAMP) and reliable
	// pub_date has many zero values (0001-01-01) and is not reliable

	for i, feed := range feeds {
		// Always use created_at for Published field to match SQL query ORDER BY
		publishedTime := feed.CreatedAt

		// Log first and last feed details for debugging
		if i == 0 || i == len(feeds)-1 {
			logger.Logger.InfoContext(ctx,
				"feed date mapping for cursor pagination",
				"index", i,
				"total", len(feeds),
				"created_at", feed.CreatedAt,
				"published_parsed", publishedTime,
				"link", feed.WebsiteURL,
				"article_id", feed.ArticleID,
			)
		}

		feedItem, err := mapFeedCursor(feed, false)
		if err != nil {
			logger.SafeErrorContext(ctx, "Error reading feed id from unread cursor page", "error", err)
			return nil, errors.New("error fetching unread feeds with cursor")
		}

		feedItems = append(feedItems, feedItem)
	}

	return feedItems, nil
}

func (g *FetchFeedsGateway) FetchReadFeedsListCursor(ctx context.Context, cursor *time.Time, limit int) ([]*domain.FeedItem, error) {
	ctx, span := otel.Tracer("alt-backend").Start(ctx, "gateway.FetchReadFeedsListCursor")
	defer span.End()

	feeds, err := g.store.FetchReadFeedsListCursor(ctx, cursor, limit)
	if err != nil {
		logger.SafeErrorContext(ctx, "Error fetching read feeds with cursor", "error", err)
		return nil, errors.New("error fetching read feeds with cursor")
	}

	var feedItems []*domain.FeedItem
	for _, feed := range feeds {
		feedItem, err := mapFeedCursor(feed, false)
		if err != nil {
			logger.SafeErrorContext(ctx, "Error reading feed id from read cursor page", "error", err)
			return nil, errors.New("error fetching read feeds with cursor")
		}
		feedItems = append(feedItems, feedItem)
	}

	return feedItems, nil
}

func (g *FetchFeedsGateway) FetchFavoriteFeedsListCursor(ctx context.Context, cursor *time.Time, limit int) ([]*domain.FeedItem, error) {
	feeds, err := g.store.FetchFavoriteFeedsListCursor(ctx, cursor, limit)
	if err != nil {
		logger.SafeErrorContext(ctx, "Error fetching favorite feeds with cursor", "error", err)
		return nil, errors.New("error fetching favorite feeds with cursor")
	}

	var feedItems []*domain.FeedItem
	for _, feed := range feeds {
		feedItem, err := mapFeedCursor(feed, false)
		if err != nil {
			logger.SafeErrorContext(ctx, "Error reading feed id from favorite cursor page", "error", err)
			return nil, errors.New("error fetching favorite feeds with cursor")
		}
		feedItems = append(feedItems, feedItem)
	}

	return feedItems, nil
}
