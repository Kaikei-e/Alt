package reading_queue_usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"alt/domain"
	"alt/orchestrator/port/article_url_lookup_port"
	"alt/orchestrator/port/reading_queue_port"
	"alt/shared/port/knowledge_event_port"
)

type QueueArticleResult struct {
	Outcome domain.QueueArticleOutcome
	Items   []domain.HydratedReadingQueueItem
	Cap     int
}

type QueueArticleUsecase struct {
	queuePort         reading_queue_port.GetReadingQueuePort
	appendPort        knowledge_event_port.AppendKnowledgeEventPort
	articleLookupPort article_url_lookup_port.ArticleURLLookupPort
	articleBatchPort  reading_queue_port.FetchArticlesByIDsPort
	ogImagePort       reading_queue_port.GetOgImageURLsPort
	cap               int
	ttlHours          int
	now               func() time.Time
}

func NewQueueArticleUsecase(
	queuePort reading_queue_port.GetReadingQueuePort,
	appendPort knowledge_event_port.AppendKnowledgeEventPort,
	articleLookupPort article_url_lookup_port.ArticleURLLookupPort,
	articleBatchPort reading_queue_port.FetchArticlesByIDsPort,
	ogImagePort reading_queue_port.GetOgImageURLsPort,
	cap int,
	ttlHours int,
	now func() time.Time,
) *QueueArticleUsecase {
	if queuePort == nil {
		panic("reading_queue_usecase: queuePort is required")
	}
	if appendPort == nil {
		panic("reading_queue_usecase: appendPort is required")
	}
	if articleLookupPort == nil {
		panic("reading_queue_usecase: articleLookupPort is required")
	}
	if articleBatchPort == nil {
		panic("reading_queue_usecase: articleBatchPort is required")
	}
	if ogImagePort == nil {
		panic("reading_queue_usecase: ogImagePort is required")
	}
	if cap <= 0 {
		panic("reading_queue_usecase: cap must be positive")
	}
	if ttlHours <= 0 {
		panic("reading_queue_usecase: ttlHours must be positive")
	}
	if now == nil {
		panic("reading_queue_usecase: now clock is required")
	}
	return &QueueArticleUsecase{
		queuePort:         queuePort,
		appendPort:        appendPort,
		articleLookupPort: articleLookupPort,
		articleBatchPort:  articleBatchPort,
		ogImagePort:       ogImagePort,
		cap:               cap,
		ttlHours:          ttlHours,
		now:               now,
	}
}

type readingQueuedPayload struct {
	ClientEventID      string `json:"client_event_id"`
	ArticleID          string `json:"article_id"`
	Surface            string `json:"surface"`
	QueuedAt           string `json:"queued_at"`
	ExpiresAt          string `json:"expires_at"`
	Cap                int    `json:"cap"`
	TTLHours           int    `json:"ttl_hours"`
	QueuePolicyVersion int    `json:"queue_policy_version"`
	ReceivedAt         string `json:"received_at"`
	ClientOccurredAt   string `json:"client_occurred_at,omitempty"`
}

