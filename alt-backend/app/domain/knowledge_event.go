package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Event type constants for knowledge events.
const (
	EventArticleCreated        = "ArticleCreated"
	EventSummaryVersionCreated = "SummaryVersionCreated"
	EventTagSetVersionCreated  = "TagSetVersionCreated"
	EventHomeItemsSeen         = "HomeItemsSeen"
	EventHomeItemOpened        = "HomeItemOpened"
	EventHomeItemDismissed     = "HomeItemDismissed"
	EventHomeItemAsked         = "HomeItemAsked"
	EventHomeItemListened      = "HomeItemListened"
	EventRecallSnoozed         = "RecallSnoozed"
	EventRecallDismissed       = "RecallDismissed"
	EventSummarySuperseded     = "SummarySuperseded"
	EventTagSetSuperseded      = "TagSetSuperseded"
	EventHomeItemSuperseded    = "HomeItemSuperseded"
)

// Actor type constants.
const (
	ActorSystem  = "system"
	ActorUser    = "user"
	ActorService = "service"
)

// Aggregate type constants.
const (
	AggregateArticle     = "article"
	AggregateRecap       = "recap"
	AggregateHomeSession = "home_session"
)

// KnowledgeEvent represents a single event in the knowledge event store.
type KnowledgeEvent struct {
	EventID       uuid.UUID       `json:"event_id" db:"event_id"`
	EventSeq      int64           `json:"event_seq" db:"event_seq"`
	OccurredAt    time.Time       `json:"occurred_at" db:"occurred_at"`
	TenantID      uuid.UUID       `json:"tenant_id" db:"tenant_id"`
	UserID        *uuid.UUID      `json:"user_id" db:"user_id"`
	ActorType     string          `json:"actor_type" db:"actor_type"`
	ActorID       string          `json:"actor_id" db:"actor_id"`
	EventType     string          `json:"event_type" db:"event_type"`
	AggregateType string          `json:"aggregate_type" db:"aggregate_type"`
	AggregateID   string          `json:"aggregate_id" db:"aggregate_id"`
	CorrelationID *uuid.UUID      `json:"correlation_id" db:"correlation_id"`
	CausationID   *uuid.UUID      `json:"causation_id" db:"causation_id"`
	DedupeKey     string          `json:"dedupe_key" db:"dedupe_key"`
	Payload       json.RawMessage `json:"payload" db:"payload"`
}
