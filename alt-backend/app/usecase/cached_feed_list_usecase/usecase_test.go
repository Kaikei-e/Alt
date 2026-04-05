package cached_feed_list_usecase

import (
	"context"
	"fmt"
	"testing"
	"time"

	"alt/domain"

	"github.com/google/uuid"
)

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

type unreadFeedPortStub struct {
	feeds []*domain.FeedItem
	err   error
}

func (s *unreadFeedPortStub) FetchUnreadFeedsListCursor(ctx context.Context, cursor *time.Time, limit int, excludeFeedLinkID *uuid.UUID) ([]*domain.FeedItem, error) {
	return s.feeds, s.err
}

type allFeedPortStub struct {
	feeds []*domain.FeedItem
	err   error
}

func (s *allFeedPortStub) FetchFeedsListCursor(ctx context.Context, cursor *time.Time, limit int, excludeFeedLinkID *uuid.UUID) ([]*domain.FeedItem, error) {
	return s.feeds, s.err
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

// =============================================================================
// Unread feeds: delegates to SQL-backed UnreadFeedCursorPort
// =============================================================================

func TestFetchUnreadFeedsListCursor_Basic(t *testing.T) {
	ctx := newTestContext(t)
	now := time.Now()

	usecase := NewCachedFeedListUsecase(
		&legacyFeedPortStub{},
		&unreadFeedPortStub{feeds: []*domain.FeedItem{
			{Title: "newer", Link: "http://newer.com", PublishedParsed: now},
		}},
		&allFeedPortStub{},
	)

	feeds, hasMore, err := usecase.FetchUnreadFeedsListCursor(ctx, nil, 10, nil)
	if err != nil {
		t.Fatalf("error = %v", err)
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

func TestFetchUnreadFeedsListCursor_HasMore(t *testing.T) {
	ctx := newTestContext(t)
	now := time.Now()

	// Port returns limit+1 items → hasMore=true, result trimmed
	portFeeds := make([]*domain.FeedItem, 4)
	for i := range 4 {
		portFeeds[i] = &domain.FeedItem{
			Title:           fmt.Sprintf("feed-%d", i),
			Link:            fmt.Sprintf("http://example.com/%d", i),
			PublishedParsed: now.Add(-time.Duration(i) * time.Minute),
		}
	}

	usecase := NewCachedFeedListUsecase(
		&legacyFeedPortStub{},
		&unreadFeedPortStub{feeds: portFeeds},
		&allFeedPortStub{},
	)

	feeds, hasMore, err := usecase.FetchUnreadFeedsListCursor(ctx, nil, 3, nil)
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if !hasMore {
		t.Fatal("hasMore = false, want true")
	}
	if len(feeds) != 3 {
		t.Fatalf("len(feeds) = %d, want 3", len(feeds))
	}
}

func TestFetchUnreadFeedsListCursor_HasMore_ExactBoundary(t *testing.T) {
	ctx := newTestContext(t)
	now := time.Now()

	// Port returns exactly 5 items for limit=5 → hasMore=false
	portFeeds := make([]*domain.FeedItem, 5)
	for i := range 5 {
		portFeeds[i] = &domain.FeedItem{
			Title:           fmt.Sprintf("feed-%d", i),
			Link:            fmt.Sprintf("http://example.com/%d", i),
			PublishedParsed: now.Add(-time.Duration(i) * time.Minute),
		}
	}

	usecase := NewCachedFeedListUsecase(
		&legacyFeedPortStub{},
		&unreadFeedPortStub{feeds: portFeeds},
		&allFeedPortStub{},
	)

	feeds, hasMore, err := usecase.FetchUnreadFeedsListCursor(ctx, nil, 5, nil)
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if hasMore {
		t.Fatal("hasMore = true, want false (exact count)")
	}
	if len(feeds) != 5 {
		t.Fatalf("len(feeds) = %d, want 5", len(feeds))
	}
}

func TestFetchUnreadFeedsListCursor_ExcludeFeedLinkID(t *testing.T) {
	ctx := newTestContext(t)
	now := time.Now()

	// SQL port handles exclude_feed_link_id in WHERE clause
	usecase := NewCachedFeedListUsecase(
		&legacyFeedPortStub{},
		&unreadFeedPortStub{feeds: []*domain.FeedItem{
			{Title: "from-B", Link: "b", PublishedParsed: now},
		}},
		&allFeedPortStub{},
	)

	linkA := uuid.New()
	feeds, _, err := usecase.FetchUnreadFeedsListCursor(ctx, nil, 100, &linkA)
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if len(feeds) != 1 {
		t.Fatalf("len(feeds) = %d, want 1", len(feeds))
	}
	if feeds[0].Title != "from-B" {
		t.Fatalf("feeds[0].Title = %q, want from-B", feeds[0].Title)
	}
}

func TestFetchUnreadFeedsListCursor_ErrorPropagation(t *testing.T) {
	ctx := newTestContext(t)

	usecase := NewCachedFeedListUsecase(
		&legacyFeedPortStub{},
		&unreadFeedPortStub{err: fmt.Errorf("db connection refused")},
		&allFeedPortStub{},
	)

	_, _, err := usecase.FetchUnreadFeedsListCursor(ctx, nil, 10, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestFetchUnreadFeedsListCursor_InvalidLimit(t *testing.T) {
	ctx := newTestContext(t)

	usecase := NewCachedFeedListUsecase(
		&legacyFeedPortStub{},
		&unreadFeedPortStub{},
		&allFeedPortStub{},
	)

	_, _, err := usecase.FetchUnreadFeedsListCursor(ctx, nil, 0, nil)
	if err == nil {
		t.Fatal("expected error for limit=0, got nil")
	}

	_, _, err = usecase.FetchUnreadFeedsListCursor(ctx, nil, 101, nil)
	if err == nil {
		t.Fatal("expected error for limit=101, got nil")
	}
}

// =============================================================================
// All feeds: delegates to SQL-backed FeedCursorPort
// =============================================================================

func TestFetchAllFeedsListCursor_Basic(t *testing.T) {
	ctx := newTestContext(t)
	now := time.Now()

	usecase := NewCachedFeedListUsecase(
		&legacyFeedPortStub{},
		&unreadFeedPortStub{},
		&allFeedPortStub{feeds: []*domain.FeedItem{
			{Title: "all-1", Link: "http://example.com/1", PublishedParsed: now, IsRead: true},
			{Title: "all-2", Link: "http://example.com/2", PublishedParsed: now.Add(-time.Minute), IsRead: false},
		}},
	)

	feeds, err := usecase.FetchAllFeedsListCursor(ctx, nil, 10, nil)
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if len(feeds) != 2 {
		t.Fatalf("len(feeds) = %d, want 2", len(feeds))
	}
	if !feeds[0].IsRead {
		t.Fatal("feeds[0].IsRead = false, want true (preserved from SQL)")
	}
	if feeds[1].IsRead {
		t.Fatal("feeds[1].IsRead = true, want false")
	}
}

func TestFetchAllFeedsListCursor_ErrorPropagation(t *testing.T) {
	ctx := newTestContext(t)

	usecase := NewCachedFeedListUsecase(
		&legacyFeedPortStub{},
		&unreadFeedPortStub{},
		&allFeedPortStub{err: fmt.Errorf("db timeout")},
	)

	_, err := usecase.FetchAllFeedsListCursor(ctx, nil, 10, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
