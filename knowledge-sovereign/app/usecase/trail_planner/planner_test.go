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
	continuationCandidates []sovereign_db.TrailContinuationCandidate
	continuationErr        map[uuid.UUID]error
	emitted                []sovereign_db.KnowledgeEvent
	dedupeRejects          bool
}

func (f *fakePlannerRepo) ListDistinctUserIDs(context.Context) ([]uuid.UUID, error) {
	return f.users, nil
}

func (f *fakePlannerRepo) DeriveTrailContinuationCandidates(_ context.Context, userID uuid.UUID, _ int) ([]sovereign_db.TrailContinuationCandidate, error) {
	if f.continuationErr != nil {
		if err, ok := f.continuationErr[userID]; ok {
			return nil, err
		}
	}
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
		users: []uuid.UUID{user},
		continuationCandidates: []sovereign_db.TrailContinuationCandidate{
			{TenantID: uuid.New(), TargetItemKey: "article:b", TargetTitle: "Target B", Verb: "read", LastContactAt: time.Unix(0, 0)},
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
	tenant := uuid.New()
	repo := &fakePlannerRepo{
		users: []uuid.UUID{user},
		continuationCandidates: []sovereign_db.TrailContinuationCandidate{
			{TenantID: tenant, TargetItemKey: "article:b", TargetTitle: "Target B", Verb: "read", LastContactAt: time.Unix(0, 0)},
		},
	}

	logger, buf := captureLogger()
	p := NewPlanner(repo, logger, Config{Clock: func() time.Time { return time.Unix(0, 0) }})
	require.NoError(t, p.RunBatch(context.Background()))

	require.Len(t, repo.emitted, 1)
	assert.Equal(t, tenant, repo.emitted[0].TenantID)
	out := buf.String()
	assert.Contains(t, out, "\"msg\":\"trail.branch_proposed\"")
	assert.Contains(t, out, "\"relation_kind\":\"continuation\"")
	assert.Contains(t, out, "\"event_seq\":1")
}

func TestPlanner_SkipsTitlelessCandidate(t *testing.T) {
	user := uuid.New()
	repo := &fakePlannerRepo{
		users: []uuid.UUID{user},
		continuationCandidates: []sovereign_db.TrailContinuationCandidate{
			{TenantID: uuid.New(), TargetItemKey: "article:x", TargetTitle: "   ", Verb: "read", LastContactAt: time.Unix(0, 0)},
		},
	}
	logger, buf := captureLogger()
	p := NewPlanner(repo, logger, Config{Clock: func() time.Time { return time.Unix(0, 0) }})
	require.NoError(t, p.RunBatch(context.Background()))

	assert.Empty(t, repo.emitted)
	assert.Contains(t, buf.String(), "trail.branch_anchor_unresolved")
	assert.Contains(t, buf.String(), "title_unresolved")
}

func TestPlanner_UnphrasableVerbSuppresses(t *testing.T) {
	user := uuid.New()
	repo := &fakePlannerRepo{
		users: []uuid.UUID{user},
		continuationCandidates: []sovereign_db.TrailContinuationCandidate{
			{TenantID: uuid.New(), TargetItemKey: "article:a", TargetTitle: "Dismissed Item", Verb: "dismissed", LastContactAt: time.Unix(0, 0)},
		},
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

func TestPlanner_PanicsWhenUnwired(t *testing.T) {
	p := NewPlanner(nil, nil, Config{})
	assert.PanicsWithValue(t, "trail_planner: repository not wired", func() {
		_ = p.RunBatch(context.Background())
	})
}

func TestPlanner_ContinuesAfterUserError(t *testing.T) {
	good1, bad, good2 := uuid.New(), uuid.New(), uuid.New()
	repo := &fakePlannerRepo{
		users: []uuid.UUID{good1, bad, good2},
		continuationCandidates: []sovereign_db.TrailContinuationCandidate{
			{TenantID: uuid.New(), TargetItemKey: "article:x", TargetTitle: "X", Verb: "read", LastContactAt: time.Unix(0, 0)},
		},
		continuationErr: map[uuid.UUID]error{
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
	tenantID := uuid.New()
	repo := &fakePlannerRepo{
		users: []uuid.UUID{userID},
		continuationCandidates: []sovereign_db.TrailContinuationCandidate{
			{TenantID: tenantID, TargetItemKey: "article:q", TargetTitle: "Async Rust", Verb: "read", LastContactAt: time.Unix(0, 0)},
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
		users: []uuid.UUID{userID},
		continuationCandidates: []sovereign_db.TrailContinuationCandidate{
			{TenantID: uuid.New(), TargetItemKey: "article:q", TargetTitle: "Async Rust", Verb: "read", LastContactAt: time.Unix(100, 0)},
			{TenantID: uuid.New(), TargetItemKey: "article:r", TargetTitle: "Distributed Systems", Verb: "read", LastContactAt: time.Unix(50, 0)},
		},
	}
	p := NewPlanner(repo, nil, Config{Clock: func() time.Time { return time.Unix(0, 0) }})
	require.NoError(t, p.RunBatch(context.Background()))

	continuationEvents := continuationEventsOf(t, repo.emitted)
	assert.Len(t, continuationEvents, 1, "at most one continuation branch per user per run")
}

func TestPlanner_NoContinuationCandidatesEmitsNothing(t *testing.T) {
	userID := uuid.New()
	repo := &fakePlannerRepo{
		users:                  []uuid.UUID{userID},
		continuationCandidates: nil,
	}
	p := NewPlanner(repo, nil, Config{Clock: func() time.Time { return time.Unix(0, 0) }})
	require.NoError(t, p.RunBatch(context.Background()))

	assert.Empty(t, repo.emitted)
}

func TestPlanner_ContinuationDedupeKeyIsDeterministicBranchKey(t *testing.T) {
	userID := uuid.New()
	repo := &fakePlannerRepo{
		users: []uuid.UUID{userID},
		continuationCandidates: []sovereign_db.TrailContinuationCandidate{
			{TenantID: uuid.New(), TargetItemKey: "article:q", TargetTitle: "Async Rust", Verb: "read", LastContactAt: time.Unix(0, 0)},
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

func TestPlanner_NeverProposesClusterBranches(t *testing.T) {
	userID := uuid.New()
	tenantID := uuid.New()
	repo := &fakePlannerRepo{
		users: []uuid.UUID{userID},
		continuationCandidates: []sovereign_db.TrailContinuationCandidate{
			{TenantID: tenantID, TargetItemKey: "article:q", TargetTitle: "Quiet Thread", Verb: "read", LastContactAt: time.Unix(0, 0)},
		},
	}

	p := NewPlanner(repo, nil, Config{Clock: func() time.Time { return time.Unix(0, 0) }})
	require.NoError(t, p.RunBatch(context.Background()))

	require.Len(t, repo.emitted, 1)
	assert.Equal(t, tenantID, repo.emitted[0].TenantID)
	var payload BranchProposedPayload
	require.NoError(t, json.Unmarshal(repo.emitted[0].Payload, &payload))
	assert.NotEqual(t, "cluster", payload.RelationKind)
	assert.Equal(t, "continuation", payload.RelationKind)
}
