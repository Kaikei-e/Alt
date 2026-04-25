package job

import (
	"alt/domain"
	"alt/port/knowledge_loop_port"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// fakeLoopRepo is a minimal in-memory recorder that captures the sequence of upserts.
// It does NOT simulate DB-level seq_hiwater guard; the guard is exercised separately at the
// driver layer. Here we verify the projector emits the same upsert payload for the same event
// on replay — a reproject-safety invariant.
type fakeLoopRepo struct {
	entries []domain.KnowledgeLoopEntry
	session []domain.KnowledgeLoopSessionState
	surface []domain.KnowledgeLoopSurface
}

func (f *fakeLoopRepo) UpsertKnowledgeLoopEntry(ctx context.Context, entry *domain.KnowledgeLoopEntry) (*knowledge_loop_port.UpsertResult, error) {
	f.entries = append(f.entries, *entry)
	return &knowledge_loop_port.UpsertResult{Applied: true, ProjectionRevision: 1, ProjectionSeqHiwater: entry.ProjectionSeqHiwater}, nil
}

func (f *fakeLoopRepo) UpsertKnowledgeLoopSessionState(ctx context.Context, state *domain.KnowledgeLoopSessionState) (*knowledge_loop_port.UpsertResult, error) {
	f.session = append(f.session, *state)
	return &knowledge_loop_port.UpsertResult{Applied: true, ProjectionRevision: 1, ProjectionSeqHiwater: state.ProjectionSeqHiwater}, nil
}

func (f *fakeLoopRepo) UpsertKnowledgeLoopSurface(ctx context.Context, s *domain.KnowledgeLoopSurface) (*knowledge_loop_port.UpsertResult, error) {
	f.surface = append(f.surface, *s)
	return &knowledge_loop_port.UpsertResult{Applied: true, ProjectionRevision: 1, ProjectionSeqHiwater: s.ProjectionSeqHiwater}, nil
}

func makeLoopEvent(t *testing.T, eventType string, seq int64, userID uuid.UUID) *domain.KnowledgeEvent {
	t.Helper()
	payload, _ := json.Marshal(map[string]string{
		"entry_key": "article:42",
	})
	return &domain.KnowledgeEvent{
		EventID:       uuid.New(),
		EventSeq:      seq,
		OccurredAt:    time.Date(2026, 4, 23, 10, 0, 0, 0, time.UTC),
		TenantID:      uuid.New(),
		UserID:        &userID,
		EventType:     eventType,
		AggregateType: "article",
		AggregateID:   "article:42",
		DedupeKey:     eventType + ":" + uuid.NewString(),
		Payload:       payload,
	}
}

// TestProjectLoopEvent_HomeItemOpened exercises the main projection path.
func TestProjectLoopEvent_HomeItemOpened(t *testing.T) {
	repo := &fakeLoopRepo{}
	userID := uuid.New()
	ev := makeLoopEvent(t, domain.EventHomeItemOpened, 100, userID)

	res, err := projectLoopEvent(context.Background(), ev, repo, repo, repo)
	require.NoError(t, err)
	require.NotNil(t, res)
	require.True(t, res.Applied)

	require.Len(t, repo.entries, 1, "one entry upsert expected")
	require.Len(t, repo.session, 1, "one session upsert expected")

	entry := repo.entries[0]
	require.Equal(t, domain.LoopStageAct, entry.ProposedStage)
	require.Equal(t, domain.SurfaceContinue, entry.SurfaceBucket)
	require.Equal(t, domain.DismissCompleted, entry.DismissState)
	require.Equal(t, ev.OccurredAt, entry.FreshnessAt, "freshness_at must come from event occurred_at, not wall-clock")

	state := repo.session[0]
	require.Equal(t, domain.LoopStageAct, state.CurrentStage)
	require.Equal(t, ev.OccurredAt, state.CurrentStageEnteredAt,
		"current_stage_entered_at must come from event occurred_at, not wall-clock (reproject-safety)")
}

// TestProjectLoopEvent_ReplayIsIdempotent verifies that projecting the same event twice
// produces identical payloads. This is the core reproject-safety invariant: replaying
// the event log must converge to the same projection state regardless of when it runs.
func TestProjectLoopEvent_ReplayIsIdempotent(t *testing.T) {
	userID := uuid.New()
	ev := makeLoopEvent(t, domain.EventHomeItemOpened, 200, userID)

	repoA := &fakeLoopRepo{}
	_, errA := projectLoopEvent(context.Background(), ev, repoA, repoA, repoA)
	require.NoError(t, errA)

	repoB := &fakeLoopRepo{}
	_, errB := projectLoopEvent(context.Background(), ev, repoB, repoB, repoB)
	require.NoError(t, errB)

	require.Equal(t, repoA.entries, repoB.entries,
		"projecting the same event twice must yield identical entry upserts")
	require.Equal(t, repoA.session, repoB.session,
		"projecting the same event twice must yield identical session upserts")
}

// TestProjectLoopEvent_NoUserIDIsNoOp guards against panicking on system-level events
// that lack a user_id (e.g. ArticleCreated). They should be a no-op in this projector.
func TestProjectLoopEvent_NoUserIDIsNoOp(t *testing.T) {
	ev := makeLoopEvent(t, domain.EventArticleCreated, 50, uuid.Nil)
	ev.UserID = nil

	repo := &fakeLoopRepo{}
	res, err := projectLoopEvent(context.Background(), ev, repo, repo, repo)
	require.NoError(t, err)
	require.Nil(t, res)
	require.Empty(t, repo.entries)
	require.Empty(t, repo.session)
}

// TestProjectLoopEvent_SummaryVersionCreated_SeedsObserveDecisionOptions verifies
// that an Observe-stage entry receives stage-appropriate CTAs per canonical
// contract §7. SummaryVersionCreated lands in Observe; the only allowed forward
// transition is observe → orient (mapped to the `revisit` intent), plus
// non-transitional `ask` and `snooze`. Earlier seeds emitted open/save/snooze
// which require observe → act and rendered as disabled buttons.
func TestProjectLoopEvent_SummaryVersionCreated_SeedsObserveDecisionOptions(t *testing.T) {
	repo := &fakeLoopRepo{}
	userID := uuid.New()
	ev := makeLoopEvent(t, domain.EventSummaryVersionCreated, 400, userID)

	_, err := projectLoopEvent(context.Background(), ev, repo, repo, repo)
	require.NoError(t, err)
	require.Len(t, repo.entries, 1)

	entry := repo.entries[0]
	require.Equal(t, domain.WhyKindSource, entry.WhyKind)
	require.NotEmpty(t, entry.DecisionOptions, "Observe-stage entry must have CTA seed")

	var seeded []map[string]string
	require.NoError(t, json.Unmarshal(entry.DecisionOptions, &seeded))
	intents := make([]string, 0, len(seeded))
	for _, s := range seeded {
		intents = append(intents, s["intent"])
	}
	require.Equal(t, []string{"revisit", "ask", "snooze"}, intents,
		"Observe entries must propose §7-allowed transitions; observe → act is forbidden")
}

// TestSeedDecisionOptions_StageAppropriate covers each LoopStage's canonical
// CTA shape against the §7 transition allowlist.
func TestSeedDecisionOptions_StageAppropriate(t *testing.T) {
	cases := []struct {
		stage   domain.LoopStage
		intents []string
	}{
		{domain.LoopStageObserve, []string{"revisit", "ask", "snooze"}},
		{domain.LoopStageOrient, []string{"compare", "ask", "snooze"}},
		{domain.LoopStageDecide, []string{"open", "save", "ask"}},
		{domain.LoopStageAct, []string{"revisit", "ask"}},
	}
	for _, tc := range cases {
		raw := seedDecisionOptions(domain.WhyKindSource, tc.stage)
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

// TestProjectLoopEvent_HomeItemDismissed_NoDecisionSeed documents the narrowing: entries
// that have already been dismissed do not get a fresh CTA seed on projection.
func TestProjectLoopEvent_HomeItemDismissed_NoDecisionSeed(t *testing.T) {
	repo := &fakeLoopRepo{}
	userID := uuid.New()
	ev := makeLoopEvent(t, domain.EventHomeItemDismissed, 410, userID)

	_, err := projectLoopEvent(context.Background(), ev, repo, repo, repo)
	require.NoError(t, err)
	require.Len(t, repo.entries, 1)
	require.Empty(t, repo.entries[0].DecisionOptions,
		"Dismissed entries should not get a fresh CTA seed")
}

// ---------------------------------------------------------------------------
// PR-L5: Knowledge Loop transition events project into session_state only.
//
// These events are emitted by the /loop UI via TransitionKnowledgeLoopUsecase.
// The projector must update knowledge_loop_session_state with:
//   - CurrentStage from the payload's to_stage
//   - CurrentStageEnteredAt from event.OccurredAt (reproject-safe, never time.Now())
//   - Last<stage>EntryKey pointing at the payload's entry_key
//   - ProjectionSeqHiwater for the sovereign's merge-safe guard
// They must NOT create or mutate knowledge_loop_entries; entry rows flow from
// article-side events (SummaryVersionCreated / HomeItem* / etc).
// ---------------------------------------------------------------------------

func makeLoopTransitionEvent(
	t *testing.T,
	eventType string,
	seq int64,
	userID uuid.UUID,
	entryKey, fromStage, toStage string,
) *domain.KnowledgeEvent {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"entry_key":                    entryKey,
		"lens_mode_id":                 "default",
		"from_stage":                   fromStage,
		"to_stage":                     toStage,
		"trigger":                      "TRANSITION_TRIGGER_USER_TAP",
		"observed_projection_revision": 1,
	})
	require.NoError(t, err)
	return &domain.KnowledgeEvent{
		EventID:       uuid.New(),
		EventSeq:      seq,
		OccurredAt:    time.Date(2026, 4, 24, 9, 30, 0, 0, time.UTC),
		TenantID:      uuid.New(),
		UserID:        &userID,
		EventType:     eventType,
		AggregateType: domain.AggregateLoopSession,
		AggregateID:   entryKey,
		DedupeKey:     uuid.NewString(),
		Payload:       payload,
	}
}

func TestProjectLoopEvent_KnowledgeLoopObservedUpdatesSession(t *testing.T) {
	repo := &fakeLoopRepo{}
	userID := uuid.New()
	ev := makeLoopTransitionEvent(t, domain.EventKnowledgeLoopObserved, 500, userID,
		"article:42", "LOOP_STAGE_OBSERVE", "LOOP_STAGE_OBSERVE")

	_, err := projectLoopEvent(context.Background(), ev, repo, repo, repo)
	require.NoError(t, err)
	require.Empty(t, repo.entries, "Loop transition events must not create entries")
	require.Len(t, repo.session, 1)

	state := repo.session[0]
	require.Equal(t, domain.LoopStageObserve, state.CurrentStage)
	require.Equal(t, ev.OccurredAt, state.CurrentStageEnteredAt)
	require.NotNil(t, state.LastObservedEntryKey)
	require.Equal(t, "article:42", *state.LastObservedEntryKey)
	require.Equal(t, int64(500), state.ProjectionSeqHiwater)
	require.Equal(t, "default", state.LensModeID)
	require.Equal(t, *ev.UserID, state.UserID)
	require.Equal(t, ev.TenantID, state.TenantID)
}

func TestProjectLoopEvent_KnowledgeLoopOrientedUpdatesSession(t *testing.T) {
	repo := &fakeLoopRepo{}
	userID := uuid.New()
	ev := makeLoopTransitionEvent(t, domain.EventKnowledgeLoopOriented, 501, userID,
		"article:42", "LOOP_STAGE_OBSERVE", "LOOP_STAGE_ORIENT")

	_, err := projectLoopEvent(context.Background(), ev, repo, repo, repo)
	require.NoError(t, err)
	require.Len(t, repo.session, 1)

	state := repo.session[0]
	require.Equal(t, domain.LoopStageOrient, state.CurrentStage)
	require.NotNil(t, state.LastOrientedEntryKey)
	require.Equal(t, "article:42", *state.LastOrientedEntryKey)
	require.Nil(t, state.LastObservedEntryKey,
		"Oriented event must not populate last_observed_entry_key; COALESCE on the DB side preserves prior value")
}

func TestProjectLoopEvent_KnowledgeLoopDecisionPresentedUpdatesSession(t *testing.T) {
	repo := &fakeLoopRepo{}
	userID := uuid.New()
	ev := makeLoopTransitionEvent(t, domain.EventKnowledgeLoopDecisionPresented, 502, userID,
		"article:42", "LOOP_STAGE_ORIENT", "LOOP_STAGE_DECIDE")

	_, err := projectLoopEvent(context.Background(), ev, repo, repo, repo)
	require.NoError(t, err)
	require.Len(t, repo.session, 1)

	state := repo.session[0]
	require.Equal(t, domain.LoopStageDecide, state.CurrentStage)
	require.NotNil(t, state.LastDecidedEntryKey)
	require.Equal(t, "article:42", *state.LastDecidedEntryKey)
}

func TestProjectLoopEvent_KnowledgeLoopActedUpdatesSession(t *testing.T) {
	repo := &fakeLoopRepo{}
	userID := uuid.New()
	ev := makeLoopTransitionEvent(t, domain.EventKnowledgeLoopActed, 503, userID,
		"article:42", "LOOP_STAGE_DECIDE", "LOOP_STAGE_ACT")

	_, err := projectLoopEvent(context.Background(), ev, repo, repo, repo)
	require.NoError(t, err)
	require.Len(t, repo.session, 1)

	state := repo.session[0]
	require.Equal(t, domain.LoopStageAct, state.CurrentStage)
	require.NotNil(t, state.LastActedEntryKey)
	require.Equal(t, "article:42", *state.LastActedEntryKey)
}

func TestProjectLoopEvent_KnowledgeLoopReturnedUpdatesSession(t *testing.T) {
	repo := &fakeLoopRepo{}
	userID := uuid.New()
	ev := makeLoopTransitionEvent(t, domain.EventKnowledgeLoopReturned, 504, userID,
		"article:42", "LOOP_STAGE_ACT", "LOOP_STAGE_OBSERVE")

	_, err := projectLoopEvent(context.Background(), ev, repo, repo, repo)
	require.NoError(t, err)
	require.Len(t, repo.session, 1)

	state := repo.session[0]
	require.Equal(t, domain.LoopStageObserve, state.CurrentStage)
	require.NotNil(t, state.LastReturnedEntryKey)
	require.Equal(t, "article:42", *state.LastReturnedEntryKey)
}

func TestProjectLoopEvent_KnowledgeLoopDeferredUpdatesSession(t *testing.T) {
	repo := &fakeLoopRepo{}
	userID := uuid.New()
	ev := makeLoopTransitionEvent(t, domain.EventKnowledgeLoopDeferred, 505, userID,
		"article:42", "LOOP_STAGE_ORIENT", "LOOP_STAGE_ORIENT")

	_, err := projectLoopEvent(context.Background(), ev, repo, repo, repo)
	require.NoError(t, err)
	require.Len(t, repo.session, 1)

	state := repo.session[0]
	require.NotNil(t, state.LastDeferredEntryKey)
	require.Equal(t, "article:42", *state.LastDeferredEntryKey)
}

// TestProjectLoopEvent_LoopEventDoesNotCreateEntry pins the single-emission rule.
// Only /feeds HomeItem* events create entry rows; /loop transitions update session state.
func TestProjectLoopEvent_LoopEventDoesNotCreateEntry(t *testing.T) {
	repo := &fakeLoopRepo{}
	userID := uuid.New()
	ev := makeLoopTransitionEvent(t, domain.EventKnowledgeLoopActed, 600, userID,
		"article:42", "LOOP_STAGE_DECIDE", "LOOP_STAGE_ACT")

	_, err := projectLoopEvent(context.Background(), ev, repo, repo, repo)
	require.NoError(t, err)
	require.Empty(t, repo.entries,
		"Loop transition events must not upsert entries (ADR-000831 §3.8 single-emission rule)")
}

// TestProjectLoopEvent_HomeItemSupersededSetsPointer checks supersede pointer handling.
func TestProjectLoopEvent_HomeItemSupersededSetsPointer(t *testing.T) {
	repo := &fakeLoopRepo{}
	userID := uuid.New()
	payload, _ := json.Marshal(map[string]string{
		"entry_key":     "article:42",
		"new_entry_key": "article:43",
	})
	ev := makeLoopEvent(t, domain.EventHomeItemSuperseded, 300, userID)
	ev.Payload = payload

	_, err := projectLoopEvent(context.Background(), ev, repo, repo, repo)
	require.NoError(t, err)
	require.Len(t, repo.entries, 1)
	require.NotNil(t, repo.entries[0].SupersededByEntryKey)
	require.Equal(t, "article:43", *repo.entries[0].SupersededByEntryKey)
}
