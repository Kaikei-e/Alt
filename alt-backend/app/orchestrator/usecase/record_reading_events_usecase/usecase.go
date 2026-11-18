package record_reading_events_usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"time"

	"alt/domain"
	"alt/orchestrator/port/article_status_port"
	"alt/orchestrator/port/article_url_lookup_port"
	"alt/shared/port/knowledge_event_port"

	"github.com/google/uuid"
)

var (
	ErrInvalidInput        = domain.ErrInvalidReadingEvent
	ErrReadSyncUnavailable = errors.New("read sync unavailable")
)

type Result struct {
	Accepted       int32
	Duplicates     int32
	ReadArticleIDs []string
}

type RecordReadingEventsUsecase struct {
	appendPort        knowledge_event_port.AppendKnowledgeEventPort
	articleLookupPort article_url_lookup_port.ArticleURLLookupPort
	articleStatusPort article_status_port.UpdateArticleStatusPort
	now               func() time.Time
}

func NewRecordReadingEventsUsecase(
	appendPort knowledge_event_port.AppendKnowledgeEventPort,
	articleLookupPort article_url_lookup_port.ArticleURLLookupPort,
	articleStatusPort article_status_port.UpdateArticleStatusPort,
	now func() time.Time,
) *RecordReadingEventsUsecase {
	if appendPort == nil {
		panic("record_reading_events_usecase: appendPort is required")
	}
	if articleLookupPort == nil {
		panic("record_reading_events_usecase: articleLookupPort is required")
	}
	if articleStatusPort == nil {
		panic("record_reading_events_usecase: articleStatusPort is required")
	}
	if now == nil {
		panic("record_reading_events_usecase: now clock is required")
	}
	return &RecordReadingEventsUsecase{
		appendPort:        appendPort,
		articleLookupPort: articleLookupPort,
		articleStatusPort: articleStatusPort,
		now:               now,
	}
}

type surfaceViewedPayload struct {
	ClientEventID    string `json:"client_event_id"`
	Surface          string `json:"surface"`
	ReceivedAt       string `json:"received_at"`
	ClientOccurredAt string `json:"client_occurred_at,omitempty"`
}

type articleSurfacePayload struct {
	ClientEventID    string `json:"client_event_id"`
	ArticleID        string `json:"article_id"`
	Surface          string `json:"surface"`
	ReceivedAt       string `json:"received_at"`
	ClientOccurredAt string `json:"client_occurred_at,omitempty"`
}

type sessionEndedPayload struct {
	ClientEventID    string  `json:"client_event_id"`
	ArticleID        string  `json:"article_id"`
	Surface          string  `json:"surface"`
	ActiveDwellMs    int64   `json:"active_dwell_ms"`
	MaxScrollRatio   float64 `json:"max_scroll_ratio"`
	BodyLengthChars  int32   `json:"body_length_chars"`
	ReachedEnd       bool    `json:"reached_end"`
	ReceivedAt       string  `json:"received_at"`
	ClientOccurredAt string  `json:"client_occurred_at,omitempty"`
}

func (u *RecordReadingEventsUsecase) Execute(
	ctx context.Context,
	userID uuid.UUID,
	tenantID uuid.UUID,
	events []domain.ReadingEventInput,
) (Result, error) {
	if len(events) == 0 {
		return Result{}, nil
	}
	if err := domain.ValidateReadingEventBatch(events); err != nil {
		return Result{}, err
	}

	receivedAt := u.now().UTC()
	var res Result
	seenRead := make(map[string]struct{})
	var syncErrs []error

	for _, ev := range events {
		ke, err := buildKnowledgeEvent(userID, tenantID, ev, receivedAt)
		if err != nil {
			return Result{}, fmt.Errorf("record reading events: %w", err)
		}
		seq, err := u.appendPort.AppendKnowledgeEvent(ctx, ke)
		if err != nil {
			return Result{}, fmt.Errorf("record reading events: %w", err)
		}
		if seq == 0 {
			res.Duplicates++
		} else {
			res.Accepted++
		}

		if se, ok := ev.Payload.(domain.SessionEndedInput); ok {
			if domain.EvaluateReadRuleV1(se.ReachedEnd, se.ActiveDwellMs, se.BodyLengthChars) {
				canonicalArticleID := uuid.MustParse(se.ArticleID).String()
				if _, already := seenRead[canonicalArticleID]; !already {
					seenRead[canonicalArticleID] = struct{}{}
					marked, syncErr := u.markArticleRead(ctx, canonicalArticleID, userID)
					if syncErr != nil {
						syncErrs = append(syncErrs, syncErr)
					} else if marked {
						res.ReadArticleIDs = append(res.ReadArticleIDs, canonicalArticleID)
					}
				}
			}
		}
	}
	if len(syncErrs) > 0 {
		return res, fmt.Errorf("%w: %w", ErrReadSyncUnavailable, errors.Join(syncErrs...))
	}
	return res, nil
}

