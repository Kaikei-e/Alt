package recap_gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"alt/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecapGateway_GetEveningPulse(t *testing.T) {
	t.Run("success - returns pulse data with 3 topics", func(t *testing.T) {
		tier1Count := 5
		trendMultiplier := 4.2
		genre := "Technology"
		pulseData := map[string]any{
			"job_id":       "test-job-123",
			"date":         "2026-01-31",
			"generated_at": "2026-01-31T18:00:00Z",
			"status":       "normal",
			"topics": []map[string]any{
				{
					"cluster_id":    12345,
					"role":          "need_to_know",
					"title":         "日銀、追加利上げを決定",
					"rationale":     map[string]any{"text": "12媒体が報道、Tier1: 5件", "confidence": "high"},
					"article_count": 45,
					"source_count":  12,
					"tier1_count":   tier1Count,
					"time_ago":      "3時間前",
					"genre":         genre,
					"article_ids":   []string{"art-001", "art-002"},
				},
				{
					"cluster_id":       12346,
					"role":             "trend",
					"title":            "新型AIチップの発表で半導体株急騰",
					"rationale":        map[string]any{"text": "3時間で+18件、通常の4.2倍", "confidence": "high"},
					"article_count":    28,
					"source_count":     8,
					"time_ago":         "1時間前",
					"trend_multiplier": trendMultiplier,
					"genre":            "Technology",
					"article_ids":      []string{"art-010"},
				},
				{
					"cluster_id":    12347,
					"role":          "serendipity",
					"title":         "深海で新種の発光生物を発見",
					"rationale":     map[string]any{"text": "普段と異なるジャンル: Science", "confidence": "medium"},
					"article_count": 5,
					"source_count":  3,
					"time_ago":      "5時間前",
					"genre":         "Science",
					"article_ids":   []string{"art-020"},
				},
			},
		}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/v1/pulse/latest", r.URL.Path)
			assert.Equal(t, "2026-01-31", r.URL.Query().Get("date"))
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(pulseData)
		}))
		defer server.Close()

		gw := newRecapGatewayWithURL(server.URL)
		result, err := gw.GetEveningPulse(context.Background(), "2026-01-31")

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, "test-job-123", result.JobID)
		assert.Equal(t, "2026-01-31", result.Date)
		assert.Equal(t, domain.PulseStatusNormal, result.Status)
		assert.Len(t, result.Topics, 3)

		// Verify first topic (NeedToKnow)
		assert.Equal(t, int64(12345), result.Topics[0].ClusterID)
		assert.Equal(t, domain.TopicRoleNeedToKnow, result.Topics[0].Role)
		assert.Equal(t, "日銀、追加利上げを決定", result.Topics[0].Title)
		assert.Equal(t, domain.ConfidenceHigh, result.Topics[0].Rationale.Confidence)
		assert.Equal(t, 45, result.Topics[0].ArticleCount)
		require.NotNil(t, result.Topics[0].Tier1Count)
		assert.Equal(t, 5, *result.Topics[0].Tier1Count)

		// Verify second topic (Trend)
		assert.Equal(t, domain.TopicRoleTrend, result.Topics[1].Role)
		require.NotNil(t, result.Topics[1].TrendMultiplier)
		assert.InDelta(t, 4.2, *result.Topics[1].TrendMultiplier, 0.01)

		// Verify third topic (Serendipity)
		assert.Equal(t, domain.TopicRoleSerendipity, result.Topics[2].Role)
		assert.Equal(t, domain.ConfidenceMedium, result.Topics[2].Rationale.Confidence)
	})

	t.Run("success - returns quiet day", func(t *testing.T) {
		pulseData := map[string]any{
			"job_id":       "quiet-job-456",
			"date":         "2026-01-31",
			"generated_at": "2026-01-31T18:00:00Z",
			"status":       "quiet_day",
			"topics":       []any{},
			"quiet_day": map[string]any{
				"message": "今日は静かな一日でした。特筆すべきニュースは見つかりませんでした。",
				"weekly_highlights": []map[string]any{
					{
						"id":    "highlight-001",
						"title": "今週のトップニュース",
						"date":  "2026-01-29",
						"role":  "need_to_know",
					},
				},
			},
		}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(pulseData)
		}))
		defer server.Close()

		gw := newRecapGatewayWithURL(server.URL)
		result, err := gw.GetEveningPulse(context.Background(), "")

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, domain.PulseStatusQuietDay, result.Status)
		assert.Len(t, result.Topics, 0)
		require.NotNil(t, result.QuietDay)
		assert.Contains(t, result.QuietDay.Message, "静かな一日")
		assert.Len(t, result.QuietDay.WeeklyHighlights, 1)
	})

	t.Run("not found - returns ErrEveningPulseNotFound", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{
				"error": "No evening pulse found for date 2026-01-31",
			})
		}))
		defer server.Close()

		gw := newRecapGatewayWithURL(server.URL)
		_, err := gw.GetEveningPulse(context.Background(), "2026-01-31")

		assert.ErrorIs(t, err, domain.ErrEveningPulseNotFound)
	})

	t.Run("server error - returns error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("internal server error"))
		}))
		defer server.Close()

		gw := newRecapGatewayWithURL(server.URL)
		_, err := gw.GetEveningPulse(context.Background(), "")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "500")
	})

	t.Run("empty date uses no query param", func(t *testing.T) {
		pulseData := map[string]any{
			"job_id":       "test-job",
			"date":         "2026-01-31",
			"generated_at": "2026-01-31T18:00:00Z",
			"status":       "normal",
			"topics":       []any{},
		}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/v1/pulse/latest", r.URL.Path)
			assert.Empty(t, r.URL.Query().Get("date"))
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(pulseData)
		}))
		defer server.Close()

		gw := newRecapGatewayWithURL(server.URL)
		_, err := gw.GetEveningPulse(context.Background(), "")

		require.NoError(t, err)
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

		_, err := gw.GetEveningPulse(ctx, "")

		require.Error(t, err)
	})
}

func TestRecapGateway_GetThreeDayRecapCards(t *testing.T) {
	t.Run("success - returns job and cards", func(t *testing.T) {
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
		}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/v1/recaps/3days/cards", r.URL.Path)
			assert.Equal(t, http.MethodGet, r.Method)
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(mockData); err != nil {
				t.Fatalf("encode mock data failed: %v", err)
			}
		}))
		defer server.Close()

		gw := newRecapGatewayWithURL(server.URL)
		result, err := gw.GetThreeDayRecapCards(context.Background())

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
	})

	t.Run("success - empty when no completed cards job exists", func(t *testing.T) {
		mockData := map[string]any{
			"job":   nil,
			"cards": []any{},
		}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/v1/recaps/3days/cards", r.URL.Path)
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(mockData); err != nil {
				t.Fatalf("encode mock data failed: %v", err)
			}
		}))
		defer server.Close()

		gw := newRecapGatewayWithURL(server.URL)
		result, err := gw.GetThreeDayRecapCards(context.Background())

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Nil(t, result.Job)
		assert.Empty(t, result.Cards)
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
		result, err := gw.GetThreeDayRecapCards(context.Background())

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

		_, err := gw.GetThreeDayRecapCards(ctx)
		require.Error(t, err)
	})
}

// newRecapGatewayWithURL creates a RecapGateway with a custom URL for testing
func newRecapGatewayWithURL(url string) *RecapGateway {
	return NewRecapGatewayWithConfig(nil, url, nil)
}
