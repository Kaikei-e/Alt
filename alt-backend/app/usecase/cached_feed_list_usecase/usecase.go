package cached_feed_list_usecase

import (
	"alt/domain"
	"alt/port/feed_page_cache_port"
	"alt/port/fetch_feed_port"
	"alt/port/user_read_state_port"
	"context"
	"errors"
	"sort"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
)

type CachedFeedListUsecase struct {
	feedPageCache  feed_page_cache_port.FeedPageCachePort
	userReadState  user_read_state_port.UserReadStatePort
	legacyFeedPort fetch_feed_port.ReadAndFavoriteFeedCursorPort
}

func NewCachedFeedListUsecase(
	feedPageCache feed_page_cache_port.FeedPageCachePort,
	userReadState user_read_state_port.UserReadStatePort,
	legacyFeedPort fetch_feed_port.ReadAndFavoriteFeedCursorPort,
) *CachedFeedListUsecase {
	return &CachedFeedListUsecase{
		feedPageCache:  feedPageCache,
		userReadState:  userReadState,
		legacyFeedPort: legacyFeedPort,
	}
}

func (u *CachedFeedListUsecase) FetchUnreadFeedsListCursor(
	ctx context.Context,
	cursor *time.Time,
	limit int,
	excludeFeedLinkID *uuid.UUID,
) ([]*domain.FeedItem, bool, error) {
	if err := validateLimit(limit); err != nil {
		return nil, false, err
	}

	user, err := domain.GetUserFromContext(ctx)
	if err != nil {
		return nil, false, err
	}

	merged, err := u.loadMergedFeeds(ctx, user.UserID, excludeFeedLinkID)
	if err != nil {
		return nil, false, err
	}

	filtered := applyCursor(merged, cursor)
	if len(filtered) == 0 {
		return []*domain.FeedItem{}, false, nil
	}

	readMap, err := u.userReadState.GetAllReadFeedIDs(ctx, user.UserID)
	if err != nil {
		return nil, false, err
	}

	unreadOnly := make([]*domain.FeedItem, 0, len(filtered))
	for _, feed := range filtered {
		if !readMap[feedIDToUUID(feed)] {
			unreadOnly = append(unreadOnly, feed)
		}
	}

	hasMore := len(unreadOnly) > limit
	if hasMore {
		unreadOnly = unreadOnly[:limit]
	}
	return unreadOnly, hasMore, nil
}

func (u *CachedFeedListUsecase) FetchAllFeedsListCursor(
	ctx context.Context,
	cursor *time.Time,
	limit int,
	excludeFeedLinkID *uuid.UUID,
) ([]*domain.FeedItem, error) {
	if err := validateLimit(limit); err != nil {
		return nil, err
	}

	user, err := domain.GetUserFromContext(ctx)
	if err != nil {
		return nil, err
	}

	merged, err := u.loadMergedFeeds(ctx, user.UserID, excludeFeedLinkID)
	if err != nil {
		return nil, err
	}
	filtered := applyCursor(merged, cursor)
	readMap, err := u.userReadState.GetAllReadFeedIDs(ctx, user.UserID)
	if err != nil {
		return nil, err
	}
	for _, feed := range filtered {
		feed.IsRead = readMap[feedIDToUUID(feed)]
	}
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}
	return filtered, nil
}

func (u *CachedFeedListUsecase) FetchReadFeedsListCursor(ctx context.Context, cursor *time.Time, limit int) ([]*domain.FeedItem, error) {
	return u.legacyFeedPort.FetchReadFeedsListCursor(ctx, cursor, limit)
}

func (u *CachedFeedListUsecase) FetchFavoriteFeedsListCursor(ctx context.Context, cursor *time.Time, limit int) ([]*domain.FeedItem, error) {
	return u.legacyFeedPort.FetchFavoriteFeedsListCursor(ctx, cursor, limit)
}

