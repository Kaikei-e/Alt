package knowledge_loop_projector

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"knowledge-sovereign/driver/sovereign_db"
	sovereignv1 "knowledge-sovereign/gen/proto/services/sovereign/v1"
)

// fakeRepo records the upsert calls and returns canned results. It does not
// simulate the DB-side seq-hiwater guard; that guard is exercised at the driver
// layer's own tests. Here we verify the projector emits the same upsert payload
// for the same event on replay — the core reproject-safety invariant.
type fakeRepo struct {
	checkpoint     int64
	events         []sovereign_db.KnowledgeEvent
	entries        []*sovereignv1.KnowledgeLoopEntry
	sessions       []*sovereignv1.KnowledgeLoopSessionState
	entrySessions  []entrySessionCall
	surfaces       []*sovereignv1.KnowledgeLoopSurface
	patches        []patchCall
	dismissPatches []dismissPatchCall
	surfacePatches []surfacePlanPatchCall
	checkpoints    []int64
}

// patchCall records the arguments to PatchKnowledgeLoopEntryWhy so tests can
// assert that the patch path was invoked with the right shape and that the
// upsert path was NOT invoked (which would clobber dismiss_state).
type patchCall struct {
	UserID, TenantID, LensModeID, EntryKey string
	EventSeq                               int64
	Why                                    *sovereignv1.KnowledgeLoopWhyPayload
}

// dismissPatchCall records arguments to PatchKnowledgeLoopEntryDismissState so
// the Deferred projector branch can be asserted in isolation: the test verifies
// that exactly the dismiss_state column was patched, and that the broader
// UpsertKnowledgeLoopEntry path (which would clobber freshness/why) was NOT
// invoked alongside.
type dismissPatchCall struct {
	UserID, TenantID, LensModeID, EntryKey string
	EventSeq                               int64
	DismissState                           sovereignv1.DismissState
}

// surfacePlanPatchCall records the arguments to PatchKnowledgeLoopEntrySurfacePlan
// so SurfacePlanRecomputed can be asserted as a patch-only projector branch.
type surfacePlanPatchCall struct {
	UserID, TenantID, LensModeID, EntryKey string
	EventSeq                               int64
	SurfaceBucket                          sovereignv1.SurfaceBucket
	RenderDepthHint                        int32
	LoopPriority                           sovereignv1.LoopPriority
	PlannerVersion                         sovereignv1.SurfacePlannerVersion
	ScoreInputs                            []byte
}

type entrySessionCall struct {
	UserID, TenantID, LensModeID, EntryKey string
	CurrentStage                           sovereignv1.LoopStage
	CurrentStageEnteredAt                  time.Time
	EventSeq                               int64
}

