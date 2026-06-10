package projection_health

import (
	"context"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"

	"knowledge-sovereign/driver/sovereign_db"
)

type fakeRepo struct {
	cov  sovereign_db.KnowledgeLoopRelationCoverage
	ages map[string]float64
}

func (f fakeRepo) GetKnowledgeLoopRelationCoverage24h(context.Context) (sovereign_db.KnowledgeLoopRelationCoverage, error) {
	return f.cov, nil
}

func (f fakeRepo) GetKnowledgeEventLastOccurrenceAges(_ context.Context, _ []string) (map[string]float64, error) {
	return f.ages, nil
}

func TestRunOnce_PublishesHonestCoverageRatio(t *testing.T) {
	repo := fakeRepo{
		cov:  sovereign_db.KnowledgeLoopRelationCoverage{Total: 200, WithRelations: 10},
		ages: map[string]float64{"SummaryVersionCreated": 60},
	}
	require.NoError(t, New(repo, nil).RunOnce(context.Background()))

	require.InDelta(t, 0.05, testutil.ToFloat64(relationCoverageRatio24h), 1e-9,
		"ratio is with_relations/total computed from DB truth, not a rate over an idle window")
	require.Equal(t, 200.0, testutil.ToFloat64(entries24h))
}

func TestRunOnce_ZeroEntriesYieldsZeroRatioNotNaN(t *testing.T) {
	repo := fakeRepo{cov: sovereign_db.KnowledgeLoopRelationCoverage{Total: 0, WithRelations: 0}}
	require.NoError(t, New(repo, nil).RunOnce(context.Background()))
	require.Equal(t, 0.0, testutil.ToFloat64(relationCoverageRatio24h),
		"an idle window must not divide by zero; the entries_24h guard keeps the alert quiet")
	require.Equal(t, 0.0, testutil.ToFloat64(entries24h))
}

func TestRunOnce_NeverSeenProducerReadsAsStale(t *testing.T) {
	// recap.topic_snapshotted.v1 absent from the ages map = never emitted (the
	// production bug). It must publish a very large age, not vanish.
	repo := fakeRepo{
		cov:  sovereign_db.KnowledgeLoopRelationCoverage{Total: 1, WithRelations: 1},
		ages: map[string]float64{"SummaryVersionCreated": 30},
	}
	require.NoError(t, New(repo, nil).RunOnce(context.Background()))

	recapAge := testutil.ToFloat64(eventLastOccurrenceAge.WithLabelValues("recap.topic_snapshotted.v1"))
	require.Equal(t, neverSeenAgeSeconds, recapAge,
		"a producer that never emitted must read as extremely stale, not as an absent series")
	require.Equal(t, 30.0, testutil.ToFloat64(eventLastOccurrenceAge.WithLabelValues("SummaryVersionCreated")))
}
