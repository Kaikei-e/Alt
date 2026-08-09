package trail_planner

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"knowledge-sovereign/driver/sovereign_db"
)

type fakePlannerRepo struct {
	users      []uuid.UUID
	anchor     string
	anchorVerb string
	anchorOK   bool
	candidates []sovereign_db.TrailClusterCandidate
	emitted    []sovereign_db.KnowledgeEvent
	anchorErr  map[uuid.UUID]error // per-user anchor failures

	// anchorTitle/anchorTitleOK back GetItemTitle (D28 — anchored why): a
	// planner that cannot name the anchor must skip the user entirely.
	anchorTitle   string
	anchorTitleOK bool
	titleErr      error

	// continuationCandidates backs DeriveTrailContinuationCandidates (Wave 11,
	// D27/D28). Deliberately not limit-truncated by the fake — several tests
	// rely on the planner itself enforcing the "at most one per run" cap
	// rather than trusting the repository to have already done so.
	continuationCandidates []sovereign_db.TrailContinuationCandidate

	// dedupeRejects makes every append report "already registered" — the
	// production shape in which the planner emitted nothing for 13 days.
	dedupeRejects bool
}

func (f *fakePlannerRepo) ListDistinctUserIDs(context.Context) ([]uuid.UUID, error) {
	return f.users, nil
}
func (f *fakePlannerRepo) GetLatestFootprintAnchor(_ context.Context, userID uuid.UUID) (sovereign_db.FootprintAnchor, bool, error) {
	if f.anchorErr != nil {
		if err, ok := f.anchorErr[userID]; ok {
			return sovereign_db.FootprintAnchor{}, false, err
		}
	}
	verb := f.anchorVerb
	if verb == "" {
		verb = "read" // the ordinary case; tests that care set it explicitly
	}
	return sovereign_db.FootprintAnchor{ItemKey: f.anchor, TenantID: uuid.New(), Verb: verb}, f.anchorOK, nil
}
func (f *fakePlannerRepo) GetItemTitle(_ context.Context, _ uuid.UUID, _ string) (string, bool, error) {
	if f.titleErr != nil {
		return "", false, f.titleErr
	}
	return f.anchorTitle, f.anchorTitleOK, nil
}
func (f *fakePlannerRepo) DeriveTrailClusterCandidates(context.Context, uuid.UUID, int) ([]sovereign_db.TrailClusterCandidate, error) {
	return f.candidates, nil
}
func (f *fakePlannerRepo) DeriveTrailContinuationCandidates(context.Context, uuid.UUID, int) ([]sovereign_db.TrailContinuationCandidate, error) {
	return f.continuationCandidates, nil
}
func (f *fakePlannerRepo) AppendKnowledgeEventIfNew(_ context.Context, e sovereign_db.KnowledgeEvent) (int64, bool, error) {
	if f.dedupeRejects {
		return 0, false, nil
	}
	f.emitted = append(f.emitted, e)
	return int64(len(f.emitted)), true, nil
}

// captureLogger returns a logger whose records are readable as JSON text, so a
// test can assert on what the planner claimed to have done.
func captureLogger() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})), &buf
}

// TestPlanner_DedupeRejectedBranchIsNotClaimedAsProposed pins the invariant
// that the planner may only claim work the event log actually accepted: when
// the append is rejected by the dedupe registry, no trail.branch_proposed may
// be logged. Re-proposal being rejected is correct behaviour; reporting it as
// an emission is what produced ~600 false claims/hour against zero appended
// events.
func TestPlanner_DedupeRejectedBranchIsNotClaimedAsProposed(t *testing.T) {
	logger, buf := captureLogger()
	repo := &fakePlannerRepo{
		users:         []uuid.UUID{uuid.New()},
		anchor:        "article:a",
		anchorOK:      true,
		anchorTitle:   "US military courts in the UK",
		anchorTitleOK: true,
		candidates: []sovereign_db.TrailClusterCandidate{
			{TargetItemKey: "article:z", TargetTitle: "Async Rust", SharedTags: []string{"rust"}},
		},
		dedupeRejects: true,
	}
	p := NewPlanner(repo, logger, Config{Clock: func() time.Time { return time.Unix(0, 0) }})
	require.NoError(t, p.RunBatch(context.Background()))

	logs := buf.String()
	assert.NotContains(t, logs, "trail.branch_proposed",
		"a dedupe-rejected append must never be logged as a proposal")
	assert.Contains(t, logs, "trail.branch_dedupe_rejected",
		"the rejection must still be observable — silence is indistinguishable from a dead planner")
}

