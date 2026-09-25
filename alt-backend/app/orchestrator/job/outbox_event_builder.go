package job

import (
	"alt/domain"
	"alt/orchestrator/port/rag_integration_port"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	errMissingUserID     = errors.New("missing owner user_id")
	errSkipInvalidUserID = errors.New("invalid user_id for knowledge event, skipping")
)

type errInvalidUserID struct {
	userID string
}

func (e *errInvalidUserID) Error() string {
	return errSkipInvalidUserID.Error()
}

func (e *errInvalidUserID) Is(target error) bool {
	return target == errSkipInvalidUserID
}

type errInvalidUpdatedAt struct {
	articleID string
	updatedAt string
	err       error
}

func (e *errInvalidUpdatedAt) Error() string {
	return fmt.Sprintf("parse outbox updated_at for occurred_at: %v", e.err)
}

func (e *errInvalidUpdatedAt) Unwrap() error {
	return e.err
}

type errMarshalKnowledgePayload struct {
	articleID string
	err       error
}

func (e *errMarshalKnowledgePayload) Error() string {
	return fmt.Sprintf("marshal knowledge ArticleCreated payload: %v", e.err)
}

func (e *errMarshalKnowledgePayload) Unwrap() error {
	return e.err
}

func parseArticleUpsertPayload(payload []byte) (rag_integration_port.UpsertArticleInput, error) {
	var input rag_integration_port.UpsertArticleInput
	if err := json.Unmarshal(payload, &input); err != nil {
		return rag_integration_port.UpsertArticleInput{}, err
	}
	if strings.TrimSpace(input.UserID) == "" {
		return input, errMissingUserID
	}
	return input, nil
}

type articleCreatedRawPayload struct {
	ArticleID string `json:"article_id"`
	URL       string `json:"url"`
	Title     string `json:"title"`
	UserID    string `json:"user_id"`
	// UpdatedAt is stamped once at outbox-enqueue time (save_article_driver.go),
	// i.e. when the article-upsert fact actually occurred. Reused below as
	// PublishedAt instead of re-stamping wall-clock time here: this handler
	// can run at an arbitrary, possibly much later time (worker poll delay,
	// crash-and-reprocess), so reading time.Now() here would make the same
	// event replay to a different PublishedAt each time it's processed.
	UpdatedAt string `json:"updated_at"`
}

func buildArticleCreatedKnowledgeEvent(payload []byte, now time.Time) (*domain.KnowledgeEvent, error) {
	var p articleCreatedRawPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil, fmt.Errorf("unmarshal outbox payload for knowledge event: %w", err)
	}

	userID, err := uuid.Parse(p.UserID)
	if err != nil {
		// Invalid user_id is a permanent payload defect: skipping (nil error)
		// lets the caller ACK rather than retry forever on the same bad row.
		return nil, &errInvalidUserID{userID: p.UserID}
	}

	publishedAt := p.UpdatedAt
	if publishedAt == "" {
		// Only reachable for outbox rows enqueued before this field existed.
		publishedAt = now.Format(time.RFC3339)
	}

	// occurred_at is the article-upsert fact's own timestamp, minted once at
	// outbox-enqueue time (same source as published_at above). Re-stamping
	// time.Now() here made the same event replay to a different occurred_at on
	// every reprocess (worker poll delay, crash-and-reprocess), breaking the
	// reproject-safe / no-business-fact-time.Now() invariant. Deriving it from
	// updated_at keeps the append idempotent under the article-scoped dedupe_key.
	occurredAt, occurredErr := time.Parse(time.RFC3339, publishedAt)
	if occurredErr != nil {
		// publishedAt is always RFC3339 (p.UpdatedAt from save_article_driver, or
		// the wall-clock fallback above), so this is defensive. Surface it as a
		// retryable failure rather than fabricating a fresh occurred_at.
		return nil, &errInvalidUpdatedAt{
			articleID: p.ArticleID,
			updatedAt: publishedAt,
			err:       occurredErr,
		}
	}

	// Marshal through the canonical domain.ArticleCreatedPayload struct so
	// the wire key for the article URL is locked to "url" — using a raw
	// map[string]any literal here historically wrote the legacy "link" key
	// which silently broke the projector (PM-2026-041). The shared struct
	// is the single source of truth for this wire schema.
	eventPayload, err := json.Marshal(domain.ArticleCreatedPayload{
		ArticleID:   p.ArticleID,
		Title:       p.Title,
		PublishedAt: publishedAt,
		TenantID:    p.UserID,
		URL:         p.URL,
	})
	if err != nil {
		return nil, &errMarshalKnowledgePayload{
			articleID: p.ArticleID,
			err:       err,
		}
	}

	kevent := domain.KnowledgeEvent{
		EventID:       uuid.New(),
		OccurredAt:    occurredAt,
		TenantID:      userID,
		UserID:        &userID,
		ActorType:     domain.ActorService,
		ActorID:       "outbox-worker",
		EventType:     domain.EventArticleCreated,
		AggregateType: domain.AggregateArticle,
		AggregateID:   p.ArticleID,
		DedupeKey:     fmt.Sprintf(domain.DedupeKeyArticleCreated, p.ArticleID),
		Payload:       eventPayload,
	}
	return &kevent, nil
}
