package recap_gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecapGateway_GetTopicCards(t *testing.T) {
	t.Run("success - returns job, cards, and completed surface_status", func(t *testing.T) {
		continuesCardID := "44444444-4444-4444-4444-444444444444"
		whyJa := "日本語処理の効率化が期待される。[1]"
		genre := "Technology"
		pubDate := "2026-09-22T10:00:00Z"
		mockData := map[string]any{
			"job": map[string]any{
				"job_id":         "11111111-1111-1111-1111-111111111111",
				"kicked_at":      "2026-09-22T17:00:00Z",
				"from":           "2026-09-19T17:00:00Z",
				"to":             "2026-09-22T17:00:00Z",
				"params_version": "cards-v0.2",
				"cards_selected": 1,
				"degraded":       false,
			},
			"cards": []map[string]any{
				{
					"id":                "22222222-2222-2222-2222-222222222222",
					"rank":              1,
					"story_id":          "33333333-3333-3333-3333-333333333333",
					"continues_card_id": continuesCardID,
					"headline_ja":       "新モデル発表",
					"what_ja":           "推論モデルが公開された。[1]",
					"why_ja":            whyJa,
					"genre":             genre,
					"sources": []map[string]any{
						{
							"n":        1,
							"feed_id":  "55555555-5555-5555-5555-555555555555",
							"url":      "https://example.com/ai",
							"host":     "example.com",
							"title":    "AI News",
							"pub_date": pubDate,
						},
					},
					"created_at": "2026-09-22T17:05:00Z",
				},
			},
			"surface_status": map[string]any{
				"generated_at": "2026-09-22T17:05:00Z",
				"window_start": "2026-09-19T17:00:00Z",
				"window_end":   "2026-09-22T17:00:00Z",
				"is_stale":     false,
				"stale_after":  "2026-09-23T23:05:00Z",
				"latest_run": map[string]any{
					"job_id":     "11111111-1111-1111-1111-111111111111",
					"status":     "completed",
					"kicked_at":  "2026-09-22T17:00:00Z",
					"updated_at": "2026-09-22T17:05:00Z",
				},
				"degradation_reasons": []any{},
			},
		}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/v1/topic-cards", r.URL.Path)
			assert.Equal(t, http.MethodGet, r.Method)
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(mockData); err != nil {
				t.Fatalf("encode mock data failed: %v", err)
			}
		}))
		defer server.Close()

		gw := newRecapGatewayWithURL(server.URL)
		result, err := gw.GetTopicCards(context.Background())

		require.NoError(t, err)
		require.NotNil(t, result)
		require.NotNil(t, result.Job)
		assert.Equal(t, "11111111-1111-1111-1111-111111111111", result.Job.JobID)
		assert.Equal(t, "cards-v0.2", result.Job.ParamsVersion)
		assert.False(t, result.Job.Degraded)
		require.Len(t, result.Cards, 1)
		card := result.Cards[0]
		assert.Equal(t, "22222222-2222-2222-2222-222222222222", card.ID)
		assert.Equal(t, 1, card.Rank)
		assert.Equal(t, "新モデル発表", card.HeadlineJa)
		require.NotNil(t, card.ContinuesCardID)
		assert.Equal(t, continuesCardID, *card.ContinuesCardID)
		require.NotNil(t, card.WhyJa)
		assert.Equal(t, whyJa, *card.WhyJa)
		require.NotNil(t, card.Genre)
		assert.Equal(t, genre, *card.Genre)
		require.Len(t, card.Sources, 1)
		assert.Equal(t, 1, card.Sources[0].N)
		assert.Equal(t, "example.com", card.Sources[0].Host)
		require.NotNil(t, card.Sources[0].PubDate)
		assert.Equal(t, pubDate, *card.Sources[0].PubDate)
		require.NotNil(t, result.SurfaceStatus)
		assert.Equal(t, "2026-09-22T17:05:00Z", result.SurfaceStatus.GeneratedAt)
		require.NotNil(t, result.SurfaceStatus.LatestRun)
		assert.Equal(t, "11111111-1111-1111-1111-111111111111", result.SurfaceStatus.LatestRun.JobID)
		assert.Equal(t, "completed", result.SurfaceStatus.LatestRun.Status)
		assert.Equal(t, "2026-09-22T17:00:00Z", result.SurfaceStatus.LatestRun.KickedAt)
		assert.Equal(t, "2026-09-22T17:05:00Z", result.SurfaceStatus.LatestRun.UpdatedAt)
	})

	t.Run("success - empty when no completed cards job exists and surface_status is null", func(t *testing.T) {
		mockData := map[string]any{
			"job":            nil,
			"cards":          []any{},
			"surface_status": nil,
		}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/v1/topic-cards", r.URL.Path)
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(mockData); err != nil {
				t.Fatalf("encode mock data failed: %v", err)
			}
		}))
		defer server.Close()

		gw := newRecapGatewayWithURL(server.URL)
		result, err := gw.GetTopicCards(context.Background())

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Nil(t, result.Job)
		assert.Empty(t, result.Cards)
		assert.Nil(t, result.SurfaceStatus)
	})

	t.Run("success - returns job and failed latest_run", func(t *testing.T) {
		mockData := map[string]any{
			"job": map[string]any{
				"job_id":         "11111111-1111-1111-1111-111111111111",
				"kicked_at":      "2026-09-22T17:00:00Z",
				"from":           "2026-09-19T17:00:00Z",
				"to":             "2026-09-22T17:00:00Z",
				"params_version": "cards-v0.2",
				"cards_selected": 0,
				"degraded":       false,
			},
			"cards": []any{},
			"surface_status": map[string]any{
				"generated_at": "2026-09-22T17:05:00Z",
				"window_start": "2026-09-19T17:00:00Z",
				"window_end":   "2026-09-22T17:00:00Z",
				"is_stale":     false,
				"stale_after":  "2026-09-23T23:05:00Z",
				"latest_run": map[string]any{
					"job_id":     "99999999-9999-9999-9999-999999999999",
					"status":     "failed",
					"kicked_at":  "2026-09-22T18:00:00Z",
					"updated_at": "2026-09-22T18:02:00Z",
				},
				"degradation_reasons": []any{},
			},
		}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/v1/topic-cards", r.URL.Path)
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(mockData); err != nil {
				t.Fatalf("encode mock data failed: %v", err)
			}
		}))
		defer server.Close()

		gw := newRecapGatewayWithURL(server.URL)
		result, err := gw.GetTopicCards(context.Background())

		require.NoError(t, err)
		require.NotNil(t, result)
		require.NotNil(t, result.Job)
		assert.Equal(t, "11111111-1111-1111-1111-111111111111", result.Job.JobID)
		require.NotNil(t, result.SurfaceStatus)
		require.NotNil(t, result.SurfaceStatus.LatestRun)
		assert.Equal(t, "99999999-9999-9999-9999-999999999999", result.SurfaceStatus.LatestRun.JobID)
		assert.Equal(t, "failed", result.SurfaceStatus.LatestRun.Status)
		assert.Equal(t, "2026-09-22T18:00:00Z", result.SurfaceStatus.LatestRun.KickedAt)
		assert.Equal(t, "2026-09-22T18:02:00Z", result.SurfaceStatus.LatestRun.UpdatedAt)
	})

	t.Run("non-200 status error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			if _, err := w.Write([]byte("internal error")); err != nil {
				t.Fatalf("write response failed: %v", err)
			}
		}))
		defer server.Close()

		gw := newRecapGatewayWithURL(server.URL)
		result, err := gw.GetTopicCards(context.Background())

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "recap-worker returned status 500")
	})

	t.Run("context cancellation", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(100 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		gw := newRecapGatewayWithURL(server.URL)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := gw.GetTopicCards(ctx)
		require.Error(t, err)
	})
}

// newRecapGatewayWithURL creates a RecapGateway with a custom URL for testing
func newRecapGatewayWithURL(url string) *RecapGateway {
	return NewRecapGatewayWithConfig(nil, url)
}
