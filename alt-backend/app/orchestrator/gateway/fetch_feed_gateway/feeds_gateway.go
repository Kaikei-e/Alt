package fetch_feed_gateway

import (
	"alt/domain"
	"alt/orchestrator/driver/models"
	"alt/utils"
	"alt/utils/logger"
	"alt/utils/rate_limiter"
	"alt/utils/sanitize"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/mmcdole/gofeed"
	"go.opentelemetry.io/otel"
)

// maxFeedBodyBytes bounds how much of a feed body gofeed reads. gofeed treats a
// zero MaxByteSize as "no limit", and the transport transparently decompresses
// gzip, so without a ceiling a few KB on the wire can expand into GBs resident.
const maxFeedBodyBytes = 10 * 1024 * 1024

// newBoundedFeedParser builds a gofeed parser that refuses bodies larger than
// maxFeedBodyBytes instead of reading them in full.
func newBoundedFeedParser(client *http.Client) *gofeed.Parser {
	fp := gofeed.NewParser()
	fp.Client = client
	fp.MaxByteSize = maxFeedBodyBytes
	fp.UserAgent = "Alt-RSS-Reader/1.0 (+https://alt.example.com)"
	return fp
}

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

type FetchFeedsGateway struct {
	store       FeedListStore
	rateLimiter *rate_limiter.HostRateLimiter
	httpClient  *http.Client
}

func NewFetchFeedsGateway(store FeedListStore) *FetchFeedsGateway {
	return NewFetchFeedsGatewayWithRateLimiter(store, nil)
}

func NewFetchFeedsGatewayWithRateLimiter(store FeedListStore, rateLimiter *rate_limiter.HostRateLimiter) *FetchFeedsGateway {
	if store == nil {
		panic("fetch_feed_gateway: FeedListStore is required — a nil one would make every feed list " +
			"fail identically to a user with no subscriptions (see .claude/rules/di-wiring.md)")
	}
	return &FetchFeedsGateway{
		store:       store,
		rateLimiter: rateLimiter,
		httpClient:  nil,
	}
}

