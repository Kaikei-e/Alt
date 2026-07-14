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

// TestAppendKnowledgeEvent_SummaryVersionCreatedCarriesSummaryText pins the
// SummaryVersionCreated wire payload INCLUDING the `summary_text` and
// `quality_score` fields. The Knowledge Loop projector's reproject-safe
// enricher (knowledge-sovereign/usecase/knowledge_loop_projector/enricher.go)
// needs the summary body and its quality score to render a substantive "why"
// narrative for Knowledge Home items without a latest-state lookup — a
// provider that only ever sees summary_version_id/article_id/model can't
// reconstruct that narrative. Without `summary_text`/`quality_score` the
// projector silently falls back to the generic narrative (the PM-2026-045 /
// ADR-000928 silent-fallback failure mode), so this CDC RED forces the
// producer to ship both.
//
// The `summary_text` key carries the generated summary body verbatim.
// The `quality_score` key carries the summary's quality score as a JSON
// number (nullable upstream, but pinned here as a concrete value).
// Reproject-safe: both values come from the version being created.
func TestAppendKnowledgeEvent_SummaryVersionCreatedCarriesSummaryText(t *testing.T) {
	mockProvider, err := consumer.NewV3Pact(consumer.MockHTTPProviderConfig{
		Consumer: "alt-backend",
		Provider: "knowledge-sovereign",
		PactDir:  filepath.Join(pactDir),
	})
	require.NoError(t, err)

	eventID := uuid.New()
	summaryVersionID := uuid.New()
	articleID := uuid.New()
	tenantID := uuid.New()
	userID := uuid.New()
	occurredAt := time.Date(2026, 6, 10, 10, 0, 0, 0, time.UTC)

	payload, _ := json.Marshal(map[string]any{
		"summary_version_id": summaryVersionID.String(),
		"article_id":         articleID.String(),
		"model":              "news-creator-summarizer-v1",
		"prompt_version":     "2026-06-01",
		"article_title":      "Go 1.26 Release Notes",
		"summary_text":       "Go 1.26 ships range-over-func iterators and a faster GC.",
		"quality_score":      0.87,
	})
	encoded := base64.StdEncoding.EncodeToString(payload)

	err = mockProvider.
		AddInteraction().
		Given("sovereign accepts SummaryVersionCreated events carrying the summary text and quality score").
		UponReceiving("an AppendKnowledgeEvent request for SummaryVersionCreated with summary_text and quality_score").
		WithCompleteRequest(consumer.Request{
			Method: "POST",
			Path:   matchers.String("/services.sovereign.v1.KnowledgeSovereignService/AppendKnowledgeEvent"),
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"event": matchers.Like(map[string]any{
					"eventId":       eventID.String(),
					"occurredAt":    "2026-06-10T10:00:00Z",
					"tenantId":      tenantID.String(),
					"userId":        userID.String(),
					"actorType":     "service",
					"actorId":       "news-creator",
					"eventType":     "SummaryVersionCreated",
					"aggregateType": "article",
					"aggregateId":   articleID.String(),
					"dedupeKey":     "SummaryVersionCreated:" + summaryVersionID.String(),
					// payload ships as base64 on the wire (bytes field). The pinned
					// blob decodes to {"...","summary_text":"...","quality_score":0.87}.
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
					EventId:       eventID.String(),
					OccurredAt:    timestamppb.New(occurredAt),
					TenantId:      tenantID.String(),
					UserId:        userID.String(),
					ActorType:     "service",
					ActorId:       "news-creator",
					EventType:     "SummaryVersionCreated",
					AggregateType: "article",
					AggregateId:   articleID.String(),
					DedupeKey:     "SummaryVersionCreated:" + summaryVersionID.String(),
					Payload:       payload,
				},
			}))
			return err
		})
	assert.NoError(t, err)
}
