package knowledge_loop_projector

import (
	"context"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"

	"knowledge-sovereign/driver/sovereign_db"
)

// ADR-000937 + CLAUDE.md #8: a nil score resolver means DI failed to wire it.
// The projector must fail loud (panic) rather than silently substituting the
// Null resolver, which would empty every Orient surface while looking
// "intentionally disabled" — the PM-2026-045 / ADR-000928 failure mode.
func TestResolveBucket_PanicsWhenScoreResolverUnwired(t *testing.T) {
	p := &Projector{} // scoreResolver deliberately nil — simulates a DI wiring bug
	require.Panics(t, func() {
		_ = p.resolveBucket(context.Background(), &sovereign_db.KnowledgeEvent{})
	})
}

// The Null resolver is a legitimate state (tests, a v1-only deployment with no
// cross-source evidence). It must NOT panic — only an unwired (nil) resolver is
// a bug.
func TestResolveBucket_NullResolverDoesNotPanic(t *testing.T) {
	p := &Projector{scoreResolver: NullSurfaceScoreResolver{}}
	require.NotPanics(t, func() {
		_ = p.resolveBucket(context.Background(), &sovereign_db.KnowledgeEvent{})
	})
}

// Honest SLI: relation coverage is labelled by whether the entry carries a
// relation, not by resolver type. This is what makes a producer dropping
// evidence visible (coverage → 0) instead of hidden behind a ~100% v2 ratio.
func TestObserveRelationCoverage_LabelsByPresence(t *testing.T) {
	beforeTrue := testutil.ToFloat64(relationCoverageTotal.WithLabelValues("true"))
	observeRelationCoverage(true)
	require.Equal(t, beforeTrue+1, testutil.ToFloat64(relationCoverageTotal.WithLabelValues("true")))

	beforeFalse := testutil.ToFloat64(relationCoverageTotal.WithLabelValues("false"))
	observeRelationCoverage(false)
	require.Equal(t, beforeFalse+1, testutil.ToFloat64(relationCoverageTotal.WithLabelValues("false")))
}
