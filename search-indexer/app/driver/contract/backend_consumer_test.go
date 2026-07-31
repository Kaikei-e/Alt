//go:build contract

package contract

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"connectrpc.com/connect"
	"github.com/pact-foundation/pact-go/v2/consumer"
	"github.com/pact-foundation/pact-go/v2/matchers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	backendv1 "search-indexer/gen/proto/services/backend/v1"
	"search-indexer/gen/proto/services/backend/v1/backendv1connect"
)

func newBackendPact(t *testing.T) *consumer.V3HTTPMockProvider {
	t.Helper()
	mockProvider, err := consumer.NewV3Pact(consumer.MockHTTPProviderConfig{
		Consumer: "search-indexer",
		Provider: "alt-backend",
		PactDir:  pactDir,
	})
	require.NoError(t, err)
	return mockProvider
}

func newBackendClient(config consumer.MockServerConfig) backendv1connect.BackendInternalServiceClient {
	return backendv1connect.NewBackendInternalServiceClient(
		http.DefaultClient,
		fmt.Sprintf("http://%s:%d", config.Host, config.Port),
		connect.WithProtoJSON(),
	)
}

func TestBackendListArticlesWithTagsContract(t *testing.T) {
	mockProvider := newBackendPact(t)

	err := mockProvider.
		AddInteraction().
		Given("articles with tags exist for backward pagination").
		UponReceiving("a ListArticlesWithTags request for backward keyset pagination").
		WithCompleteRequest(consumer.Request{
			Method: "POST",
			Path:   matchers.String("/services.backend.v1.BackendInternalService/ListArticlesWithTags"),
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"lastId": matchers.Like("art-000"),
				"limit":  matchers.Like(200),
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status: 200,
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"articles": matchers.EachLike(matchers.MapMatcher{
					"id":        matchers.Like("art-001"),
					"title":     matchers.Like("Test Article"),
					"content":   matchers.Like("Article content."),
					"tags":      matchers.EachLike(matchers.Like("technology"), 1),
					"createdAt": matchers.Like("2026-03-26T00:00:00Z"),
					"userId":    matchers.Like("user-001"),
					"feedId":    matchers.Like("feed-001"),
				}, 1),
				"nextId": matchers.Like("art-002"),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			client := newBackendClient(config)
			resp, err := client.ListArticlesWithTags(context.Background(), connect.NewRequest(&backendv1.ListArticlesWithTagsRequest{
				LastId: "art-000",
				Limit:  200,
			}))
			if err != nil {
				return fmt.Errorf("ListArticlesWithTags failed: %w", err)
			}

			assert.NotEmpty(t, resp.Msg.Articles)
			assert.NotEmpty(t, resp.Msg.Articles[0].Id)
			assert.NotEmpty(t, resp.Msg.Articles[0].Title)
			return nil
		})
	require.NoError(t, err)
}

// TestBackendGetArticleByIDContract pins the response shape of the single
// article fetch. search-indexer resolves every indexed document through this
// RPC, so the document's published_at is only as good as what alt-backend
// returns here — the field must be part of the contract, not an accident of
// the fat event payload.
func TestBackendGetArticleByIDContract(t *testing.T) {
	mockProvider := newBackendPact(t)

	err := mockProvider.
		AddInteraction().
		Given("an article with a source publication timestamp exists").
		UponReceiving("a GetArticleByID request").
		WithCompleteRequest(consumer.Request{
			Method: "POST",
			Path:   matchers.String("/services.backend.v1.BackendInternalService/GetArticleByID"),
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"articleId": matchers.Like("art-001"),
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status: 200,
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"article": matchers.Like(matchers.MapMatcher{
					"id":          matchers.Like("art-001"),
					"title":       matchers.Like("Test Article"),
					"content":     matchers.Like("Article content."),
					"tags":        matchers.EachLike(matchers.Like("technology"), 1),
					"createdAt":   matchers.Like("2026-03-26T00:00:00Z"),
					"userId":      matchers.Like("user-001"),
					"feedId":      matchers.Like("feed-001"),
					"language":    matchers.Like("en"),
					"publishedAt": matchers.Like("2026-03-20T09:30:00Z"),
				}),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			client := newBackendClient(config)
			resp, err := client.GetArticleByID(context.Background(), connect.NewRequest(&backendv1.GetArticleByIDRequest{
				ArticleId: "art-001",
			}))
			if err != nil {
				return fmt.Errorf("GetArticleByID failed: %w", err)
			}

			require.NotNil(t, resp.Msg.Article)
			assert.NotEmpty(t, resp.Msg.Article.Id)
			assert.NotEmpty(t, resp.Msg.Article.Title)
			require.NotNil(t, resp.Msg.Article.CreatedAt)
			// The whole point of the interaction: published_at must survive
			// the RPC instead of being reconstructed from created_at.
			require.NotNil(t, resp.Msg.Article.PublishedAt, "published_at must be exposed by GetArticleByID")
			assert.False(t, resp.Msg.Article.PublishedAt.AsTime().Equal(resp.Msg.Article.CreatedAt.AsTime()),
				"published_at must be the source timestamp, not a copy of created_at")
			return nil
		})
	require.NoError(t, err)
}

func TestBackendGetLatestArticleTimestampContract(t *testing.T) {
	mockProvider := newBackendPact(t)

	err := mockProvider.
		AddInteraction().
		Given("articles exist in the database").
		UponReceiving("a GetLatestArticleTimestamp request").
		WithCompleteRequest(consumer.Request{
			Method: "POST",
			Path:   matchers.String("/services.backend.v1.BackendInternalService/GetLatestArticleTimestamp"),
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{},
		}).
		WithCompleteResponse(consumer.Response{
			Status: 200,
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"latestCreatedAt": matchers.Like("2026-03-26T00:00:00Z"),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			client := newBackendClient(config)
			resp, err := client.GetLatestArticleTimestamp(context.Background(), connect.NewRequest(&backendv1.GetLatestArticleTimestampRequest{}))
			if err != nil {
				return fmt.Errorf("GetLatestArticleTimestamp failed: %w", err)
			}

			assert.NotNil(t, resp.Msg.LatestCreatedAt)
			return nil
		})
	require.NoError(t, err)
}
