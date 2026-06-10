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

// TestAppendKnowledgeEvent_TagSetVersionCreatedCarriesTags pins the
// TagSetVersionCreated wire payload INCLUDING the `tags` array. The
// knowledge-sovereign evidence accumulator reads the article's tag names from
// this event via readPayloadStringSlice(raw, "tags", "article_tags") to derive
// Cluster relations. Without the `tags` key the accumulator silently produces
// zero Cluster relations (the PM-2026-045 / ADR-000928 silent-fallback failure
// mode), so this CDC RED forces the producer to ship the names.
//
// The `tags` key carries the version's canonical tag names as a JSON array of
// strings. Reproject-safe: the names come from the version being created.
func TestAppendKnowledgeEvent_TagSetVersionCreatedCarriesTags(t *testing.T) {
	mockProvider, err := consumer.NewV3Pact(consumer.MockHTTPProviderConfig{
		Consumer: "alt-backend",
		Provider: "knowledge-sovereign",
		PactDir:  filepath.Join(pactDir),
	})
	require.NoError(t, err)

	eventID := uuid.New()
	tagSetVersionID := uuid.New()
	articleID := uuid.New()
	tenantID := uuid.New()
	userID := uuid.New()
	occurredAt := time.Date(2026, 6, 10, 10, 0, 0, 0, time.UTC)

	payload, _ := json.Marshal(map[string]any{
		"tag_set_version_id": tagSetVersionID.String(),
		"article_id":         articleID.String(),
		"generator":          "tag-generator",
		"tags":               []string{"go", "rust"},
	})
	encoded := base64.StdEncoding.EncodeToString(payload)

	err = mockProvider.
		AddInteraction().
		Given("sovereign accepts TagSetVersionCreated events carrying the version's tag names").
		UponReceiving("an AppendKnowledgeEvent request for TagSetVersionCreated with a tags array").
		WithCompleteRequest(consumer.Request{
			Method: "POST",
			Path:   matchers.String("/services.sovereign.v1.KnowledgeSovereignService/AppendKnowledgeEvent"),
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: loopEventBody(map[string]any{
				"eventId":       eventID.String(),
				"occurredAt":    "2026-06-10T10:00:00Z",
				"tenantId":      tenantID.String(),
				"userId":        userID.String(),
				"actorType":     "service",
				"actorId":       "tag-generator",
				"eventType":     "TagSetVersionCreated",
				"aggregateType": "article",
				"aggregateId":   articleID.String(),
				"dedupeKey":     "TagSetVersionCreated:" + tagSetVersionID.String(),
				// payload ships as base64 on the wire (bytes field). The pinned
				// blob decodes to {"...","tags":["go","rust"]}.
				"payload": encoded,
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
					ActorType:     "service",
					ActorId:       "tag-generator",
					EventType:     "TagSetVersionCreated",
					AggregateType: "article",
					AggregateId:   articleID.String(),
					DedupeKey:     "TagSetVersionCreated:" + tagSetVersionID.String(),
					Payload:       payload,
				},
			}))
			return err
		})
	assert.NoError(t, err)
}
