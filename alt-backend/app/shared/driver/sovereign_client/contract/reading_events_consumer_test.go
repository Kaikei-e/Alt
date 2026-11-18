//go:build contract

package contract

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/pact-foundation/pact-go/v2/consumer"
	"github.com/pact-foundation/pact-go/v2/matchers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	sovereignv1 "alt/gen/proto/services/sovereign/v1"
)

type readingEventContractSpec struct {
	providerState string
	description   string
	eventID       string
	clientEventID string
	eventType     string
	aggregateID   string
	payload       map[string]any
}

func assertAppendReadingEventContract(t *testing.T, spec readingEventContractSpec) {
	t.Helper()

	mockProvider := newSovereignPact(t)

	const (
		tenantID = "11111111-1111-1111-1111-111111111111"
		userID   = "22222222-2222-2222-2222-222222222222"
	)
	occurredAt := time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC)
	dedupeKey := spec.eventType + ":" + spec.clientEventID

	payload, err := json.Marshal(spec.payload)
	require.NoError(t, err)
	encoded := base64.StdEncoding.EncodeToString(payload)

	err = mockProvider.
		AddInteraction().
		Given(spec.providerState).
		UponReceiving(spec.description).
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/services.sovereign.v1.KnowledgeSovereignService/AppendKnowledgeEvent"),
			Headers: sovereignHeaders(),
			Body: matchers.MapMatcher{
				"event": matchers.StructMatcher{
					"eventId":       matchers.Like(spec.eventID),
					"occurredAt":    matchers.Like("2026-11-18T09:00:00Z"),
					"tenantId":      matchers.Like(tenantID),
					"userId":        matchers.Like(userID),
					"actorType":     matchers.String("user"),
					"actorId":       matchers.Like(userID),
					"eventType":     matchers.String(spec.eventType),
					"aggregateType": matchers.String("reading_session"),
					"aggregateId":   matchers.Like(spec.aggregateID),
					"dedupeKey":     matchers.Like(dedupeKey),
					"payload":       matchers.Like(encoded),
				},
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status: 200,
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"eventSeq": matchers.Like("123"),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			client := newSovereignClient(config)
			_, err := client.AppendKnowledgeEvent(context.Background(), connect.NewRequest(&sovereignv1.AppendKnowledgeEventRequest{
				Event: &sovereignv1.KnowledgeEvent{
					EventId:       spec.eventID,
					OccurredAt:    timestamppb.New(occurredAt),
					TenantId:      tenantID,
					UserId:        userID,
					ActorType:     "user",
					ActorId:       userID,
					EventType:     spec.eventType,
					AggregateType: "reading_session",
					AggregateId:   spec.aggregateID,
					DedupeKey:     dedupeKey,
					Payload:       payload,
				},
			}))
			return err
		})
	assert.NoError(t, err)
}

func TestAppendKnowledgeEvent_ReadingSurfaceViewed(t *testing.T) {
	const clientEventID = "019a0000-0001-7000-8000-000000000101"

	assertAppendReadingEventContract(t, readingEventContractSpec{
		providerState: "sovereign accepts reading.surface_viewed.v1 events",
		description:   "an AppendKnowledgeEvent request for reading.surface_viewed.v1",
		eventID:       "019a0000-0001-7000-8000-000000000001",
		clientEventID: clientEventID,
		eventType:     "reading.surface_viewed.v1",
		aggregateID:   "surface:gallery",
		payload: map[string]any{
			"client_event_id": clientEventID,
			"surface":         "gallery",
			"received_at":     "2026-11-18T09:00:01Z",
		},
	})
}

func TestAppendKnowledgeEvent_ReadingReachedEnd(t *testing.T) {
	const (
		clientEventID = "019a0000-0002-7000-8000-000000000102"
		articleID     = "33333333-3333-3333-3333-333333333333"
	)

	assertAppendReadingEventContract(t, readingEventContractSpec{
		providerState: "sovereign accepts reading.reached_end.v1 events",
		description:   "an AppendKnowledgeEvent request for reading.reached_end.v1",
		eventID:       "019a0000-0002-7000-8000-000000000002",
		clientEventID: clientEventID,
		eventType:     "reading.reached_end.v1",
		aggregateID:   "article:" + articleID,
		payload: map[string]any{
			"client_event_id": clientEventID,
			"article_id":      articleID,
			"surface":         "swipe",
			"received_at":     "2026-11-18T09:00:01Z",
		},
	})
}

func TestAppendKnowledgeEvent_ReadingSessionEnded(t *testing.T) {
	const (
		clientEventID = "019a0000-0003-7000-8000-000000000103"
		articleID     = "33333333-3333-3333-3333-333333333333"
	)

	assertAppendReadingEventContract(t, readingEventContractSpec{
		providerState: "sovereign accepts reading.session_ended.v1 events",
		description:   "an AppendKnowledgeEvent request for reading.session_ended.v1",
		eventID:       "019a0000-0003-7000-8000-000000000003",
		clientEventID: clientEventID,
		eventType:     "reading.session_ended.v1",
		aggregateID:   "article:" + articleID,
		payload: map[string]any{
			"client_event_id":   clientEventID,
			"article_id":        articleID,
			"surface":           "article_page",
			"active_dwell_ms":   42000,
			"max_scroll_ratio":  0.95,
			"body_length_chars": 3200,
			"reached_end":       true,
			"received_at":       "2026-11-18T09:00:01Z",
		},
	})
}
