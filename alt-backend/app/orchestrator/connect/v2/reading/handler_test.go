package reading

import (
	"context"
	"errors"
	"net/url"
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

type fakeLookupPort struct {
	sources map[string]domain.ArticleSource
}

func (f *fakeLookupPort) LookupArticleSource(_ context.Context, articleID string, _ uuid.UUID) (domain.ArticleSource, error) {
	if f.sources == nil {
		return domain.ArticleSource{}, nil
	}
	return f.sources[articleID], nil
}

type fakeStatusPort struct {
	marked []url.URL
}

func (f *fakeStatusPort) MarkArticleAsRead(_ context.Context, articleURL url.URL, _ uuid.UUID) error {
	f.marked = append(f.marked, articleURL)
	return nil
}

type fakeQueuePort struct {
	entries []domain.ReadingQueueEntry
}

func (f *fakeQueuePort) GetReadingQueue(_ context.Context, _ uuid.UUID, _ time.Time) ([]domain.ReadingQueueEntry, error) {
	return f.entries, nil
}

type fakeCountPort struct {
	count int
}

func (f *fakeCountPort) CountReadingSourcesSince(_ context.Context, _ uuid.UUID, _ time.Time) (int, error) {
	return f.count, nil
}

type fakeArticleBatchPort struct {
	articles []*domain.Article
}

func (f *fakeArticleBatchPort) FetchArticlesByIDs(_ context.Context, _ []uuid.UUID) ([]*domain.Article, error) {
	return f.articles, nil
}

type fakeOgImagePort struct{}

func (f *fakeOgImagePort) GetOgImageURLsByArticleIDs(_ context.Context, _ []string) (map[string]string, error) {
	return map[string]string{}, nil
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

func newTestHandler(port *fakeAppendPort, qp *fakeQueuePort, cp *fakeCountPort, ab *fakeArticleBatchPort) *Handler {
	recUC := record_reading_events_usecase.NewRecordReadingEventsUsecase(port, &fakeLookupPort{
		sources: map[string]domain.ArticleSource{
			"11111111-1111-4111-8111-111111111111": {URL: "https://example.com/1"},
		},
	}, &fakeStatusPort{}, fixedClock())
	qUC := reading_queue_usecase.NewQueueArticleUsecase(qp, port, ab, &fakeOgImagePort{}, 7, 48, fixedClock())
	uqUC := reading_queue_usecase.NewUnqueueArticleUsecase(port, fixedClock())
	gUC := reading_queue_usecase.NewGetReadTodayQueueUsecase(qp, cp, ab, &fakeOgImagePort{}, 7, fixedClock())
	return NewHandler(recUC, qUC, uqUC, gUC, nil, nil)
}

func TestNewHandler_NilUsecasePanics(t *testing.T) {
	port := &fakeAppendPort{}
	recUC := record_reading_events_usecase.NewRecordReadingEventsUsecase(port, &fakeLookupPort{}, &fakeStatusPort{}, fixedClock())
	qUC := reading_queue_usecase.NewQueueArticleUsecase(&fakeQueuePort{}, port, &fakeArticleBatchPort{}, &fakeOgImagePort{}, 7, 48, fixedClock())
	uqUC := reading_queue_usecase.NewUnqueueArticleUsecase(port, fixedClock())
	gUC := reading_queue_usecase.NewGetReadTodayQueueUsecase(&fakeQueuePort{}, &fakeCountPort{}, &fakeArticleBatchPort{}, &fakeOgImagePort{}, 7, fixedClock())

	require.Panics(t, func() { NewHandler(nil, qUC, uqUC, gUC, nil, nil) })
	require.Panics(t, func() { NewHandler(recUC, nil, uqUC, gUC, nil, nil) })
	require.Panics(t, func() { NewHandler(recUC, qUC, nil, gUC, nil, nil) })
	require.Panics(t, func() { NewHandler(recUC, qUC, uqUC, nil, nil, nil) })
}

func TestRecordReadingEvents_Unauthenticated(t *testing.T) {
	h := newTestHandler(&fakeAppendPort{}, &fakeQueuePort{}, &fakeCountPort{}, &fakeArticleBatchPort{})

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
		{
			name: "invalid article_id on opened",
			events: []*readingv1.ReadingEvent{
				{
					ClientEventId: validEventID,
					OccurredAt:    validTime,
					Event: &readingv1.ReadingEvent_Opened{
						Opened: &readingv1.Opened{
							ArticleId: "not-a-uuid",
							Surface:   readingv1.ReadingSurface_READING_SURFACE_GALLERY,
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			port := &fakeAppendPort{}
			h := newTestHandler(port, &fakeQueuePort{}, &fakeCountPort{}, &fakeArticleBatchPort{})

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
		h := newTestHandler(port, &fakeQueuePort{}, &fakeCountPort{}, &fakeArticleBatchPort{})

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
		seqResults: []int64{10, 0, 11, 12, 13},
	}
	h := newTestHandler(port, &fakeQueuePort{}, &fakeCountPort{}, &fakeArticleBatchPort{})

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
			{
				ClientEventId: "01960000-0000-7000-8000-000000000005",
				OccurredAt:    ts,
				Event: &readingv1.ReadingEvent_Opened{
					Opened: &readingv1.Opened{
						ArticleId: articleID,
						Surface:   readingv1.ReadingSurface_READING_SURFACE_GALLERY,
					},
				},
			},
		},
	}))

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, int32(4), resp.Msg.GetAccepted())
	assert.Equal(t, int32(1), resp.Msg.GetDuplicates())
	assert.Equal(t, []string{articleID}, resp.Msg.GetReadArticleIds())
	require.Len(t, port.events, 5)
	assert.Equal(t, "surface:gallery", port.events[0].AggregateID)
	assert.Equal(t, "reading.reached_end.v1", port.events[1].EventType)
	assert.Equal(t, "reading.session_ended.v1", port.events[2].EventType)
	assert.Equal(t, "surface:other", port.events[3].AggregateID)
	assert.Equal(t, "reading.surface_viewed.v1", port.events[3].EventType)
	assert.Equal(t, "reading.opened.v1", port.events[4].EventType)
}

func TestRecordReadingEvents_UpstreamErrorMapsToInternal(t *testing.T) {
	port := &fakeAppendPort{err: errors.New("sovereign down")}
	h := newTestHandler(port, &fakeQueuePort{}, &fakeCountPort{}, &fakeArticleBatchPort{})

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

func TestReadingQueueHandlers(t *testing.T) {
	articleID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	ts := timestamppb.New(time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC))

	t.Run("QueueArticle queues and returns hydrated item", func(t *testing.T) {
		port := &fakeAppendPort{seqResults: []int64{1}}
		ab := &fakeArticleBatchPort{articles: []*domain.Article{{
			ID:    articleID,
			Title: "Queued Article",
			URL:   "https://example.com/articles/1",
		}}}
		h := newTestHandler(port, &fakeQueuePort{}, &fakeCountPort{}, ab)

		resp, err := h.QueueArticle(authenticatedContext(t), connect.NewRequest(&readingv1.QueueArticleRequest{
			ArticleId:     articleID.String(),
			ClientEventId: "01960000-0000-7000-8000-000000000021",
			OccurredAt:    ts,
			Surface:       readingv1.ReadingSurface_READING_SURFACE_SWIPE,
		}))
		require.NoError(t, err)
		assert.Equal(t, readingv1.QueueArticleOutcome_QUEUE_ARTICLE_OUTCOME_QUEUED, resp.Msg.GetOutcome())
		assert.Equal(t, int32(7), resp.Msg.GetCap())
		require.Len(t, resp.Msg.GetItems(), 1)
		assert.Equal(t, "Queued Article", resp.Msg.GetItems()[0].GetTitle())
		assert.Equal(t, "example.com", resp.Msg.GetItems()[0].GetSourceName())
	})

	t.Run("UnqueueArticle appends reading.unqueued.v1", func(t *testing.T) {
		port := &fakeAppendPort{seqResults: []int64{1}}
		h := newTestHandler(port, &fakeQueuePort{}, &fakeCountPort{}, &fakeArticleBatchPort{})

		_, err := h.UnqueueArticle(authenticatedContext(t), connect.NewRequest(&readingv1.UnqueueArticleRequest{
			ArticleId:     articleID.String(),
			ClientEventId: "01960000-0000-7000-8000-000000000022",
			OccurredAt:    ts,
		}))
		require.NoError(t, err)
		require.Len(t, port.events, 1)
		assert.Equal(t, domain.EventReadingUnqueued, port.events[0].EventType)
	})

	t.Run("GetReadTodayQueue returns items, cap, and read_count_since", func(t *testing.T) {
		port := &fakeAppendPort{}
		qp := &fakeQueuePort{entries: []domain.ReadingQueueEntry{{
			ArticleID: articleID,
			QueuedAt:  ts.AsTime(),
			ExpiresAt: ts.AsTime().Add(48 * time.Hour),
		}}}
		cp := &fakeCountPort{count: 2}
		ab := &fakeArticleBatchPort{articles: []*domain.Article{{
			ID:    articleID,
			Title: "Queued Article",
			URL:   "https://example.com/articles/1",
		}}}
		h := newTestHandler(port, qp, cp, ab)

		resp, err := h.GetReadTodayQueue(authenticatedContext(t), connect.NewRequest(&readingv1.GetReadTodayQueueRequest{
			ReadSince: ts,
		}))
		require.NoError(t, err)
		assert.Equal(t, int32(7), resp.Msg.GetCap())
		assert.Equal(t, int32(2), resp.Msg.GetReadCountSince())
		require.Len(t, resp.Msg.GetItems(), 1)
		assert.Equal(t, "Queued Article", resp.Msg.GetItems()[0].GetTitle())
	})
}
