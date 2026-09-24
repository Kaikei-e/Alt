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
	users                  []uuid.UUID
	anchor                 string
	anchorVerb             string
	anchorOK               bool
	candidates             []sovereign_db.TrailClusterCandidate
	emitted                []sovereign_db.KnowledgeEvent
	anchorErr              map[uuid.UUID]error
	anchorTitle            string
	anchorTitleOK          bool
	titleErr               error
	continuationCandidates []sovereign_db.TrailContinuationCandidate
	dedupeRejects          bool
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
		verb = "read"
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

func captureLogger() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})), &buf
}

func TestPlanner_DedupeRejectedBranchIsNotClaimedAsProposed(t *testing.T) {
	user := uuid.New()
	repo := &fakePlannerRepo{
		users:         []uuid.UUID{user},
		anchor:        "article:a",
		anchorOK:      true,
		anchorTitle:   "Anchor Title",
		anchorTitleOK: true,
		candidates: []sovereign_db.TrailClusterCandidate{
			{TargetItemKey: "article:b", TargetTitle: "Target B", SharedTags: []string{"rust"}},
		},
		dedupeRejects: true,
	}

	logger, buf := captureLogger()
	p := NewPlanner(repo, logger, Config{Clock: func() time.Time { return time.Unix(0, 0) }})
	require.NoError(t, p.RunBatch(context.Background()))

	out := buf.String()
	assert.Contains(t, out, "trail.branch_dedupe_rejected")
	assert.NotContains(t, out, "\"msg\":\"trail.branch_proposed\"")
}

func TestPlanner_GenuineAppendLogsProposedWithEventSeq(t *testing.T) {
	user := uuid.New()
	repo := &fakePlannerRepo{
		users:         []uuid.UUID{user},
		anchor:        "article:a",
		anchorOK:      true,
		anchorTitle:   "Anchor Title",
		anchorTitleOK: true,
		candidates: []sovereign_db.TrailClusterCandidate{
			{TargetItemKey: "article:b", TargetTitle: "Target B", SharedTags: []string{"rust"}},
		},
	}

	logger, buf := captureLogger()
	p := NewPlanner(repo, logger, Config{Clock: func() time.Time { return time.Unix(0, 0) }})
	require.NoError(t, p.RunBatch(context.Background()))

	out := buf.String()
	assert.Contains(t, out, "\"msg\":\"trail.branch_proposed\"")
	assert.Contains(t, out, "\"relation_kind\":\"cluster\"")
	assert.Contains(t, out, "\"event_seq\":1")
}

func TestPlanner_NoEligibleAnchorSuppressesAndSaysSo(t *testing.T) {
	user := uuid.New()
	repo := &fakePlannerRepo{
		users:    []uuid.UUID{user},
		anchorOK: false,
	}
	logger, buf := captureLogger()
	p := NewPlanner(repo, logger, Config{Clock: func() time.Time { return time.Unix(0, 0) }})
	require.NoError(t, p.RunBatch(context.Background()))

	assert.Empty(t, repo.emitted)
	assert.Contains(t, buf.String(), "trail.branch_anchor_unresolved")
	assert.Contains(t, buf.String(), "no_eligible_footprint")
}

func TestPlanner_AnchorVerbWithoutPhraseSuppresses(t *testing.T) {
	user := uuid.New()
	repo := &fakePlannerRepo{
		users:      []uuid.UUID{user},
		anchor:     "article:a",
		anchorVerb: "dismissed",
		anchorOK:   true,
	}
	logger, buf := captureLogger()
	p := NewPlanner(repo, logger, Config{Clock: func() time.Time { return time.Unix(0, 0) }})
	require.NoError(t, p.RunBatch(context.Background()))

	assert.Empty(t, repo.emitted)
	assert.Contains(t, buf.String(), "trail.branch_anchor_unresolved")
	assert.Contains(t, buf.String(), "unphrasable_verb")
}

func TestNewPlanner_DefaultClockIsWallClock(t *testing.T) {
	p := NewPlanner(&fakePlannerRepo{}, slog.Default(), Config{})
	now := p.cfg.Clock()
	assert.True(t, now.After(time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)), "default clock should return wall clock time, got %v", now)
}

