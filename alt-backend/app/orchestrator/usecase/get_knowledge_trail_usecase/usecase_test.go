package get_knowledge_trail_usecase

import (
	"context"
	"errors"
	"testing"

	"alt/domain"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeTrailPort struct {
	footprints []domain.TrailFootprint
	branches   []domain.TrailBranch
	nextCursor string
	hasMore    bool
	err        error
	gotLimit   int
	gotCursor  string
	gotTags    []string
}

func (f *fakeTrailPort) GetTrailFootprints(_ context.Context, _ uuid.UUID, cursor string, limit int, filterTags []string) ([]domain.TrailFootprint, []domain.TrailBranch, string, bool, error) {
	f.gotCursor = cursor
	f.gotLimit = limit
	f.gotTags = filterTags
	return f.footprints, f.branches, f.nextCursor, f.hasMore, f.err
}

func TestExecute_ReturnsFootprints(t *testing.T) {
	port := &fakeTrailPort{
		footprints: []domain.TrailFootprint{{FootprintKey: "open:a", Verb: "read"}},
		nextCursor: "cur",
		hasMore:    true,
	}
	uc := NewGetKnowledgeTrailUsecase(port)

	res, err := uc.Execute(context.Background(), uuid.New(), "", 20, []string{"rust", "async"})
	require.NoError(t, err)
	assert.Len(t, res.Footprints, 1)
	assert.Equal(t, "cur", res.NextCursor)
	assert.True(t, res.HasMore)
	assert.Equal(t, []string{"rust", "async"}, port.gotTags, "theme-lens filter tags pass through to the port")
}

func TestExecute_NormalizesLimit(t *testing.T) {
	port := &fakeTrailPort{}
	uc := NewGetKnowledgeTrailUsecase(port)

	_, err := uc.Execute(context.Background(), uuid.New(), "", 0, nil)
	require.NoError(t, err)
	assert.Equal(t, defaultLimit, port.gotLimit, "non-positive limit falls back to the default")

	_, err = uc.Execute(context.Background(), uuid.New(), "", 9999, nil)
	require.NoError(t, err)
	assert.Equal(t, defaultLimit, port.gotLimit, "oversized limit falls back to the default")
}

func TestExecute_PropagatesError(t *testing.T) {
	port := &fakeTrailPort{err: errors.New("sovereign down")}
	uc := NewGetKnowledgeTrailUsecase(port)

	_, err := uc.Execute(context.Background(), uuid.New(), "", 20, nil)
	require.Error(t, err)
}
