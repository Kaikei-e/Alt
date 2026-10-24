package projection_health

import (
	"context"
	"errors"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
)

type fakeRepo struct {
	ages map[string]float64
	err  error
}

func (f fakeRepo) GetKnowledgeEventLastOccurrenceAges(_ context.Context, _ []string) (map[string]float64, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.ages, nil
}

func TestRunOnce_NeverSeenProducerReadsAsStale(t *testing.T) {
	// recap.topic_snapshotted.v1 and augur.conversation_linked.v1 absent from
	// the ages map = never emitted (the production bug). They must publish a
	// very large age, not vanish.
	repo := fakeRepo{
		ages: map[string]float64{"SummaryVersionCreated": 30},
	}
	require.NoError(t, New(repo, nil).RunOnce(context.Background()))

	recapAge := testutil.ToFloat64(eventLastOccurrenceAge.WithLabelValues("recap.topic_snapshotted.v1"))
	require.Equal(t, neverSeenAgeSeconds, recapAge,
		"a producer that never emitted must read as extremely stale, not as an absent series")
	augurAge := testutil.ToFloat64(eventLastOccurrenceAge.WithLabelValues("augur.conversation_linked.v1"))
	require.Equal(t, neverSeenAgeSeconds, augurAge,
		"augur.conversation_linked.v1 that never emitted must read as extremely stale")
	require.Equal(t, 30.0, testutil.ToFloat64(eventLastOccurrenceAge.WithLabelValues("SummaryVersionCreated")))
}

// TestRunOnce_RecentStaleAndNeverSeenAreDistinguishable pins the full trio the
// PM-2026-045 gate reads, so a cheaper liveness query underneath cannot quietly
// change what the gauge means. A stale producer must publish its real age — not
// the never-seen sentinel and not a value clamped to some lookback window —
// because the alert joins a climbing age against a fresh SummaryVersionCreated
// to tell "the producer died" from "no usage".
func TestRunOnce_RecentStaleAndNeverSeenAreDistinguishable(t *testing.T) {
	const fortyDays = 40 * 24 * 3600.0
	repo := fakeRepo{ages: map[string]float64{
		"SummaryVersionCreated":        30,        // recent
		"SummarySuperseded":            fortyDays, // stale, far beyond any window
		"TagSetVersionCreated":         30,
		"augur.conversation_linked.v1": 120,
		// recap.topic_snapshotted.v1 absent = never seen
	}}
	require.NoError(t, New(repo, nil).RunOnce(context.Background()))

	require.Contains(t, WatchedEventTypes, "augur.conversation_linked.v1")
	require.Equal(t, 30.0, testutil.ToFloat64(eventLastOccurrenceAge.WithLabelValues("SummaryVersionCreated")))
	require.Equal(t, 30.0, testutil.ToFloat64(eventLastOccurrenceAge.WithLabelValues("TagSetVersionCreated")))
	require.Equal(t, 120.0, testutil.ToFloat64(eventLastOccurrenceAge.WithLabelValues("augur.conversation_linked.v1")))
	require.Equal(t, fortyDays, testutil.ToFloat64(eventLastOccurrenceAge.WithLabelValues("SummarySuperseded")),
		"a stale producer must keep its exact age, distinct from the never-seen sentinel")
	require.Equal(t, neverSeenAgeSeconds, testutil.ToFloat64(eventLastOccurrenceAge.WithLabelValues("recap.topic_snapshotted.v1")))
	require.NotEqual(t, neverSeenAgeSeconds, testutil.ToFloat64(eventLastOccurrenceAge.WithLabelValues("SummarySuperseded")),
		"collapsing stale into the never-seen sentinel would change what the alert means")
}

func TestRunOnce_RepositoryErrorPropagates(t *testing.T) {
	repoErr := errors.New("db connection refused")
	repo := fakeRepo{err: repoErr}
	err := New(repo, nil).RunOnce(context.Background())
	require.Error(t, err)
	require.ErrorIs(t, err, repoErr)
}