func (f *fakeRepo) ListKnowledgeEventsSince(ctx context.Context, afterSeq int64, limit int) ([]sovereign_db.KnowledgeEvent, error) {
	out := make([]sovereign_db.KnowledgeEvent, 0)
	for _, e := range f.events {
		if e.EventSeq > afterSeq {
			out = append(out, e)
			if len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}

func (f *fakeRepo) GetProjectionCheckpoint(ctx context.Context, _ string) (int64, error) {
	return f.checkpoint, nil
}

func (f *fakeRepo) UpdateProjectionCheckpoint(ctx context.Context, _ string, lastSeq int64) error {
	f.checkpoint = lastSeq
	f.checkpoints = append(f.checkpoints, lastSeq)
	return nil
}

func (f *fakeRepo) UpsertKnowledgeLoopEntry(ctx context.Context, e *sovereignv1.KnowledgeLoopEntry) (*sovereign_db.KnowledgeLoopUpsertResult, error) {
	f.entries = append(f.entries, e)
	return &sovereign_db.KnowledgeLoopUpsertResult{Applied: true, ProjectionRevision: 1, ProjectionSeqHiwater: e.ProjectionSeqHiwater}, nil
}

func (f *fakeRepo) UpsertKnowledgeLoopSessionState(ctx context.Context, s *sovereignv1.KnowledgeLoopSessionState) (*sovereign_db.KnowledgeLoopUpsertResult, error) {
	f.sessions = append(f.sessions, s)
	return &sovereign_db.KnowledgeLoopUpsertResult{Applied: true, ProjectionRevision: 1, ProjectionSeqHiwater: s.ProjectionSeqHiwater}, nil
}

func (f *fakeRepo) UpsertKnowledgeLoopEntrySessionState(ctx context.Context, userID, tenantID, lensModeID, entryKey string, currentStage sovereignv1.LoopStage, currentStageEnteredAt time.Time, eventSeq int64) (*sovereign_db.KnowledgeLoopUpsertResult, error) {
	f.entrySessions = append(f.entrySessions, entrySessionCall{
		UserID: userID, TenantID: tenantID, LensModeID: lensModeID, EntryKey: entryKey,
		CurrentStage: currentStage, CurrentStageEnteredAt: currentStageEnteredAt, EventSeq: eventSeq,
	})
	return &sovereign_db.KnowledgeLoopUpsertResult{Applied: true, ProjectionRevision: 1, ProjectionSeqHiwater: eventSeq}, nil
}

func (f *fakeRepo) UpsertKnowledgeLoopSurface(ctx context.Context, s *sovereignv1.KnowledgeLoopSurface) (*sovereign_db.KnowledgeLoopUpsertResult, error) {
	f.surfaces = append(f.surfaces, s)
	return &sovereign_db.KnowledgeLoopUpsertResult{Applied: true, ProjectionRevision: 1, ProjectionSeqHiwater: s.ProjectionSeqHiwater}, nil
}

func (f *fakeRepo) GetKnowledgeLoopEntries(ctx context.Context, filter sovereign_db.GetKnowledgeLoopEntriesFilter) ([]*sovereignv1.KnowledgeLoopEntry, error) {
	out := make([]*sovereignv1.KnowledgeLoopEntry, 0)
	for _, e := range f.entries {
		if e.UserId != filter.UserID.String() || e.TenantId != filter.TenantID.String() || e.LensModeId != filter.LensModeID {
			continue
		}
		if filter.SurfaceBucket != nil && e.SurfaceBucket != *filter.SurfaceBucket {
			continue
		}
		if !filter.IncludeDismissed && e.VisibilityState != sovereignv1.LoopVisibilityState_LOOP_VISIBILITY_STATE_VISIBLE {
			continue
		}
		out = append(out, e)
		if filter.Limit > 0 && len(out) >= filter.Limit {
			break
		}
	}
	return out, nil
}

func (f *fakeRepo) PatchKnowledgeLoopEntryWhy(ctx context.Context, userID, tenantID, lensModeID, entryKey string, eventSeq int64, why *sovereignv1.KnowledgeLoopWhyPayload) (*sovereign_db.KnowledgeLoopUpsertResult, error) {
	f.patches = append(f.patches, patchCall{
		UserID: userID, TenantID: tenantID, LensModeID: lensModeID,
		EntryKey: entryKey, EventSeq: eventSeq, Why: why,
	})
	return &sovereign_db.KnowledgeLoopUpsertResult{
		Applied: true, ProjectionRevision: 2, ProjectionSeqHiwater: eventSeq,
	}, nil
}

func (f *fakeRepo) PatchKnowledgeLoopEntryDismissState(ctx context.Context, userID, tenantID, lensModeID, entryKey string, eventSeq int64, dismissState sovereignv1.DismissState) (*sovereign_db.KnowledgeLoopUpsertResult, error) {
	f.dismissPatches = append(f.dismissPatches, dismissPatchCall{
		UserID: userID, TenantID: tenantID, LensModeID: lensModeID,
		EntryKey: entryKey, EventSeq: eventSeq, DismissState: dismissState,
	})
	for _, e := range f.entries {
		if e.UserId == userID && e.TenantId == tenantID && e.LensModeId == lensModeID && e.EntryKey == entryKey {
			e.DismissState = dismissState
			switch dismissState {
			case sovereignv1.DismissState_DISMISS_STATE_ACTIVE:
				e.VisibilityState = sovereignv1.LoopVisibilityState_LOOP_VISIBILITY_STATE_VISIBLE
				e.CompletionState = sovereignv1.LoopCompletionState_LOOP_COMPLETION_STATE_OPEN
			case sovereignv1.DismissState_DISMISS_STATE_DEFERRED:
				e.VisibilityState = sovereignv1.LoopVisibilityState_LOOP_VISIBILITY_STATE_SNOOZED
				e.CompletionState = sovereignv1.LoopCompletionState_LOOP_COMPLETION_STATE_OPEN
			case sovereignv1.DismissState_DISMISS_STATE_DISMISSED:
				e.VisibilityState = sovereignv1.LoopVisibilityState_LOOP_VISIBILITY_STATE_HIDDEN
				e.CompletionState = sovereignv1.LoopCompletionState_LOOP_COMPLETION_STATE_DISMISSED
			case sovereignv1.DismissState_DISMISS_STATE_COMPLETED:
				e.VisibilityState = sovereignv1.LoopVisibilityState_LOOP_VISIBILITY_STATE_HIDDEN
				e.CompletionState = sovereignv1.LoopCompletionState_LOOP_COMPLETION_STATE_COMPLETED
			}
			e.ProjectionSeqHiwater = eventSeq
		}
	}
	return &sovereign_db.KnowledgeLoopUpsertResult{
		Applied: true, ProjectionRevision: 3, ProjectionSeqHiwater: eventSeq,
	}, nil
}

func (f *fakeRepo) PatchKnowledgeLoopEntrySurfacePlan(ctx context.Context, userID, tenantID, lensModeID, entryKey string, eventSeq int64, surfaceBucket sovereignv1.SurfaceBucket, renderDepthHint int32, loopPriority sovereignv1.LoopPriority, plannerVersion sovereignv1.SurfacePlannerVersion, scoreInputs []byte) (*sovereign_db.KnowledgeLoopUpsertResult, error) {
	f.surfacePatches = append(f.surfacePatches, surfacePlanPatchCall{
		UserID: userID, TenantID: tenantID, LensModeID: lensModeID,
		EntryKey: entryKey, EventSeq: eventSeq, SurfaceBucket: surfaceBucket,
		RenderDepthHint: renderDepthHint, LoopPriority: loopPriority,
		PlannerVersion: plannerVersion, ScoreInputs: append([]byte(nil), scoreInputs...),
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
			e.ProjectionSeqHiwater = eventSeq
			e.SourceEventSeq = eventSeq
			return &sovereign_db.KnowledgeLoopUpsertResult{
				Applied: true, ProjectionRevision: 4, ProjectionSeqHiwater: eventSeq,
			}, nil
		}
	}
	return &sovereign_db.KnowledgeLoopUpsertResult{SkippedBySeqHiwater: true}, nil
}

func newProjector(repo Repository) *Projector {
	return NewProjector(repo, slog.New(slog.NewTextHandler(testWriter{}, nil)), Config{BatchSize: 100})
}

type testWriter struct{}

func (testWriter) Write(p []byte) (int, error) { return len(p), nil }

func makeEvent(t *testing.T, eventType string, seq int64, userID uuid.UUID, payload map[string]any) sovereign_db.KnowledgeEvent {
	t.Helper()
	body, err := json.Marshal(payload)
	require.NoError(t, err)
	return sovereign_db.KnowledgeEvent{
		EventID:       uuid.New(),
		EventSeq:      seq,
		OccurredAt:    time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC),
		TenantID:      uuid.New(),
		UserID:        &userID,
		EventType:     eventType,
		AggregateType: "article",
		AggregateID:   "article:42",
		DedupeKey:     eventType + ":" + uuid.NewString(),
		Payload:       body,
	}
}

func TestRunBatch_HomeItemOpened_ProjectsEntryAndSession(t *testing.T) {
	userID := uuid.New()
	ev := makeEvent(t, EventHomeItemOpened, 100, userID, map[string]any{
		"entry_key":   "article:42",
		"opened_at":   "2026-04-26T09:57:00Z",
		"action_type": "open",
	})

	repo := &fakeRepo{events: []sovereign_db.KnowledgeEvent{ev}}
	p := newProjector(repo)
	require.NoError(t, p.RunBatch(context.Background()))

	require.Len(t, repo.entries, 1, "one entry upsert expected")
	require.Len(t, repo.sessions, 1, "one session upsert expected")

	entry := repo.entries[0]
	require.Equal(t, sovereignv1.LoopStage_LOOP_STAGE_ACT, entry.ProposedStage)
	require.Equal(t, sovereignv1.SurfaceBucket_SURFACE_BUCKET_CONTINUE, entry.SurfaceBucket)
	require.Equal(t, sovereignv1.DismissState_DISMISS_STATE_COMPLETED, entry.DismissState)
	require.Equal(t, sovereignv1.LoopVisibilityState_LOOP_VISIBILITY_STATE_VISIBLE, entry.VisibilityState)
	require.Equal(t, sovereignv1.LoopCompletionState_LOOP_COMPLETION_STATE_COMPLETED, entry.CompletionState)
	require.Equal(t, ev.OccurredAt.UTC(), entry.FreshnessAt.AsTime().UTC(),
		"freshness_at must come from event occurred_at, not wall-clock")
	require.NotNil(t, entry.SourceObservedAt)
	require.Equal(t, time.Date(2026, 4, 26, 9, 57, 0, 0, time.UTC), entry.SourceObservedAt.AsTime().UTC(),
		"source_observed_at must come from payload interaction time")
	require.NotEmpty(t, entry.ContinueContext, "Continue entries must carry resume context")
	var ctxPayload struct {
		Summary            string   `json:"summary"`
		RecentActionLabels []string `json:"recent_action_labels"`
		LastInteractedAt   string   `json:"last_interacted_at"`
	}
	require.NoError(t, json.Unmarshal(entry.ContinueContext, &ctxPayload))
	require.Equal(t, "Last opened before this loop pass.", ctxPayload.Summary)
	require.Equal(t, []string{"open"}, ctxPayload.RecentActionLabels)
	require.Equal(t, "2026-04-26T09:57:00Z", ctxPayload.LastInteractedAt)

	state := repo.sessions[0]
	require.Equal(t, sovereignv1.LoopStage_LOOP_STAGE_ACT, state.CurrentStage)
	require.Equal(t, ev.OccurredAt.UTC(), state.CurrentStageEnteredAt.AsTime().UTC(),
		"current_stage_entered_at must come from event occurred_at, not wall-clock")

	require.Equal(t, int64(100), repo.checkpoint, "checkpoint advances to last processed seq")
}

func TestRunBatch_HomeItemOpened_RemainsVisibleInContinueSurface(t *testing.T) {
	userID := uuid.New()
	ev := makeEvent(t, EventHomeItemOpened, 110, userID, map[string]any{"entry_key": "article:continue-visible"})

	repo := &fakeRepo{events: []sovereign_db.KnowledgeEvent{ev}}
	p := newProjector(repo)
	require.NoError(t, p.RunBatch(context.Background()))

	var continueSurface *sovereignv1.KnowledgeLoopSurface
	for _, s := range repo.surfaces {
		if s.SurfaceBucket == sovereignv1.SurfaceBucket_SURFACE_BUCKET_CONTINUE {
			continueSurface = s
			break
		}
	}
	require.NotNil(t, continueSurface)
	require.NotNil(t, continueSurface.PrimaryEntryKey)
	require.Equal(t, "article:continue-visible", *continueSurface.PrimaryEntryKey,
		"HomeItemOpened carries completion metadata but must remain read-visible in Continue")
	require.JSONEq(t, `{"active_count":1}`, string(continueSurface.LoopHealth))
}

func TestRunBatch_HomeItemDismissed_RemainsVisibleInReviewSurface(t *testing.T) {
	userID := uuid.New()
	ev := makeEvent(t, EventHomeItemDismissed, 120, userID, map[string]any{
		"entry_key":   "article:review-visible",
		"opened_at":   "2026-04-26T09:58:00Z",
		"action_type": "dismiss",
	})

	repo := &fakeRepo{events: []sovereign_db.KnowledgeEvent{ev}}
	p := newProjector(repo)
	require.NoError(t, p.RunBatch(context.Background()))

	entry := repo.entries[0]
	require.Equal(t, sovereignv1.SurfaceBucket_SURFACE_BUCKET_REVIEW, entry.SurfaceBucket)
	require.Equal(t, sovereignv1.DismissState_DISMISS_STATE_DISMISSED, entry.DismissState)
	require.Equal(t, sovereignv1.LoopVisibilityState_LOOP_VISIBILITY_STATE_VISIBLE, entry.VisibilityState)
	require.Equal(t, sovereignv1.LoopCompletionState_LOOP_COMPLETION_STATE_DISMISSED, entry.CompletionState)
	require.NotNil(t, entry.SourceObservedAt)
	require.Equal(t, time.Date(2026, 4, 26, 9, 58, 0, 0, time.UTC), entry.SourceObservedAt.AsTime().UTC())
	require.Contains(t, entry.WhyPrimary.Text, "Dismissed")
	require.NotEmpty(t, entry.WhyPrimary.EvidenceRefs)

	var reviewSurface *sovereignv1.KnowledgeLoopSurface
	for _, s := range repo.surfaces {
		if s.SurfaceBucket == sovereignv1.SurfaceBucket_SURFACE_BUCKET_REVIEW {
			reviewSurface = s
			break
		}
	}
	require.NotNil(t, reviewSurface)
	require.NotNil(t, reviewSurface.PrimaryEntryKey)
	require.Equal(t, "article:review-visible", *reviewSurface.PrimaryEntryKey,
		"HomeItemDismissed is Review work, not a read-path hide")
}

func TestRunBatch_SummaryVersionCreated_ObserveSeed(t *testing.T) {
	userID := uuid.New()
	ev := makeEvent(t, EventSummaryVersionCreated, 200, userID, map[string]any{
		"summary_version_id": "sv-1",
		"article_title":      "A Talk on Distributed Systems",
	})
	repo := &fakeRepo{events: []sovereign_db.KnowledgeEvent{ev}}
	p := newProjector(repo)
	require.NoError(t, p.RunBatch(context.Background()))

	require.Len(t, repo.entries, 1)
	entry := repo.entries[0]
	require.Equal(t, sovereignv1.LoopStage_LOOP_STAGE_OBSERVE, entry.ProposedStage)
	require.Contains(t, entry.WhyPrimary.Text, "A Talk on Distributed Systems",
		"narrative must inline the article title for real context")

	var seeded []map[string]string
	require.NoError(t, json.Unmarshal(entry.DecisionOptions, &seeded))
	intents := make([]string, 0, len(seeded))
	for _, s := range seeded {
		intents = append(intents, s["intent"])
	}
	require.Equal(t, []string{"revisit", "ask", "snooze"}, intents,
		"Observe entries must propose §7-allowed transitions; observe → act is forbidden")

	var targets []map[string]string
	require.NoError(t, json.Unmarshal(entry.ActTargets, &targets))
	require.Equal(t, []map[string]string{{
		"target_type": "article",
		"target_ref":  "article:42",
		"route":       "/articles/article:42",
	}}, targets)
}

func TestRunBatch_WithRealScoreResolverMarksPlannerV2AndStoresInputs(t *testing.T) {
	userID := uuid.New()
	ev := makeEvent(t, EventSummaryVersionCreated, 210, userID, map[string]any{
		"entry_key":     "article:42",
		"article_title": "Planner evidence",
		"tags":          []string{"ai"},
	})
	repo := &fakeRepo{events: []sovereign_db.KnowledgeEvent{ev}}
	p := newProjector(repo).WithScoreResolver(fakeResolver{out: SurfaceScoreInputs{
		TopicOverlapCount: 1,
	}})

	require.NoError(t, p.RunBatch(context.Background()))
	require.Len(t, repo.entries, 1)

	entry := repo.entries[0]
	require.Equal(t, sovereignv1.SurfacePlannerVersion_SURFACE_PLANNER_VERSION_V2, entry.GetSurfacePlannerVersion())
	require.JSONEq(t,
		`{"topic_overlap_count":1,"tag_overlap_count":0,"has_augur_link":false,"version_drift_count":0,"has_open_interaction":false,"freshness_at":"2026-04-26T10:00:00Z","event_type":"SummaryVersionCreated"}`,
		string(entry.SurfaceScoreInputs),
	)
}

func TestRunBatch_NoUserIDIsNoOp(t *testing.T) {
	ev := sovereign_db.KnowledgeEvent{
		EventID:    uuid.New(),
		EventSeq:   50,
		OccurredAt: time.Now().UTC(),
		EventType:  EventArticleCreated,
		UserID:     nil,
		Payload:    json.RawMessage(`{}`),
	}
	repo := &fakeRepo{events: []sovereign_db.KnowledgeEvent{ev}}
	p := newProjector(repo)
	require.NoError(t, p.RunBatch(context.Background()))
	require.Empty(t, repo.entries)
	require.Empty(t, repo.sessions)
	require.Equal(t, int64(50), repo.checkpoint, "checkpoint still advances past skipped events")
}

func TestRunBatch_ReplayIsIdempotent(t *testing.T) {
	userID := uuid.New()
	ev := makeEvent(t, EventHomeItemOpened, 300, userID, map[string]any{"entry_key": "article:42"})

	repoA := &fakeRepo{events: []sovereign_db.KnowledgeEvent{ev}}
	require.NoError(t, newProjector(repoA).RunBatch(context.Background()))

	repoB := &fakeRepo{events: []sovereign_db.KnowledgeEvent{ev}}
	require.NoError(t, newProjector(repoB).RunBatch(context.Background()))

	require.Equal(t, len(repoA.entries), len(repoB.entries))
	require.Equal(t, repoA.entries[0].EntryKey, repoB.entries[0].EntryKey)
	require.Equal(t, repoA.entries[0].FreshnessAt.AsTime(), repoB.entries[0].FreshnessAt.AsTime(),
		"reproject must produce identical freshness_at from event.occurred_at")
	require.Equal(t, repoA.entries[0].WhyPrimary.Text, repoB.entries[0].WhyPrimary.Text,
		"reproject must produce identical why_text from event payload alone")
}

func TestSeedDecisionOptions_StageAppropriate(t *testing.T) {
	cases := []struct {
		stage   sovereignv1.LoopStage
		intents []string
	}{
		{sovereignv1.LoopStage_LOOP_STAGE_OBSERVE, []string{"revisit", "ask", "snooze"}},
		{sovereignv1.LoopStage_LOOP_STAGE_ORIENT, []string{"compare", "ask", "snooze"}},
		{sovereignv1.LoopStage_LOOP_STAGE_DECIDE, []string{"open", "save", "ask"}},
		{sovereignv1.LoopStage_LOOP_STAGE_ACT, []string{"revisit", "ask"}},
	}
	for _, tc := range cases {
		raw := seedDecisionOptions(tc.stage)
		require.NotEmpty(t, raw, "stage %s must produce a seed", tc.stage)
		var seeded []map[string]string
		require.NoError(t, json.Unmarshal(raw, &seeded))
		got := make([]string, 0, len(seeded))
		for _, s := range seeded {
			got = append(got, s["intent"])
		}
		require.Equal(t, tc.intents, got, "stage %s seed intents", tc.stage)
	}
}

func TestRunBatch_SummaryNarrativeBackfilled_PatchesWhyOnly(t *testing.T) {
	// ADR-000846: discovered event repairs historic entries' why_text via the
	// patch path, NOT the full UPSERT. dismiss_state and every other field
	// must remain the projector's responsibility — preserved by the dedicated
	// patch SQL.
	userID := uuid.New()
	ev := makeEvent(t, EventSummaryNarrativeBackfilled, 400, userID, map[string]any{
		"summary_version_id": "sv-bf-1",
		"article_id":         "art-bf-1",
		"article_title":      "Discovered Title",
	})
	repo := &fakeRepo{events: []sovereign_db.KnowledgeEvent{ev}}
	require.NoError(t, newProjector(repo).RunBatch(context.Background()))

	require.Empty(t, repo.entries,
		"backfill must NOT call UpsertKnowledgeLoopEntry — the full upsert "+
			"would clobber dismiss_state and other entry fields")
	require.Empty(t, repo.sessions,
		"backfill must NOT touch session state")
	require.Len(t, repo.patches, 1, "patch path must be invoked exactly once")

	patch := repo.patches[0]
	require.Equal(t, userID.String(), patch.UserID)
	require.Equal(t, ev.TenantID.String(), patch.TenantID)
	require.Equal(t, defaultLensModeID, patch.LensModeID)
	require.Equal(t, "article:article:42", patch.EntryKey,
		"entry_key derives from aggregate_type + aggregate_id; the test event "+
			"uses the shared makeEvent fixture which sets aggregateID=\"article:42\"")
	require.Equal(t, int64(400), patch.EventSeq)
	require.NotNil(t, patch.Why)
	require.Contains(t, patch.Why.Text, "Discovered Title",
		"discovered article_title flows through enrichSummaryVersion's title "+
			"branch, producing a narrative that inlines the title")
	require.Contains(t, patch.Why.Text, "fresh summary ready to read",
		"narrative shape matches enrichSummaryVersion (the enricher dispatches "+
			"on event type — adding the new case must reuse the same shape)")
}

func TestRunBatch_SummaryNarrativeBackfilled_NoUserIDIsNoOp(t *testing.T) {
	ev := sovereign_db.KnowledgeEvent{
		EventID:    uuid.New(),
		EventSeq:   500,
		OccurredAt: time.Now().UTC(),
		EventType:  EventSummaryNarrativeBackfilled,
		UserID:     nil,
		Payload:    json.RawMessage(`{"article_title":"X"}`),
	}
	repo := &fakeRepo{events: []sovereign_db.KnowledgeEvent{ev}}
	require.NoError(t, newProjector(repo).RunBatch(context.Background()))
	require.Empty(t, repo.patches)
	require.Empty(t, repo.entries)
}

func TestRunBatch_EmptyBatchIsNoOp(t *testing.T) {
	repo := &fakeRepo{}
	p := newProjector(repo)
	require.NoError(t, p.RunBatch(context.Background()))
	require.Empty(t, repo.entries)
	require.Empty(t, repo.sessions)
	require.Equal(t, int64(0), repo.checkpoint)
}

// TestRunBatch_KnowledgeLoopDeferred_FlipsDismissState pins the persistence
// fix for the dismiss bug: the canonical contract §8.2 Deferred event must
// flip the entry's dismiss_state to DEFERRED via the patch path. The full
// UpsertKnowledgeLoopEntry path must NOT run for this event — that would
// re-seed why_text / freshness / decision_options from the (sparse) Deferred
// payload, clobbering the existing entry. Session state still updates so
// `last_deferred_entry_key` reflects the user's action.
func TestRunBatch_KnowledgeLoopDeferred_FlipsDismissState(t *testing.T) {
	userID := uuid.New()
	ev := makeEvent(t, EventKnowledgeLoopDeferred, 600, userID, map[string]any{
		"entry_key":    "article:42",
		"lens_mode_id": "default",
		"from_stage":   "LOOP_STAGE_OBSERVE",
		"to_stage":     "LOOP_STAGE_OBSERVE",
		"trigger":      "TRANSITION_TRIGGER_DEFER",
	})
	repo := &fakeRepo{events: []sovereign_db.KnowledgeEvent{ev}}

	require.NoError(t, newProjector(repo).RunBatch(context.Background()))

	// The Deferred branch must call PatchKnowledgeLoopEntryDismissState exactly
	// once with the event's seq so the driver's seq-hiwater guard makes replay
	// idempotent.
	require.Len(t, repo.dismissPatches, 1, "Deferred event must flip dismiss_state via patch path")
	patch := repo.dismissPatches[0]
	require.Equal(t, "article:42", patch.EntryKey)
	require.Equal(t, "default", patch.LensModeID)
	require.Equal(t, int64(600), patch.EventSeq)
	require.Equal(t, sovereignv1.DismissState_DISMISS_STATE_DEFERRED, patch.DismissState)
	require.Equal(t, userID.String(), patch.UserID)

	// Critically: the projector MUST NOT run the full upsert path for Deferred
	// events. Doing so would overwrite freshness_at / why / decision_options
	// from the sparse Deferred payload (canonical contract §3 immutable invariants).
	require.Empty(t, repo.entries, "Deferred must not call UpsertKnowledgeLoopEntry — that would clobber other fields")

	// Session state still tracks last_deferred_entry_key so /loop UI can
	// reflect the user's deferral action.
	require.Len(t, repo.sessions, 1)
	require.NotNil(t, repo.sessions[0].LastDeferredEntryKey)
	require.Equal(t, "article:42", *repo.sessions[0].LastDeferredEntryKey)
}

func TestRunBatch_KnowledgeLoopReviewed_UsesTriggerForDismissState(t *testing.T) {
	userID := uuid.New()
	cases := []struct {
		name    string
		trigger string
		want    sovereignv1.DismissState
	}{
		{"recheck re-arms", "TRANSITION_TRIGGER_RECHECK", sovereignv1.DismissState_DISMISS_STATE_ACTIVE},
		{"archive completes", "TRANSITION_TRIGGER_ARCHIVE", sovereignv1.DismissState_DISMISS_STATE_COMPLETED},
		{"mark reviewed completes", "TRANSITION_TRIGGER_MARK_REVIEWED", sovereignv1.DismissState_DISMISS_STATE_COMPLETED},
	}

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ev := makeEvent(t, EventKnowledgeLoopReviewed, int64(700+i), userID, map[string]any{
				"entry_key":    "article:42",
				"lens_mode_id": "default",
				"from_stage":   "LOOP_STAGE_OBSERVE",
				"to_stage":     "LOOP_STAGE_OBSERVE",
				"trigger":      tc.trigger,
			})
			repo := &fakeRepo{events: []sovereign_db.KnowledgeEvent{ev}}

			require.NoError(t, newProjector(repo).RunBatch(context.Background()))

			require.Len(t, repo.dismissPatches, 1)
			require.Equal(t, tc.want, repo.dismissPatches[0].DismissState)
		})
	}
}

