// Package sovereign_client wraps the knowledge-sovereign Connect-RPC
// AppendKnowledgeEvent endpoint with rag-orchestrator's
// usecase.KnowledgeEventEmitter port. It is a thin adapter — usecase code
// stays unaware of Connect-RPC framing.
package sovereign_client

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	sovereignv1 "alt/gen/proto/services/sovereign/v1"
	"alt/gen/proto/services/sovereign/v1/sovereignv1connect"

	"rag-orchestrator/internal/usecase"
)

// AppendEventClient implements usecase.KnowledgeEventEmitter against
// knowledge-sovereign's KnowledgeSovereignService.AppendKnowledgeEvent RPC.
//
// Reproject-safety contract: the payload is composed strictly from event-
// time-bound inputs supplied by the caller; the client itself never reads
// wall-clock for business facts. dedupe_key folds in the entry_key and
// conversation_id so retries are idempotent.
type AppendEventClient struct {
	rpc sovereignv1connect.KnowledgeSovereignServiceClient
}

// NewAppendEventClient wires a Connect-RPC client. baseURL is the
// knowledge-sovereign service address (no trailing slash). httpClient is
// the pre-configured transport — the caller injects mTLS / service token
// configuration there.
func NewAppendEventClient(baseURL string, httpClient *http.Client) *AppendEventClient {
	return &AppendEventClient{
		rpc: sovereignv1connect.NewKnowledgeSovereignServiceClient(httpClient, baseURL),
	}
}

// EmitAugurConversationLinked publishes augur.conversation_linked.v1 into
// the knowledge_events log. Payload mirrors canonical contract §6.4.1
// (Wave 4-A in ADR-000853 / ADR-000854):
//
//	{
//	  "conversation_id": "<uuid>",
//	  "entry_key":       "<entry_key>",
//	  "lens_mode_id":    "default",
//	  "linked_at":       "<unix_ms>"
//	}
//
// dedupe_key is "augur.conversation_linked.v1:<entry_key>:<conversation_id>"
// so retries don't double-emit.
func (c *AppendEventClient) EmitAugurConversationLinked(
	ctx context.Context,
	in usecase.AugurConversationLinkedInput,
) error {
	if in.UserID == uuid.Nil {
		return errors.New("AugurConversationLinkedInput.UserID required")
	}
	if in.ConversationID == uuid.Nil {
		return errors.New("AugurConversationLinkedInput.ConversationID required")
	}
	if in.EntryKey == "" {
		return errors.New("AugurConversationLinkedInput.EntryKey required")
	}
	if in.LinkedAt == 0 {
		return errors.New("AugurConversationLinkedInput.LinkedAt must be non-zero (event.occurred_at)")
	}

	occurredAt := time.UnixMilli(in.LinkedAt)
	dedupeKey := fmt.Sprintf("augur.conversation_linked.v1:%s:%s",
		in.EntryKey, in.ConversationID.String())

	payload := fmt.Sprintf(
		`{"conversation_id":%q,"entry_key":%q,"lens_mode_id":%q,"linked_at":%d}`,
		in.ConversationID.String(), in.EntryKey, in.LensModeID, in.LinkedAt,
	)

	tenantID := ""
	if in.TenantID != uuid.Nil {
		tenantID = in.TenantID.String()
	}

	req := connect.NewRequest(&sovereignv1.AppendKnowledgeEventRequest{
		Event: &sovereignv1.KnowledgeEvent{
			EventId:       uuid.New().String(),
			OccurredAt:    timestamppb.New(occurredAt),
			TenantId:      tenantID,
			UserId:        in.UserID.String(),
			ActorType:     "service:rag-orchestrator",
			ActorId:       "augur-handler",
			EventType:     "augur.conversation_linked.v1",
			AggregateType: "knowledge_loop_session",
			AggregateId:   in.EntryKey,
			DedupeKey:     dedupeKey,
			Payload:       []byte(payload),
		},
	})
	if _, err := c.rpc.AppendKnowledgeEvent(ctx, req); err != nil {
		return fmt.Errorf("AppendKnowledgeEvent: %w", err)
	}
	return nil
}
