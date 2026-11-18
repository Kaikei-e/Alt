package reading

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	"alt/connect/errorhandler"
	"alt/connect/v2/middleware"
	"alt/domain"
	readingv1 "alt/gen/proto/alt/reading/v1"
	"alt/gen/proto/alt/reading/v1/readingv1connect"
	"alt/orchestrator/usecase/image_proxy_usecase"
	"alt/orchestrator/usecase/reading_queue_usecase"
	"alt/orchestrator/usecase/record_reading_events_usecase"
	"alt/utils/safeconv"
)

type Handler struct {
	recordUsecase   *record_reading_events_usecase.RecordReadingEventsUsecase
	queueUsecase    *reading_queue_usecase.QueueArticleUsecase
	unqueueUsecase  *reading_queue_usecase.UnqueueArticleUsecase
	getQueueUsecase *reading_queue_usecase.GetReadTodayQueueUsecase
	imageProxy      *image_proxy_usecase.ImageProxyUsecase
	logger          *slog.Logger
}

var _ readingv1connect.ReadingServiceHandler = (*Handler)(nil)

func NewHandler(
	recordUsecase *record_reading_events_usecase.RecordReadingEventsUsecase,
	queueUsecase *reading_queue_usecase.QueueArticleUsecase,
	unqueueUsecase *reading_queue_usecase.UnqueueArticleUsecase,
	getQueueUsecase *reading_queue_usecase.GetReadTodayQueueUsecase,
	imageProxy *image_proxy_usecase.ImageProxyUsecase,
	logger *slog.Logger,
) *Handler {
	if recordUsecase == nil {
		panic("reading: RecordReadingEventsUsecase is required")
	}
	if queueUsecase == nil {
		panic("reading: QueueArticleUsecase is required")
	}
	if unqueueUsecase == nil {
		panic("reading: UnqueueArticleUsecase is required")
	}
	if getQueueUsecase == nil {
		panic("reading: GetReadTodayQueueUsecase is required")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{
		recordUsecase:   recordUsecase,
		queueUsecase:    queueUsecase,
		unqueueUsecase:  unqueueUsecase,
		getQueueUsecase: getQueueUsecase,
		imageProxy:      imageProxy,
		logger:          logger,
	}
}

func (h *Handler) RecordReadingEvents(
	ctx context.Context,
	req *connect.Request[readingv1.RecordReadingEventsRequest],
) (*connect.Response[readingv1.RecordReadingEventsResponse], error) {
	user, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	rawEvents := req.Msg.GetEvents()
	if len(rawEvents) > domain.MaxReadingEventsBatchSize {
		return nil, connect.NewError(
			connect.CodeInvalidArgument,
			fmt.Errorf("%w: batch exceeds maximum of %d events", domain.ErrInvalidReadingEvent, domain.MaxReadingEventsBatchSize),
		)
	}

	inputs := make([]domain.ReadingEventInput, 0, len(rawEvents))
	for i, ev := range rawEvents {
		input, convErr := eventFromProto(ev)
		if convErr != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("event[%d]: %w", i, convErr))
		}
		inputs = append(inputs, input)
	}

	res, err := h.recordUsecase.Execute(ctx, user.UserID, user.TenantID, inputs)
	if err != nil {
		if errors.Is(err, record_reading_events_usecase.ErrInvalidInput) {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
		return nil, errorhandler.HandleUpstreamError(ctx, h.logger, err, "RecordReadingEvents")
	}

	return connect.NewResponse(&readingv1.RecordReadingEventsResponse{
		Accepted:       res.Accepted,
		Duplicates:     res.Duplicates,
		ReadArticleIds: res.ReadArticleIDs,
	}), nil
}