func TestRunBatch_TransitionUpdatesEntrySessionWithoutChangingEntryProposal(t *testing.T) {
	userID := uuid.New()
	ev := makeEvent(t, EventKnowledgeLoopOriented, 800, userID, map[string]any{
		"entry_key":    "article:42",
		"lens_mode_id": "default",
		"from_stage":   "LOOP_STAGE_OBSERVE",
		"to_stage":     "LOOP_STAGE_ORIENT",
		"trigger":      "TRANSITION_TRIGGER_USER_TAP",
	})
	repo := &fakeRepo{events: []sovereign_db.KnowledgeEvent{ev}}

	require.NoError(t, newProjector(repo).RunBatch(context.Background()))

	require.Empty(t, repo.entries, "transition events must not mutate KnowledgeLoopEntry.proposed_stage")
	require.Len(t, repo.entrySessions, 1)
	entryState := repo.entrySessions[0]
	require.Equal(t, "article:42", entryState.EntryKey)
	require.Equal(t, sovereignv1.LoopStage_LOOP_STAGE_ORIENT, entryState.CurrentStage)
	require.Equal(t, ev.OccurredAt.UTC(), entryState.CurrentStageEnteredAt.UTC())
	require.Equal(t, int64(800), entryState.EventSeq)
}

func TestRunBatch_SummaryVersionCreated_RecomputesSurface(t *testing.T) {
	userID := uuid.New()
	ev := makeEvent(t, EventSummaryVersionCreated, 900, userID, map[string]any{
		"entry_key":          "article:surface-1",
		"article_id":         "art-surface-1",
		"article_title":      "Surface Candidate",
		"summary_version_id": "sv-surface-1",
	})
	repo := &fakeRepo{events: []sovereign_db.KnowledgeEvent{ev}}

	require.NoError(t, newProjector(repo).RunBatch(context.Background()))

	require.NotEmpty(t, repo.surfaces)
	var nowSurface *sovereignv1.KnowledgeLoopSurface
	for _, s := range repo.surfaces {
		if s.SurfaceBucket == sovereignv1.SurfaceBucket_SURFACE_BUCKET_NOW {
			nowSurface = s
			break
		}
	}
	require.NotNil(t, nowSurface)
	require.NotNil(t, nowSurface.PrimaryEntryKey)
	require.Equal(t, "article:surface-1", *nowSurface.PrimaryEntryKey)
	require.JSONEq(t, `{"active_count":1}`, string(nowSurface.LoopHealth))
}

