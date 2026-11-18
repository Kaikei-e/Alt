package sovereign_client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	sovereignv1 "alt/gen/proto/services/sovereign/v1"
	"alt/gen/proto/services/sovereign/v1/sovereignv1connect"
)

type fakeThemeSovereignServer struct {
	sovereignv1connect.UnimplementedKnowledgeSovereignServiceHandler
	themes    []*sovereignv1.ReadingThemeEntry
	seenUser  string
	seenLimit int32
}

func (f *fakeThemeSovereignServer) ListReadingThemes(
	_ context.Context,
	req *connect.Request[sovereignv1.ListReadingThemesRequest],
) (*connect.Response[sovereignv1.ListReadingThemesResponse], error) {
	f.seenUser = req.Msg.GetUserId()
	f.seenLimit = req.Msg.GetLimit()
	return connect.NewResponse(&sovereignv1.ListReadingThemesResponse{
		Themes: f.themes,
	}), nil
}

func TestClient_ListReadingThemes(t *testing.T) {
	theme1 := uuid.New()
	theme2 := uuid.New()
	userID := uuid.New()
	createdAt := time.Date(2026, 11, 17, 8, 0, 0, 0, time.UTC)
	updatedAt := time.Date(2026, 11, 18, 8, 0, 0, 0, time.UTC)
	openedAt := time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC)

	srv := &fakeThemeSovereignServer{
		themes: []*sovereignv1.ReadingThemeEntry{
			{
				ThemeId:           theme1.String(),
				Title:             "Theme One",
				LatestVersion:     2,
				ArticleCount:      5,
				CreatedAt:         timestamppb.New(createdAt),
				UpdatedAt:         timestamppb.New(updatedAt),
				LastOpenedAt:      timestamppb.New(openedAt),
				LastOpenedVersion: 1,
			},
			{
				ThemeId:           theme2.String(),
				Title:             "Theme Two",
				LatestVersion:     1,
				ArticleCount:      3,
				CreatedAt:         timestamppb.New(createdAt),
				UpdatedAt:         timestamppb.New(updatedAt),
				LastOpenedAt:      nil,
				LastOpenedVersion: 0,
			},
		},
	}

	mux := http.NewServeMux()
	path, handler := sovereignv1connect.NewKnowledgeSovereignServiceHandler(srv)
	mux.Handle(path, handler)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	c := NewClient(ts.URL, true)
	rows, err := c.ListReadingThemes(context.Background(), userID, 0)
	require.NoError(t, err)
	assert.Equal(t, userID.String(), srv.seenUser)
	assert.Equal(t, int32(0), srv.seenLimit)
	require.Len(t, rows, 2)

	assert.Equal(t, theme1, rows[0].ThemeID)
	assert.Equal(t, "Theme One", rows[0].Title)
	assert.Equal(t, int32(2), rows[0].LatestVersion)
	assert.Equal(t, int32(5), rows[0].ArticleCount)
	assert.Equal(t, createdAt, rows[0].CreatedAt)
	assert.Equal(t, updatedAt, rows[0].UpdatedAt)
	require.NotNil(t, rows[0].LastOpenedAt)
	assert.Equal(t, openedAt, *rows[0].LastOpenedAt)
	assert.Equal(t, int32(1), rows[0].LastOpenedVersion)

	assert.Equal(t, theme2, rows[1].ThemeID)
	assert.Nil(t, rows[1].LastOpenedAt)
	assert.Equal(t, int32(0), rows[1].LastOpenedVersion)

	t.Run("invalid theme_id fails", func(t *testing.T) {
		badSrv := &fakeThemeSovereignServer{
			themes: []*sovereignv1.ReadingThemeEntry{{ThemeId: "not-a-uuid"}},
		}
		badMux := http.NewServeMux()
		bp, bh := sovereignv1connect.NewKnowledgeSovereignServiceHandler(badSrv)
		badMux.Handle(bp, bh)
		bts := httptest.NewServer(badMux)
		defer bts.Close()

		bc := NewClient(bts.URL, true)
		_, err := bc.ListReadingThemes(context.Background(), userID, 0)
		require.Error(t, err)
	})

	t.Run("disabled client returns ErrSovereignDisabled", func(t *testing.T) {
		disabled := NewClient("", false)
		_, err := disabled.ListReadingThemes(context.Background(), userID, 0)
		assert.ErrorIs(t, err, ErrSovereignDisabled)
	})
}
