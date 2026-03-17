package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// KnowledgeUserEvent represents a user interaction with a Knowledge Home item.
type KnowledgeUserEvent struct {
	UserEventID uuid.UUID       `json:"user_event_id" db:"user_event_id"`
	OccurredAt  time.Time       `json:"occurred_at" db:"occurred_at"`
	UserID      uuid.UUID       `json:"user_id" db:"user_id"`
	TenantID    uuid.UUID       `json:"tenant_id" db:"tenant_id"`
	EventType   string          `json:"event_type" db:"event_type"`
	ItemKey     string          `json:"item_key" db:"item_key"`
	Payload     json.RawMessage `json:"payload" db:"payload"`
	DedupeKey   string          `json:"dedupe_key" db:"dedupe_key"`
}