func (u *QueueArticleUsecase) Execute(
	ctx context.Context,
	userID uuid.UUID,
	tenantID uuid.UUID,
	rawArticleID string,
	rawClientEventID string,
	occurredAt time.Time,
	surface domain.ReadingSurface,
) (QueueArticleResult, error) {
	articleID, err := uuid.Parse(rawArticleID)
	if err != nil {
		return QueueArticleResult{}, fmt.Errorf("%w: invalid article_id", ErrInvalidInput)
	}
	clientEventID, err := uuid.Parse(rawClientEventID)
	if err != nil {
		return QueueArticleResult{}, fmt.Errorf("%w: invalid client_event_id", ErrInvalidInput)
	}
	if occurredAt.IsZero() {
		return QueueArticleResult{}, fmt.Errorf("%w: occurred_at is required", ErrInvalidInput)
	}
	if !surface.IsValid() {
		return QueueArticleResult{}, fmt.Errorf("%w: invalid surface", ErrInvalidInput)
	}

	source, err := u.articleLookupPort.LookupArticleSource(ctx, articleID.String(), userID)
	if err != nil {
		if errors.Is(err, domain.ErrArticleNotFound) {
			return QueueArticleResult{}, fmt.Errorf("%w: article not found", ErrInvalidInput)
		}
		return QueueArticleResult{}, fmt.Errorf("queue article: %w", err)
	}
	if source.URL == "" {
		return QueueArticleResult{}, fmt.Errorf("%w: article not found", ErrInvalidInput)
	}

	receivedAt := u.now().UTC()
	entries, err := u.queuePort.GetReadingQueue(ctx, userID, receivedAt)
	if err != nil {
		return QueueArticleResult{}, fmt.Errorf("queue article: %w", err)
	}

	alreadyQueued := false
	for _, existing := range entries {
		if existing.ArticleID == articleID {
			alreadyQueued = true
			break
		}
	}

	if !alreadyQueued && len(entries) >= u.cap {
		items, err := hydrateQueueEntries(ctx, u.articleBatchPort, u.ogImagePort, entries)
		if err != nil {
			return QueueArticleResult{}, fmt.Errorf("queue article: %w", err)
		}
		return QueueArticleResult{
			Outcome: domain.QueueArticleOutcomeQueueFull,
			Items:   items,
			Cap:     u.cap,
		}, nil
	}

	clampedOccurredAt, clientOccurredAt := domain.ClampReadingOccurredAt(occurredAt, receivedAt)
	expiresAt := clampedOccurredAt.Add(time.Duration(u.ttlHours) * time.Hour)

	payloadBytes, err := json.Marshal(readingQueuedPayload{
		ClientEventID:      clientEventID.String(),
		ArticleID:          articleID.String(),
		Surface:            surface.String(),
		QueuedAt:           clampedOccurredAt.Format(time.RFC3339),
		ExpiresAt:          expiresAt.Format(time.RFC3339),
		Cap:                u.cap,
		TTLHours:           u.ttlHours,
		QueuePolicyVersion: domain.QueuePolicyVersionV1,
		ReceivedAt:         receivedAt.Format(time.RFC3339),
		ClientOccurredAt:   clientOccurredAt,
	})
	if err != nil {
		return QueueArticleResult{}, fmt.Errorf("marshal reading.queued.v1 payload: %w", err)
	}

	uid := userID
	ke := domain.KnowledgeEvent{
		EventID:       uuid.New(),
		OccurredAt:    clampedOccurredAt,
		TenantID:      tenantID,
		UserID:        &uid,
		ActorType:     domain.ActorUser,
		ActorID:       userID.String(),
		EventType:     domain.EventReadingQueued,
		AggregateType: domain.AggregateReadingSession,
		AggregateID:   "article:" + articleID.String(),
		DedupeKey:     domain.EventReadingQueued + ":" + clientEventID.String(),
		Payload:       payloadBytes,
	}

	seq, err := u.appendPort.AppendKnowledgeEvent(ctx, ke)
	if err != nil {
		return QueueArticleResult{}, fmt.Errorf("queue article: %w", err)
	}

	// Still append when the read model already shows the article queued so a
	// remove-then-readd within the projection lag window is not lost; return the
	// current entries without appending a duplicate row.
	outcome := domain.QueueArticleOutcomeAlreadyQueued
	entriesToHydrate := entries
	if !alreadyQueued {
		if seq != 0 {
			outcome = domain.QueueArticleOutcomeQueued
		}
		updatedEntries := make([]domain.ReadingQueueEntry, len(entries), len(entries)+1)
		copy(updatedEntries, entries)
		updatedEntries = append(updatedEntries, domain.ReadingQueueEntry{
			ArticleID:          articleID,
			QueuedAt:           clampedOccurredAt,
			ExpiresAt:          expiresAt,
			QueuePolicyVersion: domain.QueuePolicyVersionV1,
		})
		entriesToHydrate = updatedEntries
	}

	items, err := hydrateQueueEntries(ctx, u.articleBatchPort, u.ogImagePort, entriesToHydrate)
	if err != nil {
		return QueueArticleResult{}, fmt.Errorf("queue article: %w", err)
	}
	return QueueArticleResult{
		Outcome: outcome,
		Items:   items,
		Cap:     u.cap,
	}, nil
}
