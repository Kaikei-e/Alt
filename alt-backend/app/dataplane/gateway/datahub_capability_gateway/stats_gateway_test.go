package datahub_capability_gateway

import (
	"reflect"
	"testing"
	"time"

	"alt/dataplane/port/datahub_capability_port"
	"alt/shared/driver/alt_db"
)

func TestTrendSeriesFromDriver(t *testing.T) {
	now := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name  string
		stats *alt_db.TrendStats
		want  *datahub_capability_port.TrendSeries
	}{
		{
			name: "empty rows with granularity",
			stats: &alt_db.TrendStats{
				Granularity: "daily",
				Rows:        []alt_db.TrendStatsRow{},
			},
			want: &datahub_capability_port.TrendSeries{
				Granularity: "daily",
				Points:      []datahub_capability_port.TrendPoint{},
			},
		},
		{
			name: "multiple rows mapped accurately",
			stats: &alt_db.TrendStats{
				Granularity: "hourly",
				Rows: []alt_db.TrendStatsRow{
					{
						Bucket:       now,
						Articles:     5,
						Summarized:   3,
						FeedActivity: 10,
					},
					{
						Bucket:       now.Add(time.Hour),
						Articles:     8,
						Summarized:   6,
						FeedActivity: 12,
					},
				},
			},
			want: &datahub_capability_port.TrendSeries{
				Granularity: "hourly",
				Points: []datahub_capability_port.TrendPoint{
					{
						Timestamp:    now,
						Articles:     5,
						Summarized:   3,
						FeedActivity: 10,
					},
					{
						Timestamp:    now.Add(time.Hour),
						Articles:     8,
						Summarized:   6,
						FeedActivity: 12,
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := trendSeriesFromDriver(tt.stats)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("trendSeriesFromDriver() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
