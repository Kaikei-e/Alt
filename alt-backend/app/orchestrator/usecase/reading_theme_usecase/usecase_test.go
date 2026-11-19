package reading_theme_usecase

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"alt/domain"
)

type fakeListThemesPort struct {
	themes    []domain.ReadingThemeSummary
	err       error
	seenUser  uuid.UUID
	seenLimit int32
	called    bool
}

func (f *fakeListThemesPort) ListReadingThemes(_ context.Context, userID uuid.UUID, limit int32) ([]domain.ReadingThemeSummary, error) {
	f.called = true
	f.seenUser = userID
	f.seenLimit = limit
	if f.err != nil {
		return nil, f.err
	}
	return f.themes, nil
}

type fakeGetThemePort struct {
	page        *domain.ReadingThemePage
	err         error
	seenUser    uuid.UUID
	seenTheme   uuid.UUID
	seenVersion *int32
	called      bool
}

func (f *fakeGetThemePort) GetTheme(_ context.Context, userID uuid.UUID, themeID uuid.UUID, version *int32) (*domain.ReadingThemePage, error) {
	f.called = true
	f.seenUser = userID
	f.seenTheme = themeID
	f.seenVersion = version
	if f.err != nil {
		return nil, f.err
	}
	return f.page, nil
}

type fakeAppendPort struct {
	events []domain.KnowledgeEvent
	seq    int64
	err    error
}

func (f *fakeAppendPort) AppendKnowledgeEvent(_ context.Context, ev domain.KnowledgeEvent) (int64, error) {
	if f.err != nil {
		return 0, f.err
	}
	f.events = append(f.events, ev)
	return f.seq, nil
}

func TestListThemesUsecase(t *testing.T) {
	require.Panics(t, func() { NewListThemesUsecase(nil) })

	userID := uuid.New()
	expected := []domain.ReadingThemeSummary{{ThemeID: uuid.New(), Title: "Go"}}
	port := &fakeListThemesPort{themes: expected}
	uc := NewListThemesUsecase(port)

	got, err := uc.Execute(context.Background(), userID)
	require.NoError(t, err)
	assert.True(t, port.called)
	assert.Equal(t, userID, port.seenUser)
	assert.Equal(t, int32(0), port.seenLimit)
	assert.Equal(t, expected, got)

	t.Run("nil user_id fails", func(t *testing.T) {
		p := &fakeListThemesPort{}
		u := NewListThemesUsecase(p)
		_, err := u.Execute(context.Background(), uuid.Nil)
		require.ErrorIs(t, err, domain.ErrInvalidThemeInput)
		assert.False(t, p.called)
	})
}

func TestGetThemeUsecase(t *testing.T) {
	require.Panics(t, func() { NewGetThemeUsecase(nil) })

	userID := uuid.New()
	themeID := uuid.New()

	t.Run("validates theme_id and version before calling port", func(t *testing.T) {
		p := &fakeGetThemePort{}
		uc := NewGetThemeUsecase(p)

		_, err := uc.Execute(context.Background(), userID, "bad-uuid", nil)
		require.ErrorIs(t, err, domain.ErrInvalidThemeInput)
		assert.False(t, p.called)

		zero := int32(0)
		_, err = uc.Execute(context.Background(), userID, themeID.String(), &zero)
		require.ErrorIs(t, err, domain.ErrInvalidThemeInput)
		assert.False(t, p.called)

		neg := int32(-1)
		_, err = uc.Execute(context.Background(), userID, themeID.String(), &neg)
		require.ErrorIs(t, err, domain.ErrInvalidThemeInput)
		assert.False(t, p.called)

		_, err = uc.Execute(context.Background(), uuid.Nil, themeID.String(), nil)
		require.ErrorIs(t, err, domain.ErrInvalidThemeInput)
		assert.False(t, p.called)
	})

	t.Run("passes user_id, theme_id, and optional version", func(t *testing.T) {
		page := &domain.ReadingThemePage{ThemeID: themeID.String(), Version: 2}
		p := &fakeGetThemePort{page: page}
		uc := NewGetThemeUsecase(p)

		v := int32(2)
		got, err := uc.Execute(context.Background(), userID, themeID.String(), &v)
		require.NoError(t, err)
		assert.Equal(t, page, got)
		assert.Equal(t, userID, p.seenUser)
		assert.Equal(t, themeID, p.seenTheme)
		require.NotNil(t, p.seenVersion)
		assert.Equal(t, int32(2), *p.seenVersion)
	})

	t.Run("propagates ErrThemeNotFound", func(t *testing.T) {
		p := &fakeGetThemePort{err: domain.ErrThemeNotFound}
		uc := NewGetThemeUsecase(p)
		_, err := uc.Execute(context.Background(), userID, themeID.String(), nil)
		require.ErrorIs(t, err, domain.ErrThemeNotFound)
	})
}

