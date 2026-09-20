//go:build contract

// Pact CDC: alt-backend → rag-orchestrator.
//
// Pins the document ownership requirement on cross-service RAG indexing:
// POST /internal/rag/index/upsert requires user_id on every article upsert.
// POST /v1/documents/owners accepts batch owner backfills.
package contract

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"alt/adapter/augur_adapter"
	"alt/orchestrator/gateway/rag_gateway"
	"alt/orchestrator/port/rag_integration_port"

	"github.com/pact-foundation/pact-go/v2/consumer"
	"github.com/pact-foundation/pact-go/v2/matchers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const pactDir = "../../../../../pacts"

const (
	consumerBackend = "alt-backend"
	providerRag     = "rag-orchestrator"
	uuidLikePattern = `^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`
	testRagAPIToken = "test-rag-api-token-minimum-24-characters-long"
)

func newRagOrchestratorPact(t *testing.T) *consumer.V3HTTPMockProvider {
	t.Helper()
	mockProvider, err := consumer.NewV3Pact(consumer.MockHTTPProviderConfig{
		Consumer: consumerBackend,
		Provider: providerRag,
		PactDir:  filepath.Join(pactDir),
	})
	require.NoError(t, err)
	return mockProvider
}

func jsonRequestHeaders() matchers.MapMatcher {
	return matchers.MapMatcher{
		"Content-Type":  matchers.String("application/json"),
		"Authorization": matchers.Regex("Bearer "+testRagAPIToken, `^Bearer .+$`),
	}
}

func jsonResponseHeaders() matchers.MapMatcher {
	return matchers.MapMatcher{"Content-Type": matchers.Regex("application/json; charset=UTF-8", `^application/json.*`)}
}

func TestUpsertIndexContract(t *testing.T) {
	mockProvider := newRagOrchestratorPact(t)

	err := mockProvider.
		AddInteraction().
		Given("rag-orchestrator accepts article upserts with owner").
		UponReceiving("an UpsertIndex request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/internal/rag/index/upsert"),
			Headers: jsonRequestHeaders(),
			Body: matchers.MapMatcher{
				"article_id":   matchers.Regex("6f1a2f7e-1f1e-4c2a-9a3e-5b6c7d8e9f01", uuidLikePattern),
				"user_id":      matchers.Regex("11111111-2222-3333-4444-555555555555", uuidLikePattern),
				"title":        matchers.Like("Test Article Title"),
				"url":          matchers.Like("https://example.com/article"),
				"body":         matchers.Like("Full text content of the article to be indexed for RAG retrieval."),
				"published_at": matchers.Like("2026-09-20T10:00:00Z"),
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonResponseHeaders(),
			Body: matchers.MapMatcher{
				"status": matchers.Like("indexed"),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			client, err := rag_gateway.NewClientWithResponses(
				fmt.Sprintf("http://%s:%d", config.Host, config.Port),
				rag_gateway.WithBearerToken(testRagAPIToken),
			)
			if err != nil {
				return fmt.Errorf("create rag client: %w", err)
			}
			adapter := augur_adapter.NewAugurAdapter(client)
			pubAt := time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC)
			return adapter.UpsertArticle(context.Background(), rag_integration_port.UpsertArticleInput{
				ArticleID:   "6f1a2f7e-1f1e-4c2a-9a3e-5b6c7d8e9f01",
				UserID:      "11111111-2222-3333-4444-555555555555",
				Title:       "Test Article Title",
				URL:         "https://example.com/article",
				Body:        "Full text content of the article to be indexed for RAG retrieval.",
				PublishedAt: &pubAt,
			})
		})
	require.NoError(t, err)
}

