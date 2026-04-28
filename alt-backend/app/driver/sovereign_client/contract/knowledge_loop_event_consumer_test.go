//go:build contract

package contract

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/pact-foundation/pact-go/v2/consumer"
	"github.com/pact-foundation/pact-go/v2/matchers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	sovereignv1 "alt/gen/proto/services/sovereign/v1"
	"alt/gen/proto/services/sovereign/v1/sovereignv1connect"
)

// Consumer-Driven Contract tests for the Knowledge Loop event append wire
// format. alt-backend's TransitionKnowledgeLoopUsecase calls
// AppendKnowledgeEvent on sovereign with event_type strings from the
// versioned convention (knowledge_loop.observed.v1 etc, see ADR-000840).
//
// These tests pin:
//   - The event_type strings sovereign must accept without interpretation.
//   - The dedupe_key = client_transition_id mapping so the sovereign
//     unique index on knowledge_events.dedupe_key blocks double-append
//     across restart boundaries.
//   - The aggregate_type = "loop_session" convention so sovereign can
//     maintain per-aggregate read models even without parsing the payload.

// loopEventBody composes the JSON body matcher for AppendKnowledgeEventRequest.
// Nested map matchers cannot be literal MapMatcher values; wrap with
// matchers.Like so pact-go v3 accepts the shape.
func loopEventBody(event map[string]any) matchers.MapMatcher {
	return matchers.MapMatcher{
		"event": matchers.Like(event),
	}
}

func TestAppendKnowledgeEvent_KnowledgeLoopObserved(t *testing.T) {
	mockProvider, err := consumer.NewV3Pact(consumer.MockHTTPProviderConfig{
		Consumer: "alt-backend",
		Provider: "knowledge-sovereign",
		PactDir:  filepath.Join(pactDir),
	})
	require.NoError(t, err)

	eventID := uuid.New()
	clientTransitionID := uuid.NewString()
	tenantID := uuid.New()
	userID := uuid.New()
	occurredAt := time.Date(2026, 4, 24, 10, 0, 0, 0, time.UTC)
	payload, _ := json.Marshal(map[string]any{
		"entry_key":                    "article:42",
		"lens_mode_id":                 "default",
		"from_stage":                   "LOOP_STAGE_OBSERVE",
		"to_stage":                     "LOOP_STAGE_OBSERVE",
		"trigger":                      "TRANSITION_TRIGGER_DWELL",
		"observed_projection_revision": 1,
		"client_transition_id":         clientTransitionID,
	})

	err = mockProvider.
		AddInteraction().
		Given("sovereign accepts append-only Loop transition events").
		UponReceiving("an AppendKnowledgeEvent request for knowledge_loop.observed.v1").
		WithCompleteRequest(consumer.Request{
			Method: "POST",
			Path:   matchers.String("/services.sovereign.v1.KnowledgeSovereignService/AppendKnowledgeEvent"),
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: loopEventBody(map[string]any{
				"eventId":       eventID.String(),
				"occurredAt":    "2026-04-24T10:00:00Z",
				"tenantId":      tenantID.String(),
				"userId":        userID.String(),
				"actorType":     "user",
				"actorId":       userID.String(),
				"eventType":     "knowledge_loop.observed.v1",
				"aggregateType": "loop_session",
				"aggregateId":   "article:42",
				"dedupeKey":     clientTransitionID,
				// payload ships as base64 on the wire (bytes field).
				"payload": "eyJlbnRyeV9rZXkiOiJhcnRpY2xlOjQyIn0",
			}),
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
					EventId:       eventID.String(),
					OccurredAt:    timestamppb.New(occurredAt),
					TenantId:      tenantID.String(),
					UserId:        userID.String(),
					ActorType:     "user",
					ActorId:       userID.String(),
					EventType:     "knowledge_loop.observed.v1",
					AggregateType: "loop_session",
					AggregateId:   "article:42",
					DedupeKey:     clientTransitionID,
					Payload:       payload,
				},
			}))
			return err
		})
	assert.NoError(t, err)
}

