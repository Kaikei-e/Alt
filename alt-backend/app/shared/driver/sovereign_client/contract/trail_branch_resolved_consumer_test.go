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

// TestAppendKnowledgeEvent_TrailBranchResolvedCarriesOptionalDismissReason
// pins the Wave 10 (D28(d)) producer contract for trail.branch_resolved.v1:
// the payload may carry an optional dismiss_reason — a one-tap scrutability
// signal, not new event vocabulary. The provider must accept the extended
// payload the same way it accepts the reason-less one (Critical Rule 7: CDC
// RED before producer GREEN for a new payload field).
func TestAppendKnowledgeEvent_TrailBranchResolvedCarriesOptionalDismissReason(t *testing.T) {
	mockProvider := newSovereignPact(t)

	const (
		eventID   = "0198a1b2-0001-7000-8000-0123456789ab"
		tenantID  = "11111111-1111-1111-1111-111111111111"
		userID    = "22222222-2222-2222-2222-222222222222"
		branchKey = "cluster:u:article:z"
	)
	occurredAt := time.Date(2026, 7, 18, 11, 0, 0, 0, time.UTC)

	payload, _ := json.Marshal(map[string]any{
		"branch_key":     branchKey,
		"resolution":     "dismissed",
		"dismiss_reason": "not_following_topic",
	})
	encoded := base64.StdEncoding.EncodeToString(payload)

	err := mockProvider.
		AddInteraction().
		Given("sovereign accepts trail.branch_resolved.v1 events carrying an optional dismiss reason").
		UponReceiving("an AppendKnowledgeEvent request for trail.branch_resolved.v1 with a dismiss_reason payload").
		WithCompleteRequest(consumer.Request{
			Method: "POST",
			Path:   matchers.String("/services.sovereign.v1.KnowledgeSovereignService/AppendKnowledgeEvent"),
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"event": matchers.Like(map[string]any{
					"eventId":       eventID,
					"occurredAt":    "2026-07-18T11:00:00Z",
					"tenantId":      tenantID,
					"userId":        userID,
					"actorType":     "user",
					"actorId":       userID,
					"eventType":     "trail.branch_resolved.v1",
					"aggregateType": "trail_branch",
					"aggregateId":   branchKey,
					"dedupeKey":     "trail.branch_resolved.v1:01938e82-7c00-7a7b-9b10-0123456789ab",
					// payload ships base64 (bytes field); decodes to
					// {"branch_key":...,"resolution":"dismissed","dismiss_reason":
					// "not_following_topic"}.
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
					EventType:     "trail.branch_resolved.v1",
					AggregateType: "trail_branch",
					AggregateId:   branchKey,
					DedupeKey:     "trail.branch_resolved.v1:01938e82-7c00-7a7b-9b10-0123456789ab",
					Payload:       payload,
				},
			}))
			return err
		})
	assert.NoError(t, err)
}
