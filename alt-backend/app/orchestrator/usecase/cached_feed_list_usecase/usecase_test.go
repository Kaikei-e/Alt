package cached_feed_list_usecase

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"alt/domain"
	"alt/utils/logger"

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
	feeds   []*domain.FeedItem
	err     error
	queryFn func(ctx context.Context, cursor *time.Time, cursorID *uuid.UUID, limit int, excludeFeedLinkIDs []uuid.UUID) ([]*domain.FeedItem, error)
}

func (s *unreadFeedPortStub) FetchUnreadFeedsListCursor(ctx context.Context, cursor *time.Time, limit int, excludeFeedLinkIDs []uuid.UUID) ([]*domain.FeedItem, error) {
	if s.queryFn != nil {
		return s.queryFn(ctx, cursor, nil, limit, excludeFeedLinkIDs)
	}
	return s.feeds, s.err
}

func (s *unreadFeedPortStub) FetchUnreadFeedsListCursorWithID(ctx context.Context, cursor *time.Time, cursorID *uuid.UUID, limit int, excludeFeedLinkIDs []uuid.UUID) ([]*domain.FeedItem, error) {
	if s.queryFn != nil {
		return s.queryFn(ctx, cursor, cursorID, limit, excludeFeedLinkIDs)
	}
	return s.feeds, s.err
}

type allFeedPortStub struct {
	feeds []*domain.FeedItem
	err   error
}