// TestAppendKnowledgeEvent_KnowledgeLoopDeferred pins the event_type for the
// canonical contract §8.2 "passive dismiss / snooze" path. The shape mirrors
// the other Loop events but is special in two ways:
//   - The trigger is TRANSITION_TRIGGER_DEFER (introduced after the initial
//     Loop rollout to fix the dismiss persistence regression — ADR follow-up).
//   - from_stage == to_stage; the alt-backend classifier rejects the same
//     pair for any non-DEFER trigger.
//
// Sovereign must accept this event_type without rejecting on the same-stage
// shape. Its projector then patches dismiss_state to deferred (covered by the
// projector unit test in the sovereign repo, not here).
func TestAppendKnowledgeEvent_KnowledgeLoopDeferred(t *testing.T) {
	mockProvider, err := consumer.NewV3Pact(consumer.MockHTTPProviderConfig{
		Consumer: "alt-backend",
		Provider: "knowledge-sovereign",
		PactDir:  filepath.Join(pactDir),
	})
	require.NoError(t, err)

	eventID := uuid.New()
	clientTransitionID := uuid.NewString()
	tenantID := uuid.New()
	userID := uuid.New()
	occurredAt := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
	payload, _ := json.Marshal(map[string]any{
		"entry_key":                    "article:42",
		"lens_mode_id":                 "default",
		"from_stage":                   "LOOP_STAGE_OBSERVE",
		"to_stage":                     "LOOP_STAGE_OBSERVE",
		"trigger":                      "TRANSITION_TRIGGER_DEFER",
		"observed_projection_revision": 1,
		"client_transition_id":         clientTransitionID,
	})

	err = mockProvider.
		AddInteraction().
		Given("sovereign accepts Deferred Loop events with same-stage transitions").
		UponReceiving("an AppendKnowledgeEvent request for knowledge_loop.deferred.v1").
		WithCompleteRequest(consumer.Request{
			Method: "POST",
			Path:   matchers.String("/services.sovereign.v1.KnowledgeSovereignService/AppendKnowledgeEvent"),
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: loopEventBody(map[string]any{
				"eventId":       eventID.String(),
				"occurredAt":    "2026-04-26T10:00:00Z",
				"tenantId":      tenantID.String(),
				"userId":        userID.String(),
				"actorType":     "user",
				"actorId":       userID.String(),
				"eventType":     "knowledge_loop.deferred.v1",
				"aggregateType": "loop_session",
				"aggregateId":   "article:42",
				"dedupeKey":     clientTransitionID,
				"payload":       "eyJlbnRyeV9rZXkiOiJhcnRpY2xlOjQyIn0",
			}),
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
					EventId:       eventID.String(),
					OccurredAt:    timestamppb.New(occurredAt),
					TenantId:      tenantID.String(),
					UserId:        userID.String(),
					ActorType:     "user",
					ActorId:       userID.String(),
					EventType:     "knowledge_loop.deferred.v1",
					AggregateType: "loop_session",
					AggregateId:   "article:42",
					DedupeKey:     clientTransitionID,
					Payload:       payload,
				},
			}))
			return err
		})
	assert.NoError(t, err)
}