func TestRunBatch_SurfacePlanRecomputed_PatchesEntryAndRecomputesSurfaces(t *testing.T) {
	userID := uuid.New()
	ev := makeEvent(t, EventKnowledgeLoopSurfacePlanRecomputed, 950, userID, map[string]any{
		"lens_mode_id":    "default",
		"planner_version": "SURFACE_PLANNER_VERSION_V2",
		"plan_seq":        1,
		"entry_inputs": []map[string]any{{
			"entry_key":            "article:replan-1",
			"version_drift_count":  1,
			"freshness_at":         "2026-04-26T10:00:00Z",
			"recap_topic_snapshot": "ignored-by-test",
		}},
	})
	v1 := sovereignv1.SurfacePlannerVersion_SURFACE_PLANNER_VERSION_V1
	repo := &fakeRepo{
		events: []sovereign_db.KnowledgeEvent{ev},
		entries: []*sovereignv1.KnowledgeLoopEntry{{
			UserId:                userID.String(),
			TenantId:              ev.TenantID.String(),
			LensModeId:            "default",
			EntryKey:              "article:replan-1",
			SourceItemKey:         "article:replan-1",
			ProposedStage:         sovereignv1.LoopStage_LOOP_STAGE_OBSERVE,
			SurfaceBucket:         sovereignv1.SurfaceBucket_SURFACE_BUCKET_NOW,
			ProjectionSeqHiwater:  100,
			SourceEventSeq:        100,
			FreshnessAt:           timestamppb.New(ev.OccurredAt.Add(-time.Hour)),
			DismissState:          sovereignv1.DismissState_DISMISS_STATE_ACTIVE,
			VisibilityState:       sovereignv1.LoopVisibilityState_LOOP_VISIBILITY_STATE_VISIBLE,
			CompletionState:       sovereignv1.LoopCompletionState_LOOP_COMPLETION_STATE_OPEN,
			RenderDepthHint:       pickRenderDepth(sovereignv1.SurfaceBucket_SURFACE_BUCKET_NOW),
			LoopPriority:          pickLoopPriority(sovereignv1.SurfaceBucket_SURFACE_BUCKET_NOW),
			SurfacePlannerVersion: &v1,
			SurfaceScoreInputs:    []byte(`{"event_type":"SummaryVersionCreated"}`),
		}},
	}

	require.NoError(t, newProjector(repo).RunBatch(context.Background()))

	require.Len(t, repo.entries, 1, "replan must not use full entry upsert")
	require.Len(t, repo.surfacePatches, 1, "replan must patch existing entries")
	patch := repo.surfacePatches[0]
	require.Equal(t, "article:replan-1", patch.EntryKey)
	require.Equal(t, int64(950), patch.EventSeq)
	require.Equal(t, sovereignv1.SurfaceBucket_SURFACE_BUCKET_CHANGED, patch.SurfaceBucket)
	require.Equal(t, sovereignv1.SurfacePlannerVersion_SURFACE_PLANNER_VERSION_V2, patch.PlannerVersion)
	require.JSONEq(t,
		`{"topic_overlap_count":0,"tag_overlap_count":0,"has_augur_link":false,"version_drift_count":1,"has_open_interaction":false,"freshness_at":"2026-04-26T10:00:00Z","event_type":"knowledge_loop.surface_plan_recomputed.v1"}`,
		string(patch.ScoreInputs),
	)

	entry := repo.entries[0]
	require.Equal(t, sovereignv1.SurfaceBucket_SURFACE_BUCKET_CHANGED, entry.SurfaceBucket)
	require.Equal(t, pickRenderDepth(sovereignv1.SurfaceBucket_SURFACE_BUCKET_CHANGED), entry.RenderDepthHint)
	require.Equal(t, pickLoopPriority(sovereignv1.SurfaceBucket_SURFACE_BUCKET_CHANGED), entry.LoopPriority)
	require.Equal(t, sovereignv1.SurfacePlannerVersion_SURFACE_PLANNER_VERSION_V2, entry.GetSurfacePlannerVersion())
	require.Equal(t, int64(950), entry.ProjectionSeqHiwater)
	require.Equal(t, sovereignv1.DismissState_DISMISS_STATE_ACTIVE, entry.DismissState,
		"surface replan must not alter lifecycle state")

	var changedSurface *sovereignv1.KnowledgeLoopSurface
	var nowSurface *sovereignv1.KnowledgeLoopSurface
	for _, s := range repo.surfaces {
		switch s.SurfaceBucket {
		case sovereignv1.SurfaceBucket_SURFACE_BUCKET_CHANGED:
			changedSurface = s
		case sovereignv1.SurfaceBucket_SURFACE_BUCKET_NOW:
			nowSurface = s
		}
	}
	require.NotNil(t, changedSurface)
	require.NotNil(t, changedSurface.PrimaryEntryKey)
	require.Equal(t, "article:replan-1", *changedSurface.PrimaryEntryKey)
	require.NotNil(t, nowSurface)
	require.Nil(t, nowSurface.PrimaryEntryKey, "recompute must clear the old bucket surface")
}
