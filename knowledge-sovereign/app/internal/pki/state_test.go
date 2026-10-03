package pki

import (
	"testing"
	"time"
)

func TestClassifyRemaining(t *testing.T) {
	nb := time.Date(2026, 4, 16, 0, 0, 0, 0, time.UTC)
	na := nb.Add(24 * time.Hour)
	cases := []struct {
		name     string
		now      time.Time
		fraction float64
		want     State
	}{
		{"fresh at start", nb.Add(1 * time.Hour), 0.66, StateFresh},
		{"fresh just before threshold", nb.Add(15 * time.Hour), 0.66, StateFresh},
		{"near_expiry at 2/3", nb.Add(16 * time.Hour), 0.66, StateNearExpiry},
		{"near_expiry past threshold", nb.Add(20 * time.Hour), 0.66, StateNearExpiry},
		{"expired at not_after", na, 0.66, StateExpired},
		{"expired after not_after", na.Add(time.Minute), 0.66, StateExpired},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ClassifyRemaining(nb, na, c.now, c.fraction); got != c.want {
				t.Fatalf("got %s want %s", got, c.want)
			}
		})
	}
}

func TestClassifyRemaining_ZeroWindow(t *testing.T) {
	nb := time.Date(2026, 4, 16, 0, 0, 0, 0, time.UTC)
	if got := ClassifyRemaining(nb, nb, nb.Add(-time.Second), 0.66); got != StateExpired {
		t.Fatalf("got %s", got)
	}
}
