package fetch_tag_cloud_usecase

import (
	"alt/domain"
	"alt/utils/logger"
	"context"
	"errors"
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
}

func (m *mockFetchTagCloudPort) FetchTagCloud(_ context.Context, _ int) ([]*domain.TagCloudItem, error) {
	m.callCount.Add(1)
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
			got, err := usecase.Execute(ctx, tt.limit)

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
		got1, err := usecase.Execute(ctx, 300)
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
		got2, err := usecase.Execute(ctx, 300)
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
			},
		}

		usecase := NewFetchTagCloudUsecase(port, 30*time.Minute)

		got1, _ := usecase.Execute(ctx, 300)
		got1[0].ArticleCount = 999 // Mutate returned copy

		got2, _ := usecase.Execute(ctx, 300)
		if got2[0].ArticleCount == 999 {
			t.Error("cache should return deep copy, mutation leaked through")
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
		port := &mockFetchTagCloudPort{
			items: []*domain.TagCloudItem{
				{TagName: "AI", ArticleCount: 100},
			},
		}

		usecase := NewFetchTagCloudUsecase(port, 1*time.Millisecond)

		usecase.Execute(ctx, 300)
		time.Sleep(5 * time.Millisecond) // Wait for cache expiry
		usecase.Execute(ctx, 300)

		if port.callCount.Load() != 2 {
			t.Errorf("expired cache should refetch, got %d port calls", port.callCount.Load())
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
		_, err := usecase.Execute(ctx, 300)
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
		_, err = usecase.Execute(ctx, 300)
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if port.callCount.Load() != 1 {
			t.Errorf("Execute after Refresh should use cache, got %d port calls", port.callCount.Load())
		}
	})
}