func (h *Handler) QueueArticle(
	ctx context.Context,
	req *connect.Request[readingv1.QueueArticleRequest],
) (*connect.Response[readingv1.QueueArticleResponse], error) {
	user, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	var occurredAt time.Time
	if ts := req.Msg.GetOccurredAt(); ts != nil {
		if !ts.IsValid() {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("%w: invalid occurred_at", domain.ErrInvalidReadingEvent))
		}
		occurredAt = ts.AsTime()
	}

	res, err := h.queueUsecase.Execute(
		ctx,
		user.UserID,
		user.TenantID,
		req.Msg.GetArticleId(),
		req.Msg.GetClientEventId(),
		occurredAt,
		surfaceFromProto(req.Msg.GetSurface()),
	)
	if err != nil {
		if errors.Is(err, reading_queue_usecase.ErrInvalidInput) {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
		return nil, errorhandler.HandleUpstreamError(ctx, h.logger, err, "QueueArticle")
	}

	return connect.NewResponse(&readingv1.QueueArticleResponse{
		Outcome: outcomeToProto(res.Outcome),
		Items:   h.mapQueueItems(res.Items),
		Cap:     safeconv.Int32(res.Cap),
	}), nil
}

func (h *Handler) UnqueueArticle(
	ctx context.Context,
	req *connect.Request[readingv1.UnqueueArticleRequest],
) (*connect.Response[readingv1.UnqueueArticleResponse], error) {
	user, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	var occurredAt time.Time
	if ts := req.Msg.GetOccurredAt(); ts != nil {
		if !ts.IsValid() {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("%w: invalid occurred_at", domain.ErrInvalidReadingEvent))
		}
		occurredAt = ts.AsTime()
	}

	if err := h.unqueueUsecase.Execute(
		ctx,
		user.UserID,
		user.TenantID,
		req.Msg.GetArticleId(),
		req.Msg.GetClientEventId(),
		occurredAt,
	); err != nil {
		if errors.Is(err, reading_queue_usecase.ErrInvalidInput) {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
		return nil, errorhandler.HandleUpstreamError(ctx, h.logger, err, "UnqueueArticle")
	}

	return connect.NewResponse(&readingv1.UnqueueArticleResponse{}), nil
}

func (h *Handler) GetReadTodayQueue(
	ctx context.Context,
	req *connect.Request[readingv1.GetReadTodayQueueRequest],
) (*connect.Response[readingv1.GetReadTodayQueueResponse], error) {
	user, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	var readSince time.Time
	if ts := req.Msg.GetReadSince(); ts != nil {
		if !ts.IsValid() {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("%w: invalid read_since", domain.ErrInvalidReadingEvent))
		}
		readSince = ts.AsTime()
	}

	res, err := h.getQueueUsecase.Execute(ctx, user.UserID, readSince)
	if err != nil {
		if errors.Is(err, reading_queue_usecase.ErrInvalidInput) {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
		return nil, errorhandler.HandleUpstreamError(ctx, h.logger, err, "GetReadTodayQueue")
	}

	return connect.NewResponse(&readingv1.GetReadTodayQueueResponse{
		Items:          h.mapQueueItems(res.Items),
		Cap:            safeconv.Int32(res.Cap),
		ReadCountSince: res.ReadCountSince,
	}), nil
}

func (h *Handler) mapQueueItems(items []domain.HydratedReadingQueueItem) []*readingv1.ReadingQueueItem {
	out := make([]*readingv1.ReadingQueueItem, 0, len(items))
	for _, it := range items {
		imageURL := ""
		if it.ImageURL != "" && h.imageProxy != nil {
			imageURL = h.imageProxy.GenerateProxyURL(it.ImageURL)
		}
		out = append(out, &readingv1.ReadingQueueItem{
			ArticleId:  it.ArticleID.String(),
			Title:      it.Title,
			Url:        it.URL,
			SourceName: it.SourceName,
			ImageUrl:   imageURL,
			QueuedAt:   timestamppb.New(it.QueuedAt),
			ExpiresAt:  timestamppb.New(it.ExpiresAt),
		})
	}
	return out
}

