package reading

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"alt/domain"
	readingv1 "alt/gen/proto/alt/reading/v1"
	"alt/orchestrator/usecase/reading_queue_usecase"
	"alt/orchestrator/usecase/reading_theme_usecase"
	"alt/orchestrator/usecase/record_reading_events_usecase"
)

type fakeListThemesPort struct {
	themes []domain.ReadingThemeSummary
	err    error
}

func (f *fakeListThemesPort) ListReadingThemes(_ context.Context, _ uuid.UUID, _ int32) ([]domain.ReadingThemeSummary, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.themes, nil
}

type fakeGetThemePort struct {
	page        *domain.ReadingThemePage
	err         error
	seenUserID  uuid.UUID
	seenThemeID uuid.UUID
	seenVersion *int32
	called      bool
}

func (f *fakeGetThemePort) GetTheme(_ context.Context, userID uuid.UUID, themeID uuid.UUID, version *int32) (*domain.ReadingThemePage, error) {
	f.called = true
	f.seenUserID = userID
	f.seenThemeID = themeID
	f.seenVersion = version
	if f.err != nil {
		return nil, f.err
	}
	return f.page, nil
}

func newThemeTestHandler(
	appendPort *fakeAppendPort,
	listPort *fakeListThemesPort,
	getPort *fakeGetThemePort,
) *Handler {
	lookup := &fakeLookupPort{}
	recUC := record_reading_events_usecase.NewRecordReadingEventsUsecase(appendPort, lookup, &fakeStatusPort{}, fixedClock())
	qUC := reading_queue_usecase.NewQueueArticleUsecase(&fakeQueuePort{}, appendPort, lookup, &fakeArticleBatchPort{}, &fakeOgImagePort{}, 7, 48, fixedClock())
	uqUC := reading_queue_usecase.NewUnqueueArticleUsecase(appendPort, fixedClock())
	gUC := reading_queue_usecase.NewGetReadTodayQueueUsecase(&fakeQueuePort{}, &fakeCountPort{}, &fakeArticleBatchPort{}, &fakeOgImagePort{}, 7, fixedClock())
	ltUC := reading_theme_usecase.NewListThemesUsecase(listPort)
	gtUC := reading_theme_usecase.NewGetThemeUsecase(getPort)
	rtoUC := reading_theme_usecase.NewRecordThemeOpenedUsecase(appendPort, fixedClock())
	return NewHandler(recUC, qUC, uqUC, gUC, ltUC, gtUC, rtoUC, nil, nil)
}

