package reading

import (
	"context"
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
	"alt/orchestrator/usecase/record_reading_events_usecase"
)

type fakeAppendPort struct {
	events     []domain.KnowledgeEvent
	seqResults []int64
	err        error
}

func (f *fakeAppendPort) AppendKnowledgeEvent(_ context.Context, event domain.KnowledgeEvent) (int64, error) {
	if f.err != nil {
		return 0, f.err
	}
	idx := len(f.events)
	f.events = append(f.events, event)
	if idx < len(f.seqResults) {
		return f.seqResults[idx], nil
	}
	return int64(idx + 1), nil
}

func authenticatedContext(t *testing.T) context.Context {
	t.Helper()
	uid := uuid.MustParse("3f2b1c8e-5a4d-4e6f-9b0a-1c2d3e4f5a6b")
	tid := uuid.MustParse("7a8b9c0d-1e2f-4a3b-8c5d-6e7f8a9b0c1d")
	return domain.SetUserContext(context.Background(), &domain.UserContext{
		UserID:    uid,
		Email:     "reader@example.com",
		Role:      domain.UserRoleUser,
		TenantID:  tid,
		ExpiresAt: time.Now().Add(time.Hour),
	})
}

func fixedClock() func() time.Time {
	now := time.Date(2026, 11, 18, 0, 5, 0, 0, time.UTC)
	return func() time.Time { return now }
}

func TestNewHandler_NilUsecasePanics(t *testing.T) {
	require.Panics(t, func() {
		NewHandler(nil, nil)
	})
}

