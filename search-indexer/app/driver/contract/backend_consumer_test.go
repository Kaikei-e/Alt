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
