//go:build contract

package contract

import (
	"context"
	"encoding/base64"
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

// TestAppendKnowledgeEvent_KnowledgeLoopActOutcome pins the system-emitted
// closure event introduced by ADR-000908 §Δ1. The alt-backend view tracker
// emits this event when dwell ≥ 30s or ask conversation turn count ≥ 3
// clears; a separate cron in knowledge-sovereign emits the 7-day
// no_engagement fallback. The wire shape this consumer test pins is the
// immediate-emit path only — the cron path lives entirely inside
// knowledge-sovereign and is not a cross-service contract.
//
// Three invariants the provider must honour:
//
//  1. actor_type = "system" — even though the trigger is a user action,
//     the outcome event is system-derived (the view tracker, not the
//     user, decides when an outcome has materialised). Sovereign's
//     user-emittable allowlist must NOT include this event type.
//  2. aggregate_type = "knowledge_loop_entry" — distinct from the
//     "loop_session" aggregate that user transitions use, so projector
//     dispatch can match on aggregate_type without parsing payload.
//  3. dedupe_key = `knowledge_loop.act_outcome.v1:<acted_event_id>:<outcome>` —
//     keyed on the originating Acted event id so the immediate and cron
//     paths cannot double-emit the same logical closure. The sovereign
//     unique index on knowledge_events.dedupe_key enforces idempotency at
//     the slow path.
func TestAppendKnowledgeEvent_KnowledgeLoopActOutcome(t *testing.T) {
	mockProvider, err := consumer.NewV3Pact(consumer.MockHTTPProviderConfig{
		Consumer: "alt-backend",
		Provider: "knowledge-sovereign",
		PactDir:  filepath.Join(pactDir),
	})
	require.NoError(t, err)

	eventID := uuid.New()
	actedEventID := uuid.New()
	tenantID := uuid.New()
	userID := uuid.New()
	occurredAt := time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC)

	outcomePayload := map[string]any{
		"acted_event_id": actedEventID.String(),
		"entry_key":      "article:42",
		"lens_mode_id":   "default",
		"outcome":        "engaged",
		"observed_at":    "2026-05-23T12:00:00Z",
	}
	payload, _ := json.Marshal(outcomePayload)
	encoded := base64.StdEncoding.EncodeToString(payload)
	dedupeKey := "knowledge_loop.act_outcome.v1:" + actedEventID.String() + ":engaged"

	err = mockProvider.
		AddInteraction().
		Given("sovereign accepts system-emitted ActOutcome closure events").
		UponReceiving("an AppendKnowledgeEvent request for knowledge_loop.act_outcome.v1 with outcome=engaged").
		WithCompleteRequest(consumer.Request{
			Method: "POST",
			Path:   matchers.String("/services.sovereign.v1.KnowledgeSovereignService/AppendKnowledgeEvent"),
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: loopEventBody(map[string]any{
				"eventId":       eventID.String(),
				"occurredAt":    "2026-05-23T12:00:00Z",
				"tenantId":      tenantID.String(),
				"userId":        userID.String(),
				"actorType":     "system",
				"actorId":       "alt-backend-view-tracker",
				"eventType":     "knowledge_loop.act_outcome.v1",
				"aggregateType": "knowledge_loop_entry",
				"aggregateId":   "article:42",
				"dedupeKey":     dedupeKey,
				"payload":       encoded,
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
					ActorType:     "system",
					ActorId:       "alt-backend-view-tracker",
					EventType:     "knowledge_loop.act_outcome.v1",
					AggregateType: "knowledge_loop_entry",
					AggregateId:   "article:42",
					DedupeKey:     dedupeKey,
					Payload:       payload,
				},
			}))
			return err
		})
	assert.NoError(t, err)
}

