package knowledge_loop_session_state

import (
	"encoding/json"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"knowledge-sovereign/driver/sovereign_db"
)

const (
	eventActed      = "knowledge_loop.acted.v1"
	eventReviewed   = "knowledge_loop.reviewed.v1"
	eventActOutcome = "knowledge_loop.act_outcome.v1"
	defaultLookback = 7 * 24 * time.Hour
)

func defaultWeights() LensModeWeights {
	return LookupLensModeWeights(DefaultLensModeID)
}

func mkEvent(t *testing.T, evType, entryKey string, payload map[string]any, occurredAt time.Time, seq int64) sovereign_db.KnowledgeEvent {
	t.Helper()
	payload["entry_key"] = entryKey
	body, err := json.Marshal(payload)
	require.NoError(t, err)
	uid := uuid.New()
	return sovereign_db.KnowledgeEvent{
		EventID:       uuid.New(),
		EventSeq:      seq,
		OccurredAt:    occurredAt,
		TenantID:      uuid.New(),
		UserID:        &uid,
		EventType:     evType,
		AggregateType: "knowledge_loop_entry",
		AggregateID:   entryKey,
		Payload:       body,
	}
}

func TestBuildMacroState_EmptyEventsYieldsZeroState(t *testing.T) {
	t.Parallel()
	windowEnd := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)

	got := BuildMacroState(nil, windowEnd, defaultLookback, defaultWeights(), LensWeightsVersion)

	require.Equal(t, uint32(0), got.ActiveContinueThreads)
	require.Equal(t, uint32(0), got.PendingReviewCount)
	require.Equal(t, uint32(0), got.RecentInternalizedCount)
	require.Equal(t, CognitiveLoadHintUnspecified, got.CognitiveLoadHint)
	require.Equal(t, int64(0), got.SeqHiwater)
	require.Equal(t, LensWeightsVersion, got.LensWeightsVersion)
	require.Equal(t, windowEnd.UTC(), got.WindowEndAt)
	require.Equal(t, windowEnd.Add(-defaultLookback).UTC(), got.WindowStartAt)
}

func TestBuildMacroState_UsesConsumeEventOccurredAtAsWindowRightEdge(t *testing.T) {
	t.Parallel()
	// Event-time purity: the window right edge is exactly windowEnd,
	// not time.Now(). The pure fn must echo the supplied windowEnd back
	// in WindowEndAt regardless of when it ran.
	windowEnd := time.Date(2025, 1, 15, 9, 30, 0, 0, time.UTC)

	got := BuildMacroState(nil, windowEnd, defaultLookback, defaultWeights(), LensWeightsVersion)

	require.Equal(t, windowEnd.UTC(), got.WindowEndAt)
	require.Equal(t, windowEnd.Add(-defaultLookback).UTC(), got.WindowStartAt)
}

func TestBuildMacroState_ExcludesEventsOutsideWindow(t *testing.T) {
	t.Parallel()
	windowEnd := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	beforeWindow := windowEnd.Add(-8 * 24 * time.Hour) // outside 7d lookback
	afterWindow := windowEnd.Add(1 * time.Hour)        // future event

	events := []sovereign_db.KnowledgeEvent{
		mkEvent(t, eventActed, "entry-too-old", map[string]any{"continue_flag": true}, beforeWindow, 10),
		mkEvent(t, eventActed, "entry-future", map[string]any{"continue_flag": true}, afterWindow, 20),
	}

	got := BuildMacroState(events, windowEnd, defaultLookback, defaultWeights(), LensWeightsVersion)

	require.Equal(t, uint32(0), got.ActiveContinueThreads, "events outside window must be ignored")
	require.Equal(t, int64(0), got.SeqHiwater, "out-of-window events must not advance seq_hiwater")
}