// TestPlanner_GenuineAppendLogsProposedWithEventSeq pins the other half: a
// branch that really landed is logged at INFO and carries the assigned event
// sequence as evidence that the append happened.
func TestPlanner_GenuineAppendLogsProposedWithEventSeq(t *testing.T) {
	logger, buf := captureLogger()
	repo := &fakePlannerRepo{
		users:         []uuid.UUID{uuid.New()},
		anchor:        "article:a",
		anchorOK:      true,
		anchorTitle:   "US military courts in the UK",
		anchorTitleOK: true,
		candidates: []sovereign_db.TrailClusterCandidate{
			{TargetItemKey: "article:z", TargetTitle: "Async Rust", SharedTags: []string{"rust"}},
		},
	}
	p := NewPlanner(repo, logger, Config{Clock: func() time.Time { return time.Unix(0, 0) }})
	require.NoError(t, p.RunBatch(context.Background()))

	logs := buf.String()
	assert.Contains(t, logs, "trail.branch_proposed")
	assert.Contains(t, logs, `"event_seq":1`, "the claim must carry the sequence the append returned")
	assert.NotContains(t, logs, "trail.branch_dedupe_rejected")
}

// readAnchor is the common case: the user read the anchor item.
func readAnchor(itemKey, title string) anchorRef {
	phrase, ok := whyPhraseForVerb("read")
	if !ok {
		panic("read must be an engagement verb")
	}
	return anchorRef{itemKey: itemKey, title: title, whyPhrase: phrase}
}

// TestWhyPhraseForVerb_CoversEveryEngagementVerb pins the two halves of the
// anchored-why contract to each other: every verb the derivation layer admits
// as an anchor or as continuation contact must have a phrase that makes the
// why true, and no other verb may. Divergence here is how "Because you read X"
// ended up on articles the user dismissed.
func TestWhyPhraseForVerb_CoversEveryEngagementVerb(t *testing.T) {
	for _, verb := range sovereign_db.EngagementVerbs {
		phrase, ok := whyPhraseForVerb(verb)
		assert.True(t, ok, "engagement verb %q has no why phrase", verb)
		assert.NotEmpty(t, phrase)
	}
	_, ok := whyPhraseForVerb("dismissed")
	assert.False(t, ok, "a dismissal can never phrase a why — it is a refusal")
	_, ok = whyPhraseForVerb("")
	assert.False(t, ok)
}

// TestBuildClusterBranch_WhyNamesTheActThatHappened pins that the why is
// phrased from the anchor's own verb rather than a hard-coded "you read".
func TestBuildClusterBranch_WhyNamesTheActThatHappened(t *testing.T) {
	tests := []struct {
		name string
		verb string
		want string
	}{
		{name: "read", verb: "read", want: "Because you read"},
		{name: "asked", verb: "asked", want: "Because you asked about"},
		{name: "listened", verb: "listened", want: "Because you listened to"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			phrase, ok := whyPhraseForVerb(tt.verb)
			require.True(t, ok)
			b := buildClusterBranch(uuid.New(),
				anchorRef{itemKey: "article:a", title: "US military courts in the UK", whyPhrase: phrase},
				sovereign_db.TrailClusterCandidate{
					TargetItemKey: "article:z", TargetTitle: "Async Rust", SharedTags: []string{"rust"},
				})
			assert.Contains(t, b.Why, tt.want)
			assert.Contains(t, b.Why, `"US military courts in the UK"`)
		})
	}
}

