package record_reading_events_usecase

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"alt/domain"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeAppendPort struct {
	events     []domain.KnowledgeEvent
	seqResults []int64
	errAtCall  int
	err        error
}

func (f *fakeAppendPort) AppendKnowledgeEvent(_ context.Context, event domain.KnowledgeEvent) (int64, error) {
	callIdx := len(f.events)
	if f.err != nil && (f.errAtCall == 0 || callIdx+1 == f.errAtCall) {
		return 0, f.err
	}
	f.events = append(f.events, event)
	if callIdx < len(f.seqResults) {
		return f.seqResults[callIdx], nil
	}
	return int64(callIdx + 1), nil
}

func TestNewRecordReadingEventsUsecase_PanicsOnNilDeps(t *testing.T) {
	fixedNow := func() time.Time { return time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC) }

	assert.Panics(t, func() {
		NewRecordReadingEventsUsecase(nil, fixedNow)
	})
	assert.Panics(t, func() {
		NewRecordReadingEventsUsecase(&fakeAppendPort{}, nil)
	})
}

func TestRecordReadingEventsUsecase_EmptyBatchIsNoOp(t *testing.T) {
	port := &fakeAppendPort{}
	fixedNow := func() time.Time { return time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC) }
	uc := NewRecordReadingEventsUsecase(port, fixedNow)

	userID := uuid.New()
	tenantID := uuid.New()

	res, err := uc.Execute(context.Background(), userID, tenantID, nil)
	require.NoError(t, err)
	assert.Equal(t, Result{Accepted: 0, Duplicates: 0}, res)
	assert.Empty(t, port.events)
}

func TestRecordReadingEventsUsecase_InvalidBatchAppendsNothing(t *testing.T) {
	fixedTime := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	fixedNow := func() time.Time { return fixedTime }
	validID1 := "01960000-0000-7000-8000-000000000001"
	validArticleID := "11111111-1111-4111-8111-111111111111"

	validEvent := domain.ReadingEventInput{
		ClientEventID: validID1,
		OccurredAt:    fixedTime,
		Payload:       domain.SurfaceViewedInput{Surface: domain.ReadingSurfaceGallery},
	}

	tests := []struct {
		name   string
		events []domain.ReadingEventInput
	}{
		{
			name: "second event has invalid client_event_id",
			events: []domain.ReadingEventInput{
				validEvent,
				{
					ClientEventID: "not-a-uuid",
					OccurredAt:    fixedTime,
					Payload:       domain.SurfaceViewedInput{Surface: domain.ReadingSurfaceSwipe},
				},
			},
		},
		{
			name: "missing occurred_at",
			events: []domain.ReadingEventInput{
				{
					ClientEventID: validID1,
					Payload:       domain.SurfaceViewedInput{Surface: domain.ReadingSurfaceGallery},
				},
			},
		},
		{
			name: "unspecified surface",
			events: []domain.ReadingEventInput{
				{
					ClientEventID: validID1,
					OccurredAt:    fixedTime,
					Payload:       domain.SurfaceViewedInput{Surface: ""},
				},
			},
		},
		{
			name: "invalid article_id on reached_end",
			events: []domain.ReadingEventInput{
				{
					ClientEventID: validID1,
					OccurredAt:    fixedTime,
					Payload: domain.ReachedEndInput{
						ArticleID: "not-uuid",
						Surface:   domain.ReadingSurfaceGallery,
					},
				},
			},
		},
		{
			name: "invalid metrics on session_ended",
			events: []domain.ReadingEventInput{
				{
					ClientEventID: validID1,
					OccurredAt:    fixedTime,
					Payload: domain.SessionEndedInput{
						ArticleID:       validArticleID,
						Surface:         domain.ReadingSurfaceArticlePage,
						ActiveDwellMs:   -5,
						MaxScrollRatio:  0.5,
						BodyLengthChars: 100,
					},
				},
			},
		},
		{
			name: "more than 50 events",
			events: func() []domain.ReadingEventInput {
				out := make([]domain.ReadingEventInput, 51)
				for i := range out {
					out[i] = validEvent
				}
				return out
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			port := &fakeAppendPort{}
			uc := NewRecordReadingEventsUsecase(port, fixedNow)

			_, err := uc.Execute(context.Background(), uuid.New(), uuid.New(), tt.events)
			require.Error(t, err)
			assert.True(t, errors.Is(err, ErrInvalidInput))
			assert.True(t, errors.Is(err, domain.ErrInvalidReadingEvent))
			assert.Empty(t, port.events)
		})
	}
}