func (s *allFeedPortStub) FetchFeedsListCursor(ctx context.Context, cursor *time.Time, limit int, excludeFeedLinkIDs []uuid.UUID) ([]*domain.FeedItem, error) {
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
	feeds, _, err := usecase.FetchUnreadFeedsListCursor(ctx, nil, 100, []uuid.UUID{linkA})
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

func intPtr(v int) *int { return &v }

func TestFetchUnreadFeedsListCursor_DailyCapFilteringAndRefill(t *testing.T) {
	ctx := newTestContext(t)
	base := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)

	t.Run("hides rows exceeding daily cap and keeps rows within cap or uncapped", func(t *testing.T) {
		id1, id2, id3 := uuid.New(), uuid.New(), uuid.New()
		uc := NewCachedFeedListUsecase(
			&legacyFeedPortStub{},
			&unreadFeedPortStub{feeds: []*domain.FeedItem{
				{FeedID: id1, Title: "over-cap", PublishedParsed: base, DailyRank: intPtr(4), DailyCap: intPtr(3)},
				{FeedID: id2, Title: "at-cap", PublishedParsed: base.Add(-time.Minute), DailyRank: intPtr(3), DailyCap: intPtr(3)},
				{FeedID: id3, Title: "uncapped", PublishedParsed: base.Add(-2 * time.Minute)},
			}},
			&allFeedPortStub{},
		)

		feeds, hasMore, err := uc.FetchUnreadFeedsListCursor(ctx, nil, 10, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if hasMore {
			t.Fatal("hasMore = true, want false")
		}
		if len(feeds) != 2 || feeds[0].Title != "at-cap" || feeds[1].Title != "uncapped" {
			t.Fatalf("unexpected visible items: %+v", feeds)
		}
	})

	t.Run("fails loudly when daily_cap is set without daily_rank", func(t *testing.T) {
		uc := NewCachedFeedListUsecase(
			&legacyFeedPortStub{},
			&unreadFeedPortStub{feeds: []*domain.FeedItem{
				{FeedID: uuid.New(), Title: "missing-rank", PublishedParsed: base, DailyCap: intPtr(2)},
			}},
			&allFeedPortStub{},
		)

		_, _, err := uc.FetchUnreadFeedsListCursor(ctx, nil, 10, nil)
		if err == nil {
			t.Fatal("expected error when daily_cap is set without daily_rank")
		}
	})

	t.Run("refills across pages using tie-breaking cursorID without skipping or repeating", func(t *testing.T) {
		sameTime := base
		id1, id2, id3, id4, id5 := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
		dataset := []*domain.FeedItem{
			{FeedID: id1, Title: "keep-1", PublishedParsed: sameTime, DailyRank: intPtr(1), DailyCap: intPtr(1)},
			{FeedID: id2, Title: "drop-2", PublishedParsed: sameTime, DailyRank: intPtr(2), DailyCap: intPtr(1)},
			{FeedID: id3, Title: "drop-3", PublishedParsed: sameTime, DailyRank: intPtr(3), DailyCap: intPtr(1)},
			{FeedID: id4, Title: "keep-4", PublishedParsed: sameTime, DailyRank: intPtr(1), DailyCap: intPtr(2)},
			{FeedID: id5, Title: "keep-5", PublishedParsed: sameTime.Add(-time.Minute), DailyRank: intPtr(1), DailyCap: intPtr(2)},
		}

		var calls int
		uc := NewCachedFeedListUsecase(
			&legacyFeedPortStub{},
			&unreadFeedPortStub{queryFn: func(_ context.Context, cursor *time.Time, cursorID *uuid.UUID, limit int, _ []uuid.UUID) ([]*domain.FeedItem, error) {
				calls++
				if calls > 1 && cursorID == nil {
					t.Fatal("expected cursorID on refill round")
				}
				start := 0
				if cursor != nil {
					for i, item := range dataset {
						if item.PublishedParsed.Equal(*cursor) && (cursorID == nil || item.FeedID == *cursorID) {
							start = i + 1
							break
						}
					}
				}
				end := start + limit
				if end > len(dataset) {
					end = len(dataset)
				}
				return dataset[start:end], nil
			}},
			&allFeedPortStub{},
		)

		feeds, hasMore, err := uc.FetchUnreadFeedsListCursor(ctx, nil, 2, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !hasMore {
			t.Fatal("hasMore = false, want true")
		}
		if len(feeds) != 2 || feeds[0].FeedID != id1 || feeds[1].FeedID != id4 {
			t.Fatalf("got feeds = %+v, want [keep-1, keep-4]", feeds)
		}
		if calls < 2 {
			t.Fatalf("calls = %d, expected at least 2 (refill round)", calls)
		}
	})

	t.Run("bounds refill rounds when consecutive rows exceed cap", func(t *testing.T) {
		var calls int
		uc := NewCachedFeedListUsecase(
			&legacyFeedPortStub{},
			&unreadFeedPortStub{queryFn: func(_ context.Context, cursor *time.Time, _ *uuid.UUID, limit int, _ []uuid.UUID) ([]*domain.FeedItem, error) {
				calls++
				out := make([]*domain.FeedItem, limit)
				tStart := base.Add(-time.Duration(calls) * time.Hour)
				for i := range out {
					out[i] = &domain.FeedItem{
						FeedID:          uuid.New(),
						PublishedParsed: tStart.Add(-time.Duration(i) * time.Millisecond),
						DailyRank:       intPtr(5),
						DailyCap:        intPtr(1),
					}
				}
				return out, nil
			}},
			&allFeedPortStub{},
		)

		feeds, hasMore, err := uc.FetchUnreadFeedsListCursor(ctx, nil, 5, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if hasMore {
			t.Fatal("hasMore = true when empty after max refill rounds, want false")
		}
		if len(feeds) != 0 {
			t.Fatalf("len(feeds) = %d, want 0", len(feeds))
		}
		if calls != maxDailyCapRefillRounds+1 {
			t.Fatalf("calls = %d, want %d", calls, maxDailyCapRefillRounds+1)
		}
	})

	t.Run("multi-page pagination with cap=2 over 5 items on one UTC day and interleaved feeds has no duplicates, gaps, or short pages", func(t *testing.T) {
		day := time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC)
		// Descending created_at order matching SQL ORDER BY f.created_at DESC, f.id DESC.
		// Feed A (cap=2) has 5 items on one UTC day (ranks 5..1); ranks 5, 4, 3 are hidden, 2 and 1 visible.
		// Feed B (NULL cap) and Feed C (cap=3) are interleaved.
		dataset := []*domain.FeedItem{
			{FeedID: uuid.New(), Title: "A5", PublishedParsed: day.Add(15 * time.Hour), DailyRank: intPtr(5), DailyCap: intPtr(2)},
			{FeedID: uuid.New(), Title: "B3", PublishedParsed: day.Add(14*time.Hour + 30*time.Minute), DailyRank: intPtr(3), DailyCap: nil},
			{FeedID: uuid.New(), Title: "A4", PublishedParsed: day.Add(14 * time.Hour), DailyRank: intPtr(4), DailyCap: intPtr(2)},
			{FeedID: uuid.New(), Title: "C2", PublishedParsed: day.Add(13*time.Hour + 30*time.Minute), DailyRank: intPtr(2), DailyCap: intPtr(3)},
			{FeedID: uuid.New(), Title: "A3", PublishedParsed: day.Add(13 * time.Hour), DailyRank: intPtr(3), DailyCap: intPtr(2)},
			{FeedID: uuid.New(), Title: "B2", PublishedParsed: day.Add(12 * time.Hour), DailyRank: intPtr(2), DailyCap: nil},
			{FeedID: uuid.New(), Title: "A2", PublishedParsed: day.Add(11 * time.Hour), DailyRank: intPtr(2), DailyCap: intPtr(2)},
			{FeedID: uuid.New(), Title: "C1", PublishedParsed: day.Add(10 * time.Hour), DailyRank: intPtr(1), DailyCap: intPtr(3)},
			{FeedID: uuid.New(), Title: "A1", PublishedParsed: day.Add(9 * time.Hour), DailyRank: intPtr(1), DailyCap: intPtr(2)},
			{FeedID: uuid.New(), Title: "B1", PublishedParsed: day.Add(8 * time.Hour), DailyRank: intPtr(1), DailyCap: nil},
		}

		uc := NewCachedFeedListUsecase(
			&legacyFeedPortStub{},
			&unreadFeedPortStub{queryFn: func(_ context.Context, cursor *time.Time, cursorID *uuid.UUID, limit int, _ []uuid.UUID) ([]*domain.FeedItem, error) {
				var filtered []*domain.FeedItem
				for _, item := range dataset {
					if cursor == nil {
						filtered = append(filtered, item)
					} else if cursorID == nil {
						if item.PublishedParsed.Before(*cursor) {
							filtered = append(filtered, item)
						}
					} else {
						if item.PublishedParsed.Before(*cursor) || (item.PublishedParsed.Equal(*cursor) && item.FeedID.String() < cursorID.String()) {
							filtered = append(filtered, item)
						}
					}
				}
				if len(filtered) > limit {
					filtered = filtered[:limit]
				}
				return filtered, nil
			}},
			&allFeedPortStub{},
		)

		wantTitles := []string{"B3", "C2", "B2", "A2", "C1", "A1", "B1"}
		for _, pageSize := range []int{2, 3} {
			var gotTitles []string
			var cur *time.Time
			for pageNum := 0; pageNum < 10; pageNum++ {
				page, hasMore, err := uc.FetchUnreadFeedsListCursor(ctx, cur, pageSize, nil)
				if err != nil {
					t.Fatalf("pageSize=%d page=%d error: %v", pageSize, pageNum, err)
				}
				if hasMore && len(page) != pageSize {
					t.Fatalf("pageSize=%d page=%d returned short page len=%d while hasMore=true", pageSize, pageNum, len(page))
				}
				for _, item := range page {
					gotTitles = append(gotTitles, item.Title)
				}
				if !hasMore {
					break
				}
				next := page[len(page)-1].PublishedParsed
				cur = &next
			}
			if len(gotTitles) != len(wantTitles) {
				t.Fatalf("pageSize=%d got %v, want %v", pageSize, gotTitles, wantTitles)
			}
			for i := range wantTitles {
				if gotTitles[i] != wantTitles[i] {
					t.Fatalf("pageSize=%d index %d got %q, want %q (all=%v)", pageSize, i, gotTitles[i], wantTitles[i], gotTitles)
				}
			}
		}
	})

	t.Run("NULL cap behaves identically to uncapped without extra refill calls", func(t *testing.T) {
		var calls int
		items := []*domain.FeedItem{
			{FeedID: uuid.New(), Title: "null-cap-high-rank", PublishedParsed: base, DailyRank: intPtr(99), DailyCap: nil},
			{FeedID: uuid.New(), Title: "null-cap-nil-rank", PublishedParsed: base.Add(-time.Minute), DailyRank: nil, DailyCap: nil},
		}
		uc := NewCachedFeedListUsecase(
			&legacyFeedPortStub{},
			&unreadFeedPortStub{queryFn: func(_ context.Context, _ *time.Time, _ *uuid.UUID, _ int, _ []uuid.UUID) ([]*domain.FeedItem, error) {
				calls++
				return items, nil
			}},
			&allFeedPortStub{},
		)

		feeds, hasMore, err := uc.FetchUnreadFeedsListCursor(ctx, nil, 10, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if hasMore {
			t.Fatal("hasMore = true, want false")
		}
		if calls != 1 {
			t.Fatalf("calls = %d, want 1", calls)
		}
		if len(feeds) != 2 {
			t.Fatalf("len(feeds) = %d, want 2", len(feeds))
		}
	})
}

func makeKeysetQueryFn(dataset []*domain.FeedItem, onCall func(limit int)) func(context.Context, *time.Time, *uuid.UUID, int, []uuid.UUID) ([]*domain.FeedItem, error) {
	return func(_ context.Context, cursor *time.Time, cursorID *uuid.UUID, limit int, _ []uuid.UUID) ([]*domain.FeedItem, error) {
		if onCall != nil {
			onCall(limit)
		}
		var out []*domain.FeedItem
		for _, item := range dataset {
			switch {
			case cursor == nil:
				out = append(out, item)
			case cursorID == nil:
				if item.PublishedParsed.Before(*cursor) {
					out = append(out, item)
				}
			default:
				if item.PublishedParsed.Before(*cursor) || (item.PublishedParsed.Equal(*cursor) && item.FeedID.String() < cursorID.String()) {
					out = append(out, item)
				}
			}
			if len(out) == limit {
				break
			}
		}
		return out, nil
	}
}

func TestFetchUnreadFeedsListCursor_RefillsPast3000ConsecutiveHiddenRows(t *testing.T) {
	ctx := newTestContext(t)
	base := time.Date(2026, 11, 18, 20, 0, 0, 0, time.UTC)

	const hiddenCount = 3000
	const visibleCount = 25
	dataset := make([]*domain.FeedItem, 0, hiddenCount+visibleCount)
	for i := 0; i < hiddenCount; i++ {
		dataset = append(dataset, &domain.FeedItem{
			FeedID:          uuid.New(),
			Title:           fmt.Sprintf("hidden-%d", i),
			PublishedParsed: base.Add(-time.Duration(i) * time.Millisecond),
			DailyRank:       intPtr(10),
			DailyCap:        intPtr(2),
		})
	}
	for i := 0; i < visibleCount; i++ {
		dataset = append(dataset, &domain.FeedItem{
			FeedID:          uuid.New(),
			Title:           fmt.Sprintf("visible-%d", i),
			PublishedParsed: base.Add(-time.Duration(hiddenCount+i) * time.Millisecond),
		})
	}

	var buf bytes.Buffer
	prev := logger.Logger
	logger.Logger = slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	t.Cleanup(func() { logger.Logger = prev })

	var batchSizes []int
	uc := NewCachedFeedListUsecase(
		&legacyFeedPortStub{},
		&unreadFeedPortStub{queryFn: makeKeysetQueryFn(dataset, func(limit int) {
			batchSizes = append(batchSizes, limit)
		})},
		&allFeedPortStub{},
	)

	feeds, hasMore, err := uc.FetchUnreadFeedsListCursor(ctx, nil, 20, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hasMore {
		t.Fatal("hasMore = false, want true")
	}
	if len(feeds) != 20 {
		t.Fatalf("len(feeds) = %d, want 20", len(feeds))
	}
	for i, item := range feeds {
		want := fmt.Sprintf("visible-%d", i)
		if item.Title != want {
			t.Fatalf("feeds[%d].Title = %q, want %q", i, item.Title, want)
		}
	}
	if len(batchSizes) < 2 || batchSizes[0] != 21 || batchSizes[1] != 500 {
		t.Fatalf("batchSizes = %v, expected round 0=21 and refill rounds=500", batchSizes)
	}
	if strings.Contains(buf.String(), "unread_list.daily_cap_refill_exhausted") {
		t.Fatalf("unexpected exhaustion log: %s", buf.String())
	}
}

func TestFetchUnreadFeedsListCursor_ExhaustedBudgetLogsWarningAndReturnsCollected(t *testing.T) {
	userID := uuid.MustParse("11111111-2222-3333-4444-555555555555")
	now := time.Now()
	ctx := domain.SetUserContext(context.Background(), &domain.UserContext{
		UserID:    userID,
		Email:     "test@example.com",
		Role:      domain.UserRoleUser,
		TenantID:  userID,
		LoginAt:   now,
		ExpiresAt: now.Add(time.Hour),
	})
	base := time.Date(2026, 11, 18, 20, 0, 0, 0, time.UTC)

	const collectedBeforeRun = 2
	const hiddenRun = 11000
	const trailingVisible = 5
	dataset := make([]*domain.FeedItem, 0, collectedBeforeRun+hiddenRun+trailingVisible)
	for i := 0; i < collectedBeforeRun; i++ {
		dataset = append(dataset, &domain.FeedItem{
			FeedID:          uuid.New(),
			Title:           fmt.Sprintf("secret-title-visible-%d", i),
			PublishedParsed: base.Add(-time.Duration(i) * time.Millisecond),
		})
	}
	for i := 0; i < hiddenRun; i++ {
		dataset = append(dataset, &domain.FeedItem{
			FeedID:          uuid.New(),
			Title:           fmt.Sprintf("secret-title-hidden-%d", i),
			PublishedParsed: base.Add(-time.Duration(collectedBeforeRun+i) * time.Millisecond),
			DailyRank:       intPtr(99),
			DailyCap:        intPtr(1),
		})
	}
	for i := 0; i < trailingVisible; i++ {
		dataset = append(dataset, &domain.FeedItem{
			FeedID:          uuid.New(),
			Title:           fmt.Sprintf("trailing-%d", i),
			PublishedParsed: base.Add(-time.Duration(collectedBeforeRun+hiddenRun+i) * time.Millisecond),
		})
	}

	var buf bytes.Buffer
	prev := logger.Logger
	logger.Logger = slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	t.Cleanup(func() { logger.Logger = prev })

	var calls int
	uc := NewCachedFeedListUsecase(
		&legacyFeedPortStub{},
		&unreadFeedPortStub{queryFn: makeKeysetQueryFn(dataset, func(_ int) {
			calls++
		})},
		&allFeedPortStub{},
	)

	const requestedLimit = 20
	feeds, hasMore, err := uc.FetchUnreadFeedsListCursor(ctx, nil, requestedLimit, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hasMore {
		t.Fatal("hasMore = true, want false when refill budget is exhausted")
	}
	if len(feeds) != collectedBeforeRun {
		t.Fatalf("len(feeds) = %d, want %d", len(feeds), collectedBeforeRun)
	}
	if calls != 21 {
		t.Fatalf("calls = %d, want 21 (round 0 + 20 refill rounds)", calls)
	}

	logOutput := buf.String()
	for _, want := range []string{
		"level=WARN",
		"unread_list.daily_cap_refill_exhausted",
		"raw_rows_scanned=10021",
		"visible_rows_collected=2",
		"requested_limit=20",
	} {
		if !strings.Contains(logOutput, want) {
			t.Fatalf("log output %q missing %q", logOutput, want)
		}
	}
	for _, forbidden := range []string{userID.String(), "secret-title"} {
		if strings.Contains(logOutput, forbidden) {
			t.Fatalf("log output leaked %q: %s", forbidden, logOutput)
		}
	}
}

func TestFetchUnreadFeedsListCursor_UncappedPathIssuesSinglePortCall(t *testing.T) {
	ctx := newTestContext(t)
	base := time.Date(2026, 11, 18, 20, 0, 0, 0, time.UTC)

	dataset := make([]*domain.FeedItem, 25)
	for i := range dataset {
		dataset[i] = &domain.FeedItem{
			FeedID:          uuid.New(),
			Title:           fmt.Sprintf("uncapped-%d", i),
			PublishedParsed: base.Add(-time.Duration(i) * time.Millisecond),
		}
	}

	var calls int
	var gotLimits []int
	uc := NewCachedFeedListUsecase(
		&legacyFeedPortStub{},
		&unreadFeedPortStub{queryFn: makeKeysetQueryFn(dataset, func(limit int) {
			calls++
			gotLimits = append(gotLimits, limit)
		})},
		&allFeedPortStub{},
	)

	feeds, hasMore, err := uc.FetchUnreadFeedsListCursor(ctx, nil, 20, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hasMore {
		t.Fatal("hasMore = false, want true")
	}
	if len(feeds) != 20 {
		t.Fatalf("len(feeds) = %d, want 20", len(feeds))
	}
	if calls != 1 || len(gotLimits) != 1 || gotLimits[0] != 21 {
		t.Fatalf("calls=%d gotLimits=%v, want 1 call with limit 21", calls, gotLimits)
	}
}