// TestBuildContinuationBranch_WhyNamesTheActThatHappened pins the same
// contract on the self-referential branch: the why is phrased from the verb of
// the contact that qualified the thread.
func TestBuildContinuationBranch_WhyNamesTheActThatHappened(t *testing.T) {
	phrase, ok := whyPhraseForVerb("listened")
	require.True(t, ok)
	b := buildContinuationBranch(uuid.New(), phrase, sovereign_db.TrailContinuationCandidate{
		TargetItemKey: "article:q", TargetTitle: "Async Rust", Verb: "listened",
	})
	assert.Contains(t, b.Why, "Because you listened to")
	assert.Contains(t, b.Why, `"Async Rust"`)
}

// TestPlanner_NoEligibleAnchorSuppressesAndSaysSo pins the fall-through: when
// the user's spine holds nothing that can truthfully anchor a why (e.g. only
// dismissals), the planner proposes nothing and says why, rather than
// inventing an anchor.
func TestPlanner_NoEligibleAnchorSuppressesAndSaysSo(t *testing.T) {
	logger, buf := captureLogger()
	repo := &fakePlannerRepo{
		users:    []uuid.UUID{uuid.New()},
		anchorOK: false, // no footprint carries an engagement verb
		candidates: []sovereign_db.TrailClusterCandidate{
			{TargetItemKey: "article:z", TargetTitle: "Async Rust", SharedTags: []string{"rust"}},
		},
	}
	p := NewPlanner(repo, logger, Config{Clock: func() time.Time { return time.Unix(0, 0) }})
	require.NoError(t, p.RunBatch(context.Background()))

	assert.Empty(t, repo.emitted, "no truthful anchor → no branch")
	assert.Contains(t, buf.String(), "trail.branch_anchor_unresolved",
		"suppression must be observable, not silent")
}

// TestPlanner_AnchorVerbWithoutPhraseSuppresses pins the belt-and-braces gate:
// if the derivation layer ever admits a verb the why cannot phrase, the
// planner suppresses instead of emitting a claim it cannot back.
func TestPlanner_AnchorVerbWithoutPhraseSuppresses(t *testing.T) {
	logger, buf := captureLogger()
	repo := &fakePlannerRepo{
		users:         []uuid.UUID{uuid.New()},
		anchor:        "article:a",
		anchorOK:      true,
		anchorVerb:    "dismissed",
		anchorTitle:   "US military courts in the UK",
		anchorTitleOK: true,
		candidates: []sovereign_db.TrailClusterCandidate{
			{TargetItemKey: "article:z", TargetTitle: "Async Rust", SharedTags: []string{"rust"}},
		},
	}
	p := NewPlanner(repo, logger, Config{Clock: func() time.Time { return time.Unix(0, 0) }})
	require.NoError(t, p.RunBatch(context.Background()))

	assert.Empty(t, repo.emitted, "a verb the why cannot name must not produce a branch")
	assert.Contains(t, buf.String(), "trail.branch_anchor_unresolved")
}

func TestBuildClusterBranch_AlwaysPopulatesFourTuple(t *testing.T) {
	b := buildClusterBranch(uuid.New(), readAnchor("article:a", "US military courts in the UK"), sovereign_db.TrailClusterCandidate{
		TargetItemKey: "article:z", TargetTitle: "Async Rust", SharedTags: []string{"rust", "async"},
	})
	assert.True(t, b.Valid(), "a derived branch must always carry the four-tuple")
	assert.Equal(t, "cluster", b.RelationKind)
	assert.Equal(t, "corroborated", b.Confidence, "two shared tags reads as corroborated")
	assert.GreaterOrEqual(t, len(b.EvidenceRefs), 2, "evidence = shared tags + target item")
}

func TestBuildClusterBranch_SingleTagIsPlausible(t *testing.T) {
	b := buildClusterBranch(uuid.New(), readAnchor("article:a", "US military courts in the UK"), sovereign_db.TrailClusterCandidate{
		TargetItemKey: "article:z", SharedTags: []string{"rust"},
	})
	assert.Equal(t, "plausible", b.Confidence)
}