func TestRecordReadingEventsUsecase_AppendsWireFormatAndCountsDuplicates(t *testing.T) {
	receivedAt := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	occurredAt := time.Date(2026, 11, 18, 11, 59, 0, 0, time.UTC)
	fixedNow := func() time.Time { return receivedAt }

	userID := uuid.MustParse("3f2b1c8e-5a4d-4e6f-9b0a-1c2d3e4f5a6b")
	tenantID := uuid.MustParse("7a8b9c0d-1e2f-4a3b-8c5d-6e7f8a9b0c1d")
	articleID := "11111111-1111-4111-8111-111111111111"
	id1 := "01960000-0000-7000-8000-000000000001"
	id2 := "01960000-0000-7000-8000-000000000002"
	id3 := "01960000-0000-7000-8000-000000000003"
	id4 := "01960000-0000-7000-8000-000000000004"

	port := &fakeAppendPort{
		seqResults: []int64{101, 0, 102, 103},
	}
	uc := NewRecordReadingEventsUsecase(port, fixedNow)

	events := []domain.ReadingEventInput{
		{
			ClientEventID: id1,
			OccurredAt:    occurredAt,
			Payload:       domain.SurfaceViewedInput{Surface: domain.ReadingSurfaceGallery},
		},
		{
			ClientEventID: id2,
			OccurredAt:    occurredAt,
			Payload: domain.ReachedEndInput{
				ArticleID: articleID,
				Surface:   domain.ReadingSurfaceSwipe,
			},
		},
		{
			ClientEventID: id3,
			OccurredAt:    occurredAt,
			Payload: domain.SessionEndedInput{
				ArticleID:       articleID,
				Surface:         domain.ReadingSurfaceArticlePage,
				ActiveDwellMs:   0,
				MaxScrollRatio:  0,
				BodyLengthChars: 0,
				ReachedEnd:      false,
			},
		},
		{
			ClientEventID: id4,
			OccurredAt:    occurredAt,
			Payload:       domain.SurfaceViewedInput{Surface: domain.ReadingSurfaceOther},
		},
	}

	res, err := uc.Execute(context.Background(), userID, tenantID, events)
	require.NoError(t, err)
	assert.Equal(t, Result{Accepted: 3, Duplicates: 1}, res)
	require.Len(t, port.events, 4)

	// Event 1: surface_viewed
	e1 := port.events[0]
	assert.NotEqual(t, uuid.Nil, e1.EventID)
	assert.Equal(t, occurredAt, e1.OccurredAt)
	assert.Equal(t, tenantID, e1.TenantID)
	require.NotNil(t, e1.UserID)
	assert.Equal(t, userID, *e1.UserID)
	assert.Equal(t, domain.ActorUser, e1.ActorType)
	assert.Equal(t, userID.String(), e1.ActorID)
	assert.Equal(t, "reading.surface_viewed.v1", e1.EventType)
	assert.Equal(t, "reading_session", e1.AggregateType)
	assert.Equal(t, "surface:gallery", e1.AggregateID)
	assert.Equal(t, "reading.surface_viewed.v1:"+id1, e1.DedupeKey)

	var p1 map[string]any
	require.NoError(t, json.Unmarshal(e1.Payload, &p1))
	assert.Equal(t, map[string]any{
		"client_event_id": id1,
		"surface":         "gallery",
		"received_at":     "2026-11-18T12:00:00Z",
	}, p1)

	// Event 2: reached_end
	e2 := port.events[1]
	assert.Equal(t, "reading.reached_end.v1", e2.EventType)
	assert.Equal(t, "reading_session", e2.AggregateType)
	assert.Equal(t, "article:"+articleID, e2.AggregateID)
	assert.Equal(t, "reading.reached_end.v1:"+id2, e2.DedupeKey)

	var p2 map[string]any
	require.NoError(t, json.Unmarshal(e2.Payload, &p2))
	assert.Equal(t, map[string]any{
		"client_event_id": id2,
		"article_id":      articleID,
		"surface":         "swipe",
		"received_at":     "2026-11-18T12:00:00Z",
	}, p2)

	// Event 3: session_ended (verifying zero values are serialized explicitly)
	e3 := port.events[2]
	assert.Equal(t, "reading.session_ended.v1", e3.EventType)
	assert.Equal(t, "reading_session", e3.AggregateType)
	assert.Equal(t, "article:"+articleID, e3.AggregateID)
	assert.Equal(t, "reading.session_ended.v1:"+id3, e3.DedupeKey)

	var p3 map[string]any
	require.NoError(t, json.Unmarshal(e3.Payload, &p3))
	assert.Equal(t, map[string]any{
		"client_event_id":   id3,
		"article_id":        articleID,
		"surface":           "article_page",
		"active_dwell_ms":   float64(0),
		"max_scroll_ratio":  float64(0),
		"body_length_chars": float64(0),
		"reached_end":       false,
		"received_at":       "2026-11-18T12:00:00Z",
	}, p3)

	// Event 4: surface_viewed with other surface
	e4 := port.events[3]
	assert.Equal(t, "reading.surface_viewed.v1", e4.EventType)
	assert.Equal(t, "reading_session", e4.AggregateType)
	assert.Equal(t, "surface:other", e4.AggregateID)
	assert.Equal(t, "reading.surface_viewed.v1:"+id4, e4.DedupeKey)

	var p4 map[string]any
	require.NoError(t, json.Unmarshal(e4.Payload, &p4))
	assert.Equal(t, map[string]any{
		"client_event_id": id4,
		"surface":         "other",
		"received_at":     "2026-11-18T12:00:00Z",
	}, p4)
}

