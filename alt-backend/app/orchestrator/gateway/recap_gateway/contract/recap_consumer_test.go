//go:build contract

// Pact CDC: alt-backend → recap-worker.
//
// Pins the topic cards requirement on recap-worker:
// GET /v1/recaps/3days/cards returns the latest completed 3-day topic cards job and cards,
// or 200 with job: null and cards: [] when no completed cards job exists.
package contract

import (
	"context"
	"fmt"
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
	consumerBackend = "alt-backend"
	providerRecap   = "recap-worker"
	uuidLikePattern = `^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`
)

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

func TestGetThreeDayRecapCardsContract(t *testing.T) {
	mockProvider := newRecapWorkerPact(t)

	err := mockProvider.
		AddInteraction().
		Given("a completed cards job with cards exists").
		UponReceiving("a request for 3-day recap cards").
		WithCompleteRequest(consumer.Request{
			Method: "GET",
			Path:   matchers.String("/v1/recaps/3days/cards"),
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
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := recap_gateway.NewRecapGatewayWithConfig(nil, fmt.Sprintf("http://%s:%d", config.Host, config.Port))
			result, err := gw.GetThreeDayRecapCards(context.Background())
			if err != nil {
				return fmt.Errorf("GetThreeDayRecapCards failed: %w", err)
			}
			assert.NotNil(t, result)
			assert.NotNil(t, result.Job)
			assert.Equal(t, "11111111-2222-3333-4444-555555555555", result.Job.JobID)
			assert.False(t, result.Job.Degraded)
			assert.Len(t, result.Cards, 1)
			assert.Equal(t, "22222222-3333-4444-5555-666666666666", result.Cards[0].ID)
			assert.Equal(t, 1, result.Cards[0].Rank)
			assert.Equal(t, "日本のAIスタートアップが新モデルを発表", result.Cards[0].HeadlineJa)
			return nil
		})
	require.NoError(t, err)
}

func TestGetThreeDayRecapCardsMissContract(t *testing.T) {
	mockProvider := newRecapWorkerPact(t)

	err := mockProvider.
		AddInteraction().
		Given("no cards job exists").
		UponReceiving("a request for 3-day recap cards when no job exists").
		WithCompleteRequest(consumer.Request{
			Method: "GET",
			Path:   matchers.String("/v1/recaps/3days/cards"),
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonResponseHeaders(),
			Body: map[string]interface{}{
				"job":   nil,
				"cards": []interface{}{},
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := recap_gateway.NewRecapGatewayWithConfig(nil, fmt.Sprintf("http://%s:%d", config.Host, config.Port))
			result, err := gw.GetThreeDayRecapCards(context.Background())
			if err != nil {
				return fmt.Errorf("GetThreeDayRecapCards failed: %w", err)
			}
			assert.NotNil(t, result)
			assert.Nil(t, result.Job)
			assert.Empty(t, result.Cards)
			return nil
		})
	require.NoError(t, err)
}

func TestGetThreeDayRecapCardsNoGenreNoWhyContract(t *testing.T) {
	mockProvider := newRecapWorkerPact(t)

	err := mockProvider.
		AddInteraction().
		Given("a completed cards job whose card has no genre and no why").
		UponReceiving("a request for 3-day recap cards whose card has no genre and no why").
		WithCompleteRequest(consumer.Request{
			Method: "GET",
			Path:   matchers.String("/v1/recaps/3days/cards"),
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
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := recap_gateway.NewRecapGatewayWithConfig(nil, fmt.Sprintf("http://%s:%d", config.Host, config.Port))
			result, err := gw.GetThreeDayRecapCards(context.Background())
			if err != nil {
				return fmt.Errorf("GetThreeDayRecapCards failed: %w", err)
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
			return nil
		})
	require.NoError(t, err)
}