func TestBuildMacroState_CountsDistinctContinuingEntries(t *testing.T) {
	t.Parallel()
	windowEnd := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	t1 := windowEnd.Add(-3 * 24 * time.Hour)
	t2 := windowEnd.Add(-2 * 24 * time.Hour)
	t3 := windowEnd.Add(-1 * 24 * time.Hour)

	events := []sovereign_db.KnowledgeEvent{
		mkEvent(t, eventActed, "entry-a", map[string]any{"continue_flag": true}, t1, 10),
		mkEvent(t, eventActed, "entry-b", map[string]any{"continue_flag": true}, t2, 20),
		// Same entry-a tapped again — must not double-count.
		mkEvent(t, eventActed, "entry-a", map[string]any{"continue_flag": true}, t3, 30),
	}

	got := BuildMacroState(events, windowEnd, defaultLookback, defaultWeights(), LensWeightsVersion)

	require.Equal(t, uint32(2), got.ActiveContinueThreads)
	require.Equal(t, int64(30), got.SeqHiwater)
}

func TestBuildMacroState_ContinueFalseLaterWithdrawsEntry(t *testing.T) {
	t.Parallel()
	windowEnd := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	t1 := windowEnd.Add(-2 * 24 * time.Hour)
	t2 := windowEnd.Add(-1 * 24 * time.Hour)

	events := []sovereign_db.KnowledgeEvent{
		mkEvent(t, eventActed, "entry-a", map[string]any{"continue_flag": true}, t1, 10),
		// Later acted event explicitly closes the thread.
		mkEvent(t, eventActed, "entry-a", map[string]any{"continue_flag": false}, t2, 20),
	}

	got := BuildMacroState(events, windowEnd, defaultLookback, defaultWeights(), LensWeightsVersion)

	require.Equal(t, uint32(0), got.ActiveContinueThreads, "later continue_flag=false must withdraw the entry")
}

func TestBuildMacroState_CountsRecheckAsPendingReview(t *testing.T) {
	t.Parallel()
	windowEnd := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	t1 := windowEnd.Add(-2 * 24 * time.Hour)

	events := []sovereign_db.KnowledgeEvent{
		mkEvent(t, eventReviewed, "entry-r1", map[string]any{"trigger": "TRANSITION_TRIGGER_RECHECK"}, t1, 100),
	}

	got := BuildMacroState(events, windowEnd, defaultLookback, defaultWeights(), LensWeightsVersion)

	require.Equal(t, uint32(1), got.PendingReviewCount)
}

func TestBuildMacroState_MarkReviewedSupersedesEarlierRecheck(t *testing.T) {
	t.Parallel()
	windowEnd := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	t1 := windowEnd.Add(-3 * 24 * time.Hour)
	t2 := windowEnd.Add(-1 * 24 * time.Hour)

	events := []sovereign_db.KnowledgeEvent{
		mkEvent(t, eventReviewed, "entry-r1", map[string]any{"trigger": "TRANSITION_TRIGGER_RECHECK"}, t1, 100),
		mkEvent(t, eventReviewed, "entry-r1", map[string]any{"trigger": "TRANSITION_TRIGGER_MARK_REVIEWED"}, t2, 200),
	}

	got := BuildMacroState(events, windowEnd, defaultLookback, defaultWeights(), LensWeightsVersion)

	require.Equal(t, uint32(0), got.PendingReviewCount, "later mark_reviewed must clear the recheck")
}

func TestBuildMacroState_ArchiveSupersedesEarlierRecheck(t *testing.T) {
	t.Parallel()
	windowEnd := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	t1 := windowEnd.Add(-3 * 24 * time.Hour)
	t2 := windowEnd.Add(-1 * 24 * time.Hour)

	events := []sovereign_db.KnowledgeEvent{
		mkEvent(t, eventReviewed, "entry-r1", map[string]any{"trigger": "TRANSITION_TRIGGER_RECHECK"}, t1, 100),
		mkEvent(t, eventReviewed, "entry-r1", map[string]any{"trigger": "TRANSITION_TRIGGER_ARCHIVE"}, t2, 200),
	}

	got := BuildMacroState(events, windowEnd, defaultLookback, defaultWeights(), LensWeightsVersion)

	require.Equal(t, uint32(0), got.PendingReviewCount, "later archive must clear the recheck")
}

