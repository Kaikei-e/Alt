package knowledge_loop_projector

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"

	"knowledge-sovereign/driver/sovereign_db"
	sovereignv1 "knowledge-sovereign/gen/proto/services/sovereign/v1"
)

// --- in-memory accumulator fake ---------------------------------------------
//
// These methods make *fakeRepo implement the ADR-000939 evidence store. They
// replicate the real driver's ON CONFLICT semantics — newest-32 ring trim,
// last_event_seq guard, pin COALESCE — so the projector's derive path runs for
// real against the same fold the SQL performs (validated against Postgres in
// the migration's integration check).

type evKey struct{ user, kind, ref, sig string }

type evState struct {
	facts          []sovereign_db.KnowledgeLoopEvidenceFact
	factsTotal     int64
	pinnedText     string
	pinnedPayload  []byte
	lastOccurredAt time.Time
	lastEventSeq   int64
}

type relationPatchCall struct {
	EntryKey       string
	EventSeq       int64
	SurfaceBucket  sovereignv1.SurfaceBucket
	PlannerVersion sovereignv1.SurfacePlannerVersion
	Relations      []byte
}

func (f *fakeRepo) UpsertKnowledgeLoopEvidence(_ context.Context, w sovereign_db.KnowledgeLoopEvidenceWrite) error {
	if f.evidenceFailOnApply {
		return fmt.Errorf("forced accumulator write failure")
	}
	if f.evidence == nil {
		f.evidence = map[evKey]*evState{}
	}
	k := evKey{w.UserID.String(), w.ScopeKind, w.ScopeRef, w.SignalKind}
	st, ok := f.evidence[k]
	if !ok {
		st = &evState{lastOccurredAt: w.OccurredAt, lastEventSeq: w.EventSeq}
		if w.NewFact != nil {
			st.facts = []sovereign_db.KnowledgeLoopEvidenceFact{*w.NewFact}
			st.factsTotal = 1
		}
		if w.PinnedText != "" {
			st.pinnedText = w.PinnedText
		}
		if len(w.PinnedPayload) > 0 {
			st.pinnedPayload = append([]byte(nil), w.PinnedPayload...)
		}
		f.evidence[k] = st
		return nil
	}
	// ON CONFLICT seq guard: only advance for a strictly higher event_seq.
	if st.lastEventSeq >= w.EventSeq {
		return nil
	}
	if w.NewFact != nil {
		merged := append(append([]sovereign_db.KnowledgeLoopEvidenceFact{}, st.facts...), *w.NewFact)
		sort.SliceStable(merged, func(i, j int) bool { return merged[i].EventSeq < merged[j].EventSeq })
		if len(merged) > 32 {
			merged = merged[len(merged)-32:]
		}
		st.facts = merged
		st.factsTotal++
	}
	if w.PinnedText != "" {
		st.pinnedText = w.PinnedText
	}
	if len(w.PinnedPayload) > 0 {
		st.pinnedPayload = append([]byte(nil), w.PinnedPayload...)
	}
	if w.OccurredAt.After(st.lastOccurredAt) {
		st.lastOccurredAt = w.OccurredAt
	}
	st.lastEventSeq = w.EventSeq
	return nil
}

func (f *fakeRepo) GetKnowledgeLoopEvidenceForScopes(_ context.Context, userID uuid.UUID, scopes []sovereign_db.KnowledgeLoopEvidenceScope) ([]sovereign_db.KnowledgeLoopEvidenceState, error) {
	if f.evidenceFailOnRead {
		return nil, fmt.Errorf("forced accumulator read failure")
	}
	if userID == uuid.Nil {
		return nil, fmt.Errorf("nil user_id")
	}
	want := make(map[[2]string]bool, len(scopes))
	for _, s := range scopes {
		want[[2]string{s.ScopeKind, s.ScopeRef}] = true
	}
	var out []sovereign_db.KnowledgeLoopEvidenceState
	for k, st := range f.evidence {
		if k.user != userID.String() {
			continue
		}
		if !want[[2]string{k.kind, k.ref}] {
			continue
		}
		out = append(out, sovereign_db.KnowledgeLoopEvidenceState{
			ScopeKind:      k.kind,
			ScopeRef:       k.ref,
			SignalKind:     k.sig,
			Facts:          append([]sovereign_db.KnowledgeLoopEvidenceFact(nil), st.facts...),
			FactsTotal:     st.factsTotal,
			PinnedText:     st.pinnedText,
			PinnedPayload:  st.pinnedPayload,
			LastOccurredAt: st.lastOccurredAt,
			LastEventSeq:   st.lastEventSeq,
		})
	}
	return out, nil
}

