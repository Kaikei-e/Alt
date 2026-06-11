package trail_planner

import (
	"context"
	"encoding/json"
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
	anchorOK   bool
	candidates []sovereign_db.TrailClusterCandidate
	emitted    []sovereign_db.KnowledgeEvent
}

func (f *fakePlannerRepo) ListDistinctUserIDs(context.Context) ([]uuid.UUID, error) {
	return f.users, nil
}
func (f *fakePlannerRepo) GetLatestFootprintAnchor(context.Context, uuid.UUID) (string, uuid.UUID, bool, error) {
	return f.anchor, uuid.New(), f.anchorOK, nil
}
func (f *fakePlannerRepo) DeriveTrailClusterCandidates(context.Context, uuid.UUID, int) ([]sovereign_db.TrailClusterCandidate, error) {
	return f.candidates, nil
}
func (f *fakePlannerRepo) AppendKnowledgeEvent(_ context.Context, e sovereign_db.KnowledgeEvent) (int64, error) {
	f.emitted = append(f.emitted, e)
	return int64(len(f.emitted)), nil
}

func TestBuildClusterBranch_AlwaysPopulatesFourTuple(t *testing.T) {
	b := buildClusterBranch(uuid.New(), "article:a", sovereign_db.TrailClusterCandidate{
		TargetItemKey: "article:z", TargetTitle: "Async Rust", SharedTags: []string{"rust", "async"},
	})
	assert.True(t, b.Valid(), "a derived branch must always carry the four-tuple")
	assert.Equal(t, "cluster", b.RelationKind)
	assert.Equal(t, "corroborated", b.Confidence, "two shared tags reads as corroborated")
	assert.GreaterOrEqual(t, len(b.EvidenceRefs), 2, "evidence = shared tags + target item")
}

func TestBuildClusterBranch_SingleTagIsPlausible(t *testing.T) {
	b := buildClusterBranch(uuid.New(), "article:a", sovereign_db.TrailClusterCandidate{
		TargetItemKey: "article:z", SharedTags: []string{"rust"},
	})
	assert.Equal(t, "plausible", b.Confidence)
}

func TestPlanner_EmitsBranchProposedPerCandidate(t *testing.T) {
	repo := &fakePlannerRepo{
		users:    []uuid.UUID{uuid.New()},
		anchor:   "article:a",
		anchorOK: true,
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
}

func TestPlanner_SkipsTitlelessCandidate(t *testing.T) {
	repo := &fakePlannerRepo{
		users:    []uuid.UUID{uuid.New()},
		anchor:   "article:a",
		anchorOK: true,
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

func TestPlanner_PanicsWhenUnwired(t *testing.T) {
	p := &Planner{} // repo nil — a wiring bug
	assert.Panics(t, func() { _ = p.RunBatch(context.Background()) },
		"Rule 8: an unwired producer must fail loud, not silently no-op")
}