// TestAppendKnowledgeEvent_KnowledgeLoopReviewed pins the Review-lane event
// contract. The action distinction is the TransitionTrigger value in payload;
// alt-backend must not invent a parallel `action` field.
func TestAppendKnowledgeEvent_KnowledgeLoopReviewed(t *testing.T) {
	mockProvider, err := consumer.NewV3Pact(consumer.MockHTTPProviderConfig{
		Consumer: "alt-backend",
		Provider: "knowledge-sovereign",
		PactDir:  filepath.Join(pactDir),
	})
	require.NoError(t, err)

	eventID := uuid.New()
	clientTransitionID := uuid.NewString()
	tenantID := uuid.New()
	userID := uuid.New()
	occurredAt := time.Date(2026, 4, 28, 10, 0, 0, 0, time.UTC)
	payload, _ := json.Marshal(map[string]any{
		"entry_key":                    "article:42",
		"lens_mode_id":                 "default",
		"from_stage":                   "LOOP_STAGE_OBSERVE",
		"to_stage":                     "LOOP_STAGE_OBSERVE",
		"trigger":                      "TRANSITION_TRIGGER_RECHECK",
		"observed_projection_revision": 1,
		"client_transition_id":         clientTransitionID,
	})

	err = mockProvider.
		AddInteraction().
		Given("sovereign accepts Reviewed Loop events with trigger-based actions").
		UponReceiving("an AppendKnowledgeEvent request for knowledge_loop.reviewed.v1").
		WithCompleteRequest(consumer.Request{
			Method: "POST",
			Path:   matchers.String("/services.sovereign.v1.KnowledgeSovereignService/AppendKnowledgeEvent"),
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: loopEventBody(map[string]any{
				"eventId":       eventID.String(),
				"occurredAt":    "2026-04-28T10:00:00Z",
				"tenantId":      tenantID.String(),
				"userId":        userID.String(),
				"actorType":     "user",
				"actorId":       userID.String(),
				"eventType":     "knowledge_loop.reviewed.v1",
				"aggregateType": "loop_session",
				"aggregateId":   "article:42",
				"dedupeKey":     clientTransitionID,
				"payload":       "eyJlbnRyeV9rZXkiOiJhcnRpY2xlOjQyIn0",
			}),
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
					EventId:       eventID.String(),
					OccurredAt:    timestamppb.New(occurredAt),
					TenantId:      tenantID.String(),
					UserId:        userID.String(),
					ActorType:     "user",
					ActorId:       userID.String(),
					EventType:     "knowledge_loop.reviewed.v1",
					AggregateType: "loop_session",
					AggregateId:   "article:42",
					DedupeKey:     clientTransitionID,
					Payload:       payload,
				},
			}))
			return err
		})
	assert.NoError(t, err)
}

// TestAppendKnowledgeEvent_KnowledgeLoopActed pins the event_type for the
// Act stage so sovereign's projection can react to it distinctly from
// HomeItemOpened (single-emission rule, ADR-000831 §3.8).
func TestAppendKnowledgeEvent_KnowledgeLoopActed(t *testing.T) {
	mockProvider, err := consumer.NewV3Pact(consumer.MockHTTPProviderConfig{
		Consumer: "alt-backend",
		Provider: "knowledge-sovereign",
		PactDir:  filepath.Join(pactDir),
	})
	require.NoError(t, err)

	eventID := uuid.New()
	clientTransitionID := uuid.NewString()
	tenantID := uuid.New()
	userID := uuid.New()
	occurredAt := time.Date(2026, 4, 24, 10, 0, 0, 0, time.UTC)
	payload, _ := json.Marshal(map[string]any{
		"entry_key":                    "article:42",
		"lens_mode_id":                 "default",
		"from_stage":                   "LOOP_STAGE_DECIDE",
		"to_stage":                     "LOOP_STAGE_ACT",
		"trigger":                      "TRANSITION_TRIGGER_USER_TAP",
		"observed_projection_revision": 1,
		"client_transition_id":         clientTransitionID,
	})

	err = mockProvider.
		AddInteraction().
		Given("sovereign accepts Act-stage Loop events without inferring HomeItemOpened").
		UponReceiving("an AppendKnowledgeEvent request for knowledge_loop.acted.v1").
		WithCompleteRequest(consumer.Request{
			Method: "POST",
			Path:   matchers.String("/services.sovereign.v1.KnowledgeSovereignService/AppendKnowledgeEvent"),
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: loopEventBody(map[string]any{
				"eventId":       eventID.String(),
				"occurredAt":    "2026-04-24T10:00:00Z",
				"tenantId":      tenantID.String(),
				"userId":        userID.String(),
				"actorType":     "user",
				"actorId":       userID.String(),
				"eventType":     "knowledge_loop.acted.v1",
				"aggregateType": "loop_session",
				"aggregateId":   "article:42",
				"dedupeKey":     clientTransitionID,
				"payload":       "eyJlbnRyeV9rZXkiOiJhcnRpY2xlOjQyIn0",
			}),
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
					EventId:       eventID.String(),
					OccurredAt:    timestamppb.New(occurredAt),
					TenantId:      tenantID.String(),
					UserId:        userID.String(),
					ActorType:     "user",
					ActorId:       userID.String(),
					EventType:     "knowledge_loop.acted.v1",
					AggregateType: "loop_session",
					AggregateId:   "article:42",
					DedupeKey:     clientTransitionID,
					Payload:       payload,
				},
			}))
			return err
		})
	assert.NoError(t, err)
}
