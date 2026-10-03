//go:build contract

// Pact consumer contract tests for alt-backend → search-indexer (Connect-RPC).
//
// search-indexer answers SearchArticles only for the user whose JWT arrives in
// X-Alt-Backend-Token, and refuses a call without one with 401. The driver's
// interceptor forwards the JWT from the request context, so the contract pins
// the header. The client certificate the production transport presents cannot
// be demanded by the Pact mock and stays with the driver tests.
package contract

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/pact-foundation/pact-go/v2/consumer"
	"github.com/pact-foundation/pact-go/v2/matchers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	searchv2 "alt/gen/proto/services/search/v2"
	"alt/orchestrator/driver/search_indexer_connect"
	"alt/shared/domain/authcontext"
)

const pactDir = "../../../../../pacts"

// searchIndexerUserJWT is a placeholder with a JWT's three-segment shape. The
// provider verification swaps it for a token minted for the request's userId,
// so only the header's presence and shape are contractual.
const searchIndexerUserJWT = "pact-header.pact-claims.pact-signature"

func newContractClient(config consumer.MockServerConfig) *search_indexer_connect.Client {
	return search_indexer_connect.NewClientWithHTTPClient(
		fmt.Sprintf("http://%s:%d", config.Host, config.Port),
		&http.Client{Timeout: 5 * time.Second},
	)
}

func newSearchIndexerPact(t *testing.T) *consumer.V3HTTPMockProvider {
	t.Helper()
	mockProvider, err := consumer.NewV3Pact(consumer.MockHTTPProviderConfig{
		Consumer: "alt-backend",
		Provider: "search-indexer",
		PactDir:  filepath.Join(pactDir),
	})
	require.NoError(t, err)
	return mockProvider
}

func TestSearchIndexerSearchArticlesContract(t *testing.T) {
	mockProvider := newSearchIndexerPact(t)

	err := mockProvider.
		AddInteraction().
		Given("a service token is configured and articles are indexed").
		UponReceiving("an authenticated SearchArticles Connect-RPC call from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method: "POST",
			Path:   matchers.String("/services.search.v2.SearchService/SearchArticles"),
			Headers: matchers.MapMatcher{
				"Content-Type":        matchers.String("application/json"),
				"X-Alt-Backend-Token": matchers.Regex(searchIndexerUserJWT, `^[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+$`),
			},
			Body: matchers.MapMatcher{
				"query":  matchers.Like("LLM"),
				"userId": matchers.Like("user-1"),
				"limit":  matchers.Like(20),
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status: 200,
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"hits": matchers.EachLike(map[string]interface{}{
					"id":      matchers.Like("article-1"),
					"title":   matchers.Like("An LLM primer"),
					"content": matchers.Like("body"),
					"tags":    matchers.EachLike("ai", 1),
				}, 1),
				"estimatedTotalHits": matchers.Like("1"),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			ctx := authcontext.WithJWT(context.Background(), searchIndexerUserJWT)
			hits, err := newContractClient(config).SearchArticles(ctx, "LLM", "user-1")
			if err != nil {
				return fmt.Errorf("SearchArticles failed: %w", err)
			}
			assert.NotEmpty(t, hits)
			assert.NotEmpty(t, hits[0].ID)
			return nil
		})
	require.NoError(t, err)
}

func TestSearchIndexerSearchRecapsByTagContract(t *testing.T) {
	mockProvider := newSearchIndexerPact(t)

	err := mockProvider.
		AddInteraction().
		Given("a service token is configured and recap jobs are indexed under a tag").
		UponReceiving("an authenticated SearchRecaps by tag Connect-RPC call from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method: "POST",
			Path:   matchers.String("/services.search.v2.SearchService/SearchRecaps"),
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"tagName": matchers.Like("technology"),
				"limit":   matchers.Like(10),
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status: 200,
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"hits": matchers.EachLike(map[string]interface{}{
					"jobId":      matchers.Like("job-1"),
					"executedAt": matchers.Like("2026-04-10T00:00:00Z"),
					"windowDays": matchers.Like(7),
					"genre":      matchers.Like("technology"),
					"summary":    matchers.Like("weekly recap"),
					"topTerms":   matchers.EachLike("ai", 1),
					"tags":       matchers.EachLike("technology", 1),
					"bullets":    matchers.EachLike("bullet", 1),
				}, 1),
				"estimatedTotalHits": matchers.Like("1"),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			results, err := newContractClient(config).SearchRecapsByTag(context.Background(), "technology", 10)
			if err != nil {
				return fmt.Errorf("SearchRecapsByTag failed: %w", err)
			}
			assert.NotEmpty(t, results)
			assert.NotEmpty(t, results[0].JobID)
			return nil
		})
	require.NoError(t, err)
}

// Keep protobuf symbol references in scope so the import is meaningful
// for future extensions (avoids unused-import churn).
var _ = (&searchv2.SearchArticlesRequest{}).Query
var _ = (&connect.Request[searchv2.SearchArticlesRequest]{}).Header
