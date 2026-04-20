package rest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"search-indexer/domain"
	"search-indexer/usecase"
)

// mockDateFilterEngine records the date arguments passed into the engine
// so the handler-level contract can be verified without booting Meilisearch.
type mockDateFilterEngine struct {
	mockSearchEngine
	gotPublishedAfter  *time.Time
	gotPublishedBefore *time.Time
	dateFilterResults  []domain.SearchDocument
	dateFilterErr      error
}

func (m *mockDateFilterEngine) SearchWithDateFilter(
	ctx context.Context,
	query string,
	publishedAfter, publishedBefore *time.Time,
	limit int,
) ([]domain.SearchDocument, error) {
	_ = ctx
	_ = query
	_ = limit
	m.gotPublishedAfter = publishedAfter
	m.gotPublishedBefore = publishedBefore
	return m.dateFilterResults, m.dateFilterErr
}

func TestHandler_SearchArticles_ForwardsPublishedAfterParam(t *testing.T) {
	publishedAt := time.Date(2026, 4, 18, 9, 0, 0, 0, time.UTC)
	engine := &mockDateFilterEngine{
		dateFilterResults: []domain.SearchDocument{
			{ID: "a-1", Title: "recent hit", PublishedAt: publishedAt},
		},
	}

	handler := NewHandler(
		usecase.NewSearchByUserUsecase(engine),
		usecase.NewSearchArticlesUsecase(engine),
	)

	req := httptest.NewRequest(http.MethodGet,
		"/v1/search?q=iran&limit=20&published_after=2026-04-12T00:00:00Z", nil)
	rec := httptest.NewRecorder()
	handler.SearchArticles(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if engine.gotPublishedAfter == nil {
		t.Fatalf("expected SearchWithDateFilter to be called with published_after")
	}
	want := time.Date(2026, 4, 12, 0, 0, 0, 0, time.UTC)
	if !engine.gotPublishedAfter.Equal(want) {
		t.Errorf("published_after = %v, want %v", engine.gotPublishedAfter, want)
	}
}

func TestHandler_SearchArticles_ForwardsPublishedBeforeParam(t *testing.T) {
	engine := &mockDateFilterEngine{
		dateFilterResults: []domain.SearchDocument{},
	}
	handler := NewHandler(
		usecase.NewSearchByUserUsecase(engine),
		usecase.NewSearchArticlesUsecase(engine),
	)

	req := httptest.NewRequest(http.MethodGet,
		"/v1/search?q=iran&limit=20&published_before=2026-04-20T00:00:00Z", nil)
	rec := httptest.NewRecorder()
	handler.SearchArticles(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if engine.gotPublishedBefore == nil {
		t.Fatalf("expected SearchWithDateFilter to be called with published_before")
	}
}

func TestHandler_SearchArticles_DateFilterResponseExposesPublishedAt(t *testing.T) {
	publishedAt := time.Date(2026, 4, 18, 9, 0, 0, 0, time.UTC)
	engine := &mockDateFilterEngine{
		dateFilterResults: []domain.SearchDocument{
			{ID: "a-1", Title: "t", Content: "c", Tags: []string{}, PublishedAt: publishedAt},
		},
	}
	handler := NewHandler(
		usecase.NewSearchByUserUsecase(engine),
		usecase.NewSearchArticlesUsecase(engine),
	)

	req := httptest.NewRequest(http.MethodGet,
		"/v1/search?q=iran&published_after=2026-04-12T00:00:00Z", nil)
	rec := httptest.NewRecorder()
	handler.SearchArticles(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var resp SearchArticlesResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Hits) != 1 {
		t.Fatalf("hits = %d, want 1", len(resp.Hits))
	}
	if resp.Hits[0].PublishedAt != publishedAt.Format(time.RFC3339) {
		t.Errorf("hit.PublishedAt = %q, want %q", resp.Hits[0].PublishedAt, publishedAt.Format(time.RFC3339))
	}
}

func TestHandler_SearchArticles_RejectsMalformedPublishedAfter(t *testing.T) {
	engine := &mockDateFilterEngine{}
	handler := NewHandler(
		usecase.NewSearchByUserUsecase(engine),
		usecase.NewSearchArticlesUsecase(engine),
	)

	req := httptest.NewRequest(http.MethodGet,
		"/v1/search?q=iran&published_after=not-a-date", nil)
	rec := httptest.NewRecorder()
	handler.SearchArticles(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for malformed published_after", rec.Code)
	}
}