func (f *fakeRepo) PatchKnowledgeLoopEntryRelations(_ context.Context, userID, tenantID, lensModeID, entryKey string, eventSeq int64, surfaceBucket sovereignv1.SurfaceBucket, renderDepthHint int32, loopPriority sovereignv1.LoopPriority, plannerVersion sovereignv1.SurfacePlannerVersion, scoreInputs []byte, reviewReason sovereignv1.ReviewReason, relations []byte) (*sovereign_db.KnowledgeLoopUpsertResult, error) {
	f.relationPatchCalls = append(f.relationPatchCalls, relationPatchCall{
		EntryKey: entryKey, EventSeq: eventSeq, SurfaceBucket: surfaceBucket,
		PlannerVersion: plannerVersion, Relations: append([]byte(nil), relations...),
	})
	for _, e := range f.entries {
		if e.UserId == userID && e.TenantId == tenantID && e.LensModeId == lensModeID && e.EntryKey == entryKey {
			if e.ProjectionSeqHiwater > eventSeq {
				return &sovereign_db.KnowledgeLoopUpsertResult{SkippedBySeqHiwater: true}, nil
			}
			e.SurfaceBucket = surfaceBucket
			e.RenderDepthHint = renderDepthHint
			e.LoopPriority = loopPriority
			e.SurfacePlannerVersion = plannerVersion.Enum()
			e.SurfaceScoreInputs = append([]byte(nil), scoreInputs...)
			e.ReviewReason = reviewReason
			e.Relations = append([]byte(nil), relations...)
			e.ProjectionSeqHiwater = eventSeq
			e.SourceEventSeq = eventSeq
			return &sovereign_db.KnowledgeLoopUpsertResult{Applied: true, ProjectionRevision: 7, ProjectionSeqHiwater: eventSeq}, nil
		}
	}
	return &sovereign_db.KnowledgeLoopUpsertResult{SkippedBySeqHiwater: true}, nil
}

// --- vertical truth test v2 (ADR-000939 gate) -------------------------------

// TestRunBatch_VerticalTruth_DenseLogStillCarriesContradiction is the gate the
// production regression demanded. The old resolver read the 7d window
// ORDER BY event_seq ASC LIMIT 256, so at real log density it read only the
// OLDEST 256 events and missed the supersede sitting at the tail — every entry
// projected relations=[] while the unit tests on extractRelations passed.
//
// This seeds a DENSE log (well over any per-query LIMIT) of unrelated churn,
// puts the contradiction fuel (a prior supersede + the projecting summary
// version on the same article) at the TAIL, and asserts the projected entry
// still carries a Contradiction relation. The accumulator makes this pass; the
// old truncating resolver would not.
func TestRunBatch_VerticalTruth_DenseLogStillCarriesContradiction(t *testing.T) {
	userID := uuid.New()
	tenantID := uuid.New()
	base := time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC)

	var events []sovereign_db.KnowledgeEvent
	mk := func(seq int64, etype string, occ time.Time, payload map[string]any, aggID string) sovereign_db.KnowledgeEvent {
		ev := makeEvent(t, etype, seq, userID, payload)
		ev.TenantID = tenantID
		ev.OccurredAt = occ
		ev.AggregateID = aggID
		ev.AggregateType = "article"
		return ev
	}

	// 5,000 unrelated SummaryVersionCreated events for OTHER articles, all
	// inside the 7d window — an order of magnitude past the old LIMIT 256.
	const noise = 5000
	for i := 0; i < noise; i++ {
		seq := int64(1000 + i)
		occ := base.Add(time.Duration(i) * time.Minute)
		aid := fmt.Sprintf("noise-%d", i)
		events = append(events, mk(seq, EventSummaryVersionCreated, occ, map[string]any{
			"entry_key": "article:" + aid, "article_id": aid, "summary_version_id": uuid.NewString(),
		}, "article:"+aid))
	}

	// The contradiction fuel for the article under test, at the TAIL of the log:
	// the original summary, then a supersede, then a fresh summary version that
	// projects the entry. occurred_at is the day after the noise so the window
	// still covers the prior supersede.
	tail := base.Add(noise * time.Minute).Add(time.Hour)
	events = append(events,
		mk(20001, EventSummaryVersionCreated, tail, map[string]any{
			"entry_key": "article:v", "article_id": "art-v", "summary_version_id": uuid.NewString(),
		}, "article:v"),
		mk(20002, EventSummarySuperseded, tail.Add(time.Minute), map[string]any{
			"entry_key": "article:v", "article_id": "art-v",
		}, "article:v"),
		mk(20003, EventSummaryVersionCreated, tail.Add(2*time.Minute), map[string]any{
			"entry_key": "article:v", "article_id": "art-v", "article_title": "Contradicted piece",
			"summary_version_id": uuid.NewString(),
		}, "article:v"),
	)

	repo := &fakeRepo{events: events}
	p := NewProjector(repo, testLogger(), Config{BatchSize: 10000, MaxBatchesPerTick: 1})
	require.NoError(t, p.RunBatch(context.Background()))

	var found *sovereignv1.KnowledgeLoopEntry
	for _, e := range repo.entries {
		if e.EntryKey == "article:v" {
			found = e
		}
	}
	require.NotNil(t, found, "entry for article:v must be projected")
	require.NotEmpty(t, found.Relations,
		"ADR-000939: a superseded entry must carry relations even at production log density — relations=[] was the bug")

	rels := parseRelations(found.Relations)
	var contradiction *Relation
	for i := range rels {
		if rels[i].Kind == RelationKindContradiction {
			contradiction = &rels[i]
		}
	}
	require.NotNil(t, contradiction, "the contradiction fuel must survive the dense log, got %#v", rels)
	require.Equal(t, sovereignv1.SurfaceBucket_SURFACE_BUCKET_CHANGED, found.SurfaceBucket)
	require.Equal(t, sovereignv1.SurfacePlannerVersion_SURFACE_PLANNER_VERSION_V2, found.GetSurfacePlannerVersion())
}

