package global_search_gateway

import (
	"alt/domain"
	"alt/utils/logger"
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubTagSearcher struct {
	hits      []domain.GlobalTagHit
	err       error
	gotPrefix string
	gotLimit  int
}

func (s *stubTagSearcher) SearchTagsByPrefix(_ context.Context, prefix string, limit int) ([]domain.GlobalTagHit, error) {
	s.gotPrefix, s.gotLimit = prefix, limit
	return s.hits, s.err
}

// A nil searcher is a panic at construction rather than an error per call.
// The old behaviour returned "tag repository not available" from
// SearchTagsByPrefix, which the global search usecase folds into an empty tag
// section — so an unwired gateway looked exactly like a query that matched
// nothing (CLAUDE.md rule 8).
func TestNewTagSearchGateway_PanicsOnNilSearcher(t *testing.T) {
	assert.Panics(t, func() { NewTagSearchGateway(nil) })
}

func TestTagSearchGateway_SearchTagsByPrefix(t *testing.T) {
	logger.InitLogger()

	searcher := &stubTagSearcher{hits: []domain.GlobalTagHit{
		{TagName: "AI", ArticleCount: 42},
		{TagName: "Algorithms", ArticleCount: 7},
	}}
	gw := NewTagSearchGateway(searcher)

	section, err := gw.SearchTagsByPrefix(context.Background(), "A", 10)
	require.NoError(t, err)
	assert.Equal(t, "A", searcher.gotPrefix)
	assert.Equal(t, 10, searcher.gotLimit)
	assert.Len(t, section.Hits, 2)
	assert.Equal(t, int64(2), section.Total)
}

// TestTagSearchGateway_WrapsTheSourceError keeps the cause reachable: the
// previous version returned the raw error, so which of the search sections had
// failed was not in the message.
func TestTagSearchGateway_WrapsTheSourceError(t *testing.T) {
	logger.InitLogger()

	cause := errors.New("connection refused")
	gw := NewTagSearchGateway(&stubTagSearcher{err: cause})

	section, err := gw.SearchTagsByPrefix(context.Background(), "ai", 10)
	require.Error(t, err)
	assert.ErrorIs(t, err, cause)
	assert.Nil(t, section)
}
