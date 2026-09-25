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

func TestMapArticleHitsToDomain(t *testing.T) {
	hits := []search_indexer_connect.ArticleHit{
		{
			ID:      "art-1",
			Title:   "Title 1",
			Content: "Content 1",
			Tags:    []string{"t1"},
		},
	}
	res := mapArticleHitsToDomain(hits)
	if len(res) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(res))
	}
	if res[0].ID != "art-1" || res[0].Title != "Title 1" || res[0].Content != "Content 1" {
		t.Errorf("unexpected mapped hit: %+v", res[0])
	}
}

func TestMapRecapHitsToDomain(t *testing.T) {
	hits := []search_indexer_connect.RecapHit{
		{
			JobID:      "job-1",
			ExecutedAt: "2026-09-24T00:00:00Z",
			WindowDays: 7,
			Genre:      "tech",
			Summary:    "summary",
			TopTerms:   []string{"term1"},
			Tags:       []string{"t1"},
			Bullets:    []string{"b1"},
		},
	}
	res := mapRecapHitsToDomain(hits)
	if len(res) != 1 {
		t.Fatalf("expected 1 result, got %d", len(res))
	}
	if res[0].JobID != "job-1" || res[0].Genre != "tech" || res[0].Summary != "summary" {
		t.Errorf("unexpected mapped recap: %+v", res[0])
	}
}