// TestAppendKnowledgeEvent_KnowledgeLoopActOutcome_DeepEngagement pins the
// deep_engagement outcome label (ask turn ≥ 3 path). The wire shape is
// identical to the engaged variant; the outcome string and dedupe_key
// suffix are what changes, and the provider must accept both without
// any payload interpretation.
func TestAppendKnowledgeEvent_KnowledgeLoopActOutcome_DeepEngagement(t *testing.T) {
	mockProvider, err := consumer.NewV3Pact(consumer.MockHTTPProviderConfig{
		Consumer: "alt-backend",
		Provider: "knowledge-sovereign",
		PactDir:  filepath.Join(pactDir),
	})
	require.NoError(t, err)

	eventID := uuid.New()
	actedEventID := uuid.New()
	tenantID := uuid.New()
	userID := uuid.New()
	occurredAt := time.Date(2026, 5, 23, 12, 5, 0, 0, time.UTC)

	outcomePayload := map[string]any{
		"acted_event_id": actedEventID.String(),
		"entry_key":      "article:42",
		"lens_mode_id":   "default",
		"outcome":        "deep_engagement",
		"observed_at":    "2026-05-23T12:05:00Z",
	}
	payload, _ := json.Marshal(outcomePayload)
	encoded := base64.StdEncoding.EncodeToString(payload)
	dedupeKey := "knowledge_loop.act_outcome.v1:" + actedEventID.String() + ":deep_engagement"

	err = mockProvider.
		AddInteraction().
		Given("sovereign accepts deep_engagement outcomes for prolonged Augur conversations").
		UponReceiving("an AppendKnowledgeEvent request for knowledge_loop.act_outcome.v1 with outcome=deep_engagement").
		WithCompleteRequest(consumer.Request{
			Method: "POST",
			Path:   matchers.String("/services.sovereign.v1.KnowledgeSovereignService/AppendKnowledgeEvent"),
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: loopEventBody(map[string]any{
				"eventId":       eventID.String(),
				"occurredAt":    "2026-05-23T12:05:00Z",
				"tenantId":      tenantID.String(),
				"userId":        userID.String(),
				"actorType":     "system",
				"actorId":       "alt-backend-view-tracker",
				"eventType":     "knowledge_loop.act_outcome.v1",
				"aggregateType": "knowledge_loop_entry",
				"aggregateId":   "article:42",
				"dedupeKey":     dedupeKey,
				"payload":       encoded,
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
					ActorType:     "system",
					ActorId:       "alt-backend-view-tracker",
					EventType:     "knowledge_loop.act_outcome.v1",
					AggregateType: "knowledge_loop_entry",
					AggregateId:   "article:42",
					DedupeKey:     dedupeKey,
					Payload:       payload,
				},
			}))
			return err
		})
	assert.NoError(t, err)
}

// TestAppendKnowledgeEvent_KnowledgeLoopActed_SemanticPayload pins the Phase 2
// canonical wire shape. The KnowledgeLoopActed payload now carries semantic
// metadata (`acted_intent`, `target_type`, `target_ref`, `continue_flag`,
// `presented_intents`, `action_id`) so the projector can reproject
// `continue_context.recent_action_labels` and Surface Planner v2 can use
// `continue_flag=true` as a Continue signal.
//
// This sibling test is additive: the original
// TestAppendKnowledgeEvent_KnowledgeLoopActed continues to pin the
// stage-only legacy shape, so a regression in either direction is caught.
func TestAppendKnowledgeEvent_KnowledgeLoopActed_SemanticPayload(t *testing.T) {
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
	occurredAt := time.Date(2026, 5, 9, 10, 0, 0, 0, time.UTC)
	conversationID := uuid.NewString()

	semanticPayload := map[string]any{
		"entry_key":                    "article:42",
		"lens_mode_id":                 "default",
		"from_stage":                   "LOOP_STAGE_DECIDE",
		"to_stage":                     "LOOP_STAGE_ACT",
		"trigger":                      "TRANSITION_TRIGGER_USER_TAP",
		"observed_projection_revision": 1,
		"client_transition_id":         clientTransitionID,
		// Phase 2 semantic fields:
		"presented_intents": []string{"open", "ask", "snooze"},
		"acted_intent":      "DECISION_INTENT_ASK",
		"action_id":         "ask",
		"target_type":       "ACT_TARGET_TYPE_CONVERSATION",
		"target_ref":        conversationID,
		"continue_flag":     true,
	}
	payload, _ := json.Marshal(semanticPayload)
	encoded := base64.StdEncoding.EncodeToString(payload)

	err = mockProvider.
		AddInteraction().
		Given("sovereign accepts Acted events with Phase 2 semantic metadata").
		UponReceiving("an AppendKnowledgeEvent request with acted_intent + target_type + continue_flag").
		WithCompleteRequest(consumer.Request{
			Method: "POST",
			Path:   matchers.String("/services.sovereign.v1.KnowledgeSovereignService/AppendKnowledgeEvent"),
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: loopEventBody(map[string]any{
				"eventId":       eventID.String(),
				"occurredAt":    "2026-05-09T10:00:00Z",
				"tenantId":      tenantID.String(),
				"userId":        userID.String(),
				"actorType":     "user",
				"actorId":       userID.String(),
				"eventType":     "knowledge_loop.acted.v1",
				"aggregateType": "loop_session",
				"aggregateId":   "article:42",
				"dedupeKey":     clientTransitionID,
				"payload":       encoded,
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
