package rest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"search-indexer/domain"
	"search-indexer/usecase"
)

func TestHandler_SearchArticles_AcceptsPublishedAfterWithUserID(t *testing.T) {
	mock := &mockSearchEngine{}
	handler := NewHandler(
		usecase.NewSearchByUserUsecase(mock),
	)

	req := httptest.NewRequest(http.MethodGet,
		"/v1/search?q=iran&user_id=u1&published_after=2026-04-12T00:00:00Z", nil)
	rec := httptest.NewRecorder()
	handler.SearchArticles(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandler_SearchArticles_AcceptsPublishedBeforeWithUserID(t *testing.T) {
	mock := &mockSearchEngine{}
	handler := NewHandler(
		usecase.NewSearchByUserUsecase(mock),
	)

	req := httptest.NewRequest(http.MethodGet,
		"/v1/search?q=iran&user_id=u1&published_before=2026-04-20T00:00:00Z", nil)
	rec := httptest.NewRecorder()
	handler.SearchArticles(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
}

// TestHandler_SearchArticles_AcceptsDateWindowWithUserID also guards against
// a regression that silently drops the date window: it asserts the search
// engine actually received the published_after/published_before bounds
// parsed from the query string, not just that a 200 came back.
func TestHandler_SearchArticles_AcceptsDateWindowWithUserID(t *testing.T) {
	mock := &mockSearchEngine{}
	handler := NewHandler(
		usecase.NewSearchByUserUsecase(mock),
	)

	req := httptest.NewRequest(http.MethodGet,
		"/v1/search?q=iran&user_id=u1&published_after=2026-04-12T00:00:00Z&published_before=2026-04-20T00:00:00Z", nil)
	rec := httptest.NewRecorder()
	handler.SearchArticles(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}

	if mock.dateFilterCalls != 1 {
		t.Fatalf("dateFilterCalls = %d, want 1", mock.dateFilterCalls)
	}
	if mock.gotDateFilterQuery != "iran" || mock.gotDateFilterUserID != "u1" {
		t.Errorf("search engine received query=%q userID=%q, want query=%q userID=%q",
			mock.gotDateFilterQuery, mock.gotDateFilterUserID, "iran", "u1")
	}

	wantAfter := time.Date(2026, 4, 12, 0, 0, 0, 0, time.UTC)
	wantBefore := time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC)
	if mock.gotPublishedAfter == nil || !mock.gotPublishedAfter.Equal(wantAfter) {
		t.Errorf("search engine received publishedAfter = %v, want %v", mock.gotPublishedAfter, wantAfter)
	}
	if mock.gotPublishedBefore == nil || !mock.gotPublishedBefore.Equal(wantBefore) {
		t.Errorf("search engine received publishedBefore = %v, want %v", mock.gotPublishedBefore, wantBefore)
	}
}

func TestHandler_SearchArticles_RejectsInvertedDateFilter(t *testing.T) {
	mock := &mockSearchEngine{}
	handler := NewHandler(
		usecase.NewSearchByUserUsecase(mock),
	)

	req := httptest.NewRequest(http.MethodGet,
		"/v1/search?q=iran&user_id=u1&published_after=2026-04-20T00:00:00Z&published_before=2026-04-10T00:00:00Z", nil)
	rec := httptest.NewRecorder()
	handler.SearchArticles(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "published_after must not be after published_before") {
		t.Errorf("body = %q, want containing 'published_after must not be after published_before'", rec.Body.String())
	}
}

func TestHandler_SearchArticles_RejectsMalformedPublishedAfter(t *testing.T) {
	mock := &mockSearchEngine{}
	handler := NewHandler(
		usecase.NewSearchByUserUsecase(mock),
	)

	req := httptest.NewRequest(http.MethodGet,
		"/v1/search?q=iran&user_id=u1&published_after=not-a-date", nil)
	rec := httptest.NewRecorder()
	handler.SearchArticles(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for malformed published_after", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "invalid published_after (expected RFC3339)") {
		t.Errorf("body = %q, want containing 'invalid published_after (expected RFC3339)'", rec.Body.String())
	}
}

func TestHandler_SearchArticles_RejectsMalformedPublishedBefore(t *testing.T) {
	mock := &mockSearchEngine{}
	handler := NewHandler(
		usecase.NewSearchByUserUsecase(mock),
	)

	req := httptest.NewRequest(http.MethodGet,
		"/v1/search?q=iran&user_id=u1&published_before=not-a-date", nil)
	rec := httptest.NewRecorder()
	handler.SearchArticles(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for malformed published_before", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "invalid published_before (expected RFC3339)") {
		t.Errorf("body = %q, want containing 'invalid published_before (expected RFC3339)'", rec.Body.String())
	}
}

func TestHandler_SearchArticles_RejectsDateFilterWithoutUserID(t *testing.T) {
	mock := &mockSearchEngine{}
	handler := NewHandler(
		usecase.NewSearchByUserUsecase(mock),
	)

	req := httptest.NewRequest(http.MethodGet,
		"/v1/search?q=iran&published_after=2026-04-12T00:00:00Z", nil)
	rec := httptest.NewRecorder()
	handler.SearchArticles(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "user_id parameter required") {
		t.Errorf("body = %q, want containing 'user_id parameter required'", rec.Body.String())
	}
}

func TestHandler_SearchArticles_UserScopedResponseExposesPublishedAt(t *testing.T) {
	publishedAt := time.Date(2026, 4, 18, 9, 0, 0, 0, time.UTC)
	mock := &mockSearchEngine{
		searchByUserIDResult: []domain.SearchDocument{
			{ID: "a-1", Title: "t", Content: "c", Tags: []string{}, PublishedAt: publishedAt},
		},
	}
	handler := NewHandler(
		usecase.NewSearchByUserUsecase(mock),
	)

	req := httptest.NewRequest(http.MethodGet,
		"/v1/search?q=iran&user_id=u1", nil)
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
