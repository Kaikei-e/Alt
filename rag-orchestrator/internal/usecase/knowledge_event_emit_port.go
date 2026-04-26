package usecase

import (
	"context"

	"github.com/google/uuid"
)

// KnowledgeEventEmitter publishes augur-side events into the
// knowledge-sovereign event log so Knowledge Loop's Surface Planner v2
// resolver can pick them up. Implementations MUST:
//
//   - bind user_id physically into the request body / RPC parameter
//   - never mutate state on the rag-orchestrator side; emission is best-effort
//     and the caller must treat failure as non-fatal (warn-and-continue)
//   - dedupe via the supplied DedupeKey so retries don't double-emit
//
// The default DI wiring uses NoopKnowledgeEventEmitter so existing behavior
// is unchanged. Production deployments swap in a sovereign client gated by
// the RAG_ORCHESTRATOR_KNOWLEDGE_EVENT_EMIT env var (see main.go) once
// Wave 4-A's Pact CDC contract is published. See ADR-000853.
type KnowledgeEventEmitter interface {
	EmitAugurConversationLinked(
		ctx context.Context,
		input AugurConversationLinkedInput,
	) error
}

// AugurConversationLinkedInput carries the payload-resident fields the
// Knowledge Loop projector consumes (canonical contract §6.4.1). Every
// field is event-time bound — the upstream caller (the augur handler)
// never reads "now" wall-clock to compute these.
type AugurConversationLinkedInput struct {
	UserID         uuid.UUID
	TenantID       uuid.UUID
	EntryKey       string
	LensModeID     string
	ConversationID uuid.UUID
	LinkedAt       int64 // unix milli, derived from the conversation's persisted timestamp
}

// NoopKnowledgeEventEmitter is the safe default. It satisfies the
// interface without contacting any external service — Wave 4-A landed the
// wiring with this default so the live system is byte-for-byte unchanged
// until the sovereign client is explicitly enabled.
type NoopKnowledgeEventEmitter struct{}

func (NoopKnowledgeEventEmitter) EmitAugurConversationLinked(
	_ context.Context,
	_ AugurConversationLinkedInput,
) error {
	return nil
}
