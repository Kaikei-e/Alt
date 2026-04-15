//go:build contract

// Pact CDC: rag-orchestrator → recap-worker morning letter REST endpoints.
//
// recap-worker publishes immutable MorningLetterDoc aggregates that the
// rag-orchestrator retrieval pipeline reads read-only. The contract pins:
//   - GET /v1/morning/letters/latest        (most recent doc, or 404)
//   - GET /v1/morning/letters/{target_date} (specific day, or 404)
//   - Response envelope: { id, target_date, body: { lead, sections: [{key,title,bullets}] } }
//
// Missing Pact coverage here was tracked as the highest-priority gap in the
// CDC audit. Adding this test ensures a provider-side response shape or
// path change in recap-worker (Rust) is caught at CI contract time rather than
// surfacing as empty morning-letter context in the RAG pipeline.

package contract

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"testing"

	"rag-orchestrator/internal/adapter/recap_worker"

	"github.com/pact-foundation/pact-go/v2/consumer"
	"github.com/pact-foundation/pact-go/v2/matchers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newRecapWorkerPact(t *testing.T) *consumer.V3HTTPMockProvider {
	t.Helper()
	mockProvider, err := consumer.NewV3Pact(consumer.MockHTTPProviderConfig{
		Consumer: "rag-orchestrator",
		Provider: "recap-worker",
		PactDir:  filepath.Join(pactDir),
	})
	require.NoError(t, err)
	return mockProvider
}

func morningLetterBody() matchers.MapMatcher {
	return matchers.MapMatcher{
		"id":          matchers.Like("letter-001"),
		"target_date": matchers.Like("2026-04-15"),
		"body": matchers.Like(map[string]interface{}{
			"lead": matchers.Like("Today's key developments..."),
			"sections": matchers.EachLike(map[string]interface{}{
				"key":     matchers.Like("top3"),
				"title":   matchers.Like("Top Stories"),
				"bullets": matchers.EachLike("Story A", 1),
			}, 1),
		}),
	}
}

func TestRecapWorkerFetchLatestContract(t *testing.T) {
	mockProvider := newRecapWorkerPact(t)

	err := mockProvider.
		AddInteraction().
		Given("a published morning letter exists for today").
		UponReceiving("a GET for the latest morning letter").
		WithCompleteRequest(consumer.Request{
			Method: "GET",
			Path:   matchers.String("/v1/morning/letters/latest"),
		}).
		WithCompleteResponse(consumer.Response{
			Status: 200,
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: morningLetterBody(),
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			client := recap_worker.NewClient(
				fmt.Sprintf("http://%s:%d", config.Host, config.Port),
				http.DefaultClient,
			)
			doc, err := client.FetchLatest(context.Background())
			if err != nil {
				return fmt.Errorf("FetchLatest failed: %w", err)
			}
			assert.NotNil(t, doc)
			assert.NotEmpty(t, doc.Lead)
			assert.NotEmpty(t, doc.Sections)
			assert.NotEmpty(t, doc.Sections[0].Key)
			return nil
		})
	require.NoError(t, err)
}

func TestRecapWorkerFetchLatestNotFoundContract(t *testing.T) {
	mockProvider := newRecapWorkerPact(t)

	err := mockProvider.
		AddInteraction().
		Given("no morning letter has been published yet").
		UponReceiving("a GET for the latest morning letter when none exists").
		WithCompleteRequest(consumer.Request{
			Method: "GET",
			Path:   matchers.String("/v1/morning/letters/latest"),
		}).
		WithCompleteResponse(consumer.Response{
			Status: 404,
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			client := recap_worker.NewClient(
				fmt.Sprintf("http://%s:%d", config.Host, config.Port),
				http.DefaultClient,
			)
			doc, err := client.FetchLatest(context.Background())
			if err != nil {
				return fmt.Errorf("FetchLatest returned error for 404 (expected nil,nil): %w", err)
			}
			assert.Nil(t, doc, "404 must map to (nil, nil) — the caller distinguishes absence from failure")
			return nil
		})
	require.NoError(t, err)
}

func TestRecapWorkerFetchByDateContract(t *testing.T) {
	mockProvider := newRecapWorkerPact(t)

	err := mockProvider.
		AddInteraction().
		Given("a morning letter exists for 2026-04-14").
		UponReceiving("a GET for the morning letter of 2026-04-14").
		WithCompleteRequest(consumer.Request{
			Method: "GET",
			Path:   matchers.Regex("/v1/morning/letters/2026-04-14", `/v1/morning/letters/\d{4}-\d{2}-\d{2}`),
		}).
		WithCompleteResponse(consumer.Response{
			Status: 200,
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: morningLetterBody(),
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			client := recap_worker.NewClient(
				fmt.Sprintf("http://%s:%d", config.Host, config.Port),
				http.DefaultClient,
			)
			doc, err := client.FetchByDate(context.Background(), "2026-04-14")
			if err != nil {
				return fmt.Errorf("FetchByDate failed: %w", err)
			}
			assert.NotNil(t, doc)
			assert.NotEmpty(t, doc.Sections)
			return nil
		})
	require.NoError(t, err)
}
