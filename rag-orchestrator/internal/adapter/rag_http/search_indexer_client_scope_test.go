package rag_http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	rag_http "rag-orchestrator/internal/adapter/rag_http"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// search-indexer requires user_id on /v1/search to scope hits to the
// authenticated user's documents (Meilisearch user_id = "..."). Empty user_id
// must fail fast to avoid unscoped search.
func TestSearchIndexerClient_Search_SendsUserID(t *testing.T) {
	var got url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"query":"q","hits":[{"id":"a1","title":"T","content":"C","tags":["iran","oil"]}]}`))
	}))
	defer srv.Close()

	client := rag_http.NewSearchIndexerClient(srv.URL, 5, "")
	hits, err := client.Search(context.Background(), "iran oil", "user-123")
	require.NoError(t, err)

	assert.Equal(t, "user-123", got.Get("user_id"), "user_id must be sent to scope search to user's documents")
	assert.Equal(t, "iran oil", got.Get("q"))
	require.Len(t, hits, 1)
	assert.Equal(t, []string{"iran", "oil"}, hits[0].Tags)
}

func TestSearchIndexerClient_Search_EmptyUserID_Fails(t *testing.T) {
	client := rag_http.NewSearchIndexerClient("http://localhost:9999", 5, "")
	_, err := client.Search(context.Background(), "iran oil", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "user_id is required")
}

func TestSearchIndexerClient_SearchBM25_SendsUserID(t *testing.T) {
	var got url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"query":"q","hits":[{"id":"a1","title":"T","content":"C"}]}`))
	}))
	defer srv.Close()

	client := rag_http.NewSearchIndexerClient(srv.URL, 5, "")
	results, err := client.SearchBM25(context.Background(), "iran oil", 10, "user-123")
	require.NoError(t, err)

	assert.Equal(t, "user-123", got.Get("user_id"), "user_id must be sent to scope BM25 search")
	assert.Equal(t, "iran oil", got.Get("q"))
	assert.Equal(t, "10", got.Get("limit"))
	require.Len(t, results, 1)
}

func TestSearchIndexerClient_SearchBM25_EmptyUserID_Fails(t *testing.T) {
	client := rag_http.NewSearchIndexerClient("http://localhost:9999", 5, "")
	_, err := client.SearchBM25(context.Background(), "iran oil", 10, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "user_id is required")
}
