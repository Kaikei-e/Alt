//go:build contract

// Pact CDC: rag-orchestrator → alt-data-hub (alt.datahub.v1.DataHubService).
//
// ADR-000954 D7, Wave 2-B. Before this contract existed rag-orchestrator
// reached alt_db two different ways, both of them now dead:
//
//   - REST  GET  http://alt-backend:9102/v1/internal/articles/recent
//   - Connect-RPC services.backend.v1.BackendInternalService over the same
//     plaintext :9102
//
// The 3-binary split (ADR-000954 D1) moved every alt_db capability into
// cmd/datahub behind mutual TLS on :9443 and left :9102 as alt-backend's
// operator listener. Neither call above can succeed today, so this migration
// is a repair, not a refactor — which is exactly why the wire format needs to
// be pinned rather than assumed.
//
// What this pins:
//
//   - POST /alt.datahub.v1.DataHubService/{ListRecentArticles,FetchTagCloud,
//     FetchArticlesByTag} — the Connect-RPC unary path shape, i.e. the new
//     package name is on the wire and not the legacy services.backend.v1 one.
//   - Content-Type: application/json (Connect over HTTP/1.1 + protojson,
//     ADR-000764 convention).
//   - Camel-cased protojson field names (withinHours, publishedAt, feedId,
//     tagName, articleCount).
//   - The optional-field semantics ListRecentArticlesRequest was given in
//     Wave 2-A: within_hours and limit are BOTH always transmitted, because
//     limit = 0 means "no count limit, time window only" and is the mode the
//     morning-letter usecase runs in. A consumer that let protojson elide the
//     zero would silently get the server default of 100 articles instead —
//     the failure this interaction exists to catch.
//
// mTLS is deliberately NOT exercised here: the pact mock provider speaks
// plaintext HTTP, and the transport is orthogonal to the wire contract. The
// client cert path is covered by internal/infra/httpclient tests.

package contract

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"testing"

	"rag-orchestrator/internal/adapter/altdb"

	datahubv1connect "alt/gen/proto/alt/datahub/v1/datahubv1connect"

	"github.com/pact-foundation/pact-go/v2/consumer"
	"github.com/pact-foundation/pact-go/v2/matchers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testContractLogger discards adapter logs; the contract under test is the
// wire format, not the log lines.
func testContractLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}

func newDataHubPact(t *testing.T) *consumer.V3HTTPMockProvider {
	t.Helper()
	mockProvider, err := consumer.NewV3Pact(consumer.MockHTTPProviderConfig{
		Consumer: "rag-orchestrator",
		Provider: "alt-data-hub",
		PactDir:  filepath.Join(pactDir),
	})
	require.NoError(t, err)
	return mockProvider
}

// newDataHubServiceClient goes through the production constructor rather than
// datahubv1connect directly, so the codec this pact records is the codec
// production sends (protoJSON, ADR-000764) and not a test-local choice.
func newDataHubServiceClient(config consumer.MockServerConfig) datahubv1connect.DataHubServiceClient {
	return altdb.NewDataHubServiceClient(
		http.DefaultClient,
		fmt.Sprintf("http://%s:%d", config.Host, config.Port),
	)
}

