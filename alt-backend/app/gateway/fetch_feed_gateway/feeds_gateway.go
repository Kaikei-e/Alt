package fetch_feed_gateway

import (
	"alt/domain"
	"alt/driver/alt_db"
	"alt/utils"
	"alt/utils/logger"
	"alt/utils/rate_limiter"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mmcdole/gofeed"
)

type FetchFeedsGateway struct {
	alt_db      *alt_db.AltDBRepository
	rateLimiter *rate_limiter.HostRateLimiter
	httpClient  *http.Client
}

func NewFetchFeedsGateway(pool *pgxpool.Pool) *FetchFeedsGateway {
	return &FetchFeedsGateway{
		alt_db:      alt_db.NewAltDBRepositoryWithPool(pool),
		rateLimiter: nil, // No rate limiting for backward compatibility
		httpClient:  nil,
	}
}

func NewFetchFeedsGatewayWithRateLimiter(pool *pgxpool.Pool, rateLimiter *rate_limiter.HostRateLimiter) *FetchFeedsGateway {
	return &FetchFeedsGateway{
		alt_db:      alt_db.NewAltDBRepositoryWithPool(pool),
		rateLimiter: rateLimiter,
		httpClient:  nil,
	}
}

func (g *FetchFeedsGateway) FetchFeeds(ctx context.Context, link string) ([]*domain.FeedItem, error) {
	// Apply rate limiting if rate limiter is configured
	if g.rateLimiter != nil {
		slog.Info("Applying rate limiting for external feed request", "url", link)
		if err := g.rateLimiter.WaitForHost(ctx, link); err != nil {
			slog.Error("Rate limiting failed", "url", link, "error", err)
			return nil, errors.New("rate limiting failed")
		}
		slog.Info("Rate limiting passed, proceeding with feed request", "url", link)
	}

	// Use provided HTTP client if available, otherwise create a secure one
	httpClient := g.httpClient
	if httpClient == nil {
		factory := utils.NewHTTPClientFactory()
		httpClient = factory.CreateHTTPClient()
	}

	fp := gofeed.NewParser()
	fp.Client = httpClient
	feed, err := fp.ParseURL(link)
	if err != nil {
		logger.SafeError("Error parsing feed", "error", err)
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
	if g.alt_db == nil {
		return nil, errors.New("database connection not available")
	}
	feeds, err := g.alt_db.FetchFeedsList(ctx)
	if err != nil {
		logger.SafeError("Error fetching feeds list", "error", err)
		return nil, errors.New("error fetching feeds list")
	}

	feedItems := make([]*domain.FeedItem, 0, len(feeds))
	for _, feed := range feeds {
		feedItems = append(feedItems, &domain.FeedItem{
			Title:       feed.Title,
			Description: feed.Description,
			Link:        feed.Link,
			Published:   feed.CreatedAt.Format(time.RFC3339),
		})
	}
	return feedItems, nil
}

func (g *FetchFeedsGateway) FetchFeedsListLimit(ctx context.Context, offset int) ([]*domain.FeedItem, error) {
	if g.alt_db == nil {
		return nil, errors.New("database connection not available")
	}
	feeds, err := g.alt_db.FetchFeedsListLimit(ctx, offset)
	if err != nil {
		logger.SafeError("Error fetching feeds list offset", "error", err)
		return nil, errors.New("error fetching feeds list offset")
	}

	var feedItems []*domain.FeedItem
	for _, feed := range feeds {
		feedItems = append(feedItems, &domain.FeedItem{
			Title:       feed.Title,
			Description: feed.Description,
			Link:        feed.Link,
			Published:   feed.CreatedAt.Format(time.RFC3339),
		})
	}

	return feedItems, nil
}

func (g *FetchFeedsGateway) FetchFeedsListPage(ctx context.Context, page int) ([]*domain.FeedItem, error) {
	if g.alt_db == nil {
		return nil, errors.New("database connection not available")
	}

	// TDD Fix: No dangerous fallback! Only fetch unread feeds
	feeds, err := g.alt_db.FetchUnreadFeedsListPage(ctx, page)
	if err != nil {
		logger.SafeError("Error fetching unread feeds", "error", err)
		return nil, errors.New("error fetching unread feeds list page")
	}

	var feedItems []*domain.FeedItem
	for _, feed := range feeds {
		feedItems = append(feedItems, &domain.FeedItem{
			Title:       feed.Title,
			Description: feed.Description,
			Link:        feed.Link,
			Published:   feed.CreatedAt.Format(time.RFC3339),
		})
	}

	return feedItems, nil
}

func (g *FetchFeedsGateway) FetchFeedsListCursor(ctx context.Context, cursor *time.Time, limit int) ([]*domain.FeedItem, error) {
	if g.alt_db == nil {
		return nil, errors.New("database connection not available")
	}

	feeds, err := g.alt_db.FetchUnreadFeedsListCursor(ctx, cursor, limit)
	if err != nil {
		logger.SafeError("Error fetching feeds with cursor", "error", err)
		return nil, errors.New("error fetching feeds with cursor")
	}

	var feedItems []*domain.FeedItem
	for _, feed := range feeds {
		feedItems = append(feedItems, &domain.FeedItem{
			Title:       feed.Title,
			Description: feed.Description,
			Link:        feed.Link,
			Published:   feed.CreatedAt.Format(time.RFC3339),
		})
	}

	return feedItems, nil
}

func (g *FetchFeedsGateway) FetchUnreadFeedsListCursor(ctx context.Context, cursor *time.Time, limit int) ([]*domain.FeedItem, error) {
	if g.alt_db == nil {
		return nil, errors.New("database connection not available")
	}

	feeds, err := g.alt_db.FetchUnreadFeedsListCursor(ctx, cursor, limit)
	if err != nil {
		logger.SafeError("Error fetching unread feeds with cursor", "error", err)
		return nil, errors.New("error fetching unread feeds with cursor")
	}

	var feedItems []*domain.FeedItem
	for _, feed := range feeds {
		// Use pub_date for Published field, fallback to created_at if pub_date is zero/empty
		var publishedTime time.Time
		if !feed.PubDate.IsZero() {
			publishedTime = feed.PubDate
		} else {
			publishedTime = feed.CreatedAt
		}
		feedItems = append(feedItems, &domain.FeedItem{
			Title:       feed.Title,
			Description: feed.Description,
			Link:        feed.Link,
			Published:   publishedTime.Format(time.RFC3339),
		})
	}

	return feedItems, nil
}

func (g *FetchFeedsGateway) FetchReadFeedsListCursor(ctx context.Context, cursor *time.Time, limit int) ([]*domain.FeedItem, error) {
	if g.alt_db == nil {
		return nil, errors.New("database connection not available")
	}

	feeds, err := g.alt_db.FetchReadFeedsListCursor(ctx, cursor, limit)
	if err != nil {
		logger.SafeError("Error fetching read feeds with cursor", "error", err)
		return nil, errors.New("error fetching read feeds with cursor")
	}

	var feedItems []*domain.FeedItem
	for _, feed := range feeds {
		feedItems = append(feedItems, &domain.FeedItem{
			Title:       feed.Title,
			Description: feed.Description,
			Link:        feed.Link,
			Published:   feed.CreatedAt.Format(time.RFC3339),
		})
	}

	return feedItems, nil
}

func (g *FetchFeedsGateway) FetchFavoriteFeedsListCursor(ctx context.Context, cursor *time.Time, limit int) ([]*domain.FeedItem, error) {
	if g.alt_db == nil {
		return nil, errors.New("database connection not available")
	}

	feeds, err := g.alt_db.FetchFavoriteFeedsListCursor(ctx, cursor, limit)
	if err != nil {
		logger.SafeError("Error fetching favorite feeds with cursor", "error", err)
		return nil, errors.New("error fetching favorite feeds with cursor")
	}

	var feedItems []*domain.FeedItem
	for _, feed := range feeds {
		feedItems = append(feedItems, &domain.FeedItem{
			Title:       feed.Title,
			Description: feed.Description,
			Link:        feed.Link,
			Published:   feed.CreatedAt.Format(time.RFC3339),
		})
	}

	return feedItems, nil
}
