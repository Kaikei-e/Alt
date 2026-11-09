//go:build contract

// Pact CDC: alt-backend → recap-worker (Morning Letter).
package contract

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"alt/domain"
	"alt/orchestrator/gateway/morning_gateway"

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
)

type noopArticleReader struct{}

func (noopArticleReader) FetchArticlesByIDs(_ context.Context, _ []uuid.UUID) ([]*domain.Article, error) {
	return nil, nil
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
				"model":               matchers.Like("gemma4-e4b-12k"),
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