// TestBuildClusterBranch_WhyReferencesAnchorTitleInQuotes pins D28(a): a why
// that does not reference its anchor is forbidden. buildClusterBranch composes
// the why from the anchor's title, quoted, so the contract is enforced by
// construction.
func TestBuildClusterBranch_WhyReferencesAnchorTitleInQuotes(t *testing.T) {
	b := buildClusterBranch(uuid.New(), readAnchor("article:a", "US military courts in the UK"), sovereign_db.TrailClusterCandidate{
		TargetItemKey: "article:z", TargetTitle: "Async Rust", SharedTags: []string{"rust"},
	})
	assert.Contains(t, b.Why, `"US military courts in the UK"`, "why must reference the anchor title in quotes")
	assert.Contains(t, b.Why, "rust", "why must still surface the shared-tag evidence")
}

func TestPlanner_EmitsBranchProposedPerCandidate(t *testing.T) {
	repo := &fakePlannerRepo{
		users:         []uuid.UUID{uuid.New()},
		anchor:        "article:a",
		anchorOK:      true,
		anchorTitle:   "US military courts in the UK",
		anchorTitleOK: true,
		candidates: []sovereign_db.TrailClusterCandidate{
			{TargetItemKey: "article:z", TargetTitle: "Async Rust", SharedTags: []string{"rust"}},
		},
	}
	p := NewPlanner(repo, nil, Config{Clock: func() time.Time { return time.Unix(0, 0) }})
	require.NoError(t, p.RunBatch(context.Background()))

	require.Len(t, repo.emitted, 1)
	e := repo.emitted[0]
	assert.Equal(t, EventTrailBranchProposed, e.EventType)
	assert.Equal(t, EventTrailBranchProposed+":cluster:"+repo.users[0].String()+":article:z", e.DedupeKey)
	var payload BranchProposedPayload
	require.NoError(t, json.Unmarshal(e.Payload, &payload))
	assert.True(t, payload.Valid())
	assert.Contains(t, payload.Why, `"US military courts in the UK"`, "the emitted why must be anchored (D28)")
}

func TestPlanner_SkipsTitlelessCandidate(t *testing.T) {
	repo := &fakePlannerRepo{
		users:         []uuid.UUID{uuid.New()},
		anchor:        "article:a",
		anchorOK:      true,
		anchorTitle:   "US military courts in the UK",
		anchorTitleOK: true,
		candidates: []sovereign_db.TrailClusterCandidate{
			{TargetItemKey: "article:z", TargetTitle: "", SharedTags: []string{"rust"}},         // unnameable
			{TargetItemKey: "article:y", TargetTitle: "Real Title", SharedTags: []string{"go"}}, // nameable
		},
	}
	p := NewPlanner(repo, nil, Config{Clock: func() time.Time { return time.Unix(0, 0) }})
	require.NoError(t, p.RunBatch(context.Background()))

	require.Len(t, repo.emitted, 1, "a title-less target cannot be named to the user — do not propose it")
	var payload BranchProposedPayload
	require.NoError(t, json.Unmarshal(repo.emitted[0].Payload, &payload))
	assert.Equal(t, "article:y", payload.TargetItemKey)
}

func TestPlanner_NoAnchorEmitsNothing(t *testing.T) {
	repo := &fakePlannerRepo{users: []uuid.UUID{uuid.New()}, anchorOK: false}
	p := NewPlanner(repo, nil, Config{})
	require.NoError(t, p.RunBatch(context.Background()))
	assert.Empty(t, repo.emitted, "no footprints → no anchor → no branches")
}

// TestPlanner_SkipsUserWhenAnchorTitleUnresolved pins D28(a)'s enforcement
// mechanism: when the anchor's title cannot be resolved, the planner must not
// fabricate a generic why — it skips the user entirely rather than emit an
// unanchored branch.
func TestPlanner_SkipsUserWhenAnchorTitleUnresolved(t *testing.T) {
	repo := &fakePlannerRepo{
		users:         []uuid.UUID{uuid.New()},
		anchor:        "article:a",
		anchorOK:      true,
		anchorTitleOK: false, // the anchor item has no resolvable title
		candidates: []sovereign_db.TrailClusterCandidate{
			{TargetItemKey: "article:z", TargetTitle: "Async Rust", SharedTags: []string{"rust"}},
		},
	}
	p := NewPlanner(repo, nil, Config{Clock: func() time.Time { return time.Unix(0, 0) }})
	require.NoError(t, p.RunBatch(context.Background()))
	assert.Empty(t, repo.emitted, "an unresolvable anchor title must suppress emission, not fall back to a generic why")
}

