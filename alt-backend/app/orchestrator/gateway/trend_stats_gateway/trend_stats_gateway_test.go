package trend_stats_gateway

import (
	"alt/shared/gateway/datahub_gateway"
	"testing"
	"time"
)

func TestMapTrendSeriesToResponse(t *testing.T) {
	now := time.Now()
	series := &datahub_gateway.TrendSeries{
		Points: []datahub_gateway.TrendPoint{
			{
				Timestamp:    now,
				Articles:     10,
				Summarized:   8,
				FeedActivity: 2,
			},
		},
		Granularity: "1h",
	}

	res := mapTrendSeriesToResponse(series, "24h")
	if res == nil {
		t.Fatal("expected non-nil response")
	}
	if res.Window != "24h" || res.Granularity != "1h" || len(res.DataPoints) != 1 {
		t.Fatalf("unexpected response structure: %+v", res)
	}
	if res.DataPoints[0].Articles != 10 || res.DataPoints[0].Summarized != 8 || res.DataPoints[0].FeedActivity != 2 {
		t.Errorf("unexpected data point: %+v", res.DataPoints[0])
	}
}
