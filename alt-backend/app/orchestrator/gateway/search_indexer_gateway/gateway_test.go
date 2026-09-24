package search_indexer_gateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"alt/orchestrator/driver/search_indexer_connect"
)

func TestSearchIndexerGateway_ArticlesMapping(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"hits": [{
				"id": "art-1",
				"title": "Title 1",
				"content": "Content 1",
				"tags": ["go", "clean-arch"]
			}],
			"estimatedTotalHits": "1"
		}`))
	}))
	defer ts.Close()

	client := search_indexer_connect.NewClient(ts.URL)
	gw := NewSearchIndexerGateway(client)

	hits, err := gw.SearchArticles(context.Background(), "query", "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(hits))
	}
	if hits[0].ID != "art-1" || hits[0].Title != "Title 1" {
		t.Errorf("unexpected hit mapping: %+v", hits[0])
	}
}

func TestSearchIndexerGateway_RecapsMapping(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"hits": [{
				"jobId": "job-1",
				"executedAt": "2026-09-24T00:00:00Z",
				"windowDays": 7,
				"genre": "tech",
				"summary": "weekly recap",
				"topTerms": ["ai"],
				"tags": ["tech"],
				"bullets": ["bullet 1"]
			}],
			"estimatedTotalHits": "1"
		}`))
	}))
	defer ts.Close()

	client := search_indexer_connect.NewClient(ts.URL)
	gw := NewSearchIndexerGateway(client)

	results, err := gw.SearchRecapsByTag(context.Background(), "tech", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].JobID != "job-1" || results[0].Genre != "tech" {
		t.Errorf("unexpected recap mapping: %+v", results[0])
	}
}