func (g *FetchFeedsGateway) FetchFeeds(ctx context.Context, link string) ([]*domain.FeedItem, error) {
	// Apply rate limiting if rate limiter is configured
	if g.rateLimiter != nil {
		slog.InfoContext(ctx, "Applying rate limiting for external feed request", "url", link)
		if err := g.rateLimiter.WaitForHost(ctx, link); err != nil {
			slog.ErrorContext(ctx, "Rate limiting failed", "url", link, "error", err)
			return nil, errors.New("rate limiting failed")
		}
		slog.InfoContext(ctx, "Rate limiting passed, proceeding with feed request", "url", link)
	}

	// Use provided HTTP client if available, otherwise create a secure one
	httpClient := g.httpClient
	if httpClient == nil {
		factory := utils.NewHTTPClientFactory()
		httpClient = factory.CreateHTTPClient()
	}

	fp := newBoundedFeedParser(httpClient)
	feed, err := fp.ParseURL(link)
	if err != nil {
		logger.SafeErrorContext(ctx, "Error parsing feed", "error", err)
		return nil, errors.New("error parsing feed")
	}

	var feedItems []*domain.FeedItem
	for _, item := range feed.Items {
		feedItem := &domain.FeedItem{
			Title:       item.Title,
			Description: sanitize.SanitizeDescription(item.Description),
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
	feeds, err := g.store.FetchFeedsList(ctx)
	if err != nil {
		logger.SafeErrorContext(ctx, "Error fetching feeds list", "error", err)
		return nil, errors.New("error fetching feeds list")
	}

	feedItems := make([]*domain.FeedItem, 0, len(feeds))
	for _, feed := range feeds {
		publishedTime := feed.CreatedAt
		feedItems = append(feedItems, &domain.FeedItem{
			Title:           feed.Title,
			Description:     sanitize.SanitizeDescription(feed.Description),
			Link:            feed.WebsiteURL,
			Published:       publishedTime.Format(time.RFC3339),
			PublishedParsed: publishedTime,
		})
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
		publishedTime := feed.CreatedAt
		feedItems = append(feedItems, &domain.FeedItem{
			Title:           feed.Title,
			Description:     sanitize.SanitizeDescription(feed.Description),
			Link:            feed.WebsiteURL,
			Published:       publishedTime.Format(time.RFC3339),
			PublishedParsed: publishedTime,
		})
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
		publishedTime := feed.CreatedAt
		feedItems = append(feedItems, &domain.FeedItem{
			Title:           feed.Title,
			Description:     sanitize.SanitizeDescription(feed.Description),
			Link:            feed.WebsiteURL,
			Published:       publishedTime.Format(time.RFC3339),
			PublishedParsed: publishedTime,
		})
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
		publishedTime := feed.CreatedAt

		feedID, err := parseFeedID(feed.ID)
		if err != nil {
			logger.SafeErrorContext(ctx, "Error reading feed id from cursor page", "error", err)
			return nil, errors.New("error fetching feeds with cursor")
		}

		feedItem := &domain.FeedItem{
			FeedID:          feedID,
			Title:           feed.Title,
			Description:     sanitize.SanitizeDescription(feed.Description),
			Link:            feed.WebsiteURL,
			Published:       publishedTime.Format(time.RFC3339),
			PublishedParsed: publishedTime,
			IsRead:          feed.IsRead,
			OgImageURL:      derefString(feed.OgImageURL),
		}

		if feed.ArticleID != nil {
			feedItem.ArticleID = *feed.ArticleID
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

		feedID, err := parseFeedID(feed.ID)
		if err != nil {
			logger.SafeErrorContext(ctx, "Error reading feed id from unread cursor page", "error", err)
			return nil, errors.New("error fetching unread feeds with cursor")
		}

		feedItem := &domain.FeedItem{
			FeedID:          feedID,
			Title:           feed.Title,
			Description:     sanitize.SanitizeDescription(feed.Description),
			Link:            feed.WebsiteURL,
			Published:       publishedTime.Format(time.RFC3339),
			PublishedParsed: publishedTime,
			OgImageURL:      derefString(feed.OgImageURL),
		}

		// Set ArticleID if article exists in database
		if feed.ArticleID != nil {
			feedItem.ArticleID = *feed.ArticleID
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
		publishedTime := feed.CreatedAt
		feedID, err := parseFeedID(feed.ID)
		if err != nil {
			logger.SafeErrorContext(ctx, "Error reading feed id from read cursor page", "error", err)
			return nil, errors.New("error fetching read feeds with cursor")
		}

		feedItem := &domain.FeedItem{
			FeedID:          feedID,
			Title:           feed.Title,
			Description:     sanitize.SanitizeDescription(feed.Description),
			Link:            feed.WebsiteURL,
			Published:       publishedTime.Format(time.RFC3339),
			PublishedParsed: publishedTime,
			OgImageURL:      derefString(feed.OgImageURL),
		}
		if feed.ArticleID != nil {
			feedItem.ArticleID = *feed.ArticleID
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
		publishedTime := feed.CreatedAt
		feedID, err := parseFeedID(feed.ID)
		if err != nil {
			logger.SafeErrorContext(ctx, "Error reading feed id from favorite cursor page", "error", err)
			return nil, errors.New("error fetching favorite feeds with cursor")
		}

		feedItem := &domain.FeedItem{
			FeedID:          feedID,
			Title:           feed.Title,
			Description:     sanitize.SanitizeDescription(feed.Description),
			Link:            feed.WebsiteURL,
			Published:       publishedTime.Format(time.RFC3339),
			PublishedParsed: publishedTime,
			OgImageURL:      derefString(feed.OgImageURL),
		}
		if feed.ArticleID != nil {
			feedItem.ArticleID = *feed.ArticleID
		}
		feedItems = append(feedItems, feedItem)
	}

	return feedItems, nil
}

// parseFeedID turns a row's feeds.id column into the UUID a domain.FeedItem
// carries, so the client can hand it back to ResolveOgImages.
//
// This is the hop the on-demand og:image resolution was missing: every cursor
// query above already selects f.id and every row already carries it, but the
// four mappers dropped it here, leaving FeedItem.FeedID at uuid.Nil and the
// client with nothing to send but articles.id — which the resolver's
// `WHERE f.id = ANY($1::uuid[])` never matches.
//
// A bad value fails the page rather than blanking one row's id. feeds.id is a
// uuid PRIMARY KEY, so an unparseable one does not mean "this feed is slightly
// off"; it means the column being walked is not the column this code thinks it
// is, and a silently-empty feed_id is precisely the indistinguishable-from-
// working failure the field was added to end (CLAUDE.md rule 8).
// feed_page_cache_gateway makes the same call on the same conversion.
func parseFeedID(id string) (uuid.UUID, error) {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("parse feed id %q: %w", id, err)
	}
	return parsed, nil
}

// derefString safely dereferences a *string, returning "" if nil.
func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
