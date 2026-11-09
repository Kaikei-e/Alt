//go:build contract

// Pact CDC: alt-backend → recap-worker.
package contract

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"alt/domain"
	"alt/orchestrator/gateway/morning_gateway"
	"alt/orchestrator/gateway/recap_gateway"

	"github.com/google/uuid"
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

type noopArticleReader struct{}

func (noopArticleReader) FetchArticlesByIDs(_ context.Context, _ []uuid.UUID) ([]*domain.Article, error) {
	return nil, nil
}

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
			Body: map[string]interface{}{
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
				"surface_status": map[string]interface{}{
					"generated_at": matchers.Like("2026-09-22T17:05:00Z"),
					"window_start": matchers.Like("2026-09-19T17:00:00Z"),
					"window_end":   matchers.Like("2026-09-22T17:00:00Z"),
					"is_stale":     matchers.Like(false),
					"stale_after":  matchers.Like("2026-09-23T23:05:00Z"),
					"latest_run": map[string]interface{}{
						"job_id":     matchers.Like("11111111-2222-3333-4444-555555555555"),
						"status":     "completed",
						"kicked_at":  matchers.Like("2026-09-22T17:00:00Z"),
						"updated_at": matchers.Like("2026-09-22T17:05:00Z"),
					},
					"degradation_reasons": matchers.Like([]interface{}{}),
				},
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
			require.NotNil(t, result.SurfaceStatus)
			assert.Equal(t, "2026-09-22T17:05:00Z", result.SurfaceStatus.GeneratedAt)
			assert.Equal(t, "2026-09-19T17:00:00Z", result.SurfaceStatus.WindowStart)
			assert.Equal(t, "2026-09-22T17:00:00Z", result.SurfaceStatus.WindowEnd)
			assert.False(t, result.SurfaceStatus.IsStale)
			assert.Equal(t, "2026-09-23T23:05:00Z", result.SurfaceStatus.StaleAfter)
			require.NotNil(t, result.SurfaceStatus.LatestRun)
			assert.Equal(t, "11111111-2222-3333-4444-555555555555", result.SurfaceStatus.LatestRun.JobID)
			assert.Equal(t, "completed", result.SurfaceStatus.LatestRun.Status)
			assert.Equal(t, "2026-09-22T17:00:00Z", result.SurfaceStatus.LatestRun.KickedAt)
			assert.Equal(t, "2026-09-22T17:05:00Z", result.SurfaceStatus.LatestRun.UpdatedAt)
			assert.Empty(t, result.SurfaceStatus.DegradationReasons)
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
				"job":            nil,
				"cards":          []interface{}{},
				"surface_status": nil,
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
			assert.Nil(t, result.SurfaceStatus)
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
			Body: map[string]interface{}{
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
				"surface_status": map[string]interface{}{
					"generated_at": matchers.Like("2026-09-22T17:05:00Z"),
					"window_start": matchers.Like("2026-09-19T17:00:00Z"),
					"window_end":   matchers.Like("2026-09-22T17:00:00Z"),
					"is_stale":     matchers.Like(false),
					"stale_after":  matchers.Like("2026-09-23T23:05:00Z"),
					"latest_run": map[string]interface{}{
						"job_id":     matchers.Like("11111111-2222-3333-4444-555555555555"),
						"status":     "completed",
						"kicked_at":  matchers.Like("2026-09-22T17:00:00Z"),
						"updated_at": matchers.Like("2026-09-22T17:05:00Z"),
					},
					"degradation_reasons": matchers.Like([]interface{}{}),
				},
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
			require.NotNil(t, result.SurfaceStatus)
			assert.Equal(t, "2026-09-22T17:05:00Z", result.SurfaceStatus.GeneratedAt)
			require.NotNil(t, result.SurfaceStatus.LatestRun)
			assert.Equal(t, "11111111-2222-3333-4444-555555555555", result.SurfaceStatus.LatestRun.JobID)
			assert.Equal(t, "completed", result.SurfaceStatus.LatestRun.Status)
			assert.Equal(t, "2026-09-22T17:00:00Z", result.SurfaceStatus.LatestRun.KickedAt)
			assert.Equal(t, "2026-09-22T17:05:00Z", result.SurfaceStatus.LatestRun.UpdatedAt)
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
			Body: map[string]interface{}{
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
				"surface_status": map[string]interface{}{
					"generated_at": matchers.Like("2026-09-22T17:05:00Z"),
					"window_start": matchers.Like("2026-09-19T17:00:00Z"),
					"window_end":   matchers.Like("2026-09-22T17:00:00Z"),
					"is_stale":     matchers.Like(false),
					"stale_after":  matchers.Like("2026-09-23T23:05:00Z"),
					"latest_run": map[string]interface{}{
						"job_id":     matchers.Like("99999999-8888-7777-6666-555555555555"),
						"status":     "failed",
						"kicked_at":  matchers.Like("2026-09-22T18:00:00Z"),
						"updated_at": matchers.Like("2026-09-22T18:02:00Z"),
					},
					"degradation_reasons": matchers.Like([]interface{}{}),
				},
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
			require.NotNil(t, result.SurfaceStatus)
			require.NotNil(t, result.SurfaceStatus.LatestRun)
			assert.Equal(t, "99999999-8888-7777-6666-555555555555", result.SurfaceStatus.LatestRun.JobID)
			assert.Equal(t, "failed", result.SurfaceStatus.LatestRun.Status)
			assert.Equal(t, "2026-09-22T18:00:00Z", result.SurfaceStatus.LatestRun.KickedAt)
			assert.Equal(t, "2026-09-22T18:02:00Z", result.SurfaceStatus.LatestRun.UpdatedAt)
			return nil
		})
	require.NoError(t, err)
}