func TestRecordReadingEvents_Unauthenticated(t *testing.T) {
	port := &fakeAppendPort{}
	uc := record_reading_events_usecase.NewRecordReadingEventsUsecase(port, fixedClock())
	h := NewHandler(uc, nil)

	_, err := h.RecordReadingEvents(context.Background(), connect.NewRequest(&readingv1.RecordReadingEventsRequest{}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
}

func TestRecordReadingEvents_ValidationErrors(t *testing.T) {
	validTime := timestamppb.New(time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC))
	validEventID := "01960000-0000-7000-8000-000000000001"
	validArticleID := "11111111-1111-4111-8111-111111111111"

	tests := []struct {
		name   string
		events []*readingv1.ReadingEvent
	}{
		{
			name:   "nil event element",
			events: []*readingv1.ReadingEvent{nil},
		},
		{
			name: "missing oneof event case",
			events: []*readingv1.ReadingEvent{
				{
					ClientEventId: validEventID,
					OccurredAt:    validTime,
				},
			},
		},
		{
			name: "unspecified reading surface",
			events: []*readingv1.ReadingEvent{
				{
					ClientEventId: validEventID,
					OccurredAt:    validTime,
					Event: &readingv1.ReadingEvent_SurfaceViewed{
						SurfaceViewed: &readingv1.SurfaceViewed{
							Surface: readingv1.ReadingSurface_READING_SURFACE_UNSPECIFIED,
						},
					},
				},
			},
		},
		{
			name: "unknown numeric reading surface",
			events: []*readingv1.ReadingEvent{
				{
					ClientEventId: validEventID,
					OccurredAt:    validTime,
					Event: &readingv1.ReadingEvent_SurfaceViewed{
						SurfaceViewed: &readingv1.SurfaceViewed{
							Surface: readingv1.ReadingSurface(99),
						},
					},
				},
			},
		},
		{
			name: "missing occurred_at",
			events: []*readingv1.ReadingEvent{
				{
					ClientEventId: validEventID,
					Event: &readingv1.ReadingEvent_SurfaceViewed{
						SurfaceViewed: &readingv1.SurfaceViewed{
							Surface: readingv1.ReadingSurface_READING_SURFACE_GALLERY,
						},
					},
				},
			},
		},
		{
			name: "invalid article_id on reached_end",
			events: []*readingv1.ReadingEvent{
				{
					ClientEventId: validEventID,
					OccurredAt:    validTime,
					Event: &readingv1.ReadingEvent_ReachedEnd{
						ReachedEnd: &readingv1.ReachedEnd{
							ArticleId: "not-a-uuid",
							Surface:   readingv1.ReadingSurface_READING_SURFACE_SWIPE,
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			port := &fakeAppendPort{}
			uc := record_reading_events_usecase.NewRecordReadingEventsUsecase(port, fixedClock())
			h := NewHandler(uc, nil)

			_, err := h.RecordReadingEvents(authenticatedContext(t), connect.NewRequest(&readingv1.RecordReadingEventsRequest{
				Events: tt.events,
			}))
			require.Error(t, err)
			assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
			assert.Empty(t, port.events)
		})
	}

	t.Run("oversized batch returns InvalidArgument", func(t *testing.T) {
		port := &fakeAppendPort{}
		uc := record_reading_events_usecase.NewRecordReadingEventsUsecase(port, fixedClock())
		h := NewHandler(uc, nil)

		events := make([]*readingv1.ReadingEvent, 51)
		for i := range events {
			events[i] = &readingv1.ReadingEvent{
				ClientEventId: validEventID,
				OccurredAt:    validTime,
				Event: &readingv1.ReadingEvent_ReachedEnd{
					ReachedEnd: &readingv1.ReachedEnd{
						ArticleId: validArticleID,
						Surface:   readingv1.ReadingSurface_READING_SURFACE_GALLERY,
					},
				},
			}
		}

		_, err := h.RecordReadingEvents(authenticatedContext(t), connect.NewRequest(&readingv1.RecordReadingEventsRequest{
			Events: events,
		}))
		require.Error(t, err)
		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
		assert.Empty(t, port.events)
	})
}

func TestRecordReadingEvents_MapsSurfacesAndReturnsCounts(t *testing.T) {
	port := &fakeAppendPort{
		seqResults: []int64{10, 0, 11, 12},
	}
	uc := record_reading_events_usecase.NewRecordReadingEventsUsecase(port, fixedClock())
	h := NewHandler(uc, nil)

	ts := timestamppb.New(time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC))
	articleID := "11111111-1111-4111-8111-111111111111"

	resp, err := h.RecordReadingEvents(authenticatedContext(t), connect.NewRequest(&readingv1.RecordReadingEventsRequest{
		Events: []*readingv1.ReadingEvent{
			{
				ClientEventId: "01960000-0000-7000-8000-000000000001",
				OccurredAt:    ts,
				Event: &readingv1.ReadingEvent_SurfaceViewed{
					SurfaceViewed: &readingv1.SurfaceViewed{
						Surface: readingv1.ReadingSurface_READING_SURFACE_GALLERY,
					},
				},
			},
			{
				ClientEventId: "01960000-0000-7000-8000-000000000002",
				OccurredAt:    ts,
				Event: &readingv1.ReadingEvent_ReachedEnd{
					ReachedEnd: &readingv1.ReachedEnd{
						ArticleId: articleID,
						Surface:   readingv1.ReadingSurface_READING_SURFACE_SWIPE,
					},
				},
			},
			{
				ClientEventId: "01960000-0000-7000-8000-000000000003",
				OccurredAt:    ts,
				Event: &readingv1.ReadingEvent_SessionEnded{
					SessionEnded: &readingv1.SessionEnded{
						ArticleId:       articleID,
						Surface:         readingv1.ReadingSurface_READING_SURFACE_ARTICLE_PAGE,
						ActiveDwellMs:   45000,
						MaxScrollRatio:  0.85,
						BodyLengthChars: 2400,
						ReachedEnd:      true,
					},
				},
			},
			{
				ClientEventId: "01960000-0000-7000-8000-000000000004",
				OccurredAt:    ts,
				Event: &readingv1.ReadingEvent_SurfaceViewed{
					SurfaceViewed: &readingv1.SurfaceViewed{
						Surface: readingv1.ReadingSurface_READING_SURFACE_OTHER,
					},
				},
			},
		},
	}))

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, int32(3), resp.Msg.GetAccepted())
	assert.Equal(t, int32(1), resp.Msg.GetDuplicates())
	require.Len(t, port.events, 4)
	assert.Equal(t, "surface:gallery", port.events[0].AggregateID)
	assert.Equal(t, "reading.reached_end.v1", port.events[1].EventType)
	assert.Equal(t, "reading.session_ended.v1", port.events[2].EventType)
	assert.Equal(t, "surface:other", port.events[3].AggregateID)
	assert.Equal(t, "reading.surface_viewed.v1", port.events[3].EventType)
}

func TestRecordReadingEvents_UpstreamErrorMapsToInternal(t *testing.T) {
	port := &fakeAppendPort{err: errors.New("sovereign down")}
	uc := record_reading_events_usecase.NewRecordReadingEventsUsecase(port, fixedClock())
	h := NewHandler(uc, nil)

	_, err := h.RecordReadingEvents(authenticatedContext(t), connect.NewRequest(&readingv1.RecordReadingEventsRequest{
		Events: []*readingv1.ReadingEvent{
			{
				ClientEventId: "01960000-0000-7000-8000-000000000001",
				OccurredAt:    timestamppb.New(time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC)),
				Event: &readingv1.ReadingEvent_SurfaceViewed{
					SurfaceViewed: &readingv1.SurfaceViewed{
						Surface: readingv1.ReadingSurface_READING_SURFACE_GALLERY,
					},
				},
			},
		},
	}))

	require.Error(t, err)
	assert.Equal(t, connect.CodeInternal, connect.CodeOf(err))
}
