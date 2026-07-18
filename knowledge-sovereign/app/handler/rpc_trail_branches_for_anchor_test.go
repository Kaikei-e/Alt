package handler

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"knowledge-sovereign/driver/sovereign_db"
	sovereignv1 "knowledge-sovereign/gen/proto/services/sovereign/v1"
)

// TestGetTrailBranchesForAnchor_ReturnsMappedBranches pins the Wave 10 (D26)
// patch-exit read: branches anchored on one item map through with the full
// four-tuple, same shape as GetTrailFootprints' branch mapping.
func TestGetTrailBranchesForAnchor_ReturnsMappedBranches(t *testing.T) {
	repo := &mockRepo{anchorBranches: []sovereign_db.TrailBranch{{
		BranchKey:     "cluster:u:article:z",
		AnchorItemKey: "article:1",
		RelationKind:  "cluster",
		Why:           `Because you read "US military courts in the UK" — joins rust`,
		EvidenceRefs:  []sovereign_db.TrailEvidenceRef{{RefID: "rust", Label: "rust", Kind: "tag"}},
		Confidence:    "plausible",
		TargetItemKey: "article:z",
		TargetTitle:   "Async Rust",
	}}}
	h := NewSovereignHandler(repo)

	resp, err := h.GetTrailBranchesForAnchor(context.Background(), connect.NewRequest(&sovereignv1.GetTrailBranchesForAnchorRequest{
		UserId:        "22222222-2222-2222-2222-222222222222",
		AnchorItemKey: "article:1",
		Limit:         2,
	}))
	require.NoError(t, err)
	require.Len(t, resp.Msg.Branches, 1)
	b := resp.Msg.Branches[0]
	assert.Equal(t, "cluster:u:article:z", b.BranchKey)
	assert.Equal(t, "article:1", b.AnchorItemKey)
	assert.Equal(t, "cluster", b.RelationKind)
	assert.Contains(t, b.Why, "US military courts in the UK")
	assert.Equal(t, "plausible", b.Confidence)
	require.Len(t, b.EvidenceRefs, 1)
	assert.Equal(t, "rust", b.EvidenceRefs[0].RefId)

	assert.Equal(t, "article:1", repo.gotAnchorItemKey, "the anchor item key must reach the repository")
	assert.Equal(t, 2, repo.gotAnchorLimit, "the client-supplied limit must reach the repository")
}

func TestGetTrailBranchesForAnchor_InvalidUserIDIsInvalidArgument(t *testing.T) {
	h := NewSovereignHandler(&mockRepo{})

	_, err := h.GetTrailBranchesForAnchor(context.Background(), connect.NewRequest(&sovereignv1.GetTrailBranchesForAnchorRequest{
		UserId:        "not-a-uuid",
		AnchorItemKey: "article:1",
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

func TestGetTrailBranchesForAnchor_EmptyAnchorItemKeyIsInvalidArgument(t *testing.T) {
	h := NewSovereignHandler(&mockRepo{})

	_, err := h.GetTrailBranchesForAnchor(context.Background(), connect.NewRequest(&sovereignv1.GetTrailBranchesForAnchorRequest{
		UserId:        "22222222-2222-2222-2222-222222222222",
		AnchorItemKey: "   ",
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

// TestGetTrailBranchesForAnchor_DefaultsLimitWhenNotProvided pins a sane
// server-side fallback for a client that omits limit — the query must never
// silently run with LIMIT 0.
func TestGetTrailBranchesForAnchor_DefaultsLimitWhenNotProvided(t *testing.T) {
	repo := &mockRepo{}
	h := NewSovereignHandler(repo)

	_, err := h.GetTrailBranchesForAnchor(context.Background(), connect.NewRequest(&sovereignv1.GetTrailBranchesForAnchorRequest{
		UserId:        "22222222-2222-2222-2222-222222222222",
		AnchorItemKey: "article:1",
	}))
	require.NoError(t, err)
	assert.Greater(t, repo.gotAnchorLimit, 0, "an unset client limit must not degrade to LIMIT 0")
}