// TestDataHubListRecentArticlesContract pins the RPC that replaces
// GET /v1/internal/articles/recent. limit = 0 is sent explicitly (see the
// package comment) and the RFC3339 published_at stays a string, as the
// Wave 2-A proto chose, so the consumer keeps parsing exactly what the REST
// route used to emit.
func TestDataHubListRecentArticlesContract(t *testing.T) {
	mockProvider := newDataHubPact(t)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has articles published in the last 24 hours").
		UponReceiving("a ListRecentArticles request from rag-orchestrator with no count limit").
		WithCompleteRequest(consumer.Request{
			Method: "POST",
			Path:   matchers.String("/alt.datahub.v1.DataHubService/ListRecentArticles"),
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: map[string]interface{}{
				"withinHours": 24,
				// Explicit zero: "time window only, no count cap". Its
				// presence on the wire IS the contract.
				"limit": 0,
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status: 200,
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"articles": matchers.EachLike(map[string]interface{}{
					"id":          matchers.Regex("6f1a2f7e-1f1e-4c2a-9a3e-5b6c7d8e9f01", uuidLikePattern),
					"title":       matchers.Like("An LLM primer"),
					"url":         matchers.Like("https://example.com/llm-primer"),
					"publishedAt": matchers.Like("2026-04-14T00:30:00Z"),
					"feedId":      matchers.Regex("11111111-2222-3333-4444-555555555555", uuidLikePattern),
					"tags":        matchers.EachLike("ai", 1),
				}, 1),
				"since": matchers.Like("2026-04-14T00:00:00Z"),
				"until": matchers.Like("2026-04-15T00:00:00Z"),
				"count": matchers.Like(1),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			client := altdb.NewDataHubArticleClient(newDataHubServiceClient(config), testContractLogger())

			articles, err := client.GetRecentArticles(context.Background(), 24, 0)
			if err != nil {
				return fmt.Errorf("GetRecentArticles failed: %w", err)
			}
			require.Len(t, articles, 1)
			assert.Equal(t, "An LLM primer", articles[0].Title)
			assert.Equal(t, "https://example.com/llm-primer", articles[0].URL)
			assert.False(t, articles[0].PublishedAt.IsZero(),
				"publishedAt must parse; a zero value here means the RFC3339 string contract drifted")
			assert.Equal(t, []string{"ai"}, articles[0].Tags)
			return nil
		})
	require.NoError(t, err)
}

// TestDataHubFetchTagCloudContract pins the tag_cloud_explore tool's
// dependency. Origin: services.backend.v1.BackendInternalService/FetchTagCloud
// — wire-identical body, new package on the path.
func TestDataHubFetchTagCloudContract(t *testing.T) {
	mockProvider := newDataHubPact(t)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has tagged articles").
		UponReceiving("a FetchTagCloud request from rag-orchestrator").
		WithCompleteRequest(consumer.Request{
			Method: "POST",
			Path:   matchers.String("/alt.datahub.v1.DataHubService/FetchTagCloud"),
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: map[string]interface{}{
				"limit": 300,
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status: 200,
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"tags": matchers.EachLike(map[string]interface{}{
					"tagName":      matchers.Like("ai"),
					"articleCount": matchers.Like(42),
				}, 1),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			client := altdb.NewDataHubTagCloudClient(newDataHubServiceClient(config), testContractLogger())

			tags, err := client.FetchTagCloud(context.Background(), 300)
			if err != nil {
				return fmt.Errorf("FetchTagCloud failed: %w", err)
			}
			require.Len(t, tags, 1)
			assert.Equal(t, "ai", tags[0].TagName)
			assert.Equal(t, int32(42), tags[0].ArticleCount)
			return nil
		})
	require.NoError(t, err)
}

// TestDataHubFetchArticlesByTagContract pins the articles_by_tag tool's
// dependency. Origin:
// services.backend.v1.BackendInternalService/FetchArticlesByTag.
func TestDataHubFetchArticlesByTagContract(t *testing.T) {
	mockProvider := newDataHubPact(t)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has articles tagged ai").
		UponReceiving("a FetchArticlesByTag request from rag-orchestrator").
		WithCompleteRequest(consumer.Request{
			Method: "POST",
			Path:   matchers.String("/alt.datahub.v1.DataHubService/FetchArticlesByTag"),
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: map[string]interface{}{
				"tagName": "ai",
				"limit":   10,
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status: 200,
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"articles": matchers.EachLike(map[string]interface{}{
					"id":          matchers.Like("article-1"),
					"title":       matchers.Like("Multi-Agent Systems"),
					"url":         matchers.Like("https://example.com/mas"),
					"publishedAt": matchers.Like("2026-04-14T00:30:00Z"),
				}, 1),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			client := altdb.NewDataHubArticlesByTagClient(newDataHubServiceClient(config), testContractLogger())

			articles, err := client.FetchArticlesByTag(context.Background(), "ai", 10)
			if err != nil {
				return fmt.Errorf("FetchArticlesByTag failed: %w", err)
			}
			require.Len(t, articles, 1)
			assert.Equal(t, "article-1", articles[0].ID)
			assert.Equal(t, "Multi-Agent Systems", articles[0].Title)
			assert.Equal(t, "https://example.com/mas", articles[0].URL)
			return nil
		})
	require.NoError(t, err)
}
