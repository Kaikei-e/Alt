//go:build contract

// Pact CDC: rag-orchestrator → knowledge-sovereign AppendKnowledgeEvent
// for augur.conversation_linked.v1 (Wave 4-A, ADR-000853 / ADR-000855).
//
// Pins the Connect-RPC over HTTP/1.1+JSON wire format that
// rag-orchestrator's AppendEventClient uses when it emits the augur ↔
// Knowledge Loop link event so Surface Planner v2 can pick up the
// HasAugurLink signal. The contract pins:
//
//   - POST /services.sovereign.v1.KnowledgeSovereignService/AppendKnowledgeEvent
//   - Content-Type: application/json
//   - Camel-cased protojson field names (eventId, occurredAt, …) per
//     ADR-000764 Connect-RPC convention.
//   - tenant_id is REQUIRED (Wave 4-A strictness — Unauthenticated reject
//     when caller omits the header). The wire body therefore always
//     carries a non-empty tenantId field.
//   - dedupe_key folds entry_key and conversation_id so retries are
//     idempotent against the sovereign unique index.
//   - payload bytes encode the canonical contract §6.4.1 fields
//     (conversation_id / entry_key / lens_mode_id / linked_at) — base64
//     on the wire because the proto field is `bytes`.

package contract

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"path/filepath"
	"testing"

	"rag-orchestrator/internal/adapter/sovereign_client"
	"rag-orchestrator/internal/usecase"

	"github.com/google/uuid"
	"github.com/pact-foundation/pact-go/v2/consumer"
	"github.com/pact-foundation/pact-go/v2/matchers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newKnowledgeSovereignPact(t *testing.T) *consumer.V3HTTPMockProvider {
	t.Helper()
	mockProvider, err := consumer.NewV3Pact(consumer.MockHTTPProviderConfig{
		Consumer: "rag-orchestrator",
		Provider: "knowledge-sovereign",
		PactDir:  filepath.Join(pactDir),
	})
	require.NoError(t, err)
	return mockProvider
}

// uuidLikePattern matches RFC 4122 UUIDs in any version. event_id is
// generated at emit time with uuid.New() so the wire value is not
// deterministic across runs; the matcher pins shape, not literal.
const uuidLikePattern = `^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`

// TestAugurConversationLinkedContract pins the wire contract for the
// augur.conversation_linked.v1 emit path. With fixed tenant/user/entry/
// conversation/linked_at inputs the dedupe_key, aggregate_id, and the
// inner payload base64 are fully deterministic; only event_id varies
// per call so it uses a regex matcher.
func TestAugurConversationLinkedContract(t *testing.T) {
	mockProvider := newKnowledgeSovereignPact(t)

	tenantID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	userID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	conversationID := uuid.MustParse("33333333-3333-3333-3333-333333333333")
	entryKey := "article:42"
	lensModeID := "default"
	linkedAt := int64(1714060800000) // 2024-04-25T16:00:00Z, fixed unix milli

	// Mirror append_event_client.go's payload builder exactly so the
	// base64 the consumer puts on the wire is the same byte sequence the
	// matcher pins. Any drift in field order or escaping breaks this.
	innerPayload := fmt.Sprintf(
		`{"conversation_id":%q,"entry_key":%q,"lens_mode_id":%q,"linked_at":%d}`,
		conversationID.String(), entryKey, lensModeID, linkedAt,
	)
	expectedPayloadB64 := base64.StdEncoding.EncodeToString([]byte(innerPayload))

	// dedupe_key formula in append_event_client.go:76-77.
	expectedDedupeKey := fmt.Sprintf(
		"augur.conversation_linked.v1:%s:%s",
		entryKey, conversationID.String(),
	)

	// occurred_at is derived from linkedAt → time.UnixMilli(...). The
	// protojson Timestamp encoding for that exact instant.
	expectedOccurredAt := "2024-04-25T16:00:00Z"

	err := mockProvider.
		AddInteraction().
		Given("sovereign accepts append-only Loop transition events").
		UponReceiving("an AppendKnowledgeEvent request for augur.conversation_linked.v1").
		WithCompleteRequest(consumer.Request{
			Method: "POST",
			Path:   matchers.String("/services.sovereign.v1.KnowledgeSovereignService/AppendKnowledgeEvent"),
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"event": matchers.Like(map[string]interface{}{
					"eventId":       matchers.Regex("00000000-0000-0000-0000-000000000000", uuidLikePattern),
					"occurredAt":    expectedOccurredAt,
					"tenantId":      tenantID.String(),
					"userId":        userID.String(),
					"actorType":     "service:rag-orchestrator",
					"actorId":       "augur-handler",
					"eventType":     "augur.conversation_linked.v1",
					"aggregateType": "knowledge_loop_session",
					"aggregateId":   entryKey,
					"dedupeKey":     expectedDedupeKey,
					"payload":       expectedPayloadB64,
				}),
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status: 200,
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"success": matchers.Like(true),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			client := sovereign_client.NewAppendEventClient(
				fmt.Sprintf("http://%s:%d", config.Host, config.Port),
				http.DefaultClient,
			)
			return client.EmitAugurConversationLinked(context.Background(), usecase.AugurConversationLinkedInput{
				UserID:         userID,
				TenantID:       tenantID,
				EntryKey:       entryKey,
				LensModeID:     lensModeID,
				ConversationID: conversationID,
				LinkedAt:       linkedAt,
			})
		})
	assert.NoError(t, err)
}

// TestAugurConversationLinkedContract_TenantRequired pins the Wave 4-A
// strictness contract: tenant_id MUST be present and non-zero. The
// AppendEventClient rejects uuid.Nil with a clear error, so this never
// reaches the wire.
func TestAugurConversationLinkedContract_TenantRequired(t *testing.T) {
	client := sovereign_client.NewAppendEventClient("http://127.0.0.1:1", http.DefaultClient)

	err := client.EmitAugurConversationLinked(context.Background(), usecase.AugurConversationLinkedInput{
		UserID:         uuid.MustParse("22222222-2222-2222-2222-222222222222"),
		TenantID:       uuid.Nil,
		EntryKey:       "article:42",
		LensModeID:     "default",
		ConversationID: uuid.MustParse("33333333-3333-3333-3333-333333333333"),
		LinkedAt:       1714060800000,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "TenantID")
}