func outcomeToProto(outcome domain.QueueArticleOutcome) readingv1.QueueArticleOutcome {
	switch outcome {
	case domain.QueueArticleOutcomeQueued:
		return readingv1.QueueArticleOutcome_QUEUE_ARTICLE_OUTCOME_QUEUED
	case domain.QueueArticleOutcomeAlreadyQueued:
		return readingv1.QueueArticleOutcome_QUEUE_ARTICLE_OUTCOME_ALREADY_QUEUED
	case domain.QueueArticleOutcomeQueueFull:
		return readingv1.QueueArticleOutcome_QUEUE_ARTICLE_OUTCOME_QUEUE_FULL
	default:
		return readingv1.QueueArticleOutcome_QUEUE_ARTICLE_OUTCOME_UNSPECIFIED
	}
}

func eventFromProto(ev *readingv1.ReadingEvent) (domain.ReadingEventInput, error) {
	if ev == nil {
		return domain.ReadingEventInput{}, fmt.Errorf("%w: event is required", domain.ErrInvalidReadingEvent)
	}

	var occurredAt time.Time
	if ts := ev.GetOccurredAt(); ts != nil {
		if !ts.IsValid() {
			return domain.ReadingEventInput{}, fmt.Errorf("%w: invalid occurred_at", domain.ErrInvalidReadingEvent)
		}
		occurredAt = ts.AsTime()
	}

	var payload domain.ReadingEventPayload
	switch e := ev.GetEvent().(type) {
	case *readingv1.ReadingEvent_SurfaceViewed:
		if e.SurfaceViewed == nil {
			return domain.ReadingEventInput{}, fmt.Errorf("%w: surface_viewed payload is required", domain.ErrInvalidReadingEvent)
		}
		payload = domain.SurfaceViewedInput{
			Surface: surfaceFromProto(e.SurfaceViewed.GetSurface()),
		}
	case *readingv1.ReadingEvent_ReachedEnd:
		if e.ReachedEnd == nil {
			return domain.ReadingEventInput{}, fmt.Errorf("%w: reached_end payload is required", domain.ErrInvalidReadingEvent)
		}
		payload = domain.ReachedEndInput{
			ArticleID: e.ReachedEnd.GetArticleId(),
			Surface:   surfaceFromProto(e.ReachedEnd.GetSurface()),
		}
	case *readingv1.ReadingEvent_Opened:
		if e.Opened == nil {
			return domain.ReadingEventInput{}, fmt.Errorf("%w: opened payload is required", domain.ErrInvalidReadingEvent)
		}
		payload = domain.OpenedInput{
			ArticleID: e.Opened.GetArticleId(),
			Surface:   surfaceFromProto(e.Opened.GetSurface()),
		}
	case *readingv1.ReadingEvent_SessionEnded:
		if e.SessionEnded == nil {
			return domain.ReadingEventInput{}, fmt.Errorf("%w: session_ended payload is required", domain.ErrInvalidReadingEvent)
		}
		payload = domain.SessionEndedInput{
			ArticleID:       e.SessionEnded.GetArticleId(),
			Surface:         surfaceFromProto(e.SessionEnded.GetSurface()),
			ActiveDwellMs:   e.SessionEnded.GetActiveDwellMs(),
			MaxScrollRatio:  e.SessionEnded.GetMaxScrollRatio(),
			BodyLengthChars: e.SessionEnded.GetBodyLengthChars(),
			ReachedEnd:      e.SessionEnded.GetReachedEnd(),
		}
	default:
		return domain.ReadingEventInput{}, fmt.Errorf("%w: event payload is required", domain.ErrInvalidReadingEvent)
	}

	return domain.ReadingEventInput{
		ClientEventID: ev.GetClientEventId(),
		OccurredAt:    occurredAt,
		Payload:       payload,
	}, nil
}

func surfaceFromProto(s readingv1.ReadingSurface) domain.ReadingSurface {
	switch s {
	case readingv1.ReadingSurface_READING_SURFACE_GALLERY:
		return domain.ReadingSurfaceGallery
	case readingv1.ReadingSurface_READING_SURFACE_SWIPE:
		return domain.ReadingSurfaceSwipe
	case readingv1.ReadingSurface_READING_SURFACE_ARTICLE_PAGE:
		return domain.ReadingSurfaceArticlePage
	case readingv1.ReadingSurface_READING_SURFACE_OTHER:
		return domain.ReadingSurfaceOther
	default:
		return ""
	}
}
