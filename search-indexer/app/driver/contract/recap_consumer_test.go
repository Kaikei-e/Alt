//go:build contract

package contract

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/pact-foundation/pact-go/v2/consumer"
	"github.com/pact-foundation/pact-go/v2/matchers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const pactDir = "../../../../pacts"

// IndexableGenresResponse mirrors recap_api.IndexableGenresResponse for contract testing.
type IndexableGenresResponse struct {
	Results []RecapGenreItem `json:"results"`
	HasMore bool             `json:"has_more"`
}

// RecapGenreItem mirrors recap_api.RecapGenreItem for contract testing.
type RecapGenreItem struct {
	JobID      string   `json:"job_id"`
	ExecutedAt string   `json:"executed_at"`
	WindowDays int      `json:"window_days"`
	Genre      string   `json:"genre"`
	Summary    string   `json:"summary"`
	TopTerms   []string `json:"top_terms"`
	Tags       []string `json:"tags"`
	Bullets    []string `json:"bullets"`
}

func newRecapWorkerPact(t *testing.T) *consumer.V3HTTPMockProvider {
	t.Helper()
	mockProvider, err := consumer.NewV3Pact(consumer.MockHTTPProviderConfig{
		Consumer: "search-indexer",
		Provider: "recap-worker",
		PactDir:  filepath.Join(pactDir),
	})
	require.NoError(t, err)
	return mockProvider
}

func TestRecapWorkerIndexableGenresContract(t *testing.T) {
	mockProvider := newRecapWorkerPact(t)

	err := mockProvider.
		AddInteraction().
		Given("indexable recap genres exist").
		UponReceiving("a request for indexable recap genres").
		WithCompleteRequest(consumer.Request{
			Method: "GET",
			Path:   matchers.String("/v1/recaps/genres/indexable"),
			Query: matchers.MapMatcher{
				"limit": matchers.String("100"),
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status: 200,
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"results": matchers.EachLike(matchers.MapMatcher{
					"job_id":      matchers.Like("job-001"),
					"executed_at": matchers.Like("2026-03-26T00:00:00Z"),
					"window_days": matchers.Like(7),
					"genre":       matchers.Like("technology"),
					"summary":     matchers.Like("Weekly tech summary."),
					"top_terms":   matchers.EachLike(matchers.Like("AI"), 1),
					"tags":        matchers.EachLike(matchers.Like("technology"), 1),
					"bullets":     matchers.EachLike(matchers.Like("AI continues to advance."), 1),
				}, 1),
				"has_more": matchers.Like(false),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			apiURL := fmt.Sprintf("http://%s:%d/v1/recaps/genres/indexable?limit=100", config.Host, config.Port)

			req, err := http.NewRequestWithContext(context.Background(), "GET", apiURL, nil)
			if err != nil {
				return fmt.Errorf("failed to create request: %w", err)
			}

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return fmt.Errorf("request failed: %w", err)
			}
			defer resp.Body.Close()

			body, _ := io.ReadAll(resp.Body)
			var apiResp IndexableGenresResponse
			if err := json.Unmarshal(body, &apiResp); err != nil {
				return fmt.Errorf("failed to parse response: %w", err)
			}

			assert.Equal(t, http.StatusOK, resp.StatusCode)
			assert.NotEmpty(t, apiResp.Results)
			assert.NotEmpty(t, apiResp.Results[0].Genre)
			return nil
		})
	require.NoError(t, err)
}
