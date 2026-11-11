package fetch_tag_cloud_usecase

import (
	"alt/domain"
	"alt/utils/logger"
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// mockFetchTagCloudPort implements fetch_tag_cloud_port.FetchTagCloudPort for testing.
type mockFetchTagCloudPort struct {
	items         []*domain.TagCloudItem
	err           error
	cooccurrences []*domain.TagCooccurrence
	cooccErr      error
	callCount     atomic.Int32
	fetchFn       func(ctx context.Context, limit int) ([]*domain.TagCloudItem, error)
}

func (m *mockFetchTagCloudPort) FetchTagCloud(ctx context.Context, limit int) ([]*domain.TagCloudItem, error) {
	m.callCount.Add(1)
	if m.fetchFn != nil {
		return m.fetchFn(ctx, limit)
	}
	// Return deep copies to simulate real DB behavior
	if m.items == nil {
		return nil, m.err
	}
	copies := make([]*domain.TagCloudItem, len(m.items))
	for i, item := range m.items {
		cp := *item
		copies[i] = &cp
	}
	return copies, m.err
}

func (m *mockFetchTagCloudPort) FetchTagCooccurrences(_ context.Context, _ []string) ([]*domain.TagCooccurrence, error) {
	return m.cooccurrences, m.cooccErr
}

func TestFetchTagCloudUsecase_Execute(t *testing.T) {
	logger.InitLogger()

	ctx := context.Background()

	mockItems := []*domain.TagCloudItem{
		{TagName: "AI", ArticleCount: 142},
		{TagName: "Rust", ArticleCount: 87},
		{TagName: "Go", ArticleCount: 65},
	}

	tests := []struct {
		name      string
		limit     int
		mockItems []*domain.TagCloudItem
		mockErr   error
		wantCount int
		wantErr   bool
	}{
		{
			name:      "success with default limit",
			limit:     0,
			mockItems: mockItems,
			wantCount: 3,
		},
		{
			name:      "success with custom limit",
			limit:     100,
			mockItems: mockItems,
			wantCount: 3,
		},
		{
			name:    "limit exceeds max returns error",
			limit:   501,
			wantErr: true,
		},
		{
			name:      "negative limit uses default",
			limit:     -1,
			mockItems: mockItems,
			wantCount: 3,
		},
		{
			name:    "port returns error",
			limit:   100,
			mockErr: errors.New("database error"),
			wantErr: true,
		},
		{
			name:      "empty result",
			limit:     100,
			mockItems: nil,
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			port := &mockFetchTagCloudPort{
				items: tt.mockItems,
				err:   tt.mockErr,
			}

			usecase := NewFetchTagCloudUsecase(port, 30*time.Minute)
			got, _, err := usecase.Execute(ctx, tt.limit)

			if (err != nil) != tt.wantErr {
				t.Errorf("Execute() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && len(got) != tt.wantCount {
				t.Errorf("Execute() got %d items, want %d", len(got), tt.wantCount)
			}
		})
	}
}

func TestFetchTagCloudUsecase_Cache(t *testing.T) {
	logger.InitLogger()
	ctx := context.Background()

	t.Run("second call uses cache", func(t *testing.T) {
		port := &mockFetchTagCloudPort{
			items: []*domain.TagCloudItem{
				{TagName: "AI", ArticleCount: 100},
				{TagName: "Go", ArticleCount: 50},
			},
		}

		usecase := NewFetchTagCloudUsecase(port, 30*time.Minute)

		// First call: fetches from port
		got1, _, err := usecase.Execute(ctx, 300)
		if err != nil {
			t.Fatalf("first call: %v", err)
		}
		if len(got1) != 2 {
			t.Fatalf("first call: want 2 items, got %d", len(got1))
		}
		if port.callCount.Load() != 1 {
			t.Errorf("first call: want 1 port call, got %d", port.callCount.Load())
		}

		// Second call: should use cache (same limit)
		got2, _, err := usecase.Execute(ctx, 300)
		if err != nil {
			t.Fatalf("second call: %v", err)
		}
		if len(got2) != 2 {
			t.Fatalf("second call: want 2 items, got %d", len(got2))
		}
		if port.callCount.Load() != 1 {
			t.Errorf("second call: port should not be called again, got %d calls", port.callCount.Load())
		}
	})

	t.Run("cache returns deep copy", func(t *testing.T) {
		port := &mockFetchTagCloudPort{
			items: []*domain.TagCloudItem{
				{TagName: "AI", ArticleCount: 100},
				{TagName: "Go", ArticleCount: 80},
				{TagName: "Rust", ArticleCount: 60},
				{TagName: "Wasm", ArticleCount: 40},
				{TagName: "s1", ArticleCount: 30},
				{TagName: "s2", ArticleCount: 30},
				{TagName: "s3", ArticleCount: 30},
			},
			cooccurrences: []*domain.TagCooccurrence{
				{TagNameA: "AI", TagNameB: "Go", SharedCount: 30},
				{TagNameA: "Go", TagNameB: "Rust", SharedCount: 25},
				{TagNameA: "Rust", TagNameB: "Wasm", SharedCount: 20},
				{TagNameA: "AI", TagNameB: "Wasm", SharedCount: 15},
				{TagNameA: "s1", TagNameB: "s2", SharedCount: 20},
				{TagNameA: "s2", TagNameB: "s3", SharedCount: 20},
				{TagNameA: "s1", TagNameB: "s3", SharedCount: 20},
			},
		}

		usecase := NewFetchTagCloudUsecase(port, 30*time.Minute)

		got1, edges1, _ := usecase.Execute(ctx, 300)
		got1[0].ArticleCount = 999 // Mutate returned copy
		got1[0].ClusterID = 99
		edges1[0].Weight = 0.01

		got2, edges2, _ := usecase.Execute(ctx, 300)
		if got2[0].ArticleCount == 999 || got2[0].ClusterID != 1 {
			t.Errorf("cache should return deep copy of items, got %+v", got2[0])
		}
		if edges2[0].Weight != 1.0 {
			t.Errorf("cache should return deep copy of edges, got %+v", edges2[0])
		}
	})

	t.Run("different limit bypasses cache", func(t *testing.T) {
		port := &mockFetchTagCloudPort{
			items: []*domain.TagCloudItem{
				{TagName: "AI", ArticleCount: 100},
			},
		}

		usecase := NewFetchTagCloudUsecase(port, 30*time.Minute)

		usecase.Execute(ctx, 300)
		usecase.Execute(ctx, 100) // Different limit

		if port.callCount.Load() != 2 {
			t.Errorf("different limit should bypass cache, got %d port calls", port.callCount.Load())
		}
	})

	t.Run("expired cache refetches", func(t *testing.T) {
		currentTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		port := &mockFetchTagCloudPort{
			items: []*domain.TagCloudItem{
				{TagName: "AI", ArticleCount: 100},
			},
		}

		usecase := NewFetchTagCloudUsecase(port, 1*time.Millisecond)
		usecase.maxStale = 1 * time.Millisecond
		usecase.now = func() time.Time { return currentTime }

		usecase.Execute(ctx, 300)
		currentTime = currentTime.Add(5 * time.Millisecond)
		usecase.Execute(ctx, 300)

		if port.callCount.Load() != 2 {
			t.Errorf("expired cache should refetch, got %d port calls", port.callCount.Load())
		}
	})
}

func TestFetchTagCloudUsecase_StaleWhileRevalidate(t *testing.T) {
	logger.InitLogger()
	ctx := context.Background()

	t.Run("stale read returns old items immediately and refreshes in background", func(t *testing.T) {
		var nowMu sync.Mutex
		currentTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

		bgStarted := make(chan struct{}, 1)
		releaseBg := make(chan struct{})
		cacheUpdated := make(chan struct{}, 1)
		var notifyOnSetCache atomic.Bool

		var callNum atomic.Int32
		port := &mockFetchTagCloudPort{
			fetchFn: func(_ context.Context, _ int) ([]*domain.TagCloudItem, error) {
				n := callNum.Add(1)
				if n == 1 {
					return []*domain.TagCloudItem{
						{TagName: "AI", ArticleCount: 100},
					}, nil
				}
				bgStarted <- struct{}{}
				<-releaseBg
				notifyOnSetCache.Store(true)
				return []*domain.TagCloudItem{
					{TagName: "AI", ArticleCount: 250},
				}, nil
			},
		}

		usecase := NewFetchTagCloudUsecase(port, 30*time.Minute)
		usecase.now = func() time.Time {
			nowMu.Lock()
			now := currentTime
			nowMu.Unlock()
			if notifyOnSetCache.Load() {
				select {
				case cacheUpdated <- struct{}{}:
				default:
				}
			}
			return now
		}

		// Cold read populates cache
		got1, _, err := usecase.Execute(ctx, 300)
		if err != nil {
			t.Fatalf("initial Execute: %v", err)
		}
		if len(got1) != 1 || got1[0].ArticleCount != 100 {
			t.Fatalf("initial Execute: want ArticleCount=100, got %+v", got1)
		}

		// Advance into stale window (30m < 31m <= 24h)
		nowMu.Lock()
		currentTime = currentTime.Add(31 * time.Minute)
		nowMu.Unlock()

		// Stale read must return old items immediately while background refresh is blocked on releaseBg
		got2, _, err := usecase.Execute(ctx, 300)
		if err != nil {
			t.Fatalf("stale Execute: %v", err)
		}
		if len(got2) != 1 || got2[0].ArticleCount != 100 {
			t.Fatalf("stale Execute: want old ArticleCount=100, got %+v", got2)
		}

		select {
		case <-bgStarted:
		case <-time.After(200 * time.Millisecond):
			t.Fatal("expected background FetchTagCloud to start on stale read")
		}

		close(releaseBg)

		select {
		case <-cacheUpdated:
		case <-time.After(time.Second):
			t.Fatal("expected background refresh to update cache")
		}
		notifyOnSetCache.Store(false)

		if got := port.callCount.Load(); got != 2 {
			t.Fatalf("want 2 FetchTagCloud calls (1 initial + 1 background), got %d", got)
		}

		// Subsequent read returns the refreshed items from cache without another port call
		got3, _, err := usecase.Execute(ctx, 300)
		if err != nil {
			t.Fatalf("post-refresh Execute: %v", err)
		}
		if len(got3) != 1 || got3[0].ArticleCount != 250 {
			t.Fatalf("post-refresh Execute: want new ArticleCount=250, got %+v", got3)
		}
		if got := port.callCount.Load(); got != 2 {
			t.Errorf("post-refresh Execute should hit fresh cache, got %d port calls", got)
		}
	})

	t.Run("two stale reads during in-flight refresh trigger only one background FetchTagCloud", func(t *testing.T) {
		var nowMu sync.Mutex
		currentTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

		bgStarted := make(chan struct{}, 2)
		releaseBg := make(chan struct{})
		cacheUpdated := make(chan struct{}, 1)
		var notifyOnSetCache atomic.Bool

		var callNum atomic.Int32
		port := &mockFetchTagCloudPort{
			fetchFn: func(_ context.Context, _ int) ([]*domain.TagCloudItem, error) {
				n := callNum.Add(1)
				if n == 1 {
					return []*domain.TagCloudItem{
						{TagName: "AI", ArticleCount: 100},
					}, nil
				}
				bgStarted <- struct{}{}
				<-releaseBg
				notifyOnSetCache.Store(true)
				return []*domain.TagCloudItem{
					{TagName: "AI", ArticleCount: 200},
				}, nil
			},
		}

		usecase := NewFetchTagCloudUsecase(port, 30*time.Minute)
		usecase.now = func() time.Time {
			nowMu.Lock()
			now := currentTime
			nowMu.Unlock()
			if notifyOnSetCache.Load() {
				select {
				case cacheUpdated <- struct{}{}:
				default:
				}
			}
			return now
		}

		if _, _, err := usecase.Execute(ctx, 300); err != nil {
			t.Fatalf("initial Execute: %v", err)
		}

		nowMu.Lock()
		currentTime = currentTime.Add(31 * time.Minute)
		nowMu.Unlock()

		// First stale read starts background refresh
		got1, _, err := usecase.Execute(ctx, 300)
		if err != nil {
			t.Fatalf("first stale Execute: %v", err)
		}
		if len(got1) != 1 || got1[0].ArticleCount != 100 {
			t.Fatalf("first stale Execute: want 100, got %+v", got1)
		}

		select {
		case <-bgStarted:
		case <-time.After(200 * time.Millisecond):
			t.Fatal("expected background refresh to start")
		}

		// Second stale read while refresh is still blocked in flight
		got2, _, err := usecase.Execute(ctx, 300)
		if err != nil {
			t.Fatalf("second stale Execute: %v", err)
		}
		if len(got2) != 1 || got2[0].ArticleCount != 100 {
			t.Fatalf("second stale Execute: want 100, got %+v", got2)
		}

		close(releaseBg)

		select {
		case <-cacheUpdated:
		case <-time.After(time.Second):
			t.Fatal("expected background refresh to complete")
		}
		notifyOnSetCache.Store(false)

		if got := port.callCount.Load(); got != 2 {
			t.Errorf("want exactly 2 FetchTagCloud calls (1 initial + 1 background), got %d", got)
		}
	})

	t.Run("failed background refresh keeps serving stale items", func(t *testing.T) {
		var nowMu sync.Mutex
		currentTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

		bgFailed := make(chan struct{}, 2)
		var callNum atomic.Int32
		port := &mockFetchTagCloudPort{
			fetchFn: func(_ context.Context, _ int) ([]*domain.TagCloudItem, error) {
				n := callNum.Add(1)
				if n == 1 {
					return []*domain.TagCloudItem{
						{TagName: "AI", ArticleCount: 100},
					}, nil
				}
				defer func() {
					select {
					case bgFailed <- struct{}{}:
					default:
					}
				}()
				return nil, errors.New("background rpc error")
			},
		}

		usecase := NewFetchTagCloudUsecase(port, 30*time.Minute)
		usecase.now = func() time.Time {
			nowMu.Lock()
			defer nowMu.Unlock()
			return currentTime
		}

		if _, _, err := usecase.Execute(ctx, 300); err != nil {
			t.Fatalf("initial Execute: %v", err)
		}

		nowMu.Lock()
		currentTime = currentTime.Add(31 * time.Minute)
		nowMu.Unlock()

		got1, _, err := usecase.Execute(ctx, 300)
		if err != nil {
			t.Fatalf("stale Execute: %v", err)
		}
		if len(got1) != 1 || got1[0].ArticleCount != 100 {
			t.Fatalf("stale Execute: want 100, got %+v", got1)
		}

		select {
		case <-bgFailed:
		case <-time.After(200 * time.Millisecond):
			t.Fatal("expected background refresh to run and fail")
		}

		// Subsequent read still serves the stale items
		got2, _, err := usecase.Execute(ctx, 300)
		if err != nil {
			t.Fatalf("Execute after failed refresh returned error: %v", err)
		}
		if len(got2) != 1 || got2[0].ArticleCount != 100 {
			t.Fatalf("Execute after failed refresh: want stale ArticleCount=100, got %+v", got2)
		}
	})

	t.Run("entries older than maxStale are recomputed synchronously", func(t *testing.T) {
		var nowMu sync.Mutex
		currentTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

		var callNum atomic.Int32
		port := &mockFetchTagCloudPort{
			fetchFn: func(_ context.Context, _ int) ([]*domain.TagCloudItem, error) {
				n := callNum.Add(1)
				if n == 1 {
					return []*domain.TagCloudItem{
						{TagName: "AI", ArticleCount: 100},
					}, nil
				}
				return []*domain.TagCloudItem{
					{TagName: "AI", ArticleCount: 300},
				}, nil
			},
		}

		usecase := NewFetchTagCloudUsecase(port, 30*time.Minute)
		usecase.now = func() time.Time {
			nowMu.Lock()
			defer nowMu.Unlock()
			return currentTime
		}

		if _, _, err := usecase.Execute(ctx, 300); err != nil {
			t.Fatalf("initial Execute: %v", err)
		}

		// Advance past maxStale (24h)
		nowMu.Lock()
		currentTime = currentTime.Add(24*time.Hour + time.Minute)
		nowMu.Unlock()

		got, _, err := usecase.Execute(ctx, 300)
		if err != nil {
			t.Fatalf("expired Execute: %v", err)
		}
		if len(got) != 1 || got[0].ArticleCount != 300 {
			t.Fatalf("expired Execute: want synchronous recompute with ArticleCount=300, got %+v", got)
		}
		if gotCalls := port.callCount.Load(); gotCalls != 2 {
			t.Errorf("want 2 synchronous FetchTagCloud calls, got %d", gotCalls)
		}
	})

	t.Run("10 concurrent cold Execute calls coalesce into one FetchTagCloud and return independent copies", func(t *testing.T) {
		const callers = 10

		enteredNow := make(chan struct{}, callers*2)
		enteredFetch := make(chan struct{}, callers)
		releaseFetch := make(chan struct{})

		port := &mockFetchTagCloudPort{
			fetchFn: func(_ context.Context, _ int) ([]*domain.TagCloudItem, error) {
				enteredFetch <- struct{}{}
				<-releaseFetch
				return []*domain.TagCloudItem{
					{TagName: "AI", ArticleCount: 100},
					{TagName: "Go", ArticleCount: 50},
				}, nil
			},
		}

		usecase := NewFetchTagCloudUsecase(port, 30*time.Minute)
		usecase.now = func() time.Time {
			enteredNow <- struct{}{}
			return time.Now()
		}

		var startReady sync.WaitGroup
		startReady.Add(callers)
		startGate := make(chan struct{})

		results := make([][]*domain.TagCloudItem, callers)
		errs := make([]error, callers)
		var doneWg sync.WaitGroup
		doneWg.Add(callers)

		for i := range callers {
			go func(idx int) {
				defer doneWg.Done()
				startReady.Done()
				<-startGate
				results[idx], _, errs[idx] = usecase.Execute(ctx, 300)
			}(i)
		}

		startReady.Wait()
		close(startGate)

		// Wait until either all 10 callers checked the cache via now() and 1 entered FetchTagCloud (coalesced),
		// or all 10 entered FetchTagCloud (uncoalesced).
		nowCount := 0
		fetchCount := 0
		for (fetchCount < 1 || nowCount < callers) && fetchCount < callers {
			select {
			case <-enteredNow:
				nowCount++
			case <-enteredFetch:
				fetchCount++
			case <-time.After(time.Second):
				t.Fatalf("timed out waiting for concurrent callers (nowCount=%d, fetchCount=%d)", nowCount, fetchCount)
			}
		}
		close(releaseFetch)
		doneWg.Wait()

		if got := port.callCount.Load(); got != 1 {
			t.Fatalf("want exactly 1 FetchTagCloud call for %d concurrent cold callers, got %d", callers, got)
		}

		for i := range callers {
			if errs[i] != nil {
				t.Fatalf("caller %d error: %v", i, errs[i])
			}
			if len(results[i]) != 2 || results[i][0].ArticleCount != 100 || results[i][1].ArticleCount != 50 {
				t.Fatalf("caller %d got unexpected items: %+v", i, results[i])
			}
		}

		// Mutate caller 0's copy and verify all other callers' copies are unaffected
		results[0][0].ArticleCount = 999
		for i := 1; i < callers; i++ {
			if results[i][0].ArticleCount == 999 || results[i][0] == results[0][0] {
				t.Fatalf("caller %d shares mutable item pointer with caller 0", i)
			}
		}
	})

	t.Run("background refresh completes when request context is cancelled right after Execute returns", func(t *testing.T) {
		var nowMu sync.Mutex
		currentTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

		bgStarted := make(chan struct{}, 1)
		reqCancelled := make(chan struct{})
		cacheUpdated := make(chan struct{}, 1)
		var notifyOnSetCache atomic.Bool

		var callNum atomic.Int32
		port := &mockFetchTagCloudPort{
			fetchFn: func(fetchCtx context.Context, _ int) ([]*domain.TagCloudItem, error) {
				n := callNum.Add(1)
				if n == 1 {
					return []*domain.TagCloudItem{
						{TagName: "AI", ArticleCount: 100},
					}, nil
				}
				bgStarted <- struct{}{}
				<-reqCancelled
				if err := fetchCtx.Err(); err != nil {
					return nil, err
				}
				notifyOnSetCache.Store(true)
				return []*domain.TagCloudItem{
					{TagName: "AI", ArticleCount: 400},
				}, nil
			},
		}

		usecase := NewFetchTagCloudUsecase(port, 30*time.Minute)
		usecase.now = func() time.Time {
			nowMu.Lock()
			now := currentTime
			nowMu.Unlock()
			if notifyOnSetCache.Load() {
				select {
				case cacheUpdated <- struct{}{}:
				default:
				}
			}
			return now
		}

		if _, _, err := usecase.Execute(ctx, 300); err != nil {
			t.Fatalf("initial Execute: %v", err)
		}

		nowMu.Lock()
		currentTime = currentTime.Add(31 * time.Minute)
		nowMu.Unlock()

		reqCtx, reqCancel := context.WithCancel(ctx)
		got1, _, err := usecase.Execute(reqCtx, 300)
		reqCancel()
		close(reqCancelled)

		if err != nil {
			t.Fatalf("stale Execute: %v", err)
		}
		if len(got1) != 1 || got1[0].ArticleCount != 100 {
			t.Fatalf("stale Execute: want 100, got %+v", got1)
		}

		select {
		case <-bgStarted:
		case <-time.After(200 * time.Millisecond):
			t.Fatal("expected background refresh to start")
		}

		select {
		case <-cacheUpdated:
		case <-time.After(time.Second):
			t.Fatal("expected background refresh to complete despite cancelled request context")
		}
		notifyOnSetCache.Store(false)

		got2, _, err := usecase.Execute(ctx, 300)
		if err != nil {
			t.Fatalf("post-refresh Execute: %v", err)
		}
		if len(got2) != 1 || got2[0].ArticleCount != 400 {
			t.Fatalf("post-refresh Execute: want ArticleCount=400, got %+v", got2)
		}
	})

	t.Run("coalesced cold Execute succeeds for all callers when first caller cancels context", func(t *testing.T) {
		leaderInFetch := make(chan struct{})
		secondCheckedCache := make(chan struct{}, 2)
		releaseFetch := make(chan struct{})

		var fetchCalls atomic.Int32
		port := &mockFetchTagCloudPort{
			fetchFn: func(fetchCtx context.Context, _ int) ([]*domain.TagCloudItem, error) {
				if fetchCalls.Add(1) == 1 {
					close(leaderInFetch)
				}
				<-releaseFetch
				if err := fetchCtx.Err(); err != nil {
					return nil, err
				}
				return []*domain.TagCloudItem{
					{TagName: "AI", ArticleCount: 100},
				}, nil
			},
		}

		usecase := NewFetchTagCloudUsecase(port, 30*time.Minute)
		var nowCalls atomic.Int32
		usecase.now = func() time.Time {
			if nowCalls.Add(1) == 2 {
				secondCheckedCache <- struct{}{}
			}
			return time.Now()
		}

		ctx1, cancel1 := context.WithCancel(ctx)
		defer cancel1()

		var (
			got1, got2 []*domain.TagCloudItem
			err1, err2 error
			wg         sync.WaitGroup
		)
		wg.Add(2)

		go func() {
			defer wg.Done()
			got1, _, err1 = usecase.Execute(ctx1, 300)
		}()

		select {
		case <-leaderInFetch:
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for first caller to enter FetchTagCloud")
		}

		go func() {
			defer wg.Done()
			got2, _, err2 = usecase.Execute(ctx, 300)
		}()

		select {
		case <-secondCheckedCache:
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for second caller to check cache")
		}

		// Cancel the first caller's context before releasing the port
		cancel1()
		close(releaseFetch)
		wg.Wait()

		if err1 != nil {
			t.Fatalf("first caller Execute error: %v", err1)
		}
		if err2 != nil {
			t.Fatalf("second caller Execute error: %v", err2)
		}
		if len(got1) != 1 || got1[0].ArticleCount != 100 {
			t.Fatalf("first caller got %+v, want ArticleCount=100", got1)
		}
		if len(got2) != 1 || got2[0].ArticleCount != 100 {
			t.Fatalf("second caller got %+v, want ArticleCount=100", got2)
		}
		if got := port.callCount.Load(); got != 1 {
			t.Errorf("want 1 FetchTagCloud call, got %d", got)
		}
	})

	t.Run("getCached in-flight guard and refreshInBackground error reset", func(t *testing.T) {
		currentTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		port := &mockFetchTagCloudPort{
			items: []*domain.TagCloudItem{
				{TagName: "AI", ArticleCount: 100},
			},
		}

		usecase := NewFetchTagCloudUsecase(port, 30*time.Minute)
		usecase.now = func() time.Time { return currentTime }

		if _, _, err := usecase.Execute(ctx, 300); err != nil {
			t.Fatalf("initial Execute: %v", err)
		}

		// Advance into stale window
		currentTime = currentTime.Add(31 * time.Minute)

		items1, _, state1, start1 := usecase.getCached(300)
		if len(items1) != 1 || state1 != cacheStateStale || !start1 {
			t.Fatalf("first stale getCached = (%v, %q, %v), want (len 1, %q, true)", items1, state1, start1, cacheStateStale)
		}

		items2, _, state2, start2 := usecase.getCached(300)
		if len(items2) != 1 || state2 != cacheStateStale || start2 {
			t.Fatalf("second stale getCached = (%v, %q, %v), want (len 1, %q, false)", items2, state2, start2, cacheStateStale)
		}

		// Failed synchronous refreshInBackground must reset u.refreshing to false
		port.items = nil
		port.err = errors.New("upstream failure")
		usecase.mu.Lock()
		usecase.refreshing = true
		usecase.mu.Unlock()

		usecase.refreshInBackground(ctx, 300)

		usecase.mu.Lock()
		refreshing := usecase.refreshing
		usecase.mu.Unlock()
		if refreshing {
			t.Fatal("expected u.refreshing to be false after failed refreshInBackground")
		}
	})

	t.Run("refreshInBackground recovers panic and resets refreshing flag", func(t *testing.T) {
		port := &mockFetchTagCloudPort{
			fetchFn: func(_ context.Context, _ int) ([]*domain.TagCloudItem, error) {
				panic("unexpected driver panic")
			},
		}

		usecase := NewFetchTagCloudUsecase(port, 30*time.Minute)
		usecase.mu.Lock()
		usecase.refreshing = true
		usecase.mu.Unlock()

		usecase.refreshInBackground(ctx, 300)

		usecase.mu.Lock()
		refreshing := usecase.refreshing
		usecase.mu.Unlock()
		if refreshing {
			t.Fatal("expected u.refreshing to be false after panic in refreshInBackground")
		}
	})

	t.Run("stale-while-revalidate preserves and updates clusters and edges", func(t *testing.T) {
		var nowMu sync.Mutex
		currentTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

		bgStarted := make(chan struct{}, 1)
		releaseBg := make(chan struct{})
		cacheUpdated := make(chan struct{}, 1)
		var notifyOnSetCache atomic.Bool

		var callNum atomic.Int32
		port := &mockFetchTagCloudPort{
			fetchFn: func(_ context.Context, _ int) ([]*domain.TagCloudItem, error) {
				n := callNum.Add(1)
				if n == 1 {
					return []*domain.TagCloudItem{
						{TagName: "t1", ArticleCount: 100},
						{TagName: "t2", ArticleCount: 90},
						{TagName: "t3", ArticleCount: 80},
						{TagName: "t4", ArticleCount: 70},
						{TagName: "s1", ArticleCount: 30},
						{TagName: "s2", ArticleCount: 30},
						{TagName: "s3", ArticleCount: 30},
					}, nil
				}
				bgStarted <- struct{}{}
				<-releaseBg
				notifyOnSetCache.Store(true)
				return []*domain.TagCloudItem{
					{TagName: "t1", ArticleCount: 100},
					{TagName: "t2", ArticleCount: 90},
					{TagName: "t3", ArticleCount: 80},
					{TagName: "t4", ArticleCount: 70},
					{TagName: "t5", ArticleCount: 60},
					{TagName: "s1", ArticleCount: 30},
					{TagName: "s2", ArticleCount: 30},
					{TagName: "s3", ArticleCount: 30},
				}, nil
			},
			cooccurrences: []*domain.TagCooccurrence{
				{TagNameA: "t1", TagNameB: "t2", SharedCount: 40},
				{TagNameA: "t2", TagNameB: "t3", SharedCount: 35},
				{TagNameA: "t3", TagNameB: "t4", SharedCount: 30},
				{TagNameA: "t1", TagNameB: "t4", SharedCount: 25},
				{TagNameA: "t4", TagNameB: "t5", SharedCount: 20},
				{TagNameA: "s1", TagNameB: "s2", SharedCount: 25},
				{TagNameA: "s2", TagNameB: "s3", SharedCount: 25},
				{TagNameA: "s1", TagNameB: "s3", SharedCount: 25},
			},
		}

		usecase := NewFetchTagCloudUsecase(port, 30*time.Minute)
		usecase.now = func() time.Time {
			nowMu.Lock()
			now := currentTime
			nowMu.Unlock()
			if notifyOnSetCache.Load() {
				select {
				case cacheUpdated <- struct{}{}:
				default:
				}
			}
			return now
		}

		items1, edges1, err := usecase.Execute(ctx, 300)
		if err != nil {
			t.Fatalf("initial Execute: %v", err)
		}
		if len(items1) != 7 || items1[0].ClusterID != 1 || len(edges1) != 7 {
			t.Fatalf("initial Execute: want 7 items (t1 in cluster 1) and 7 edges, got items=%+v edges=%+v", items1, edges1)
		}

		nowMu.Lock()
		currentTime = currentTime.Add(31 * time.Minute)
		nowMu.Unlock()

		staleItems, staleEdges, err := usecase.Execute(ctx, 300)
		if err != nil {
			t.Fatalf("stale Execute: %v", err)
		}
		if len(staleItems) != 7 || staleItems[0].ClusterID != 1 || len(staleEdges) != 7 {
			t.Fatalf("stale Execute: want cached 7 items/edges, got items=%+v edges=%+v", staleItems, staleEdges)
		}

		select {
		case <-bgStarted:
		case <-time.After(200 * time.Millisecond):
			t.Fatal("expected background refresh to start")
		}
		close(releaseBg)

		select {
		case <-cacheUpdated:
		case <-time.After(time.Second):
			t.Fatal("expected background refresh to update cache")
		}
		notifyOnSetCache.Store(false)

		freshItems, freshEdges, err := usecase.Execute(ctx, 300)
		if err != nil {
			t.Fatalf("post-refresh Execute: %v", err)
		}
		if len(freshItems) != 8 || len(freshEdges) != 8 {
			t.Fatalf("post-refresh Execute: want 8 items and 8 edges, got %d items and %d edges", len(freshItems), len(freshEdges))
		}
		for _, it := range freshItems[:5] {
			if it.ClusterID != 1 {
				t.Errorf("post-refresh item %q ClusterID = %d, want 1", it.TagName, it.ClusterID)
			}
		}
		for _, it := range freshItems[5:] {
			if it.ClusterID != 0 {
				t.Errorf("post-refresh small-group item %q ClusterID = %d, want 0", it.TagName, it.ClusterID)
			}
		}
	})

	t.Run("cooccurrence failure degrades to empty edges and cluster 0", func(t *testing.T) {
		port := &mockFetchTagCloudPort{
			items: []*domain.TagCloudItem{
				{TagName: "t1", ArticleCount: 100},
				{TagName: "t2", ArticleCount: 90},
				{TagName: "t3", ArticleCount: 80},
				{TagName: "t4", ArticleCount: 70},
			},
			cooccErr: errors.New("cooccurrence rpc failed"),
		}
		usecase := NewFetchTagCloudUsecase(port, 30*time.Minute)

		items, edges, err := usecase.Execute(ctx, 300)
		if err != nil {
			t.Fatalf("Execute on cooccurrence failure: %v", err)
		}
		if len(edges) != 0 {
			t.Errorf("want no edges on cooccurrence failure, got %+v", edges)
		}
		for _, it := range items {
			if it.ClusterID != 0 {
				t.Errorf("item %q ClusterID = %d, want 0 on cooccurrence failure", it.TagName, it.ClusterID)
			}
		}
	})
}

func TestFetchTagCloudUsecase_Refresh(t *testing.T) {
	logger.InitLogger()
	ctx := context.Background()

	t.Run("Refresh always recomputes even with valid cache", func(t *testing.T) {
		port := &mockFetchTagCloudPort{
			items: []*domain.TagCloudItem{
				{TagName: "AI", ArticleCount: 100},
				{TagName: "Go", ArticleCount: 50},
			},
		}

		usecase := NewFetchTagCloudUsecase(port, 30*time.Minute)

		// First call populates cache
		_, _, err := usecase.Execute(ctx, 300)
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if port.callCount.Load() != 1 {
			t.Fatalf("want 1 port call, got %d", port.callCount.Load())
		}

		// Refresh must bypass cache and recompute
		got, err := usecase.Refresh(ctx, 300)
		if err != nil {
			t.Fatalf("Refresh: %v", err)
		}
		if len(got) != 2 {
			t.Errorf("Refresh: want 2 items, got %d", len(got))
		}
		if port.callCount.Load() != 2 {
			t.Errorf("Refresh should always fetch from port, got %d calls", port.callCount.Load())
		}
	})

	t.Run("Refresh updates cache for subsequent Execute", func(t *testing.T) {
		port := &mockFetchTagCloudPort{
			items: []*domain.TagCloudItem{
				{TagName: "AI", ArticleCount: 100},
			},
		}

		usecase := NewFetchTagCloudUsecase(port, 30*time.Minute)

		// Refresh populates cache
		_, err := usecase.Refresh(ctx, 300)
		if err != nil {
			t.Fatalf("Refresh: %v", err)
		}

		// Execute should use the cache populated by Refresh
		_, _, err = usecase.Execute(ctx, 300)
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if port.callCount.Load() != 1 {
			t.Errorf("Execute after Refresh should use cache, got %d port calls", port.callCount.Load())
		}
	})
}

func TestFetchTagCloudUsecase_ExecuteCountsOnly(t *testing.T) {
	logger.InitLogger()
	ctx := context.Background()

	port := &mockFetchTagCloudPort{
		items: []*domain.TagCloudItem{
			{TagName: "AI", ArticleCount: 100},
			{TagName: "Go", ArticleCount: 50},
		},
		cooccErr: errors.New("FetchTagCooccurrences must not be called by ExecuteCountsOnly"),
	}

	usecase := NewFetchTagCloudUsecase(port, 30*time.Minute)

	got, err := usecase.ExecuteCountsOnly(ctx, 0)
	if err != nil {
		t.Fatalf("ExecuteCountsOnly: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ExecuteCountsOnly: want 2 items, got %d", len(got))
	}
	for _, item := range got {
		if item.PositionX != 0 || item.PositionY != 0 || item.PositionZ != 0 {
			t.Errorf("ExecuteCountsOnly should not compute 3D layout positions, got (%f, %f, %f)",
				item.PositionX, item.PositionY, item.PositionZ)
		}
	}

	if _, err := usecase.ExecuteCountsOnly(ctx, 501); err == nil {
		t.Error("ExecuteCountsOnly: expected error for limit > 500")
	}
}
