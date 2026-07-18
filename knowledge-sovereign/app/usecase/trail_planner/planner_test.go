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
	anchorErr  map[uuid.UUID]error // per-user anchor failures

	// anchorTitle/anchorTitleOK back GetItemTitle (D28 — anchored why): a
	// planner that cannot name the anchor must skip the user entirely.
	anchorTitle   string
	anchorTitleOK bool
	titleErr      error
}

func (f *fakePlannerRepo) ListDistinctUserIDs(context.Context) ([]uuid.UUID, error) {
	return f.users, nil
}
func (f *fakePlannerRepo) GetLatestFootprintAnchor(_ context.Context, userID uuid.UUID) (string, uuid.UUID, bool, error) {
	if f.anchorErr != nil {
		if err, ok := f.anchorErr[userID]; ok {
			return "", uuid.Nil, false, err
		}
	}
	return f.anchor, uuid.New(), f.anchorOK, nil
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
func (f *fakePlannerRepo) AppendKnowledgeEvent(_ context.Context, e sovereign_db.KnowledgeEvent) (int64, error) {
	f.emitted = append(f.emitted, e)
	return int64(len(f.emitted)), nil
}

func TestBuildClusterBranch_AlwaysPopulatesFourTuple(t *testing.T) {
	b := buildClusterBranch(uuid.New(), "article:a", "US military courts in the UK", sovereign_db.TrailClusterCandidate{
		TargetItemKey: "article:z", TargetTitle: "Async Rust", SharedTags: []string{"rust", "async"},
	})
	assert.True(t, b.Valid(), "a derived branch must always carry the four-tuple")
	assert.Equal(t, "cluster", b.RelationKind)
	assert.Equal(t, "corroborated", b.Confidence, "two shared tags reads as corroborated")
	assert.GreaterOrEqual(t, len(b.EvidenceRefs), 2, "evidence = shared tags + target item")
}

func TestBuildClusterBranch_SingleTagIsPlausible(t *testing.T) {
	b := buildClusterBranch(uuid.New(), "article:a", "US military courts in the UK", sovereign_db.TrailClusterCandidate{
		TargetItemKey: "article:z", SharedTags: []string{"rust"},
	})
	assert.Equal(t, "plausible", b.Confidence)
}

// TestBuildClusterBranch_WhyReferencesAnchorTitleInQuotes pins D28(a): a why
// that does not reference its anchor is forbidden. buildClusterBranch composes
// the why from the anchor's title, quoted, so the contract is enforced by
// construction.
func TestBuildClusterBranch_WhyReferencesAnchorTitleInQuotes(t *testing.T) {
	b := buildClusterBranch(uuid.New(), "article:a", "US military courts in the UK", sovereign_db.TrailClusterCandidate{
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