func TestBackfillDocumentOwnersContract(t *testing.T) {
	mockProvider := newRagOrchestratorPact(t)

	err := mockProvider.
		AddInteraction().
		Given("rag-orchestrator accepts document owner backfill").
		UponReceiving("a BackfillDocumentOwners request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/v1/documents/owners"),
			Headers: jsonRequestHeaders(),
			Body: matchers.MapMatcher{
				"items": matchers.EachLike(map[string]interface{}{
					"article_id": matchers.Regex("6f1a2f7e-1f1e-4c2a-9a3e-5b6c7d8e9f01", uuidLikePattern),
					"user_id":    matchers.Regex("11111111-2222-3333-4444-555555555555", uuidLikePattern),
				}, 1),
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonResponseHeaders(),
			Body: matchers.MapMatcher{
				"updated":     matchers.Like(1),
				"already_set": matchers.Like(0),
				"not_found":   matchers.Like(0),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			client, err := rag_gateway.NewClientWithResponses(
				fmt.Sprintf("http://%s:%d", config.Host, config.Port),
				rag_gateway.WithBearerToken(testRagAPIToken),
			)
			if err != nil {
				return fmt.Errorf("create rag client: %w", err)
			}
			resp, err := client.BackfillDocumentOwnersWithResponse(context.Background(), rag_gateway.BackfillDocumentOwnersJSONRequestBody{
				Items: []rag_gateway.OwnerBackfillItem{
					{
						ArticleId: "6f1a2f7e-1f1e-4c2a-9a3e-5b6c7d8e9f01",
						UserId:    "11111111-2222-3333-4444-555555555555",
					},
				},
			})
			if err != nil {
				return fmt.Errorf("call BackfillDocumentOwners: %w", err)
			}
			require.NotNil(t, resp)
			assert.Equal(t, 200, resp.StatusCode())
			require.NotNil(t, resp.JSON200)
			assert.Equal(t, int64(1), resp.JSON200.Updated)
			return nil
		})
	require.NoError(t, err)
}

func TestRetrieveContextContract(t *testing.T) {
	mockProvider := newRagOrchestratorPact(t)

	err := mockProvider.
		AddInteraction().
		Given("rag-orchestrator retrieves context for query with owner").
		UponReceiving("a RetrieveContext request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/v1/rag/retrieve"),
			Headers: jsonRequestHeaders(),
			Body: matchers.MapMatcher{
				"query":                 matchers.Like("test search query"),
				"user_id":               matchers.Regex("11111111-2222-3333-4444-555555555555", uuidLikePattern),
				"candidate_article_ids": matchers.EachLike(matchers.String("6f1a2f7e-1f1e-4c2a-9a3e-5b6c7d8e9f01"), 1),
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonResponseHeaders(),
			Body: matchers.MapMatcher{
				"contexts": matchers.EachLike(map[string]interface{}{
					"chunk_id":         matchers.Like("chunk-1"),
					"chunk_text":       matchers.Like("sample chunk text"),
					"url":              matchers.Like("https://example.com/article"),
					"title":            matchers.Like("Test Article"),
					"published_at":     matchers.Like("2026-09-20T10:00:00Z"),
					"score":            matchers.Like(0.95),
					"document_version": matchers.Like(1),
				}, 1),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			client, err := rag_gateway.NewClientWithResponses(
				fmt.Sprintf("http://%s:%d", config.Host, config.Port),
				rag_gateway.WithBearerToken(testRagAPIToken),
			)
			if err != nil {
				return fmt.Errorf("create rag client: %w", err)
			}
			adapter := augur_adapter.NewAugurAdapter(client)
			contexts, err := adapter.RetrieveContext(context.Background(), "test search query", []string{"6f1a2f7e-1f1e-4c2a-9a3e-5b6c7d8e9f01"}, "11111111-2222-3333-4444-555555555555")
			if err != nil {
				return fmt.Errorf("call RetrieveContext: %w", err)
			}
			require.Len(t, contexts, 1)
			assert.Equal(t, "sample chunk text", contexts[0].ChunkText)
			assert.Equal(t, "https://example.com/article", contexts[0].URL)
			assert.Equal(t, "Test Article", contexts[0].Title)
			assert.InDelta(t, float32(0.95), contexts[0].Score, 0.01)
			assert.Equal(t, int64(1), contexts[0].DocumentVersion)
			return nil
		})
	require.NoError(t, err)
}