func TestRecordReadingEventsUsecase_ClockSkewHandling(t *testing.T) {
	receivedAt := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	fixedNow := func() time.Time { return receivedAt }
	userID := uuid.New()
	tenantID := uuid.New()
	id := "01960000-0000-7000-8000-000000000001"

	jst := time.FixedZone("JST", 9*3600)

	tests := []struct {
		name               string
		clientOccurredAt   time.Time
		wantEventOccurred  time.Time
		wantClientOccurred string
	}{
		{
			name:              "within 24h boundary past (-24h exact) keeps client occurred_at",
			clientOccurredAt:  receivedAt.Add(-24 * time.Hour),
			wantEventOccurred: receivedAt.Add(-24 * time.Hour),
		},
		{
			name:              "within 24h boundary future (+24h exact) keeps client occurred_at",
			clientOccurredAt:  receivedAt.Add(24 * time.Hour),
			wantEventOccurred: receivedAt.Add(24 * time.Hour),
		},
		{
			name:              "non-UTC within 24h normalizes to UTC",
			clientOccurredAt:  receivedAt.Add(-2 * time.Hour).In(jst),
			wantEventOccurred: receivedAt.Add(-2 * time.Hour),
		},
		{
			name:               "skewed past (>24h) replaces OccurredAt with received_at and keeps client_occurred_at",
			clientOccurredAt:   receivedAt.Add(-24*time.Hour - time.Second),
			wantEventOccurred:  receivedAt,
			wantClientOccurred: "2026-11-17T11:59:59Z",
		},
		{
			name:               "skewed future (>24h) replaces OccurredAt with received_at and keeps client_occurred_at",
			clientOccurredAt:   receivedAt.Add(24*time.Hour + time.Second).In(jst),
			wantEventOccurred:  receivedAt,
			wantClientOccurred: "2026-11-19T12:00:01Z",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			port := &fakeAppendPort{}
			uc := NewRecordReadingEventsUsecase(port, fixedNow)

			_, err := uc.Execute(context.Background(), userID, tenantID, []domain.ReadingEventInput{
				{
					ClientEventID: id,
					OccurredAt:    tt.clientOccurredAt,
					Payload:       domain.SurfaceViewedInput{Surface: domain.ReadingSurfaceGallery},
				},
			})
			require.NoError(t, err)
			require.Len(t, port.events, 1)

			ev := port.events[0]
			assert.Equal(t, tt.wantEventOccurred, ev.OccurredAt)

			var payload map[string]any
			require.NoError(t, json.Unmarshal(ev.Payload, &payload))
			assert.Equal(t, "2026-11-18T12:00:00Z", payload["received_at"])
			if tt.wantClientOccurred == "" {
				_, exists := payload["client_occurred_at"]
				assert.False(t, exists)
			} else {
				assert.Equal(t, tt.wantClientOccurred, payload["client_occurred_at"])
			}
		})
	}
}

func TestRecordReadingEventsUsecase_StopsOnAppendError(t *testing.T) {
	receivedAt := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	fixedNow := func() time.Time { return receivedAt }
	appendErr := errors.New("sovereign unavailable")

	port := &fakeAppendPort{
		errAtCall: 2,
		err:       appendErr,
	}
	uc := NewRecordReadingEventsUsecase(port, fixedNow)

	events := []domain.ReadingEventInput{
		{
			ClientEventID: "01960000-0000-7000-8000-000000000001",
			OccurredAt:    receivedAt,
			Payload:       domain.SurfaceViewedInput{Surface: domain.ReadingSurfaceGallery},
		},
		{
			ClientEventID: "01960000-0000-7000-8000-000000000002",
			OccurredAt:    receivedAt,
			Payload:       domain.SurfaceViewedInput{Surface: domain.ReadingSurfaceSwipe},
		},
		{
			ClientEventID: "01960000-0000-7000-8000-000000000003",
			OccurredAt:    receivedAt,
			Payload:       domain.SurfaceViewedInput{Surface: domain.ReadingSurfaceArticlePage},
		},
	}

	_, err := uc.Execute(context.Background(), uuid.New(), uuid.New(), events)
	require.Error(t, err)
	assert.True(t, errors.Is(err, appendErr))
	assert.Len(t, port.events, 1)
}