func TestPlanner_PanicsWhenUnwired(t *testing.T) {
	p := &Planner{} // repo nil — a wiring bug
	assert.Panics(t, func() { _ = p.RunBatch(context.Background()) },
		"Rule 8: an unwired producer must fail loud, not silently no-op")
}

func TestPlanner_ContinuesAfterUserError(t *testing.T) {
	failUser := uuid.New()
	okUser := uuid.New()
	repo := &fakePlannerRepo{
		users:         []uuid.UUID{failUser, okUser},
		anchor:        "article:a",
		anchorOK:      true,
		anchorTitle:   "US military courts in the UK",
		anchorTitleOK: true,
		anchorErr: map[uuid.UUID]error{
			failUser: assert.AnError,
		},
		candidates: []sovereign_db.TrailClusterCandidate{
			{TargetItemKey: "article:z", TargetTitle: "Async Rust", SharedTags: []string{"rust"}},
		},
	}
	p := NewPlanner(repo, nil, Config{Clock: func() time.Time { return time.Unix(0, 0) }})
	require.NoError(t, p.RunBatch(context.Background()), "user errors must not abort the whole batch")
	require.Len(t, repo.emitted, 1, "second user should still get a branch")
	assert.Equal(t, okUser, *repo.emitted[0].UserID)
}

// continuationEventsOf filters emitted branch_proposed events down to the
// ones carrying relation_kind "continuation" — the tests below only care
// about the Continuation slice, not the (unrelated) Cluster emissions from
// the same run.
func continuationEventsOf(t *testing.T, events []sovereign_db.KnowledgeEvent) []sovereign_db.KnowledgeEvent {
	t.Helper()
	var out []sovereign_db.KnowledgeEvent
	for _, e := range events {
		var payload BranchProposedPayload
		require.NoError(t, json.Unmarshal(e.Payload, &payload))
		if payload.RelationKind == "continuation" {
			out = append(out, e)
		}
	}
	return out
}

// TestPlanner_EmitsContinuationBranchWithAnchoredWhy pins Wave 11 (D27): a
// Continuation candidate becomes a self-referential branch (anchor == target)
// whose why quotes the candidate's OWN title (not the user's latest
// footprint) and carries the full four-tuple.
func TestPlanner_EmitsContinuationBranchWithAnchoredWhy(t *testing.T) {
	userID := uuid.New()
	repo := &fakePlannerRepo{
		users:         []uuid.UUID{userID},
		anchor:        "article:a",
		anchorOK:      true,
		anchorTitle:   "US military courts in the UK",
		anchorTitleOK: true,
		continuationCandidates: []sovereign_db.TrailContinuationCandidate{
			{TargetItemKey: "article:q", TargetTitle: "Async Rust", Verb: "read", LastContactAt: time.Unix(0, 0)},
		},
	}
	p := NewPlanner(repo, nil, Config{Clock: func() time.Time { return time.Unix(0, 0) }})
	require.NoError(t, p.RunBatch(context.Background()))

	continuationEvents := continuationEventsOf(t, repo.emitted)
	require.Len(t, continuationEvents, 1, "exactly one continuation branch must be emitted")

	var payload BranchProposedPayload
	require.NoError(t, json.Unmarshal(continuationEvents[0].Payload, &payload))
	assert.True(t, payload.Valid(), "a continuation branch must always carry the four-tuple")
	assert.Equal(t, "continuation", payload.RelationKind)
	assert.Equal(t, "article:q", payload.AnchorItemKey, "continuation is self-referential — anchor == target")
	assert.Equal(t, "article:q", payload.TargetItemKey)
	assert.Contains(t, payload.Why, `"Async Rust"`, "why must quote the target's own title (self-referential anchor)")
	assert.NotEmpty(t, payload.Confidence)
	assert.NotEmpty(t, payload.EvidenceRefs)
}

