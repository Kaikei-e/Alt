package cached_feed_list_usecase

import (
	"context"
	"testing"
	"time"

	"alt/domain"
	"alt/port/feed_page_cache_port"

	"github.com/google/uuid"
)

type feedPageCacheStub struct {
	pages map[uuid.UUID][]*feed_page_cache_port.FeedPageEntry
}

func (s *feedPageCacheStub) GetFeedPage(ctx context.Context, feedLinkID uuid.UUID) ([]*feed_page_cache_port.FeedPageEntry, error) {
	return s.pages[feedLinkID], nil
}

func (s *feedPageCacheStub) InvalidateFeedPage(ctx context.Context, feedLinkID uuid.UUID) error {
	return nil
}

type userReadStateStub struct {
	subscriptions []uuid.UUID
	readMap       map[uuid.UUID]bool
}

func (s *userReadStateStub) GetReadFeedIDs(ctx context.Context, userID uuid.UUID, feedIDs []uuid.UUID) (map[uuid.UUID]bool, error) {
	return s.readMap, nil
}

func (s *userReadStateStub) GetAllReadFeedIDs(ctx context.Context, userID uuid.UUID) (map[uuid.UUID]bool, error) {
	return s.readMap, nil
}

func (s *userReadStateStub) GetUserSubscriptions(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error) {
	return s.subscriptions, nil
}

type legacyFeedPortStub struct {
	read     []*domain.FeedItem
	favorite []*domain.FeedItem
}

func (s *legacyFeedPortStub) FetchFeeds(ctx context.Context, link string) ([]*domain.FeedItem, error) {
	return nil, nil
}
func (s *legacyFeedPortStub) FetchFeedsList(ctx context.Context) ([]*domain.FeedItem, error) {
	return nil, nil
}
func (s *legacyFeedPortStub) FetchFeedsListLimit(ctx context.Context, offset int) ([]*domain.FeedItem, error) {
	return nil, nil
}
func (s *legacyFeedPortStub) FetchFeedsListPage(ctx context.Context, page int) ([]*domain.FeedItem, error) {
	return nil, nil
}
func (s *legacyFeedPortStub) FetchFeedsListCursor(ctx context.Context, cursor *time.Time, limit int, excludeFeedLinkID *uuid.UUID) ([]*domain.FeedItem, error) {
	return nil, nil
}
func (s *legacyFeedPortStub) FetchUnreadFeedsListCursor(ctx context.Context, cursor *time.Time, limit int, excludeFeedLinkID *uuid.UUID) ([]*domain.FeedItem, error) {
	return nil, nil
}
func (s *legacyFeedPortStub) FetchReadFeedsListCursor(ctx context.Context, cursor *time.Time, limit int) ([]*domain.FeedItem, error) {
	return s.read, nil
}
func (s *legacyFeedPortStub) FetchFavoriteFeedsListCursor(ctx context.Context, cursor *time.Time, limit int) ([]*domain.FeedItem, error) {
	return s.favorite, nil
}

func TestCachedFeedListUsecase_FetchUnreadFeedsListCursor(t *testing.T) {
	userID := uuid.New()
	feedLinkID := uuid.New()
	feedID1 := uuid.New()
	feedID2 := uuid.New()
	now := time.Now()
	ctx := domain.SetUserContext(context.Background(), &domain.UserContext{
		UserID:    userID,
		Email:     "test@example.com",
		Role:      domain.UserRoleUser,
		TenantID:  uuid.New(),
		LoginAt:   now,
		ExpiresAt: now.Add(time.Hour),
	})

	usecase := NewCachedFeedListUsecase(
		&feedPageCacheStub{
			pages: map[uuid.UUID][]*feed_page_cache_port.FeedPageEntry{
				feedLinkID: {
					{FeedID: feedID1, Title: "newer", CreatedAt: now},
					{FeedID: feedID2, Title: "older", CreatedAt: now.Add(-time.Minute)},
				},
			},
		},
		&userReadStateStub{
			subscriptions: []uuid.UUID{feedLinkID},
			readMap:       map[uuid.UUID]bool{feedID2: true},
		},
		&legacyFeedPortStub{},
	)

	feeds, hasMore, err := usecase.FetchUnreadFeedsListCursor(ctx, nil, 10, nil)
	if err != nil {
		t.Fatalf("FetchUnreadFeedsListCursor() error = %v", err)
	}
	if hasMore {
		t.Fatal("hasMore = true, want false")
	}
	if len(feeds) != 1 {
		t.Fatalf("len(feeds) = %d, want 1", len(feeds))
	}
	if feeds[0].Title != "newer" {
		t.Fatalf("feeds[0].Title = %q, want newer", feeds[0].Title)
	}
}

func TestCachedFeedListUsecase_FetchAllFeedsListCursor_SetsReadFlags(t *testing.T) {
	userID := uuid.New()
	feedLinkID := uuid.New()
	feedID := uuid.New()
	now := time.Now()
	ctx := domain.SetUserContext(context.Background(), &domain.UserContext{
		UserID:    userID,
		Email:     "test@example.com",
		Role:      domain.UserRoleUser,
		TenantID:  uuid.New(),
		LoginAt:   now,
		ExpiresAt: now.Add(time.Hour),
	})

	usecase := NewCachedFeedListUsecase(
		&feedPageCacheStub{
			pages: map[uuid.UUID][]*feed_page_cache_port.FeedPageEntry{
				feedLinkID: {{FeedID: feedID, Title: "title", CreatedAt: now}},
			},
		},
		&userReadStateStub{
			subscriptions: []uuid.UUID{feedLinkID},
			readMap:       map[uuid.UUID]bool{feedID: true},
		},
		&legacyFeedPortStub{},
	)

	feeds, err := usecase.FetchAllFeedsListCursor(ctx, nil, 10, nil)
	if err != nil {
		t.Fatalf("FetchAllFeedsListCursor() error = %v", err)
	}
	if len(feeds) != 1 {
		t.Fatalf("len(feeds) = %d, want 1", len(feeds))
	}
	if !feeds[0].IsRead {
		t.Fatal("feeds[0].IsRead = false, want true")
	}
}
