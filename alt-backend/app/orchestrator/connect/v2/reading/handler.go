package reading

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"connectrpc.com/connect"

	"alt/connect/errorhandler"
	"alt/connect/v2/middleware"
	"alt/domain"
	readingv1 "alt/gen/proto/alt/reading/v1"
	"alt/gen/proto/alt/reading/v1/readingv1connect"
	"alt/orchestrator/usecase/record_reading_events_usecase"
)

type Handler struct {
	recordUsecase *record_reading_events_usecase.RecordReadingEventsUsecase
	logger        *slog.Logger
}

var _ readingv1connect.ReadingServiceHandler = (*Handler)(nil)

func NewHandler(recordUsecase *record_reading_events_usecase.RecordReadingEventsUsecase, logger *slog.Logger) *Handler {
	if recordUsecase == nil {
		panic("reading: RecordReadingEventsUsecase is required")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{
		recordUsecase: recordUsecase,
		logger:        logger,
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
		Accepted:   res.Accepted,
		Duplicates: res.Duplicates,
	}), nil
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
