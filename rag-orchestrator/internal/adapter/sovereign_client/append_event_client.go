// Package sovereign_client wraps the knowledge-sovereign Connect-RPC
// AppendKnowledgeEvent endpoint with rag-orchestrator's
// usecase.KnowledgeEventEmitter port. It is a thin adapter — usecase code
// stays unaware of Connect-RPC framing.
package sovereign_client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	sovereignv1 "alt/gen/proto/services/sovereign/v1"
	"alt/gen/proto/services/sovereign/v1/sovereignv1connect"

	"rag-orchestrator/internal/usecase"
)

// Option configures AppendEventClient.
type Option func(*clientOptions)

type clientOptions struct {
	token string
}

// WithToken configures the Bearer token for authenticating to knowledge-sovereign.
func WithToken(token string) Option {
	return func(o *clientOptions) {
		o.token = token
	}
}

type clientAuthInterceptor struct {
	token string
}

func (i *clientAuthInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		if i.token != "" {
			req.Header().Set("Authorization", "Bearer "+i.token)
		}
		return next(ctx, req)
	}
}

func (i *clientAuthInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return func(ctx context.Context, spec connect.Spec) connect.StreamingClientConn {
		conn := next(ctx, spec)
		if i.token != "" {
			conn.RequestHeader().Set("Authorization", "Bearer "+i.token)
		}
		return conn
	}
}

func (i *clientAuthInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return next
}

const minSovereignEventTokenLen = 24

// LoadSovereignEventToken resolves the caller authentication token for
// knowledge-sovereign's event listener.
// If authMode is "disabled", it returns ("", nil).
// Otherwise, tokenFile (or SOVEREIGN_EVENT_TOKEN) must be non-empty and at least 24 chars.
func LoadSovereignEventToken(tokenFile, authMode string) (string, error) {
	if strings.EqualFold(strings.TrimSpace(authMode), "disabled") {
		return "", nil
	}
	if tokenFile != "" {
		content, err := os.ReadFile(tokenFile)
		if err != nil {
			return "", fmt.Errorf("read SOVEREIGN_EVENT_TOKEN_FILE %s: %w", tokenFile, err)
		}
		token := strings.TrimSpace(string(content))
		if len(token) < minSovereignEventTokenLen {
			return "", fmt.Errorf("event token from SOVEREIGN_EVENT_TOKEN_FILE must be at least %d characters", minSovereignEventTokenLen)
		}
		return token, nil
	}
	if token := strings.TrimSpace(os.Getenv("SOVEREIGN_EVENT_TOKEN")); token != "" {
		if len(token) < minSovereignEventTokenLen {
			return "", fmt.Errorf("SOVEREIGN_EVENT_TOKEN must be at least %d characters", minSovereignEventTokenLen)
		}
		return token, nil
	}
	return "", errors.New("SOVEREIGN_EVENT_TOKEN_FILE or SOVEREIGN_EVENT_TOKEN is required unless SOVEREIGN_EVENT_AUTH=disabled")
}

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
//
// The codec is pinned to JSON via connect.WithProtoJSON() so the wire
// format matches ADR-000764 (Connect-RPC over HTTP/1.1 + JSON) and the
// Pact CDC contract that pins protojson camelCase field names. Without
// this option Connect-go defaults to application/proto, which the
// provider stub does not decode and which silently breaks the pact gate.
func NewAppendEventClient(baseURL string, httpClient *http.Client, opts ...Option) *AppendEventClient {
	var co clientOptions
	for _, opt := range opts {
		opt(&co)
	}

	clientOpts := []connect.ClientOption{connect.WithProtoJSON()}
	if co.token != "" {
		clientOpts = append(clientOpts, connect.WithInterceptors(&clientAuthInterceptor{token: co.token}))
	}

	return &AppendEventClient{
		rpc: sovereignv1connect.NewKnowledgeSovereignServiceClient(
			httpClient, baseURL, clientOpts...,
		),
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
	if in.TenantID == uuid.Nil {
		return errors.New("AugurConversationLinkedInput.TenantID required")
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

	payloadStruct := struct {
		ConversationID string `json:"conversation_id"`
		EntryKey       string `json:"entry_key"`
		LensModeID     string `json:"lens_mode_id"`
		LinkedAt       int64  `json:"linked_at"`
	}{
		ConversationID: in.ConversationID.String(),
		EntryKey:       in.EntryKey,
		LensModeID:     in.LensModeID,
		LinkedAt:       in.LinkedAt,
	}
	payload, err := json.Marshal(payloadStruct)
	if err != nil {
		return fmt.Errorf("marshal conversation_linked payload: %w", err)
	}

	req := connect.NewRequest(&sovereignv1.AppendKnowledgeEventRequest{
		Event: &sovereignv1.KnowledgeEvent{
			EventId:       uuid.New().String(),
			OccurredAt:    timestamppb.New(occurredAt),
			TenantId:      in.TenantID.String(),
			UserId:        in.UserID.String(),
			ActorType:     "service:rag-orchestrator",
			ActorId:       "augur-handler",
			EventType:     "augur.conversation_linked.v1",
			AggregateType: "knowledge_loop_session",
			AggregateId:   in.EntryKey,
			DedupeKey:     dedupeKey,
			Payload:       payload,
		},
	})
	if _, err := c.rpc.AppendKnowledgeEvent(ctx, req); err != nil {
		return fmt.Errorf("AppendKnowledgeEvent: %w", err)
	}
	return nil
}
