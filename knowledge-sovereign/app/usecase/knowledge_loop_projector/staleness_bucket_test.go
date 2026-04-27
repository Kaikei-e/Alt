package knowledge_loop_projector

import (
	"testing"
	"time"
)

// pureStalenessBucket pins the boundaries between buckets so a future change
// is forced to bump WhyMappingVersion + run a full reproject. The function is
// canonical-contract reproject-safe: deterministic in its arguments.

func TestPureStalenessBucket_Boundaries(t *testing.T) {
	t.Parallel()
	occurred := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		name string
		gap  time.Duration
		want uint32
	}{
		{"zero gap is fresh", 0, 0},
		{"30 minutes is fresh", 30 * time.Minute, 0},
		{"59m59s is fresh", 59*time.Minute + 59*time.Second, 0},
		{"exactly 1h is current", time.Hour, 1},
		{"23h is current", 23 * time.Hour, 1},
		{"exactly 1d is recent", 24 * time.Hour, 2},
		{"6d is recent", 6 * 24 * time.Hour, 2},
		{"exactly 7d is aging", 7 * 24 * time.Hour, 3},
		{"29d is aging", 29 * 24 * time.Hour, 3},
		{"exactly 30d is stale", 30 * 24 * time.Hour, 4},
		{"100d is stale", 100 * 24 * time.Hour, 4},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			source := occurred.Add(-c.gap)
			got := pureStalenessBucket(occurred, source)
			if got != c.want {
				t.Errorf("gap %v: got %d want %d", c.gap, got, c.want)
			}
		})
	}
}

func TestPureStalenessBucket_ZeroSourceTreatedAsFresh(t *testing.T) {
	t.Parallel()
	got := pureStalenessBucket(time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC), time.Time{})
	if got != 0 {
		t.Errorf("zero sourceObservedAt: got %d want 0 (fresh)", got)
	}
}

func TestPureStalenessBucket_NegativeGapTreatedAsFresh(t *testing.T) {
	t.Parallel()
	occurred := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)
	source := occurred.Add(2 * time.Hour) // source AFTER event
	got := pureStalenessBucket(occurred, source)
	if got != 0 {
		t.Errorf("negative gap: got %d want 0 (fresh fallback)", got)
	}
}

// TestPureStalenessBucket_DeterministicWithoutWallClock pins the canonical
// contract reproject-safe property: replay must produce the same bucket
// regardless of when the test runs. We call the function many times with
// the same fixed arguments and assert no drift.
func TestPureStalenessBucket_DeterministicWithoutWallClock(t *testing.T) {
	t.Parallel()
	occurred := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	source := occurred.Add(-3 * 24 * time.Hour)
	first := pureStalenessBucket(occurred, source)
	for i := range 100 {
		got := pureStalenessBucket(occurred, source)
		if got != first {
			t.Fatalf("non-deterministic on iteration %d: got %d, first was %d", i, got, first)
		}
	}
}