// TestRunBatch_VerticalTruth_ContradictionStateLadder asserts the Contradiction
// relation walks OPEN → ADVANCING → RESOLVED as the user compares the change and
// then accepts it (ADR-000938 loop closure), all derived from the event log.
func TestRunBatch_VerticalTruth_ContradictionStateLadder(t *testing.T) {
	userID := uuid.New()
	tenantID := uuid.New()
	base := time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC)

	mk := func(seq int64, etype string, occ time.Time, payload map[string]any) sovereign_db.KnowledgeEvent {
		ev := makeEvent(t, etype, seq, userID, payload)
		ev.TenantID = tenantID
		ev.OccurredAt = occ
		ev.AggregateID = "article:v"
		ev.AggregateType = "article"
		return ev
	}

	run := func(extra ...sovereign_db.KnowledgeEvent) *sovereignv1.KnowledgeLoopEntry {
		events := []sovereign_db.KnowledgeEvent{
			mk(1, EventSummaryVersionCreated, base, map[string]any{"entry_key": "article:v", "article_id": "art-v", "summary_version_id": uuid.NewString()}),
			mk(2, EventSummarySuperseded, base.Add(time.Hour), map[string]any{"entry_key": "article:v", "article_id": "art-v"}),
		}
		events = append(events, extra...)
		// Re-project the entry last so it reflects the full ladder.
		events = append(events, mk(99, EventSummaryVersionCreated, base.Add(48*time.Hour), map[string]any{
			"entry_key": "article:v", "article_id": "art-v", "summary_version_id": uuid.NewString(),
		}))
		repo := &fakeRepo{events: events}
		require.NoError(t, NewProjector(repo, testLogger(), Config{BatchSize: 1000}).RunBatch(context.Background()))
		// The fake appends every upsert, so the latest projection is the last
		// matching entry (the real driver UPSERTs a single seq-guarded row).
		var latest *sovereignv1.KnowledgeLoopEntry
		for _, e := range repo.entries {
			if e.EntryKey == "article:v" {
				latest = e
			}
		}
		return latest
	}

	stateOf := func(e *sovereignv1.KnowledgeLoopEntry) RelationState {
		for _, r := range parseRelations(e.Relations) {
			if r.Kind == RelationKindContradiction {
				return r.State
			}
		}
		return RelationStateUnspecified
	}

	// OPEN: drift exists, no compare.
	require.Equal(t, RelationStateOpen, stateOf(run()))

	// ADVANCING: a compare act on the entry.
	compare := mk(10, EventKnowledgeLoopActed, base.Add(2*time.Hour), map[string]any{
		"entry_key": "article:v", "acted_intent": "compare",
	})
	require.Equal(t, RelationStateAdvancing, stateOf(run(compare)))

	// RESOLVED: an accepted_change outcome closes the loop.
	accepted := mk(11, EventKnowledgeLoopActOutcome, base.Add(3*time.Hour), map[string]any{
		"entry_key": "article:v", "outcome": "accepted_change",
	})
	require.Equal(t, RelationStateResolved, stateOf(run(compare, accepted)))
}

