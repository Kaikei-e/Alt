package get_item_branches_usecase

import (
	"context"
	"errors"
	"testing"

	"alt/domain"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeItemBranchesPort struct {
	branches      []domain.TrailBranch
	err           error
	gotAnchor     string
	gotLimit      int
	gotAnchorUser uuid.UUID
}

func (f *fakeItemBranchesPort) GetTrailBranchesForAnchor(_ context.Context, userID uuid.UUID, anchorItemKey string, limit int) ([]domain.TrailBranch, error) {
	f.gotAnchorUser = userID
	f.gotAnchor = anchorItemKey
	f.gotLimit = limit
	return f.branches, f.err
}

func TestExecute_ReturnsBranches(t *testing.T) {
	port := &fakeItemBranchesPort{branches: []domain.TrailBranch{{BranchKey: "cluster:u:article:z"}}}
	uc := NewGetItemBranchesUsecase(port)

	branches, err := uc.Execute(context.Background(), uuid.New(), "article:1", 2)
	require.NoError(t, err)
	require.Len(t, branches, 1)
	assert.Equal(t, "cluster:u:article:z", branches[0].BranchKey)
	assert.Equal(t, "article:1", port.gotAnchor)
}

// TestExecute_DefaultsLimitToTwo pins D26 — patch-exit is 少数精鋭 (max 1-2):
// a non-positive limit falls back to the default of 2, not some larger page
// size.
func TestExecute_DefaultsLimitToTwo(t *testing.T) {
	port := &fakeItemBranchesPort{}
	uc := NewGetItemBranchesUsecase(port)

	_, err := uc.Execute(context.Background(), uuid.New(), "article:1", 0)
	require.NoError(t, err)
	assert.Equal(t, defaultLimit, port.gotLimit)

	_, err = uc.Execute(context.Background(), uuid.New(), "article:1", -5)
	require.NoError(t, err)
	assert.Equal(t, defaultLimit, port.gotLimit)
}

// TestExecute_ClampsLimitToMaxFive pins the D26 ceiling: even a client asking
// for more than 5 must be capped, since the patch-exit surface is deliberately
// a handful, not a feed.
func TestExecute_ClampsLimitToMaxFive(t *testing.T) {
	port := &fakeItemBranchesPort{}
	uc := NewGetItemBranchesUsecase(port)

	_, err := uc.Execute(context.Background(), uuid.New(), "article:1", 50)
	require.NoError(t, err)
	assert.Equal(t, maxLimit, port.gotLimit)
}

// TestExecute_WithinRangeLimitPassesThroughUnchanged pins that a valid
// within-range limit (e.g. 1) is forwarded verbatim, not silently bumped to
// the default.
func TestExecute_WithinRangeLimitPassesThroughUnchanged(t *testing.T) {
	port := &fakeItemBranchesPort{}
	uc := NewGetItemBranchesUsecase(port)

	_, err := uc.Execute(context.Background(), uuid.New(), "article:1", 1)
	require.NoError(t, err)
	assert.Equal(t, 1, port.gotLimit)
}

func TestExecute_RejectsEmptyItemKey(t *testing.T) {
	uc := NewGetItemBranchesUsecase(&fakeItemBranchesPort{})
	_, err := uc.Execute(context.Background(), uuid.New(), "   ", 2)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidRequest)
}

func TestExecute_PropagatesPortError(t *testing.T) {
	uc := NewGetItemBranchesUsecase(&fakeItemBranchesPort{err: errors.New("sovereign down")})
	_, err := uc.Execute(context.Background(), uuid.New(), "article:1", 2)
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrInvalidRequest)
}