func (u *RecordReadingEventsUsecase) markArticleRead(ctx context.Context, articleID string, userID uuid.UUID) (bool, error) {
	source, err := u.articleLookupPort.LookupArticleSource(ctx, articleID, userID)
	if err != nil {
		if errors.Is(err, domain.ErrArticleNotFound) {
			return false, nil
		}
		slog.ErrorContext(ctx, "record_reading_events: lookup article URL failed for read sync",
			"article_id", articleID, "error", err)
		return false, err
	}
	if source.URL == "" {
		return false, nil
	}
	parsedURL, err := url.Parse(source.URL)
	if err != nil || parsedURL.Scheme == "" {
		return false, nil
	}
	if err := u.articleStatusPort.MarkArticleAsRead(ctx, *parsedURL, userID); err != nil {
		if errors.Is(err, domain.ErrFeedNotFound) {
			return false, nil
		}
		slog.ErrorContext(ctx, "record_reading_events: mark article read failed",
			"article_id", articleID, "error", err)
		return false, err
	}
	return true, nil
}

func buildKnowledgeEvent(
	userID uuid.UUID,
	tenantID uuid.UUID,
	ev domain.ReadingEventInput,
	receivedAt time.Time,
) (domain.KnowledgeEvent, error) {
	clientEventID := uuid.MustParse(ev.ClientEventID).String()
	occurredAt, clientOccurredAt := domain.ClampReadingOccurredAt(ev.OccurredAt, receivedAt)
	receivedAtStr := receivedAt.Format(time.RFC3339)

	eventType, aggregateID, payloadBytes, err := encodePayload(ev.Payload, clientEventID, receivedAtStr, clientOccurredAt)
	if err != nil {
		return domain.KnowledgeEvent{}, err
	}

	uid := userID
	return domain.KnowledgeEvent{
		EventID:       uuid.New(),
		OccurredAt:    occurredAt,
		TenantID:      tenantID,
		UserID:        &uid,
		ActorType:     domain.ActorUser,
		ActorID:       userID.String(),
		EventType:     eventType,
		AggregateType: domain.AggregateReadingSession,
		AggregateID:   aggregateID,
		DedupeKey:     eventType + ":" + clientEventID,
		Payload:       payloadBytes,
	}, nil
}

func encodePayload(
	payload domain.ReadingEventPayload,
	clientEventID string,
	receivedAtStr string,
	clientOccurredAtStr string,
) (string, string, []byte, error) {
	var (
		eventType   string
		aggregateID string
		wirePayload any
	)

	switch p := payload.(type) {
	case domain.SurfaceViewedInput:
		eventType = domain.EventReadingSurfaceViewed
		aggregateID = "surface:" + p.Surface.String()
		wirePayload = surfaceViewedPayload{
			ClientEventID:    clientEventID,
			Surface:          p.Surface.String(),
			ReceivedAt:       receivedAtStr,
			ClientOccurredAt: clientOccurredAtStr,
		}
	case domain.ReachedEndInput:
		articleID := uuid.MustParse(p.ArticleID).String()
		eventType = domain.EventReadingReachedEnd
		aggregateID = "article:" + articleID
		wirePayload = articleSurfacePayload{
			ClientEventID:    clientEventID,
			ArticleID:        articleID,
			Surface:          p.Surface.String(),
			ReceivedAt:       receivedAtStr,
			ClientOccurredAt: clientOccurredAtStr,
		}
	case domain.OpenedInput:
		articleID := uuid.MustParse(p.ArticleID).String()
		eventType = domain.EventReadingOpened
		aggregateID = "article:" + articleID
		wirePayload = articleSurfacePayload{
			ClientEventID:    clientEventID,
			ArticleID:        articleID,
			Surface:          p.Surface.String(),
			ReceivedAt:       receivedAtStr,
			ClientOccurredAt: clientOccurredAtStr,
		}
	case domain.SessionEndedInput:
		articleID := uuid.MustParse(p.ArticleID).String()
		eventType = domain.EventReadingSessionEnded
		aggregateID = "article:" + articleID
		wirePayload = sessionEndedPayload{
			ClientEventID:    clientEventID,
			ArticleID:        articleID,
			Surface:          p.Surface.String(),
			ActiveDwellMs:    p.ActiveDwellMs,
			MaxScrollRatio:   p.MaxScrollRatio,
			BodyLengthChars:  p.BodyLengthChars,
			ReachedEnd:       p.ReachedEnd,
			ReceivedAt:       receivedAtStr,
			ClientOccurredAt: clientOccurredAtStr,
		}
	default:
		return "", "", nil, fmt.Errorf("%w: unknown event payload", ErrInvalidInput)
	}

	raw, err := json.Marshal(wirePayload)
	if err != nil {
		return "", "", nil, fmt.Errorf("marshal payload: %w", err)
	}
	return eventType, aggregateID, raw, nil
}
