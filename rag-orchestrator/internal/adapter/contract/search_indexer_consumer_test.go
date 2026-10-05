//go:build contract

// Package contract contains Consumer-Driven Contract tests for
// rag-orchestrator → search-indexer. search-indexer serves /v1/search only on
// its mTLS listener (:9443) and authenticates twice: the client certificate
// admits the peer, and the caller's user JWT (X-Alt-Backend-Token) must belong
// to the requested user_id or the search is refused with 401/403. The Pact
// mock cannot demand a client certificate, so the transport stays with the
// httpclient tests; the JWT header is part of the HTTP surface and is pinned
// here.
package contract

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"rag-orchestrator/internal/adapter/rag_http"
	"rag-orchestrator/internal/domain/authcontext"

	"github.com/pact-foundation/pact-go/v2/consumer"
	"github.com/pact-foundation/pact-go/v2/matchers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// searchIndexerUserJWT is a placeholder with a JWT's three-segment shape. The
// provider verification swaps it for a token minted for the interaction's
// user_id, so only the header's presence and shape are contractual.
const searchIndexerUserJWT = "pact-header.pact-claims.pact-signature"

func searchIndexerUserJWTHeader() matchers.MapMatcher {
	return matchers.MapMatcher{
		"X-Alt-Backend-Token": matchers.Regex(searchIndexerUserJWT, `^[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+$`),
	}
}

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
//   - GET /v1/search
//   - q and user_id query params; search-indexer scopes hits to the authenticated user
//   - the user's JWT in X-Alt-Backend-Token
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
				"user_id": matchers.Like("00000000-0000-0000-0000-000000000001"),
			},
			Headers: searchIndexerUserJWTHeader(),
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
				&http.Client{Timeout: 5 * time.Second},
			)
			ctx := authcontext.WithJWT(context.Background(), searchIndexerUserJWT)
			hits, err := client.Search(ctx, "LLM", "00000000-0000-0000-0000-000000000001")
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
//   - GET /v1/search with q, limit, and user_id for user-scoped BM25 hybrid search
//   - the user's JWT in X-Alt-Backend-Token
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
				"q":       matchers.Like("multi agent systems"),
				"limit":   matchers.Like("10"),
				"user_id": matchers.Like("00000000-0000-0000-0000-000000000001"),
			},
			Headers: searchIndexerUserJWTHeader(),
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
				&http.Client{Timeout: 5 * time.Second},
			)
			ctx := authcontext.WithJWT(context.Background(), searchIndexerUserJWT)
			results, err := client.SearchBM25(ctx, "multi agent systems", 10, "00000000-0000-0000-0000-000000000001")
			if err != nil {
				return fmt.Errorf("SearchBM25 failed: %w", err)
			}
			assert.NotEmpty(t, results)
			assert.NotEmpty(t, results[0].ArticleID)
			return nil
		})
	require.NoError(t, err)
}
