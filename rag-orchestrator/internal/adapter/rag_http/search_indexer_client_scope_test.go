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

// search-indexer switches engines on user_id: present means "filter to that
// user's documents" (Meilisearch `user_id = "..."`), absent means the
// unfiltered internal RAG path. Search used to pass the literal
// "rag-orchestrator-system", which owns no document, so every tag/date/related
// tool call came back with zero hits — visible in production as
// tag_search_completed hits_found=0 tags_extracted=0 on every request.
func TestSearchIndexerClient_Search_DoesNotScopeToSyntheticUser(t *testing.T) {
	var got url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"query":"q","hits":[{"id":"a1","title":"T","content":"C","tags":["iran","oil"]}]}`))
	}))
	defer srv.Close()

	client := rag_http.NewSearchIndexerClient(srv.URL, 5, "")
	hits, err := client.Search(context.Background(), "iran oil")
	require.NoError(t, err)

	assert.Empty(t, got.Get("user_id"),
		"a synthetic user_id turns this into a user-scoped search that matches nothing")
	assert.Equal(t, "iran oil", got.Get("q"))
	require.Len(t, hits, 1)
	assert.Equal(t, []string{"iran", "oil"}, hits[0].Tags)
}
