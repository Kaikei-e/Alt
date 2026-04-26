package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Event type constants for knowledge events.
const (
	EventArticleCreated        = "ArticleCreated"
	EventArticleUpdated        = "ArticleUpdated"
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
	EventReasonMerged          = "ReasonMerged"
	EventHomeItemTagClicked    = "HomeItemTagClicked"

	// EventSummaryNarrativeBackfilled is a discovered event emitted by the
	// summary-narrative-backfill job. It carries an article_title sourced from
	// the current articles row at backfill time so the Knowledge Loop projector
	// can patch why_text on entries whose original SummaryVersionCreated event
	// pre-dated the producer's article_title capture. ADR-000846.
	EventSummaryNarrativeBackfilled = "SummaryNarrativeBackfilled"

	// Knowledge Loop transition events (append-only, versioned string convention).
	// See docs/plan/knowledge-loop-canonical-contract.md §8 and ADR-000831.
	EventKnowledgeLoopObserved          = "knowledge_loop.observed.v1"
	EventKnowledgeLoopOriented          = "knowledge_loop.oriented.v1"
	EventKnowledgeLoopDecisionPresented = "knowledge_loop.decision_presented.v1"
	EventKnowledgeLoopActed             = "knowledge_loop.acted.v1"
	EventKnowledgeLoopReturned          = "knowledge_loop.returned.v1"
	EventKnowledgeLoopDeferred          = "knowledge_loop.deferred.v1"
	EventKnowledgeLoopSessionReset      = "knowledge_loop.session_reset.v1"
	EventKnowledgeLoopLensModeSwitched  = "knowledge_loop.lens_mode_switched.v1"
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
	AggregateLoopSession = "loop_session"
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
