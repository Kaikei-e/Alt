package knowledge_loop_projector

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"knowledge-sovereign/driver/sovereign_db"
	sovereignv1 "knowledge-sovereign/gen/proto/services/sovereign/v1"
)

// ADR-000938 gate #1 — the vertical-truth test. It runs the REAL projector with
// the REAL EventLogSurfaceScoreResolver over a seeded event log (a superseded
// article chain) and asserts the projected entry carries a non-empty
// relation-set with a Contradiction relation. This is the test that would have
// turned the production `relations=[]` regression RED before merge: the unit
// tests on extractRelations passed while the end-to-end projection produced
// empty relations because the resolver fuel never reached entry.Relations.
func TestRunBatch_VerticalTruth_SupersededEntryCarriesContradictionRelation(t *testing.T) {
	userID := uuid.New()

	// The contradiction fuel: a prior supersede on the same article inside the
	// resolver's 7-day window.
	prior := makeEvent(t, EventSummarySuperseded, 290, userID, map[string]any{
		"entry_key":  "article:v",
		"article_id": "art-v",
	})
	prior.AggregateID = "article:v"
	prior.OccurredAt = prior.OccurredAt.Add(-2 * time.Hour)

	// The projecting event: a fresh summary version for the same article.
	target := makeEvent(t, EventSummaryVersionCreated, 300, userID, map[string]any{
		"entry_key":     "article:v",
		"article_id":    "art-v",
		"article_title": "A piece that was contradicted",
	})
	target.AggregateID = "article:v"

	repo := &fakeRepo{events: []sovereign_db.KnowledgeEvent{prior, target}}
	p := newProjector(repo).WithScoreResolver(NewEventLogSurfaceScoreResolver(repo))
	require.NoError(t, p.RunBatch(context.Background()))

	var found *sovereignv1.KnowledgeLoopEntry
	for _, e := range repo.entries {
		if e.EntryKey == "article:v" {
			found = e
		}
	}
	require.NotNil(t, found, "entry for article:v must be projected")
	require.NotEmpty(t, found.Relations,
		"ADR-000938: a superseded entry must carry a non-empty relation-set — relations=[] was the production bug")

	rels := parseRelations(found.Relations)
	var hasContradiction bool
	for _, r := range rels {
		if r.Kind == RelationKindContradiction {
			hasContradiction = true
		}
	}
	require.True(t, hasContradiction,
		"the contradiction fuel the resolver computed must reach entry.Relations, got %#v", rels)
}