func TestBuildMacroState_InternalizedOutcomeContributesAndRemovesFromContinuing(t *testing.T) {
	t.Parallel()
	windowEnd := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	t1 := windowEnd.Add(-3 * 24 * time.Hour)
	t2 := windowEnd.Add(-1 * 24 * time.Hour)

	events := []sovereign_db.KnowledgeEvent{
		mkEvent(t, eventActed, "entry-g", map[string]any{"continue_flag": true}, t1, 10),
		mkEvent(t, eventActOutcome, "entry-g", map[string]any{"outcome": "internalized"}, t2, 20),
	}

	got := BuildMacroState(events, windowEnd, defaultLookback, defaultWeights(), LensWeightsVersion)

	require.Equal(t, uint32(1), got.RecentInternalizedCount, "internalized outcome must contribute to graduation count")
	require.Equal(t, uint32(0), got.ActiveContinueThreads, "graduation outranks continuation")
}

func TestBuildMacroState_NonInternalizedOutcomesDoNotGraduate(t *testing.T) {
	t.Parallel()
	windowEnd := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	t1 := windowEnd.Add(-3 * 24 * time.Hour)

	events := []sovereign_db.KnowledgeEvent{
		mkEvent(t, eventActed, "entry-x", map[string]any{"continue_flag": true}, t1, 10),
		mkEvent(t, eventActOutcome, "entry-x", map[string]any{"outcome": "engaged"}, t1.Add(time.Hour), 20),
		mkEvent(t, eventActOutcome, "entry-x", map[string]any{"outcome": "no_engagement"}, t1.Add(2*time.Hour), 30),
	}

	got := BuildMacroState(events, windowEnd, defaultLookback, defaultWeights(), LensWeightsVersion)

	require.Equal(t, uint32(0), got.RecentInternalizedCount)
	require.Equal(t, uint32(1), got.ActiveContinueThreads, "non-internalized outcomes leave the continue thread intact")
}

func TestBuildMacroState_SeqHiwaterIsMaxInWindow(t *testing.T) {
	t.Parallel()
	windowEnd := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	t1 := windowEnd.Add(-3 * 24 * time.Hour)

	events := []sovereign_db.KnowledgeEvent{
		mkEvent(t, eventActed, "a", map[string]any{"continue_flag": true}, t1, 17),
		mkEvent(t, eventActed, "b", map[string]any{"continue_flag": true}, t1.Add(time.Hour), 42),
		mkEvent(t, eventActed, "c", map[string]any{"continue_flag": true}, t1.Add(2*time.Hour), 23),
	}

	got := BuildMacroState(events, windowEnd, defaultLookback, defaultWeights(), LensWeightsVersion)

	require.Equal(t, int64(42), got.SeqHiwater)
}

func TestBuildMacroState_CognitiveLoadHint_DerivesFromWeights(t *testing.T) {
	t.Parallel()
	windowEnd := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	weights := LensModeWeights{MediumThreshold: 3, HeavyThreshold: 7}

	cases := []struct {
		name       string
		continuing int
		recheck    int
		wantHint   CognitiveLoadHint
	}{
		{"empty is unspecified", 0, 0, CognitiveLoadHintUnspecified},
		{"any signal is light", 1, 0, CognitiveLoadHintLight},
		{"just below medium is light", 2, 0, CognitiveLoadHintLight},
		{"at medium threshold is medium", 3, 0, CognitiveLoadHintMedium},
		{"below heavy is medium", 2, 4, CognitiveLoadHintMedium},
		{"at heavy threshold is heavy", 4, 3, CognitiveLoadHintHeavy},
		{"well above heavy is heavy", 10, 5, CognitiveLoadHintHeavy},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			evs := make([]sovereign_db.KnowledgeEvent, 0, tc.continuing+tc.recheck)
			var seq int64 = 1
			t1 := windowEnd.Add(-1 * 24 * time.Hour)
			for i := range tc.continuing {
				evs = append(evs, mkEvent(t, eventActed,
					"continue-"+string(rune('a'+i)),
					map[string]any{"continue_flag": true},
					t1, seq))
				seq++
			}
			for i := range tc.recheck {
				evs = append(evs, mkEvent(t, eventReviewed,
					"review-"+string(rune('a'+i)),
					map[string]any{"trigger": "TRANSITION_TRIGGER_RECHECK"},
					t1, seq))
				seq++
			}

			got := BuildMacroState(evs, windowEnd, defaultLookback, weights, LensWeightsVersion)
			require.Equal(t, tc.wantHint, got.CognitiveLoadHint,
				"continuing=%d recheck=%d expected hint=%q", tc.continuing, tc.recheck, tc.wantHint)
		})
	}
}

