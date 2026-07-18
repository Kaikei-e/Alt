//go:build contract

package contract

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/pact-foundation/pact-go/v2/consumer"
	"github.com/pact-foundation/pact-go/v2/matchers"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/types/known/timestamppb"

	sovereignv1 "alt/gen/proto/services/sovereign/v1"
	"alt/gen/proto/services/sovereign/v1/sovereignv1connect"
)

// TestAppendKnowledgeEvent_TrailActOutcomeCarriesRawDwell pins the
// trail.act_outcome.v1 producer contract (Critical Rule 7: CDC RED before
// producer GREEN). The payload is the raw measurement only — branch_key,
// item_key, dwell_ms. A classification field (outcome enum) appearing here
// would violate D18; the dedupe key pins first-write-wins per proposal (D19).
func TestAppendKnowledgeEvent_TrailActOutcomeCarriesRawDwell(t *testing.T) {
	mockProvider := newSovereignPact(t)

	const (
		eventID   = "0198a1b2-0000-7000-8000-0123456789ab"
		tenantID  = "11111111-1111-1111-1111-111111111111"
		userID    = "22222222-2222-2222-2222-222222222222"
		branchKey = "cluster:u:article:z"
	)
	occurredAt := time.Date(2026, 7, 18, 10, 0, 0, 0, time.UTC)

	payload, _ := json.Marshal(map[string]any{
		"branch_key": branchKey,
		"item_key":   "article:z",
		"dwell_ms":   42000,
	})
	encoded := base64.StdEncoding.EncodeToString(payload)

	err := mockProvider.
		AddInteraction().
		Given("sovereign accepts trail.act_outcome.v1 events carrying raw dwell").
		UponReceiving("an AppendKnowledgeEvent request for trail.act_outcome.v1 with branch_key, item_key and dwell_ms").
		WithCompleteRequest(consumer.Request{
			Method: "POST",
			Path:   matchers.String("/services.sovereign.v1.KnowledgeSovereignService/AppendKnowledgeEvent"),
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"event": matchers.Like(map[string]any{
					"eventId":       eventID,
					"occurredAt":    "2026-07-18T10:00:00Z",
					"tenantId":      tenantID,
					"userId":        userID,
					"actorType":     "user",
					"actorId":       userID,
					"eventType":     "trail.act_outcome.v1",
					"aggregateType": "trail_branch",
					"aggregateId":   branchKey,
					// One outcome per taken branch: the dedupe key is the
					// proposal ref, not a client-minted UUID (D19).
					"dedupeKey": "trail.act_outcome.v1:" + branchKey,
					// payload ships as base64 on the wire (bytes field). The
					// pinned blob decodes to {"branch_key":...,"item_key":...,
					// "dwell_ms":42000} — raw measurement, no outcome enum.
					"payload": encoded,
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
			client := sovereignv1connect.NewKnowledgeSovereignServiceClient(
				http.DefaultClient,
				fmt.Sprintf("http://%s:%d", config.Host, config.Port),
				connect.WithProtoJSON(),
			)
			_, err := client.AppendKnowledgeEvent(context.Background(), connect.NewRequest(&sovereignv1.AppendKnowledgeEventRequest{
				Event: &sovereignv1.KnowledgeEvent{
					EventId:       eventID,
					OccurredAt:    timestamppb.New(occurredAt),
					TenantId:      tenantID,
					UserId:        userID,
					ActorType:     "user",
					ActorId:       userID,
					EventType:     "trail.act_outcome.v1",
					AggregateType: "trail_branch",
					AggregateId:   branchKey,
					DedupeKey:     "trail.act_outcome.v1:" + branchKey,
					Payload:       payload,
				},
			}))
			return err
		})
	assert.NoError(t, err)
}
