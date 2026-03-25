package cached_feed_list_usecase

import (
	"context"
	"fmt"
	"testing"
	"time"

	"alt/domain"
	"alt/port/feed_page_cache_port"

	"github.com/google/uuid"
)

type feedPageCacheStub struct {
	pages map[uuid.UUID][]*feed_page_cache_port.FeedPageEntry
	errs  map[uuid.UUID]error
}

func (s *feedPageCacheStub) GetFeedPage(ctx context.Context, feedLinkID uuid.UUID) ([]*feed_page_cache_port.FeedPageEntry, error) {
	if s.errs != nil {
		if err, ok := s.errs[feedLinkID]; ok {
			return nil, err
		}
	}
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

func newTestContext(t *testing.T) context.Context {
	t.Helper()
	now := time.Now()
	return domain.SetUserContext(context.Background(), &domain.UserContext{
		UserID:    uuid.New(),
		Email:     "test@example.com",
		Role:      domain.UserRoleUser,
		TenantID:  uuid.New(),
		LoginAt:   now,
		ExpiresAt: now.Add(time.Hour),
	})
}

func TestLoadMergedFeeds_ExcludeFeedLinkID(t *testing.T) {
	ctx := newTestContext(t)
	linkA := uuid.New()
	linkB := uuid.New()
	now := time.Now()

	usecase := NewCachedFeedListUsecase(
		&feedPageCacheStub{
			pages: map[uuid.UUID][]*feed_page_cache_port.FeedPageEntry{
				linkA: {{FeedID: uuid.New(), Title: "from-A", Link: "a", CreatedAt: now}},
				linkB: {{FeedID: uuid.New(), Title: "from-B", Link: "b", CreatedAt: now}},
			},
		},
		&userReadStateStub{
			subscriptions: []uuid.UUID{linkA, linkB},
			readMap:       map[uuid.UUID]bool{},
		},
		&legacyFeedPortStub{},
	)

	feeds, _, err := usecase.FetchUnreadFeedsListCursor(ctx, nil, 100, &linkA)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(feeds) != 1 {
		t.Fatalf("len(feeds) = %d, want 1 (linkA should be excluded)", len(feeds))
	}
	if feeds[0].Title != "from-B" {
		t.Fatalf("feeds[0].Title = %q, want from-B", feeds[0].Title)
	}
}

func TestLoadMergedFeeds_SortStability_SameTimestamp(t *testing.T) {
	ctx := newTestContext(t)
	linkA := uuid.New()
	linkB := uuid.New()
	sameTime := time.Date(2026, 3, 22, 12, 0, 0, 0, time.UTC)

	usecase := NewCachedFeedListUsecase(
		&feedPageCacheStub{
			pages: map[uuid.UUID][]*feed_page_cache_port.FeedPageEntry{
				linkA: {
					{FeedID: uuid.New(), Title: "alpha", Link: "http://alpha.com", CreatedAt: sameTime},
				},
				linkB: {
					{FeedID: uuid.New(), Title: "zeta", Link: "http://zeta.com", CreatedAt: sameTime},
				},
			},
		},
		&userReadStateStub{
			subscriptions: []uuid.UUID{linkA, linkB},
			readMap:       map[uuid.UUID]bool{},
		},
		&legacyFeedPortStub{},
	)

	feeds, _, err := usecase.FetchUnreadFeedsListCursor(ctx, nil, 100, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(feeds) != 2 {
		t.Fatalf("len(feeds) = %d, want 2", len(feeds))
	}
	// Same timestamp: sorted by Link descending → zeta first
	if feeds[0].Link != "http://zeta.com" {
		t.Fatalf("feeds[0].Link = %q, want http://zeta.com (Link desc for same timestamp)", feeds[0].Link)
	}
	if feeds[1].Link != "http://alpha.com" {
		t.Fatalf("feeds[1].Link = %q, want http://alpha.com", feeds[1].Link)
	}
}

func TestLoadMergedFeeds_ErrorPropagation(t *testing.T) {
	ctx := newTestContext(t)
	linkOK := uuid.New()
	linkFail := uuid.New()
	now := time.Now()

	usecase := NewCachedFeedListUsecase(
		&feedPageCacheStub{
			pages: map[uuid.UUID][]*feed_page_cache_port.FeedPageEntry{
				linkOK: {{FeedID: uuid.New(), Title: "ok", CreatedAt: now}},
			},
			errs: map[uuid.UUID]error{
				linkFail: fmt.Errorf("db connection refused"),
			},
		},
		&userReadStateStub{
			subscriptions: []uuid.UUID{linkOK, linkFail},
			readMap:       map[uuid.UUID]bool{},
		},
		&legacyFeedPortStub{},
	)

	_, _, err := usecase.FetchUnreadFeedsListCursor(ctx, nil, 100, nil)
	if err == nil {
		t.Fatal("expected error from failed GetFeedPage, got nil")
	}
}

func TestFetchUnreadFeedsListCursor_HasMore_Boundary(t *testing.T) {
	ctx := newTestContext(t)
	linkID := uuid.New()
	now := time.Now()

	entries := make([]*feed_page_cache_port.FeedPageEntry, 5)
	for i := 0; i < 5; i++ {
		entries[i] = &feed_page_cache_port.FeedPageEntry{
			FeedID:    uuid.New(),
			Title:     fmt.Sprintf("feed-%d", i),
			Link:      fmt.Sprintf("http://example.com/%d", i),
			CreatedAt: now.Add(-time.Duration(i) * time.Minute),
		}
	}

	usecase := NewCachedFeedListUsecase(
		&feedPageCacheStub{
			pages: map[uuid.UUID][]*feed_page_cache_port.FeedPageEntry{linkID: entries},
		},
		&userReadStateStub{
			subscriptions: []uuid.UUID{linkID},
			readMap:       map[uuid.UUID]bool{},
		},
		&legacyFeedPortStub{},
	)

	// limit=3, 5 unread feeds → hasMore=true, returns 3
	feeds, hasMore, err := usecase.FetchUnreadFeedsListCursor(ctx, nil, 3, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hasMore {
		t.Fatal("hasMore = false, want true")
	}
	if len(feeds) != 3 {
		t.Fatalf("len(feeds) = %d, want 3", len(feeds))
	}

	// limit=5, exact match → hasMore=false
	feeds, hasMore, err = usecase.FetchUnreadFeedsListCursor(ctx, nil, 5, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hasMore {
		t.Fatal("hasMore = true, want false (exact count)")
	}
	if len(feeds) != 5 {
		t.Fatalf("len(feeds) = %d, want 5", len(feeds))
	}
}