func (u *CachedFeedListUsecase) loadMergedFeeds(ctx context.Context, userID uuid.UUID, excludeFeedLinkID *uuid.UUID) ([]*domain.FeedItem, error) {
	subscriptions, err := u.userReadState.GetUserSubscriptions(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Filter excluded subscription
	filtered := subscriptions
	if excludeFeedLinkID != nil {
		filtered = make([]uuid.UUID, 0, len(subscriptions))
		for _, id := range subscriptions {
			if id != *excludeFeedLinkID {
				filtered = append(filtered, id)
			}
		}
	}

	// Parallel fetch with bounded concurrency
	results := make([][]*domain.FeedItem, len(filtered))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(8)

	for i, feedLinkID := range filtered {
		g.Go(func() error {
			page, pageErr := u.feedPageCache.GetFeedPage(gctx, feedLinkID)
			if pageErr != nil {
				return pageErr
			}
			results[i] = convertFeedPageEntries(page, &feedLinkID)
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}

	// Merge results
	total := 0
	for _, r := range results {
		total += len(r)
	}
	allFeeds := make([]*domain.FeedItem, 0, total)
	for _, r := range results {
		allFeeds = append(allFeeds, r...)
	}

	sort.SliceStable(allFeeds, func(i, j int) bool {
		if allFeeds[i].PublishedParsed.Equal(allFeeds[j].PublishedParsed) {
			return allFeeds[i].Link > allFeeds[j].Link
		}
		return allFeeds[i].PublishedParsed.After(allFeeds[j].PublishedParsed)
	})
	return allFeeds, nil
}

// emptyAuthors is shared across all FeedItems to avoid per-item allocation.
var emptyAuthors = []domain.Author{}

func convertFeedPageEntries(entries []*feed_page_cache_port.FeedPageEntry, feedLinkID *uuid.UUID) []*domain.FeedItem {
	n := len(entries)
	if n == 0 {
		return nil
	}

	// Hoist feedLinkID.String() outside the loop
	var feedLinkIDStr string
	if feedLinkID != nil {
		feedLinkIDStr = feedLinkID.String()
	}

	// Contiguous allocation: 2 heap allocs instead of N+1
	items := make([]domain.FeedItem, n)
	feeds := make([]*domain.FeedItem, n)
	// Backing array for Links slices: 1 alloc for all entries
	linksBackingArray := make([]string, n)

	for i, entry := range entries {
		// Use pre-computed SanitizedDescription when available
		desc := entry.Description
		if entry.SanitizedDescription != "" {
			desc = entry.SanitizedDescription
		}

		// Use pre-computed strings when available
		feedIDStr := entry.FeedIDStr
		if feedIDStr == "" {
			feedIDStr = entry.FeedID.String()
		}
		publishedStr := entry.PublishedStr
		if publishedStr == "" {
			publishedStr = entry.CreatedAt.Format(time.RFC3339)
		}

		items[i] = domain.FeedItem{
			Title:           entry.Title,
			Description:     desc,
			Link:            entry.Link,
			Published:       publishedStr,
			PublishedParsed: entry.CreatedAt,
			OgImageURL:      deref(entry.OgImageURL),
			Authors:         emptyAuthors,
			FeedID:          entry.FeedID,
		}
		if entry.ArticleID != nil {
			items[i].ArticleID = *entry.ArticleID
		}
		if feedLinkID != nil {
			items[i].FeedLinkID = &feedLinkIDStr
		}
		linksBackingArray[i] = feedIDStr
		items[i].Links = linksBackingArray[i : i+1 : i+1]
		feeds[i] = &items[i]
	}
	return feeds
}

func applyCursor(feeds []*domain.FeedItem, cursor *time.Time) []*domain.FeedItem {
	if cursor == nil {
		return feeds
	}
	filtered := make([]*domain.FeedItem, 0, len(feeds))
	for _, feed := range feeds {
		if feed.PublishedParsed.Before(*cursor) {
			filtered = append(filtered, feed)
		}
	}
	return filtered
}

func feedIDToUUID(feed *domain.FeedItem) uuid.UUID {
	return feed.FeedID
}

func deref(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func validateLimit(limit int) error {
	if limit <= 0 {
		return errors.New("limit must be greater than 0")
	}
	if limit > 100 {
		return errors.New("limit cannot exceed 100")
	}
	return nil
}
