package projection_health

import (
	"context"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
)

type fakeRepo struct {
	ages map[string]float64
}

func (f fakeRepo) GetKnowledgeEventLastOccurrenceAges(_ context.Context, _ []string) (map[string]float64, error) {
	return f.ages, nil
}

func TestRunOnce_NeverSeenProducerReadsAsStale(t *testing.T) {
	// recap.topic_snapshotted.v1 absent from the ages map = never emitted (the
	// production bug). It must publish a very large age, not vanish.
	repo := fakeRepo{
		ages: map[string]float64{"SummaryVersionCreated": 30},
	}
	require.NoError(t, New(repo, nil).RunOnce(context.Background()))

	recapAge := testutil.ToFloat64(eventLastOccurrenceAge.WithLabelValues("recap.topic_snapshotted.v1"))
	require.Equal(t, neverSeenAgeSeconds, recapAge,
		"a producer that never emitted must read as extremely stale, not as an absent series")
	require.Equal(t, 30.0, testutil.ToFloat64(eventLastOccurrenceAge.WithLabelValues("SummaryVersionCreated")))
}