func TestThemeHandlers_Unauthenticated(t *testing.T) {
	h := newThemeTestHandler(&fakeAppendPort{}, &fakeListThemesPort{}, &fakeGetThemePort{})

	_, err := h.ListThemes(context.Background(), connect.NewRequest(&readingv1.ListThemesRequest{}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))

	_, err = h.GetTheme(context.Background(), connect.NewRequest(&readingv1.GetThemeRequest{
		ThemeId: uuid.NewString(),
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))

	_, err = h.RecordThemeOpened(context.Background(), connect.NewRequest(&readingv1.RecordThemeOpenedRequest{
		ThemeId:       uuid.NewString(),
		Version:       1,
		ClientEventId: uuid.NewString(),
		OpenedAt:      timestamppb.Now(),
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
}

func TestHandler_ListThemes(t *testing.T) {
	theme1 := uuid.MustParse("44444444-4444-4444-4444-444444444444")
	theme2 := uuid.MustParse("55555555-5555-5555-5555-555555555555")
	updatedAt := time.Date(2026, 11, 18, 8, 0, 0, 0, time.UTC)
	openedAt := time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC)

	h := newThemeTestHandler(
		&fakeAppendPort{},
		&fakeListThemesPort{
			themes: []domain.ReadingThemeSummary{
				{
					ThemeID:           theme1,
					Title:             "Theme One",
					LatestVersion:     2,
					ArticleCount:      5,
					UpdatedAt:         updatedAt,
					LastOpenedAt:      &openedAt,
					LastOpenedVersion: 1,
				},
				{
					ThemeID:           theme2,
					Title:             "Theme Two",
					LatestVersion:     1,
					ArticleCount:      3,
					UpdatedAt:         updatedAt,
					LastOpenedAt:      nil,
					LastOpenedVersion: 0,
				},
			},
		},
		&fakeGetThemePort{},
	)

	resp, err := h.ListThemes(authenticatedContext(t), connect.NewRequest(&readingv1.ListThemesRequest{}))
	require.NoError(t, err)
	require.Len(t, resp.Msg.GetThemes(), 2)

	t1 := resp.Msg.GetThemes()[0]
	assert.Equal(t, theme1.String(), t1.GetThemeId())
	assert.Equal(t, "Theme One", t1.GetTitle())
	assert.Equal(t, int32(2), t1.GetLatestVersion())
	assert.Equal(t, int32(5), t1.GetArticleCount())
	assert.Equal(t, updatedAt, t1.GetUpdatedAt().AsTime())
	require.NotNil(t, t1.GetLastOpenedAt())
	assert.Equal(t, openedAt, t1.GetLastOpenedAt().AsTime())
	assert.Equal(t, int32(1), t1.GetLastOpenedVersion())

	t2 := resp.Msg.GetThemes()[1]
	assert.Equal(t, theme2.String(), t2.GetThemeId())
	assert.Nil(t, t2.GetLastOpenedAt())
	assert.Equal(t, int32(0), t2.GetLastOpenedVersion())

	t.Run("upstream failure maps to Internal", func(t *testing.T) {
		errH := newThemeTestHandler(&fakeAppendPort{}, &fakeListThemesPort{err: errors.New("sovereign down")}, &fakeGetThemePort{})
		_, err := errH.ListThemes(authenticatedContext(t), connect.NewRequest(&readingv1.ListThemesRequest{}))
		require.Error(t, err)
		assert.Equal(t, connect.CodeInternal, connect.CodeOf(err))
	})
}

func TestHandler_GetTheme(t *testing.T) {
	themeID := uuid.MustParse("44444444-4444-4444-4444-444444444444")
	createdAt := time.Date(2026, 11, 18, 8, 0, 0, 0, time.UTC)

	t.Run("returns full theme page and uses authenticated user id", func(t *testing.T) {
		gp := &fakeGetThemePort{
			page: &domain.ReadingThemePage{
				ThemeID:       themeID.String(),
				Title:         "Systems",
				Version:       2,
				LatestVersion: 3,
				CreatedAt:     createdAt,
				KeyPoints:     []domain.ThemeSentence{{Text: "KP", SourceArticleIDs: []string{"a1"}}},
				Timeline:      []domain.ThemeTimelineEntry{{Date: "2026-11-18", Text: "TL", SourceArticleIDs: []string{"a1"}}},
				WhatChanged:   []domain.ThemeSentence{{Text: "WC", SourceArticleIDs: []string{"a1"}}},
				OpenQuestions: []domain.ThemeSentence{{Text: "OQ", SourceArticleIDs: []string{"a1"}}},
				Sources:       []domain.ThemeSource{{ArticleID: "a1", Title: "Title", URL: "https://example.com"}},
				Versions:      []domain.ThemeVersionRef{{Version: 3, CreatedAt: createdAt, ArticleCount: 4}},
			},
		}
		h := newThemeTestHandler(&fakeAppendPort{}, &fakeListThemesPort{}, gp)

		v := int32(2)
		resp, err := h.GetTheme(authenticatedContext(t), connect.NewRequest(&readingv1.GetThemeRequest{
			ThemeId: themeID.String(),
			Version: &v,
		}))
		require.NoError(t, err)
		assert.Equal(t, uuid.MustParse("3f2b1c8e-5a4d-4e6f-9b0a-1c2d3e4f5a6b"), gp.seenUserID)
		assert.Equal(t, themeID, gp.seenThemeID)
		require.NotNil(t, gp.seenVersion)
		assert.Equal(t, int32(2), *gp.seenVersion)

		assert.Equal(t, themeID.String(), resp.Msg.GetThemeId())
		assert.Equal(t, "Systems", resp.Msg.GetTitle())
		assert.Equal(t, int32(2), resp.Msg.GetVersion())
		assert.Equal(t, int32(3), resp.Msg.GetLatestVersion())
		assert.Equal(t, createdAt, resp.Msg.GetCreatedAt().AsTime())
		require.Len(t, resp.Msg.GetKeyPoints(), 1)
		assert.Equal(t, "KP", resp.Msg.GetKeyPoints()[0].GetText())
		require.Len(t, resp.Msg.GetTimeline(), 1)
		assert.Equal(t, "2026-11-18", resp.Msg.GetTimeline()[0].GetDate())
		require.Len(t, resp.Msg.GetWhatChanged(), 1)
		require.Len(t, resp.Msg.GetOpenQuestions(), 1)
		require.Len(t, resp.Msg.GetSources(), 1)
		require.Len(t, resp.Msg.GetVersions(), 1)
	})

	t.Run("validation errors return InvalidArgument without calling gateway", func(t *testing.T) {
		gp := &fakeGetThemePort{}
		h := newThemeTestHandler(&fakeAppendPort{}, &fakeListThemesPort{}, gp)

		_, err := h.GetTheme(authenticatedContext(t), connect.NewRequest(&readingv1.GetThemeRequest{
			ThemeId: "not-a-uuid",
		}))
		require.Error(t, err)
		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
		assert.False(t, gp.called)

		zero := int32(0)
		_, err = h.GetTheme(authenticatedContext(t), connect.NewRequest(&readingv1.GetThemeRequest{
			ThemeId: themeID.String(),
			Version: &zero,
		}))
		require.Error(t, err)
		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
		assert.False(t, gp.called)
	})

	t.Run("ErrThemeNotFound maps to NotFound", func(t *testing.T) {
		gp := &fakeGetThemePort{err: domain.ErrThemeNotFound}
		h := newThemeTestHandler(&fakeAppendPort{}, &fakeListThemesPort{}, gp)

		_, err := h.GetTheme(authenticatedContext(t), connect.NewRequest(&readingv1.GetThemeRequest{
			ThemeId: themeID.String(),
		}))
		require.Error(t, err)
		assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
	})

	t.Run("upstream error maps to Internal", func(t *testing.T) {
		gp := &fakeGetThemePort{err: errors.New("recap-worker 503")}
		h := newThemeTestHandler(&fakeAppendPort{}, &fakeListThemesPort{}, gp)

		_, err := h.GetTheme(authenticatedContext(t), connect.NewRequest(&readingv1.GetThemeRequest{
			ThemeId: themeID.String(),
		}))
		require.Error(t, err)
		assert.Equal(t, connect.CodeInternal, connect.CodeOf(err))
	})
}

func TestHandler_RecordThemeOpened(t *testing.T) {
	themeID := uuid.MustParse("44444444-4444-4444-4444-444444444444")
	clientEventID := uuid.MustParse("019a0000-0007-7000-8000-000000000107")
	openedAt := time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC)

	t.Run("appends event and returns empty response", func(t *testing.T) {
		ap := &fakeAppendPort{seqResults: []int64{10}}
		h := newThemeTestHandler(ap, &fakeListThemesPort{}, &fakeGetThemePort{})

		resp, err := h.RecordThemeOpened(authenticatedContext(t), connect.NewRequest(&readingv1.RecordThemeOpenedRequest{
			ThemeId:       themeID.String(),
			Version:       2,
			ClientEventId: clientEventID.String(),
			OpenedAt:      timestamppb.New(openedAt),
		}))
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Len(t, ap.events, 1)
		assert.Equal(t, domain.EventReadingThemeOpened, ap.events[0].EventType)
		assert.Equal(t, "reading.theme_opened:"+clientEventID.String(), ap.events[0].DedupeKey)

		var raw map[string]any
		require.NoError(t, json.Unmarshal(ap.events[0].Payload, &raw))
		assert.Equal(t, themeID.String(), raw["theme_id"])
		assert.InDelta(t, float64(2), raw["version"], 1e-9)
		assert.Equal(t, "2026-11-18T00:00:00Z", raw["opened_at"])
	})

	t.Run("validation errors return InvalidArgument", func(t *testing.T) {
		ap := &fakeAppendPort{}
		h := newThemeTestHandler(ap, &fakeListThemesPort{}, &fakeGetThemePort{})

		_, err := h.RecordThemeOpened(authenticatedContext(t), connect.NewRequest(&readingv1.RecordThemeOpenedRequest{
			ThemeId:       themeID.String(),
			Version:       1,
			ClientEventId: clientEventID.String(),
			OpenedAt:      nil,
		}))
		require.Error(t, err)
		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
		assert.Empty(t, ap.events)
	})

	t.Run("append error is returned to caller", func(t *testing.T) {
		ap := &fakeAppendPort{err: errors.New("sovereign append failed")}
		h := newThemeTestHandler(ap, &fakeListThemesPort{}, &fakeGetThemePort{})

		_, err := h.RecordThemeOpened(authenticatedContext(t), connect.NewRequest(&readingv1.RecordThemeOpenedRequest{
			ThemeId:       themeID.String(),
			Version:       1,
			ClientEventId: clientEventID.String(),
			OpenedAt:      timestamppb.New(openedAt),
		}))
		require.Error(t, err)
		assert.Equal(t, connect.CodeInternal, connect.CodeOf(err))
	})
}
