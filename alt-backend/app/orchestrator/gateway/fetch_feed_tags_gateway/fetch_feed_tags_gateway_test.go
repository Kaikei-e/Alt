package fetch_feed_tags_gateway

import (
	"alt/domain"
	"alt/utils/logger"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The nil-source cases these tests used to cover are gone: since the read
// moved to alt-data-hub (capability catalog §2.J W3-J2), a missing source is a
// panic at construction rather than "database connection not available" on
// every call. A feed with no topics and a gateway nobody wired looked the same
// to the caller, and only one of them was a bug.

type stubFeedTagReader struct {
	tags      []*domain.FeedTag
	err       error
	gotFeedID string
	gotCursor *time.Time
	gotLimit  int
}

func (s *stubFeedTagReader) FetchFeedTags(_ context.Context, feedID string, cursor *time.Time, limit int) ([]*domain.FeedTag, error) {
	s.gotFeedID, s.gotCursor, s.gotLimit = feedID, cursor, limit
	return s.tags, s.err
}

func TestFetchFeedTagsGateway_ForwardsCursorAndLimit(t *testing.T) {
	logger.InitLogger()

	cursor := time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC)
	feedID := uuid.New().String()

	tests := []struct {
		name   string
		cursor *time.Time
		limit  int
	}{
		{name: "first page", cursor: nil, limit: 20},
		{name: "subsequent page", cursor: &cursor, limit: 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := &stubFeedTagReader{tags: []*domain.FeedTag{{ID: "t1", TagName: "AI"}}}
			g := NewFetchFeedTagsGateway(reader)

			tags, err := g.FetchFeedTags(context.Background(), feedID, tt.cursor, tt.limit)
			require.NoError(t, err)
			require.Len(t, tags, 1)
			assert.Equal(t, feedID, reader.gotFeedID)
			assert.Equal(t, tt.cursor, reader.gotCursor)
			assert.Equal(t, tt.limit, reader.gotLimit)
		})
	}
}

// TestFetchFeedTagsGateway_WrapsTheSourceError keeps the cause reachable.
// The previous version replaced every failure with a fixed string, which meant
// a timeout and a malformed feed id were indistinguishable in a log.
func TestFetchFeedTagsGateway_WrapsTheSourceError(t *testing.T) {
	logger.InitLogger()

	cause := errors.New("connection refused")
	g := NewFetchFeedTagsGateway(&stubFeedTagReader{err: cause})

	tags, err := g.FetchFeedTags(context.Background(), uuid.New().String(), nil, 20)
	require.Error(t, err)
	assert.ErrorIs(t, err, cause)
	assert.Nil(t, tags)
}

func TestNewFetchFeedTagsGateway_PanicsOnNilReader(t *testing.T) {
	assert.Panics(t, func() { NewFetchFeedTagsGateway(nil) })
}
