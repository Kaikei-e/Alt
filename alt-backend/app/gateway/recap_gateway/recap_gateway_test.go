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

// newRecapGatewayWithURL creates a RecapGateway with a custom URL for testing
func newRecapGatewayWithURL(url string) *RecapGateway {
	return &RecapGateway{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		recapWorkerURL: url,
	}
}
