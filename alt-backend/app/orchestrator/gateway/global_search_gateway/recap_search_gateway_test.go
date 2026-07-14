package global_search_gateway

import (
	"alt/domain"
	"alt/mocks"
	"alt/utils/logger"
	"context"
	"errors"
	"testing"

	"go.uber.org/mock/gomock"
)

func TestRecapSearchGateway_SearchRecapsForGlobal(t *testing.T) {
	logger.InitLogger()
	ctrl := gomock.NewController(t)

	mockSearchIndexer := mocks.NewMockSearchIndexerPort(ctrl)

	gw := NewRecapSearchGateway(mockSearchIndexer)

	mockSearchIndexer.EXPECT().SearchRecapsByQuery(
		gomock.Any(), "technology", 3,
	).Return([]*domain.RecapSearchResult{
		{
			JobID:      "job-1",
			ExecutedAt: "2026-04-01T00:00:00Z",
			WindowDays: 3,
			Genre:      "Technology",
			Summary:    "Technology recap summary",
			TopTerms:   []string{"AI", "quantum"},
			Tags:       []string{"tech"},
		},
	}, int64(5), nil)

	result, err := gw.SearchRecapsForGlobal(context.Background(), "technology", 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Hits) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(result.Hits))
	}

	hit := result.Hits[0]
	if hit.ID != "job-1__Technology" {
		t.Errorf("expected composite ID 'job-1__Technology', got %q", hit.ID)
	}
	if hit.Genre != "Technology" {
		t.Errorf("expected genre 'Technology', got %q", hit.Genre)
	}
	if hit.WindowDays != 3 {
		t.Errorf("expected window_days=3, got %d", hit.WindowDays)
	}
	if result.EstimatedTotal != 5 {
		t.Errorf("expected estimated_total=5, got %d", result.EstimatedTotal)
	}
	if !result.HasMore {
		t.Error("expected has_more=true")
	}
}

func TestRecapSearchGateway_EmptyResults(t *testing.T) {
	logger.InitLogger()
	ctrl := gomock.NewController(t)

	mockSearchIndexer := mocks.NewMockSearchIndexerPort(ctrl)

	gw := NewRecapSearchGateway(mockSearchIndexer)

	mockSearchIndexer.EXPECT().SearchRecapsByQuery(
		gomock.Any(), "nonexistent", 3,
	).Return([]*domain.RecapSearchResult{}, int64(0), nil)

	result, err := gw.SearchRecapsForGlobal(context.Background(), "nonexistent", 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Hits) != 0 {
		t.Errorf("expected 0 hits, got %d", len(result.Hits))
	}
}

func TestRecapSearchGateway_SearchError(t *testing.T) {
	logger.InitLogger()
	ctrl := gomock.NewController(t)

	mockSearchIndexer := mocks.NewMockSearchIndexerPort(ctrl)

	gw := NewRecapSearchGateway(mockSearchIndexer)

	mockSearchIndexer.EXPECT().SearchRecapsByQuery(
		gomock.Any(), "tech", 3,
	).Return(nil, int64(0), errors.New("search engine down"))

	_, err := gw.SearchRecapsForGlobal(context.Background(), "tech", 3)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
