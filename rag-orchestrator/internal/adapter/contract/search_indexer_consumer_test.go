//go:build contract

// Package contract contains Consumer-Driven Contract tests for
// rag-orchestrator → search-indexer. Authentication is established at the
// transport layer (mTLS client cert); the consumer no longer sends
// application-level auth headers.
package contract

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"rag-orchestrator/internal/adapter/rag_http"

	"github.com/pact-foundation/pact-go/v2/consumer"
	"github.com/pact-foundation/pact-go/v2/matchers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newSearchIndexerPact(t *testing.T) *consumer.V3HTTPMockProvider {
	t.Helper()
	mockProvider, err := consumer.NewV3Pact(consumer.MockHTTPProviderConfig{
		Consumer: "rag-orchestrator",
		Provider: "search-indexer",
		PactDir:  filepath.Join(pactDir),
	})
	require.NoError(t, err)
	return mockProvider
}

// TestSearchIndexerSearchContract pins the `Search()` request/response:
// - GET /v1/search
// - q, user_id query params
func TestSearchIndexerSearchContract(t *testing.T) {
	mockProvider := newSearchIndexerPact(t)

	err := mockProvider.
		AddInteraction().
		Given("search has indexed articles").
		UponReceiving("a /v1/search request from rag-orchestrator").
		WithCompleteRequest(consumer.Request{
			Method: "GET",
			Path:   matchers.String("/v1/search"),
			Query: matchers.MapMatcher{
				"q":       matchers.Like("LLM"),
				"user_id": matchers.String("rag-orchestrator-system"),
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status: 200,
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"query": matchers.Like("LLM"),
				"hits": matchers.EachLike(map[string]interface{}{
					"id":      matchers.Like("article-1"),
					"title":   matchers.Like("An LLM primer"),
					"content": matchers.Like("Some content"),
					"tags":    matchers.EachLike("ai", 1),
				}, 1),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			client := rag_http.NewSearchIndexerClient(
				fmt.Sprintf("http://%s:%d", config.Host, config.Port),
				5,
				"",
			)
			hits, err := client.Search(context.Background(), "LLM")
			if err != nil {
				return fmt.Errorf("Search failed: %w", err)
			}
			assert.NotEmpty(t, hits)
			assert.NotEmpty(t, hits[0].ID)
			return nil
		})
	require.NoError(t, err)
}

// TestSearchIndexerSearchBM25Contract pins `SearchBM25()`:
// - GET /v1/search with q + limit (no user_id — global retrieval for RAG)
func TestSearchIndexerSearchBM25Contract(t *testing.T) {
	mockProvider := newSearchIndexerPact(t)

	err := mockProvider.
		AddInteraction().
		Given("search has indexed articles").
		UponReceiving("a BM25 /v1/search request from rag-orchestrator").
		WithCompleteRequest(consumer.Request{
			Method: "GET",
			Path:   matchers.String("/v1/search"),
			Query: matchers.MapMatcher{
				"q":     matchers.Like("multi agent systems"),
				"limit": matchers.Like("10"),
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status: 200,
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"query": matchers.Like("multi agent systems"),
				"hits": matchers.EachLike(map[string]interface{}{
					"id":      matchers.Like("article-42"),
					"title":   matchers.Like("Multi-Agent Systems"),
					"content": matchers.Like("Body..."),
					"tags":    matchers.EachLike("agents", 1),
				}, 1),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			client := rag_http.NewSearchIndexerClient(
				fmt.Sprintf("http://%s:%d", config.Host, config.Port),
				5,
				"",
			)
			results, err := client.SearchBM25(context.Background(), "multi agent systems", 10)
			if err != nil {
				return fmt.Errorf("SearchBM25 failed: %w", err)
			}
			assert.NotEmpty(t, results)
			assert.NotEmpty(t, results[0].ArticleID)
			return nil
		})
	require.NoError(t, err)
}