func TestBuildMacroState_ReprojectIdempotentUnderShuffle(t *testing.T) {
	t.Parallel()
	windowEnd := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	t1 := windowEnd.Add(-3 * 24 * time.Hour)
	t2 := windowEnd.Add(-2 * 24 * time.Hour)
	t3 := windowEnd.Add(-1 * 24 * time.Hour)

	original := []sovereign_db.KnowledgeEvent{
		mkEvent(t, eventActed, "alpha", map[string]any{"continue_flag": true}, t1, 10),
		mkEvent(t, eventActed, "beta", map[string]any{"continue_flag": true}, t1.Add(time.Hour), 11),
		mkEvent(t, eventReviewed, "gamma", map[string]any{"trigger": "TRANSITION_TRIGGER_RECHECK"}, t2, 20),
		mkEvent(t, eventActOutcome, "delta", map[string]any{"outcome": "internalized"}, t3, 30),
		mkEvent(t, eventActed, "delta", map[string]any{"continue_flag": true}, t2.Add(time.Hour), 25),
	}

	canonical := BuildMacroState(original, windowEnd, defaultLookback, defaultWeights(), LensWeightsVersion)

	// Reproject must be ordering-independent. The builder's contract is
	// "pure function of (events, windowEnd, lookback, weights)" — shuffle
	// the input ten different ways and assert the output is byte-identical
	// to the canonical run.
	rng := rand.New(rand.NewPCG(1, 2))
	for trial := range 10 {
		shuffled := make([]sovereign_db.KnowledgeEvent, len(original))
		copy(shuffled, original)
		rng.Shuffle(len(shuffled), func(i, j int) {
			shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
		})

		got := BuildMacroState(shuffled, windowEnd, defaultLookback, defaultWeights(), LensWeightsVersion)
		require.Equal(t, canonical, got, "shuffled replay (trial %d) must match canonical run", trial)
	}
}

func TestBuildMacroState_WallClockIsIgnored(t *testing.T) {
	t.Parallel()
	// Event-time purity: even if the projector consumes a "stale" event
	// hours after it happened, the macro state is anchored to the
	// supplied windowEnd, not to wall-clock. We can't observe time.Now()
	// directly inside the pure fn, so the contract test is structural:
	// the same input produces the same output regardless of when the
	// test runs. The shuffle test covers ordering; this one anchors
	// against a fixed historic windowEnd to make the discipline visible
	// in the test name.
	historicWindowEnd := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	t1 := historicWindowEnd.Add(-1 * 24 * time.Hour)

	events := []sovereign_db.KnowledgeEvent{
		mkEvent(t, eventActed, "x", map[string]any{"continue_flag": true}, t1, 100),
	}

	got := BuildMacroState(events, historicWindowEnd, defaultLookback, defaultWeights(), LensWeightsVersion)

	require.Equal(t, historicWindowEnd.UTC(), got.WindowEndAt, "windowEnd must be echoed regardless of wall-clock")
	require.Equal(t, uint32(1), got.ActiveContinueThreads)
	require.Equal(t, int64(100), got.SeqHiwater)
}
