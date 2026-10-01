//go:build contract

// Pact CDC: alt-backend → recap-worker.
//
// Pins the topic cards requirement on recap-worker:
// GET /v1/topic-cards returns the latest completed topic cards job and cards,
// and latest_run metadata. latest_run is null only when no cards run exists at all
// (a failed or running run still populates it).
package contract

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"alt/orchestrator/gateway/recap_gateway"

	"github.com/pact-foundation/pact-go/v2/consumer"
	"github.com/pact-foundation/pact-go/v2/matchers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const pactDir = "../../../../../../pacts"

const (
	consumerBackend     = "alt-backend"
	providerRecap       = "recap-worker"
	uuidLikePattern     = `^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`
	runStatusPattern    = `^(pending|running|completed|failed)$`
	failedStatusPattern = `^failed$`
)

func TestMain(m *testing.M) {
	// WHY: Remove existing pact file before running to prevent renamed interactions from lingering in the merged pact file.
	pactPath := filepath.Join(pactDir, fmt.Sprintf("%s-%s.json", consumerBackend, providerRecap))
	if err := os.Remove(pactPath); err != nil && !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "failed to remove existing pact file %s: %v\n", pactPath, err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

func newRecapWorkerPact(t *testing.T) *consumer.V3HTTPMockProvider {
	t.Helper()
	mockProvider, err := consumer.NewV3Pact(consumer.MockHTTPProviderConfig{
		Consumer: consumerBackend,
		Provider: providerRecap,
		PactDir:  filepath.Join(pactDir),
	})
	require.NoError(t, err)
	return mockProvider
}

func jsonResponseHeaders() matchers.MapMatcher {
	return matchers.MapMatcher{"Content-Type": matchers.Regex("application/json; charset=UTF-8", `^application/json.*`)}
}

func TestGetTopicCardsContract(t *testing.T) {
	mockProvider := newRecapWorkerPact(t)

	err := mockProvider.
		AddInteraction().
		Given("a completed cards job with cards exists").
		UponReceiving("a request for topic cards").
		WithCompleteRequest(consumer.Request{
			Method: "GET",
			Path:   matchers.String("/v1/topic-cards"),
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonResponseHeaders(),
			Body: matchers.MapMatcher{
				"job": matchers.Like(map[string]interface{}{
					"job_id":         matchers.Regex("11111111-2222-3333-4444-555555555555", uuidLikePattern),
					"kicked_at":      matchers.Like("2026-09-22T17:00:00Z"),
					"from":           matchers.Like("2026-09-19T17:00:00Z"),
					"to":             matchers.Like("2026-09-22T17:00:00Z"),
					"params_version": matchers.Like("cards-v0.2"),
					"cards_selected": matchers.Like(8),
					"degraded":       matchers.Like(false),
				}),
				"cards": matchers.EachLike(map[string]interface{}{
					"id":                matchers.Regex("22222222-3333-4444-5555-666666666666", uuidLikePattern),
					"rank":              matchers.Like(1),
					"story_id":          matchers.Regex("33333333-4444-5555-6666-777777777777", uuidLikePattern),
					"continues_card_id": nil,
					"headline_ja":       matchers.Like("日本のAIスタートアップが新モデルを発表"),
					"what_ja":           matchers.Like("最新の推論モデルが公開された。[1]ベンチマークで高い性能を示した。[2]"),
					"why_ja":            matchers.Like("日本語処理の効率化が期待される。[1]"),
					"genre":             matchers.Like("Technology"),
					"sources": matchers.EachLike(map[string]interface{}{
						"n":        matchers.Like(1),
						"feed_id":  matchers.Regex("44444444-5555-6666-7777-888888888888", uuidLikePattern),
						"url":      matchers.Like("https://example.com/ai-news"),
						"host":     matchers.Like("example.com"),
						"title":    matchers.Like("新モデル発表のニュース"),
						"pub_date": matchers.Like("2026-09-22T10:00:00Z"),
					}, 1),
					"created_at": matchers.Like("2026-09-22T17:05:00Z"),
				}, 1),
				"latest_run": matchers.Like(map[string]interface{}{
					"job_id":     matchers.Regex("11111111-2222-3333-4444-555555555555", uuidLikePattern),
					"status":     matchers.Regex("completed", runStatusPattern),
					"kicked_at":  matchers.Like("2026-09-22T17:00:00Z"),
					"updated_at": matchers.Like("2026-09-22T17:05:00Z"),
				}),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := recap_gateway.NewRecapGatewayWithConfig(nil, fmt.Sprintf("http://%s:%d", config.Host, config.Port))
			result, err := gw.GetTopicCards(context.Background())
			if err != nil {
				return fmt.Errorf("GetTopicCards failed: %w", err)
			}
			assert.NotNil(t, result)
			assert.NotNil(t, result.Job)
			assert.Equal(t, "11111111-2222-3333-4444-555555555555", result.Job.JobID)
			assert.False(t, result.Job.Degraded)
			assert.Len(t, result.Cards, 1)
			assert.Equal(t, "22222222-3333-4444-5555-666666666666", result.Cards[0].ID)
			assert.Equal(t, 1, result.Cards[0].Rank)
			assert.Equal(t, "日本のAIスタートアップが新モデルを発表", result.Cards[0].HeadlineJa)
			assert.NotNil(t, result.LatestRun)
			assert.Equal(t, "11111111-2222-3333-4444-555555555555", result.LatestRun.JobID)
			assert.Equal(t, "completed", result.LatestRun.Status)
			assert.Equal(t, "2026-09-22T17:00:00Z", result.LatestRun.KickedAt)
			assert.Equal(t, "2026-09-22T17:05:00Z", result.LatestRun.UpdatedAt)
			return nil
		})
	require.NoError(t, err)
}

func TestGetTopicCardsMissContract(t *testing.T) {
	mockProvider := newRecapWorkerPact(t)

	err := mockProvider.
		AddInteraction().
		Given("no cards job exists").
		UponReceiving("a request for topic cards when no job exists").
		WithCompleteRequest(consumer.Request{
			Method: "GET",
			Path:   matchers.String("/v1/topic-cards"),
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonResponseHeaders(),
			Body: map[string]interface{}{
				"job":        nil,
				"cards":      []interface{}{},
				"latest_run": nil,
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := recap_gateway.NewRecapGatewayWithConfig(nil, fmt.Sprintf("http://%s:%d", config.Host, config.Port))
			result, err := gw.GetTopicCards(context.Background())
			if err != nil {
				return fmt.Errorf("GetTopicCards failed: %w", err)
			}
			assert.NotNil(t, result)
			assert.Nil(t, result.Job)
			assert.Empty(t, result.Cards)
			assert.Nil(t, result.LatestRun)
			return nil
		})
	require.NoError(t, err)
}

func TestGetTopicCardsNoGenreNoWhyContract(t *testing.T) {
	mockProvider := newRecapWorkerPact(t)

	err := mockProvider.
		AddInteraction().
		Given("a completed cards job whose card has no genre and no why").
		UponReceiving("a request for topic cards whose card has no genre and no why").
		WithCompleteRequest(consumer.Request{
			Method: "GET",
			Path:   matchers.String("/v1/topic-cards"),
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonResponseHeaders(),
			Body: matchers.MapMatcher{
				"job": matchers.Like(map[string]interface{}{
					"job_id":         matchers.Regex("11111111-2222-3333-4444-555555555555", uuidLikePattern),
					"kicked_at":      matchers.Like("2026-09-22T17:00:00Z"),
					"from":           matchers.Like("2026-09-19T17:00:00Z"),
					"to":             matchers.Like("2026-09-22T17:00:00Z"),
					"params_version": matchers.Like("cards-v0.2"),
					"cards_selected": matchers.Like(8),
					"degraded":       matchers.Like(false),
				}),
				"cards": matchers.EachLike(map[string]interface{}{
					"id":                matchers.Regex("22222222-3333-4444-5555-666666666666", uuidLikePattern),
					"rank":              matchers.Like(1),
					"story_id":          matchers.Regex("33333333-4444-5555-6666-777777777777", uuidLikePattern),
					"continues_card_id": nil,
					"headline_ja":       matchers.Like("日本のAIスタートアップが新モデルを発表"),
					"what_ja":           matchers.Like("最新の推論モデルが公開された。[1]ベンチマークで高い性能を示した。[2]"),
					"why_ja":            nil,
					"genre":             nil,
					"sources": matchers.EachLike(map[string]interface{}{
						"n":        matchers.Like(1),
						"feed_id":  matchers.Regex("44444444-5555-6666-7777-888888888888", uuidLikePattern),
						"url":      matchers.Like("https://example.com/ai-news"),
						"host":     matchers.Like("example.com"),
						"title":    matchers.Like("新モデル発表のニュース"),
						"pub_date": matchers.Like("2026-09-22T10:00:00Z"),
					}, 1),
					"created_at": matchers.Like("2026-09-22T17:05:00Z"),
				}, 1),
				"latest_run": matchers.Like(map[string]interface{}{
					"job_id":     matchers.Regex("11111111-2222-3333-4444-555555555555", uuidLikePattern),
					"status":     matchers.Regex("completed", runStatusPattern),
					"kicked_at":  matchers.Like("2026-09-22T17:00:00Z"),
					"updated_at": matchers.Like("2026-09-22T17:05:00Z"),
				}),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := recap_gateway.NewRecapGatewayWithConfig(nil, fmt.Sprintf("http://%s:%d", config.Host, config.Port))
			result, err := gw.GetTopicCards(context.Background())
			if err != nil {
				return fmt.Errorf("GetTopicCards failed: %w", err)
			}
			assert.NotNil(t, result)
			assert.NotNil(t, result.Job)
			assert.Equal(t, "11111111-2222-3333-4444-555555555555", result.Job.JobID)
			assert.False(t, result.Job.Degraded)
			assert.Len(t, result.Cards, 1)
			assert.Equal(t, "22222222-3333-4444-5555-666666666666", result.Cards[0].ID)
			assert.Equal(t, 1, result.Cards[0].Rank)
			assert.Equal(t, "日本のAIスタートアップが新モデルを発表", result.Cards[0].HeadlineJa)
			assert.Nil(t, result.Cards[0].Genre)
			assert.Nil(t, result.Cards[0].WhyJa)
			assert.Nil(t, result.Cards[0].ContinuesCardID)
			assert.NotNil(t, result.LatestRun)
			assert.Equal(t, "11111111-2222-3333-4444-555555555555", result.LatestRun.JobID)
			assert.Equal(t, "completed", result.LatestRun.Status)
			assert.Equal(t, "2026-09-22T17:00:00Z", result.LatestRun.KickedAt)
			assert.Equal(t, "2026-09-22T17:05:00Z", result.LatestRun.UpdatedAt)
			return nil
		})
	require.NoError(t, err)
}

func TestGetTopicCardsFailedRunContract(t *testing.T) {
	mockProvider := newRecapWorkerPact(t)

	err := mockProvider.
		AddInteraction().
		Given("a failed topic cards run is newer than the latest completed run").
		UponReceiving("a request for topic cards whose latest run failed").
		WithCompleteRequest(consumer.Request{
			Method: "GET",
			Path:   matchers.String("/v1/topic-cards"),
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonResponseHeaders(),
			Body: matchers.MapMatcher{
				"job": matchers.Like(map[string]interface{}{
					"job_id":         matchers.Regex("11111111-2222-3333-4444-555555555555", uuidLikePattern),
					"kicked_at":      matchers.Like("2026-09-22T17:00:00Z"),
					"from":           matchers.Like("2026-09-19T17:00:00Z"),
					"to":             matchers.Like("2026-09-22T17:00:00Z"),
					"params_version": matchers.Like("cards-v0.2"),
					"cards_selected": matchers.Like(8),
					"degraded":       matchers.Like(false),
				}),
				"cards": matchers.EachLike(map[string]interface{}{
					"id":                matchers.Regex("22222222-3333-4444-5555-666666666666", uuidLikePattern),
					"rank":              matchers.Like(1),
					"story_id":          matchers.Regex("33333333-4444-5555-6666-777777777777", uuidLikePattern),
					"continues_card_id": nil,
					"headline_ja":       matchers.Like("日本のAIスタートアップが新モデルを発表"),
					"what_ja":           matchers.Like("最新の推論モデルが公開された。[1]ベンチマークで高い性能を示した。[2]"),
					"why_ja":            matchers.Like("日本語処理の効率化が期待される。[1]"),
					"genre":             matchers.Like("Technology"),
					"sources": matchers.EachLike(map[string]interface{}{
						"n":        matchers.Like(1),
						"feed_id":  matchers.Regex("44444444-5555-6666-7777-888888888888", uuidLikePattern),
						"url":      matchers.Like("https://example.com/ai-news"),
						"host":     matchers.Like("example.com"),
						"title":    matchers.Like("新モデル発表のニュース"),
						"pub_date": matchers.Like("2026-09-22T10:00:00Z"),
					}, 1),
					"created_at": matchers.Like("2026-09-22T17:05:00Z"),
				}, 1),
				"latest_run": matchers.Like(map[string]interface{}{
					"job_id":     matchers.Regex("99999999-8888-7777-6666-555555555555", uuidLikePattern),
					"status":     matchers.Regex("failed", failedStatusPattern),
					"kicked_at":  matchers.Like("2026-09-22T18:00:00Z"),
					"updated_at": matchers.Like("2026-09-22T18:02:00Z"),
				}),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := recap_gateway.NewRecapGatewayWithConfig(nil, fmt.Sprintf("http://%s:%d", config.Host, config.Port))
			result, err := gw.GetTopicCards(context.Background())
			if err != nil {
				return fmt.Errorf("GetTopicCards failed: %w", err)
			}
			assert.NotNil(t, result)
			assert.NotNil(t, result.Job)
			assert.Equal(t, "11111111-2222-3333-4444-555555555555", result.Job.JobID)
			assert.False(t, result.Job.Degraded)
			assert.Len(t, result.Cards, 1)
			assert.Equal(t, "22222222-3333-4444-5555-666666666666", result.Cards[0].ID)
			assert.Equal(t, 1, result.Cards[0].Rank)
			assert.Equal(t, "日本のAIスタートアップが新モデルを発表", result.Cards[0].HeadlineJa)
			assert.NotNil(t, result.LatestRun)
			assert.Equal(t, "99999999-8888-7777-6666-555555555555", result.LatestRun.JobID)
			assert.Equal(t, "failed", result.LatestRun.Status)
			assert.Equal(t, "2026-09-22T18:00:00Z", result.LatestRun.KickedAt)
			assert.Equal(t, "2026-09-22T18:02:00Z", result.LatestRun.UpdatedAt)
			return nil
		})
	require.NoError(t, err)
}