func TestRecordThemeOpenedUsecase(t *testing.T) {
	fixedNow := func() time.Time { return time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC) }
	require.Panics(t, func() { NewRecordThemeOpenedUsecase(nil, fixedNow) })
	require.Panics(t, func() { NewRecordThemeOpenedUsecase(&fakeAppendPort{}, nil) })

	userID := uuid.New()
	tenantID := uuid.New()
	themeID := uuid.New()
	clientEventID := uuid.New()
	openedAt := time.Date(2026, 11, 18, 11, 30, 0, 0, time.UTC)

	t.Run("validation errors reject before appending", func(t *testing.T) {
		ap := &fakeAppendPort{seq: 1}
		uc := NewRecordThemeOpenedUsecase(ap, fixedNow)

		assert.ErrorIs(t, uc.Execute(context.Background(), uuid.Nil, tenantID, themeID.String(), 1, clientEventID.String(), openedAt), domain.ErrInvalidThemeInput)
		assert.ErrorIs(t, uc.Execute(context.Background(), userID, tenantID, "not-uuid", 1, clientEventID.String(), openedAt), domain.ErrInvalidThemeInput)
		assert.ErrorIs(t, uc.Execute(context.Background(), userID, tenantID, themeID.String(), 0, clientEventID.String(), openedAt), domain.ErrInvalidThemeInput)
		assert.ErrorIs(t, uc.Execute(context.Background(), userID, tenantID, themeID.String(), -2, clientEventID.String(), openedAt), domain.ErrInvalidThemeInput)
		assert.ErrorIs(t, uc.Execute(context.Background(), userID, tenantID, themeID.String(), 1, "not-uuid", openedAt), domain.ErrInvalidThemeInput)
		assert.ErrorIs(t, uc.Execute(context.Background(), userID, tenantID, themeID.String(), 1, clientEventID.String(), time.Time{}), domain.ErrInvalidThemeInput)
		assert.Empty(t, ap.events)
	})

	t.Run("appends reading.theme_opened.v1 with exact payload and clamped event time", func(t *testing.T) {
		ap := &fakeAppendPort{seq: 42}
		uc := NewRecordThemeOpenedUsecase(ap, fixedNow)

		err := uc.Execute(context.Background(), userID, tenantID, themeID.String(), 2, clientEventID.String(), openedAt)
		require.NoError(t, err)
		require.Len(t, ap.events, 1)

		ev := ap.events[0]
		assert.Equal(t, domain.EventReadingThemeOpened, ev.EventType)
		assert.Equal(t, domain.AggregateReadingTheme, ev.AggregateType)
		assert.Equal(t, themeID.String(), ev.AggregateID)
		assert.Equal(t, "reading.theme_opened:"+clientEventID.String(), ev.DedupeKey)
		assert.Equal(t, domain.ActorUser, ev.ActorType)
		assert.Equal(t, userID.String(), ev.ActorID)
		require.NotNil(t, ev.UserID)
		assert.Equal(t, userID, *ev.UserID)
		assert.Equal(t, tenantID, ev.TenantID)
		assert.Equal(t, openedAt, ev.OccurredAt)

		var raw map[string]any
		require.NoError(t, json.Unmarshal(ev.Payload, &raw))
		assert.Equal(t, map[string]any{
			"theme_id":  themeID.String(),
			"version":   float64(2),
			"opened_at": "2026-11-18T11:30:00Z",
		}, raw)
	})

	t.Run("clamps skewed client timestamp beyond 24h to receivedAt", func(t *testing.T) {
		ap := &fakeAppendPort{seq: 43}
		uc := NewRecordThemeOpenedUsecase(ap, fixedNow)

		farFuture := time.Date(2026, 11, 25, 0, 0, 0, 0, time.UTC)
		err := uc.Execute(context.Background(), userID, tenantID, themeID.String(), 3, clientEventID.String(), farFuture)
		require.NoError(t, err)
		require.Len(t, ap.events, 1)

		ev := ap.events[0]
		assert.Equal(t, fixedNow(), ev.OccurredAt)

		var raw map[string]any
		require.NoError(t, json.Unmarshal(ev.Payload, &raw))
		assert.Equal(t, "2026-11-18T12:00:00Z", raw["opened_at"])
	})

	t.Run("returns error when append fails", func(t *testing.T) {
		ap := &fakeAppendPort{err: errors.New("sovereign down")}
		uc := NewRecordThemeOpenedUsecase(ap, fixedNow)

		err := uc.Execute(context.Background(), userID, tenantID, themeID.String(), 1, clientEventID.String(), openedAt)
		require.Error(t, err)
	})
}
