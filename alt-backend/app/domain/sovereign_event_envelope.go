package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// SovereignEnvelopeSchemaV1 is the current schema version for event envelopes.
const SovereignEnvelopeSchemaV1 = "v1"

// ValidEnvelopeSources lists the valid producer sources for sovereign event envelopes.
var ValidEnvelopeSources = []string{
	"pre-processor",
	"tag-generator",
	"recap-worker",
	"alt-backend",
	"user",
}

// SovereignEventEnvelope is the transport contract between producers and Knowledge Sovereign.
// Phase 3 defines the type; Phase 4 uses it for actual service boundary transport.
type SovereignEventEnvelope struct {
	EventID        uuid.UUID       `json:"event_id"`
	Source         string          `json:"source"`
	EventType      string          `json:"event_type"`
	OccurredAt     time.Time       `json:"occurred_at"`
	IdempotencyKey string          `json:"idempotency_key"`
	AggregateType  string          `json:"aggregate_type"`
	AggregateID    string          `json:"aggregate_id"`
	TenantID       uuid.UUID       `json:"tenant_id"`
	ActorType      string          `json:"actor_type"`
	ActorID        string          `json:"actor_id"`
	Payload        json.RawMessage `json:"payload"`
	SchemaVersion  string          `json:"schema_version"`
}

// NewSovereignEventEnvelopeFromEvent converts a KnowledgeEvent to a SovereignEventEnvelope.
func NewSovereignEventEnvelopeFromEvent(event KnowledgeEvent, source string) SovereignEventEnvelope {
	return SovereignEventEnvelope{
		EventID:        event.EventID,
		Source:         source,
		EventType:      event.EventType,
		OccurredAt:     event.OccurredAt,
		IdempotencyKey: event.DedupeKey,
		AggregateType:  event.AggregateType,
		AggregateID:    event.AggregateID,
		TenantID:       event.TenantID,
		ActorType:      event.ActorType,
		ActorID:        event.ActorID,
		Payload:        event.Payload,
		SchemaVersion:  SovereignEnvelopeSchemaV1,
	}
}