// TestPlanner_EmitsAtMostOneContinuationPerUserPerRun pins D28 (少数精鋭 —
// precision over recall): even when several candidates are available, the
// planner emits at most one Continuation branch per user per run.
func TestPlanner_EmitsAtMostOneContinuationPerUserPerRun(t *testing.T) {
	userID := uuid.New()
	repo := &fakePlannerRepo{
		users:         []uuid.UUID{userID},
		anchor:        "article:a",
		anchorOK:      true,
		anchorTitle:   "US military courts in the UK",
		anchorTitleOK: true,
		continuationCandidates: []sovereign_db.TrailContinuationCandidate{
			{TargetItemKey: "article:q", TargetTitle: "Async Rust", Verb: "read", LastContactAt: time.Unix(100, 0)},
			{TargetItemKey: "article:r", TargetTitle: "Distributed Systems", Verb: "read", LastContactAt: time.Unix(50, 0)},
		},
	}
	p := NewPlanner(repo, nil, Config{Clock: func() time.Time { return time.Unix(0, 0) }})
	require.NoError(t, p.RunBatch(context.Background()))

	continuationEvents := continuationEventsOf(t, repo.emitted)
	assert.Len(t, continuationEvents, 1, "at most one continuation branch per user per run")
}

// TestPlanner_NoContinuationCandidatesEmitsNoneAndLeavesClusterUntouched pins
// that an empty continuation candidate set is a normal no-op — it must not
// suppress or alter the existing Cluster emission from the same run.
func TestPlanner_NoContinuationCandidatesEmitsNoneAndLeavesClusterUntouched(t *testing.T) {
	userID := uuid.New()
	repo := &fakePlannerRepo{
		users:         []uuid.UUID{userID},
		anchor:        "article:a",
		anchorOK:      true,
		anchorTitle:   "US military courts in the UK",
		anchorTitleOK: true,
		candidates: []sovereign_db.TrailClusterCandidate{
			{TargetItemKey: "article:z", TargetTitle: "Async Rust", SharedTags: []string{"rust"}},
		},
		continuationCandidates: nil,
	}
	p := NewPlanner(repo, nil, Config{Clock: func() time.Time { return time.Unix(0, 0) }})
	require.NoError(t, p.RunBatch(context.Background()))

	assert.Empty(t, continuationEventsOf(t, repo.emitted), "no continuation candidates → no continuation emit")
	require.Len(t, repo.emitted, 1, "the cluster branch from the same run must be untouched")
	var payload BranchProposedPayload
	require.NoError(t, json.Unmarshal(repo.emitted[0].Payload, &payload))
	assert.Equal(t, "cluster", payload.RelationKind)
}

// TestPlanner_ContinuationDedupeKeyIsDeterministicBranchKey pins that the
// continuation event's DedupeKey is the deterministic branch key
// ("continuation:<userID>:<item_key>") so the same candidate is proposed once
// ever, mirroring the Cluster dedupe contract.
func TestPlanner_ContinuationDedupeKeyIsDeterministicBranchKey(t *testing.T) {
	userID := uuid.New()
	repo := &fakePlannerRepo{
		users:         []uuid.UUID{userID},
		anchor:        "article:a",
		anchorOK:      true,
		anchorTitle:   "US military courts in the UK",
		anchorTitleOK: true,
		continuationCandidates: []sovereign_db.TrailContinuationCandidate{
			{TargetItemKey: "article:q", TargetTitle: "Async Rust", Verb: "read", LastContactAt: time.Unix(0, 0)},
		},
	}
	p := NewPlanner(repo, nil, Config{Clock: func() time.Time { return time.Unix(0, 0) }})
	require.NoError(t, p.RunBatch(context.Background()))

	continuationEvents := continuationEventsOf(t, repo.emitted)
	require.Len(t, continuationEvents, 1)

	wantKey := "continuation:" + userID.String() + ":article:q"
	assert.Equal(t, EventTrailBranchProposed+":"+wantKey, continuationEvents[0].DedupeKey)

	var payload BranchProposedPayload
	require.NoError(t, json.Unmarshal(continuationEvents[0].Payload, &payload))
	assert.Equal(t, wantKey, payload.BranchKey)
}
