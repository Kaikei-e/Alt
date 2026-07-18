package trail_thumbnail_gateway

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeArticleHeadsDB struct {
	got    []string
	result map[string]string
	err    error
}

func (f *fakeArticleHeadsDB) FetchOgImageURLsByArticleIDs(_ context.Context, articleIDs []string) (map[string]string, error) {
	f.got = articleIDs
	return f.result, f.err
}

func TestGetOgImageURLsByArticleIDs_DelegatesToArticleHeadsLookup(t *testing.T) {
	db := &fakeArticleHeadsDB{result: map[string]string{"a1": "https://example.com/a1.png"}}
	gw := newGateway(db)

	got, err := gw.GetOgImageURLsByArticleIDs(context.Background(), []string{"a1", "a2"})
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"a1": "https://example.com/a1.png"}, got)
	assert.Equal(t, []string{"a1", "a2"}, db.got)
}

func TestGetOgImageURLsByArticleIDs_PropagatesError(t *testing.T) {
	db := &fakeArticleHeadsDB{err: errors.New("db down")}
	gw := newGateway(db)

	_, err := gw.GetOgImageURLsByArticleIDs(context.Background(), []string{"a1"})
	require.Error(t, err)
}