func TestPlanner_EmitsBranchProposedEvents(t *testing.T) {
	user := uuid.New()
	repo := &fakePlannerRepo{
		users:         []uuid.UUID{user},
		anchor:        "article:a",
		anchorOK:      true,
		anchorTitle:   "US military courts in the UK",
		anchorTitleOK: true,
		candidates: []sovereign_db.TrailClusterCandidate{
			{TargetItemKey: "article:x", TargetTitle: "X", SharedTags: []string{"tag1"}},
			{TargetItemKey: "article:y", TargetTitle: "Y", SharedTags: []string{"tag2"}},
		},
	}
	p := NewPlanner(repo, nil, Config{Clock: func() time.Time { return time.Unix(0, 0) }})
	require.NoError(t, p.RunBatch(context.Background()))

	require.Len(t, repo.emitted, 2)
	assert.Equal(t, EventTrailBranchProposed, repo.emitted[0].EventType)
	assert.Equal(t, EventTrailBranchProposed+":cluster:"+user.String()+":article:x", repo.emitted[0].DedupeKey)

	var payload BranchProposedPayload
	require.NoError(t, json.Unmarshal(repo.emitted[0].Payload, &payload))
	assert.True(t, payload.Valid(), "emitted branch payload must be valid")
	assert.Contains(t, payload.Why, `"US military courts in the UK"`, "why must anchor to anchor title")
}

func TestPlanner_SkipsTitlelessCandidate(t *testing.T) {
	user := uuid.New()
	repo := &fakePlannerRepo{
		users:         []uuid.UUID{user},
		anchor:        "article:a",
		anchorOK:      true,
		anchorTitle:   "Anchor Title",
		anchorTitleOK: true,
		candidates: []sovereign_db.TrailClusterCandidate{
			{TargetItemKey: "article:x", TargetTitle: "", SharedTags: []string{"tag1"}},
			{TargetItemKey: "article:y", TargetTitle: "Y", SharedTags: []string{"tag2"}},
		},
	}
	p := NewPlanner(repo, nil, Config{Clock: func() time.Time { return time.Unix(0, 0) }})
	require.NoError(t, p.RunBatch(context.Background()))

	require.Len(t, repo.emitted, 1)
	var payload BranchProposedPayload
	require.NoError(t, json.Unmarshal(repo.emitted[0].Payload, &payload))
	assert.Equal(t, "article:y", payload.TargetItemKey)
}

func TestPlanner_NoAnchorEmitsNothing(t *testing.T) {
	repo := &fakePlannerRepo{users: []uuid.UUID{uuid.New()}, anchorOK: false}
	p := NewPlanner(repo, nil, Config{})
	require.NoError(t, p.RunBatch(context.Background()))
	assert.Empty(t, repo.emitted)
}

func TestPlanner_SkipsUserWhenAnchorTitleUnresolved(t *testing.T) {
	repo := &fakePlannerRepo{
		users:         []uuid.UUID{uuid.New()},
		anchor:        "article:a",
		anchorOK:      true,
		anchorTitleOK: false,
		candidates: []sovereign_db.TrailClusterCandidate{
			{TargetItemKey: "article:x", TargetTitle: "X", SharedTags: []string{"tag1"}},
		},
	}
	p := NewPlanner(repo, nil, Config{Clock: func() time.Time { return time.Unix(0, 0) }})
	require.NoError(t, p.RunBatch(context.Background()))
	assert.Empty(t, repo.emitted)
}

func TestPlanner_PanicsWhenUnwired(t *testing.T) {
	p := NewPlanner(nil, nil, Config{})
	assert.PanicsWithValue(t, "trail_planner: repository not wired", func() {
		_ = p.RunBatch(context.Background())
	})
}

func TestPlanner_ContinuesAfterUserError(t *testing.T) {
	good1, bad, good2 := uuid.New(), uuid.New(), uuid.New()
	repo := &fakePlannerRepo{
		users:         []uuid.UUID{good1, bad, good2},
		anchor:        "article:a",
		anchorOK:      true,
		anchorTitle:   "Anchor Title",
		anchorTitleOK: true,
		candidates: []sovereign_db.TrailClusterCandidate{
			{TargetItemKey: "article:x", TargetTitle: "X", SharedTags: []string{"tag1"}},
		},
		anchorErr: map[uuid.UUID]error{
			bad: assert.AnError,
		},
	}

	p := NewPlanner(repo, nil, Config{Clock: func() time.Time { return time.Unix(0, 0) }})
	require.NoError(t, p.RunBatch(context.Background()))
	assert.Len(t, repo.emitted, 2)
}

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
}

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
