package register_favorite_feed_gateway

import (
	"alt/domain"
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// What is left on this side of the boundary is validation and vocabulary
// (ADR-000954 D4), so that is what these test. The nil-store case that used to
// be here is gone: a missing store is now a panic at construction rather than
// an error per call, because "the star did nothing" and "the feed does not
// exist" looked identical to the user and only one of them was a wiring bug.

type stubFavoriteStore struct {
	err       error
	gotURL    string
	gotUserID uuid.UUID
	calls     int
}

func (s *stubFavoriteStore) RegisterFavoriteFeed(_ context.Context, url string, userID uuid.UUID) error {
	s.calls++
	s.gotURL, s.gotUserID = url, userID
	return s.err
}

func (s *stubFavoriteStore) RemoveFavoriteFeed(_ context.Context, url string, userID uuid.UUID) error {
	s.calls++
	s.gotURL, s.gotUserID = url, userID
	return s.err
}

func TestRegisterFavoriteFeedGateway_RejectsUnusableURLs(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr string
	}{
		{name: "empty", url: "   ", wantErr: "empty url"},
		{name: "no scheme", url: "example.com/feed.xml", wantErr: "invalid URL format"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &stubFavoriteStore{}
			g := NewRegisterFavoriteFeedGateway(store)

			err := g.RegisterFavoriteFeed(context.Background(), tt.url, uuid.New())
			require.Error(t, err)
			assert.Equal(t, tt.wantErr, err.Error())
			assert.Zero(t, store.calls, "a string the provider could not interpret is rejected here")
		})
	}
}

func TestRegisterFavoriteFeedGateway_TrimsAndForwards(t *testing.T) {
	store := &stubFavoriteStore{}
	g := NewRegisterFavoriteFeedGateway(store)
	userID := uuid.New()

	require.NoError(t, g.RegisterFavoriteFeed(context.Background(), "  https://example.com/feed.xml  ", userID))
	assert.Equal(t, "https://example.com/feed.xml", store.gotURL)
	assert.Equal(t, userID, store.gotUserID)
}

// TestRegisterFavoriteFeedGateway_TranslatesAbsence pins the error vocabulary
// the REST layer renders. The provider reports a URL naming no feed as
// domain.ErrFeedNotFound — the same sentinel every §2.I write now uses
// (capability catalog §4-5) — and this is where it becomes the message the
// screen shows.
func TestRegisterFavoriteFeedGateway_TranslatesAbsence(t *testing.T) {
	tests := []struct {
		name    string
		remove  bool
		err     error
		wantMsg string
	}{
		{name: "add / no such feed", err: domain.ErrFeedNotFound, wantMsg: "feed not found"},
		{name: "remove / no such favorite", remove: true, err: domain.ErrFeedNotFound, wantMsg: "favorite feed not found"},
		{name: "add / transport failure", err: errors.New("connection refused"), wantMsg: "failed to register favorite feed"},
		{name: "remove / transport failure", remove: true, err: errors.New("connection refused"), wantMsg: "failed to remove favorite feed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewRegisterFavoriteFeedGateway(&stubFavoriteStore{err: tt.err})

			var err error
			if tt.remove {
				err = g.RemoveFavoriteFeed(context.Background(), "https://example.com/feed.xml", uuid.New())
			} else {
				err = g.RegisterFavoriteFeed(context.Background(), "https://example.com/feed.xml", uuid.New())
			}

			require.Error(t, err)
			assert.Equal(t, tt.wantMsg, err.Error())
		})
	}
}

func TestNewRegisterFavoriteFeedGateway_PanicsOnNilStore(t *testing.T) {
	assert.Panics(t, func() { NewRegisterFavoriteFeedGateway(nil) })
}
