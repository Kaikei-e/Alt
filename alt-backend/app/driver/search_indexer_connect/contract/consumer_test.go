//go:build contract

// Pact consumer contract tests for alt-backend → search-indexer (Connect-RPC).
//
// These tests pin the invariant from ADR-000722: search-indexer requires an
// X-Service-Token header on every call — including Connect-RPC. The previous
// driver bypassed this because it was initialised with http.DefaultClient and
// no interceptor; this contract turns that regression into a failing test.
package contract

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"connectrpc.com/connect"
	"github.com/pact-foundation/pact-go/v2/consumer"
	"github.com/pact-foundation/pact-go/v2/matchers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"alt/driver/search_indexer_connect"
	searchv2 "alt/gen/proto/services/search/v2"
)

const pactDir = "../../../../pacts"

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
				"Content-Type": matchers.String("application/json"),
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
				"estimatedTotalHits": matchers.Like(1),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			driver := search_indexer_connect.NewConnectSearchIndexerDriver(
				fmt.Sprintf("http://%s:%d", config.Host, config.Port),
				"test-service-token",
			)
			hits, err := driver.SearchArticles(context.Background(), "LLM", "user-1")
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
				"estimatedTotalHits": matchers.Like(1),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			driver := search_indexer_connect.NewConnectSearchIndexerDriver(
				fmt.Sprintf("http://%s:%d", config.Host, config.Port),
				"test-service-token",
			)
			results, err := driver.SearchRecapsByTag(context.Background(), "technology", 10)
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