func TestGetThreeDayRecapExtractiveGenreContract(t *testing.T) {
	mockProvider := newRecapWorkerPact(t)

	err := mockProvider.
		AddInteraction().
		Given("a completed 3-day recap job with one extractive genre exists").
		UponReceiving("a request for 3-day recap with an extractive genre").
		WithCompleteRequest(consumer.Request{
			Method: "GET",
			Path:   matchers.String("/v1/recaps/3days"),
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonResponseHeaders(),
			Body: map[string]interface{}{
				"job_id":         matchers.Like("11111111-2222-3333-4444-555555555555"),
				"executed_at":    matchers.Like("2026-09-22T17:05:00Z"),
				"window_start":   matchers.Like("2026-09-19T17:00:00Z"),
				"window_end":     matchers.Like("2026-09-22T17:00:00Z"),
				"total_articles": matchers.Like(42),
				"genres": matchers.EachLike(map[string]interface{}{
					"genre":         matchers.Like("technology"),
					"summary":       matchers.Like("Extractive summary excerpt."),
					"top_terms":     matchers.EachLike("ai", 1),
					"article_count": matchers.Like(10),
					"cluster_count": matchers.Like(2),
					"bullets":       matchers.EachLike("Extractive bullet.", 1),
				}, 1),
				"surface_status": map[string]interface{}{
					"generated_at": matchers.Like("2026-09-22T17:05:00Z"),
					"window_start": matchers.Like("2026-09-19T17:00:00Z"),
					"window_end":   matchers.Like("2026-09-22T17:00:00Z"),
					"is_stale":     matchers.Like(false),
					"stale_after":  matchers.Like("2026-09-23T23:05:00Z"),
					"latest_run": map[string]interface{}{
						"job_id":     matchers.Like("11111111-2222-3333-4444-555555555555"),
						"status":     "completed",
						"kicked_at":  matchers.Like("2026-09-22T17:00:00Z"),
						"updated_at": matchers.Like("2026-09-22T17:05:00Z"),
					},
					"degradation_reasons": []interface{}{
						map[string]interface{}{
							"code":   "extractive_genres",
							"detail": matchers.Like("1 of 1 genres are extractive excerpts, not AI summaries"),
						},
					},
				},
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := recap_gateway.NewRecapGatewayWithConfig(nil, fmt.Sprintf("http://%s:%d", config.Host, config.Port))
			result, err := gw.GetThreeDayRecap(context.Background())
			if err != nil {
				return fmt.Errorf("GetThreeDayRecap failed: %w", err)
			}
			require.NotNil(t, result)
			require.NotNil(t, result.SurfaceStatus)
			assert.Equal(t, "2026-09-22T17:05:00Z", result.SurfaceStatus.GeneratedAt)
			require.NotNil(t, result.SurfaceStatus.LatestRun)
			assert.Equal(t, "completed", result.SurfaceStatus.LatestRun.Status)
			require.Len(t, result.SurfaceStatus.DegradationReasons, 1)
			assert.Equal(t, "extractive_genres", result.SurfaceStatus.DegradationReasons[0].Code)
			return nil
		})
	require.NoError(t, err)
}

func TestGetEveningPulseFallbackContract(t *testing.T) {
	mockProvider := newRecapWorkerPact(t)

	err := mockProvider.
		AddInteraction().
		Given("a succeeded evening pulse generation with fallback exists").
		UponReceiving("a request for latest evening pulse with fallback").
		WithCompleteRequest(consumer.Request{
			Method: "GET",
			Path:   matchers.String("/v1/pulse/latest"),
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonResponseHeaders(),
			Body: map[string]interface{}{
				"job_id":       matchers.Like("11111111-2222-3333-4444-555555555555"),
				"date":         matchers.Like("2026-09-22"),
				"generated_at": matchers.Like("2026-09-22T17:05:00Z"),
				"status":       matchers.Like("partial"),
				"topics": matchers.EachLike(map[string]interface{}{
					"cluster_id": matchers.Like(1),
					"role":       matchers.Like("need_to_know"),
					"title":      matchers.Like("Top cluster headline"),
					"rationale": matchers.Like(map[string]interface{}{
						"text":       matchers.Like("Fallback selection"),
						"confidence": matchers.Like("medium"),
					}),
					"article_count": matchers.Like(5),
					"source_count":  matchers.Like(3),
					"time_ago":      matchers.Like("2h ago"),
					"article_ids":   matchers.EachLike("44444444-5555-6666-7777-888888888888", 1),
				}, 1),
				"surface_status": map[string]interface{}{
					"generated_at": matchers.Like("2026-09-22T17:05:00Z"),
					"window_start": matchers.Like("2026-09-19T17:00:00Z"),
					"window_end":   matchers.Like("2026-09-22T17:00:00Z"),
					"is_stale":     matchers.Like(false),
					"stale_after":  matchers.Like("2026-09-23T23:05:00Z"),
					"latest_run": map[string]interface{}{
						"job_id":     matchers.Like("11111111-2222-3333-4444-555555555555"),
						"status":     "completed",
						"kicked_at":  matchers.Like("2026-09-22T17:00:00Z"),
						"updated_at": matchers.Like("2026-09-22T17:05:00Z"),
					},
					"degradation_reasons": []interface{}{
						map[string]interface{}{
							"code":   "pulse_fallback",
							"detail": matchers.Like("fallback level 1"),
						},
					},
				},
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := recap_gateway.NewRecapGatewayWithConfig(nil, fmt.Sprintf("http://%s:%d", config.Host, config.Port))
			result, err := gw.GetEveningPulse(context.Background(), "")
			if err != nil {
				return fmt.Errorf("GetEveningPulse failed: %w", err)
			}
			require.NotNil(t, result)
			require.NotNil(t, result.SurfaceStatus)
			assert.Equal(t, "2026-09-22T17:05:00Z", result.SurfaceStatus.GeneratedAt)
			require.NotNil(t, result.SurfaceStatus.LatestRun)
			assert.Equal(t, "completed", result.SurfaceStatus.LatestRun.Status)
			require.Len(t, result.SurfaceStatus.DegradationReasons, 1)
			assert.Equal(t, "pulse_fallback", result.SurfaceStatus.DegradationReasons[0].Code)
			return nil
		})
	require.NoError(t, err)
}

func TestGetLatestMorningLetterDegradedContract(t *testing.T) {
	mockProvider := newRecapWorkerPact(t)

	err := mockProvider.
		AddInteraction().
		Given("a degraded morning letter version exists").
		UponReceiving("a request for latest morning letter when degraded").
		WithCompleteRequest(consumer.Request{
			Method: "GET",
			Path:   matchers.String("/v1/morning/letters/latest"),
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonResponseHeaders(),
			Body: map[string]interface{}{
				"id":                  matchers.Like("11111111-2222-3333-4444-555555555555"),
				"target_date":         matchers.Like("2026-09-22"),
				"edition_timezone":    matchers.Like("Asia/Tokyo"),
				"is_degraded":         matchers.Like(true),
				"schema_version":      matchers.Like(1),
				"generation_revision": matchers.Like(1),
				"created_at":          matchers.Like("2026-09-22T06:00:00Z"),
				"etag":                matchers.Like("\"11111111-2222-3333-4444-555555555555:1\""),
				"body": map[string]interface{}{
					"lead": matchers.Like("Degraded morning letter lead."),
					"sections": matchers.EachLike(map[string]interface{}{
						"key":     matchers.Like("top3"),
						"title":   matchers.Like("Top Stories"),
						"bullets": matchers.EachLike("Story A", 1),
					}, 1),
					"generated_at": matchers.Like("2026-09-22T06:00:00Z"),
				},
				"surface_status": map[string]interface{}{
					"generated_at": matchers.Like("2026-09-22T06:00:00Z"),
					"window_start": matchers.Like("2026-09-19T06:00:00Z"),
					"window_end":   matchers.Like("2026-09-22T06:00:00Z"),
					"is_stale":     matchers.Like(false),
					"stale_after":  matchers.Like("2026-09-22T09:00:00Z"),
					"latest_run": map[string]interface{}{
						"job_id":     matchers.Like("11111111-2222-3333-4444-555555555555"),
						"status":     "completed",
						"kicked_at":  matchers.Like("2026-09-22T05:55:00Z"),
						"updated_at": matchers.Like("2026-09-22T06:00:00Z"),
					},
					"degradation_reasons": []interface{}{
						map[string]interface{}{
							"code":   "placeholder_output",
							"detail": matchers.Like(""),
						},
					},
				},
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			t.Setenv("RECAP_WORKER_URL", fmt.Sprintf("http://%s:%d", config.Host, config.Port))
			gw := morning_gateway.NewMorningLetterGateway(noopArticleReader{})
			result, err := gw.GetLatestLetter(context.Background())
			if err != nil {
				return fmt.Errorf("GetLatestLetter failed: %w", err)
			}
			require.NotNil(t, result)
			assert.True(t, result.IsDegraded)
			assert.Empty(t, result.Model)
			require.NotNil(t, result.SurfaceStatus)
			assert.Equal(t, "2026-09-22T06:00:00Z", result.SurfaceStatus.GeneratedAt)
			require.NotNil(t, result.SurfaceStatus.LatestRun)
			assert.Equal(t, "completed", result.SurfaceStatus.LatestRun.Status)
			require.Len(t, result.SurfaceStatus.DegradationReasons, 1)
			assert.Equal(t, "placeholder_output", result.SurfaceStatus.DegradationReasons[0].Code)
			return nil
		})
	require.NoError(t, err)
}
