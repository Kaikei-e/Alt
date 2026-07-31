package sovereign_db

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRetentionPolicy_PartitionsEligibleForArchive(t *testing.T) {
	policy := DefaultRetentionPolicy()

	t.Run("partitions within hot window are not eligible", func(t *testing.T) {
		now := time.Date(2026, 3, 25, 0, 0, 0, 0, time.UTC)
		partitions := []PartitionInfo{
			{Name: "knowledge_events_y2026m03", RangeStart: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)},
		}
		eligible := policy.PartitionsEligibleForArchive("knowledge_events", partitions, now)
		assert.Empty(t, eligible)
	})

	t.Run("partitions beyond hot window are eligible for archive", func(t *testing.T) {
		now := time.Date(2026, 3, 25, 0, 0, 0, 0, time.UTC)
		partitions := []PartitionInfo{
			{Name: "knowledge_events_y2025m11", RangeStart: time.Date(2025, 11, 1, 0, 0, 0, 0, time.UTC)},
			{Name: "knowledge_events_y2025m12", RangeStart: time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC)},
			{Name: "knowledge_events_y2026m01", RangeStart: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
			{Name: "knowledge_events_y2026m02", RangeStart: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)},
			{Name: "knowledge_events_y2026m03", RangeStart: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)},
		}
		eligible := policy.PartitionsEligibleForArchive("knowledge_events", partitions, now)
		// system events hot = 30 days, so anything before 2026-02-23 is eligible
		// 2025-11, 2025-12, 2026-01 are fully before that date
		require.Len(t, eligible, 3)
		assert.Equal(t, "knowledge_events_y2025m11", eligible[0].Name)
		assert.Equal(t, "knowledge_events_y2025m12", eligible[1].Name)
		assert.Equal(t, "knowledge_events_y2026m01", eligible[2].Name)
	})

	t.Run("user events have shorter hot window", func(t *testing.T) {
		now := time.Date(2026, 3, 25, 0, 0, 0, 0, time.UTC)
		partitions := []PartitionInfo{
			{Name: "knowledge_user_events_y2026m02", RangeStart: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)},
			{Name: "knowledge_user_events_y2026m03", RangeStart: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)},
		}
		eligible := policy.PartitionsEligibleForArchive("knowledge_user_events", partitions, now)
		// user events hot = 7 days, so anything before 2026-03-18 is eligible
		// 2026-02 is fully before that date
		require.Len(t, eligible, 1)
		assert.Equal(t, "knowledge_user_events_y2026m02", eligible[0].Name)
	})
}

// partitionFromBoundExpr builds a PartitionInfo the same way ListPartitions
// does, so the table below drives the real pg_get_expr(relpartbound, ...)
// strings PostgreSQL reports rather than hand-picked timestamps.
func partitionFromBoundExpr(name, boundExpr string) PartitionInfo {
	p := PartitionInfo{Name: name}
	p.RangeStart, p.RangeEnd, p.UnboundedUpper = parseBoundExpr(boundExpr)
	return p
}

func TestParseBoundExpr(t *testing.T) {
	date := func(y int, m time.Month, d int) time.Time {
		return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	}

	tests := []struct {
		name          string
		expr          string
		wantStart     time.Time
		wantEnd       time.Time
		wantUnbounded string
	}{
		{
			name:      "bounded monthly range",
			expr:      "FOR VALUES FROM ('2026-03-01 00:00:00+00') TO ('2026-04-01 00:00:00+00')",
			wantStart: date(2026, 3, 1),
			wantEnd:   date(2026, 4, 1),
		},
		{
			name:          "DEFAULT partition has no bounds",
			expr:          "DEFAULT",
			wantUnbounded: UnboundedUpperDefault,
		},
		{
			name:    "MINVALUE lower bound leaves the start zero",
			expr:    "FOR VALUES FROM (MINVALUE) TO ('2026-02-01 00:00:00+00')",
			wantEnd: date(2026, 2, 1),
		},
		{
			name:          "MAXVALUE upper bound has no end",
			expr:          "FOR VALUES FROM ('2026-01-01 00:00:00+00') TO (MAXVALUE)",
			wantStart:     date(2026, 1, 1),
			wantUnbounded: UnboundedUpperMaxValue,
		},
		{
			name:          "non-range bound expression is unreadable",
			expr:          "FOR VALUES IN ('a', 'b')",
			wantUnbounded: UnboundedUpperUnreadable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, end, unbounded := parseBoundExpr(tt.expr)
			assert.Equal(t, tt.wantStart, start)
			assert.Equal(t, tt.wantEnd, end)
			assert.Equal(t, tt.wantUnbounded, unbounded)
		})
	}
}

func TestRetentionPolicy_PartitionsEligibleForArchive_BoundExprs(t *testing.T) {
	policy := DefaultRetentionPolicy()
	// system events hot = 30 days, so the cutoff is 2026-05-02
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name         string
		boundExpr    string
		wantEligible bool
	}{
		{
			name:         "bounded range entirely before the cutoff is eligible",
			boundExpr:    "FOR VALUES FROM ('2026-01-01 00:00:00+00') TO ('2026-02-01 00:00:00+00')",
			wantEligible: true,
		},
		{
			name:         "bounded range reaching into the hot window is not eligible",
			boundExpr:    "FOR VALUES FROM ('2026-05-01 00:00:00+00') TO ('2026-06-01 00:00:00+00')",
			wantEligible: false,
		},
		{
			name:         "DEFAULT partition is never eligible",
			boundExpr:    "DEFAULT",
			wantEligible: false,
		},
		{
			name:         "MINVALUE lower bound with an old upper bound is eligible",
			boundExpr:    "FOR VALUES FROM (MINVALUE) TO ('2026-02-01 00:00:00+00')",
			wantEligible: true,
		},
		{
			name:         "MAXVALUE upper bound is never eligible",
			boundExpr:    "FOR VALUES FROM ('2026-01-01 00:00:00+00') TO (MAXVALUE)",
			wantEligible: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			part := partitionFromBoundExpr("knowledge_events_part", tt.boundExpr)
			eligible := policy.PartitionsEligibleForArchive("knowledge_events", []PartitionInfo{part}, now)
			if tt.wantEligible {
				require.Len(t, eligible, 1)
				assert.Equal(t, "knowledge_events_part", eligible[0].Name)
				return
			}
			assert.Empty(t, eligible)
		})
	}
}

func TestRetentionPolicy_DefaultValues(t *testing.T) {
	p := DefaultRetentionPolicy()

	t.Run("system events hot window is 30 days", func(t *testing.T) {
		assert.Equal(t, 30*24*time.Hour, p.SystemEventsHot)
	})

	t.Run("user events hot window is 7 days", func(t *testing.T) {
		assert.Equal(t, 7*24*time.Hour, p.UserEventsHot)
	})

	t.Run("superseded versions hot window is 30 days", func(t *testing.T) {
		assert.Equal(t, 30*24*time.Hour, p.SupersededVersionsHot)
	})
}
