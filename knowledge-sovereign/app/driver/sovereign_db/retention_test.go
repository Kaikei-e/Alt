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