// TestRunBatch_LateFuel_TagSetVersionCreated asserts a brand-new article's
// entry gains a Cluster relation when a later TagSetVersionCreated reveals it
// shares a tag with another article the user is tracking (ADR-000939 §3,
// immediate re-derivation), instead of staying empty until the next contact.
func TestRunBatch_LateFuel_TagSetVersionCreated(t *testing.T) {
	userID := uuid.New()
	tenantID := uuid.New()
	base := time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC)

	// IDs are consistent the way production emits them: article_id == aggregate
	// id (bare), and entry_key derives to "article:<id>". No explicit entry_key
	// in payload, so SummaryVersionCreated and TagSetVersionCreated agree on the
	// entry scope.
	mk := func(seq int64, etype, aid string, occ time.Time, payload map[string]any) sovereign_db.KnowledgeEvent {
		ev := makeEvent(t, etype, seq, userID, payload)
		ev.TenantID = tenantID
		ev.OccurredAt = occ
		ev.AggregateID = aid
		ev.AggregateType = "article"
		return ev
	}

	events := []sovereign_db.KnowledgeEvent{
		// Another article already tagged "go" — establishes the tracked topic.
		mk(1, EventTagSetVersionCreated, "other", base, map[string]any{
			"article_id": "other", "tag_set_version_id": uuid.NewString(), "tags": []string{"go"},
		}),
		// The article under test is projected fresh (its payload has no tags yet).
		mk(2, EventSummaryVersionCreated, "v", base.Add(time.Minute), map[string]any{
			"article_id": "v", "summary_version_id": uuid.NewString(),
		}),
		// Late fuel: "v" is tagged "go" too, after its entry already exists. The
		// re-derivation sees "other"'s prior activity on tag "go" (a tracked
		// topic) and excludes "v"'s own, so the overlap is > 0.
		mk(3, EventTagSetVersionCreated, "v", base.Add(time.Hour), map[string]any{
			"article_id": "v", "tag_set_version_id": uuid.NewString(), "tags": []string{"go"},
		}),
	}

	repo := &fakeRepo{events: events}
	require.NoError(t, NewProjector(repo, testLogger(), Config{BatchSize: 1000}).RunBatch(context.Background()))

	var found *sovereignv1.KnowledgeLoopEntry
	for _, e := range repo.entries {
		if e.EntryKey == "article:v" {
			found = e
		}
	}
	require.NotNil(t, found)
	require.NotEmpty(t, repo.relationPatchCalls, "late TagSetVersionCreated must patch the entry's relations")

	// The patched relation-set (the latest write to the entry) carries Cluster.
	last := repo.relationPatchCalls[len(repo.relationPatchCalls)-1]
	var hasCluster bool
	for _, r := range parseRelations(last.Relations) {
		if r.Kind == RelationKindCluster {
			hasCluster = true
		}
	}
	require.True(t, hasCluster, "v must gain a Cluster relation from the shared tag, got %#v", parseRelations(last.Relations))
}

// --- fail-loud (ADR-000939 §4 / CLAUDE.md #8) -------------------------------

// A read failure in the derive path must surface as a batch error, never
// degrade to empty inputs (which would look like "no fuel" and empty the Orient
// surface — the PM-2026-045 silent-failure mode).
func TestRunBatch_FailsLoudOnAccumulatorReadError(t *testing.T) {
	userID := uuid.New()
	ev := makeEvent(t, EventSummaryVersionCreated, 100, userID, map[string]any{
		"entry_key": "article:v", "article_id": "art-v", "summary_version_id": uuid.NewString(),
	})
	repo := &fakeRepo{events: []sovereign_db.KnowledgeEvent{ev}, evidenceFailOnRead: true}
	err := NewProjector(repo, testLogger(), Config{BatchSize: 10}).RunBatch(context.Background())
	require.Error(t, err, "an accumulator read error must fail the batch, not degrade to v1")
}

// A write failure in the apply path must likewise surface, not be swallowed.
func TestRunBatch_FailsLoudOnAccumulatorWriteError(t *testing.T) {
	userID := uuid.New()
	ev := makeEvent(t, EventSummaryVersionCreated, 100, userID, map[string]any{
		"entry_key": "article:v", "article_id": "art-v", "summary_version_id": uuid.NewString(),
	})
	repo := &fakeRepo{events: []sovereign_db.KnowledgeEvent{ev}, evidenceFailOnApply: true}
	err := NewProjector(repo, testLogger(), Config{BatchSize: 10}).RunBatch(context.Background())
	require.Error(t, err, "an accumulator write error must fail the batch, not be swallowed")
}

// Honest SLI: relation coverage is labelled by whether the entry carries a
// relation, not by resolver type — what makes a producer dropping evidence
// visible (coverage → 0) instead of hidden behind a ~100% v2 ratio.
func TestObserveRelationCoverage_LabelsByPresence(t *testing.T) {
	beforeTrue := testutil.ToFloat64(relationCoverageTotal.WithLabelValues("true"))
	observeRelationCoverage(true)
	require.Equal(t, beforeTrue+1, testutil.ToFloat64(relationCoverageTotal.WithLabelValues("true")))

	beforeFalse := testutil.ToFloat64(relationCoverageTotal.WithLabelValues("false"))
	observeRelationCoverage(false)
	require.Equal(t, beforeFalse+1, testutil.ToFloat64(relationCoverageTotal.WithLabelValues("false")))
}

func testLogger() *slog.Logger { return slog.New(slog.NewTextHandler(testWriter{}, nil)) }
