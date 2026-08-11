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
	"github.com/pact-foundation/pact-go/v2/consumer"
	"github.com/pact-foundation/pact-go/v2/matchers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"alt/domain"
	sovereignv1 "alt/gen/proto/services/sovereign/v1"
	"alt/gen/proto/services/sovereign/v1/sovereignv1connect"
)

// TestAppendKnowledgeEvent_ArticleCreatedCarriesTitleAndURL pins the
// outbox-worker → knowledge-sovereign ArticleCreated wire payload that
// Knowledge Home / Trail title enrichment depends on.
//
// When ARTICLE_UPSERT succeeds at RAG but ArticleCreated is never
// appended (ACK-before-sovereign), SummaryVersionCreated alone creates
// Home rows with blank title/url and Trail falls back to article:<uuid>.
// This CDC RED forces the producer to ship the canonical title + url
// keys (PM-2026-041 / ADR-000865 → ADR-000867) under dedupe namespace
// article-created:<article_id>, with actor_id=outbox-worker matching
// emitArticleCreatedEvent in outbox_worker.go.
//
// Payload is built as a raw map — not via domain.ArticleCreatedPayload
// round-trip — so a struct-tag regression that reintroduces the legacy
// "link" key still turns this contract red (same pattern as
// article_url_backfill_consumer_test.go).
func TestAppendKnowledgeEvent_ArticleCreatedCarriesTitleAndURL(t *testing.T) {
	mockProvider, err := consumer.NewV3Pact(consumer.MockHTTPProviderConfig{
		Consumer: "alt-backend",
		Provider: "knowledge-sovereign",
		PactDir:  filepath.Join(pactDir),
	})
	require.NoError(t, err)

	// Fixed literal values, not uuid.New(): matchers.Like records the given
	// example verbatim in the generated pact, so random IDs would make the
	// pact content non-deterministic across CI runs for the same commit —
	// the Pact Broker rejects republishing different content under the same
	// consumer version (see https://docs.pact.io/go/versioning).
	const (
		eventID   = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
		tenantID  = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
		userID    = "cccccccc-cccc-cccc-cccc-cccccccccccc"
		articleID = "dddddddd-dddd-dddd-dddd-dddddddddddd"
	)
	occurredAt := time.Date(2026, 8, 9, 10, 0, 0, 0, time.UTC)
	dedupeKey := fmt.Sprintf(domain.DedupeKeyArticleCreated, articleID)

	payload, err := json.Marshal(map[string]any{
		"article_id":   articleID,
		"title":        "Trail must not fall back to article UUID",
		"published_at": "2026-08-09T10:00:00Z",
		"tenant_id":    tenantID,
		"url":          "https://example.com/articles/trail-title",
	})
	require.NoError(t, err)
	encoded := base64.StdEncoding.EncodeToString(payload)

	err = mockProvider.
		AddInteraction().
		Given("sovereign accepts ArticleCreated events carrying title and url").
		UponReceiving("an AppendKnowledgeEvent request for ArticleCreated with title and url").
		WithCompleteRequest(consumer.Request{
			Method: "POST",
			Path:   matchers.String("/services.sovereign.v1.KnowledgeSovereignService/AppendKnowledgeEvent"),
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"event": matchers.Like(map[string]any{
					"eventId":       eventID,
					"occurredAt":    "2026-08-09T10:00:00Z",
					"tenantId":      tenantID,
					"userId":        userID,
					"actorType":     "service",
					"actorId":       "outbox-worker",
					"eventType":     "ArticleCreated",
					"aggregateType": "article",
					"aggregateId":   articleID,
					"dedupeKey":     dedupeKey,
					// payload ships as base64 on the wire (bytes field). The pinned
					// blob decodes to {"article_id":"...","title":"...","url":"..."}.
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
				// AppendKnowledgeEventResponse declares only `int64 event_seq`.
				// protojson renders int64 as a JSON string, so the value is "123", not 123.
				"eventSeq": matchers.Like("123"),
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
					ActorType:     "service",
					ActorId:       "outbox-worker",
					EventType:     "ArticleCreated",
					AggregateType: "article",
					AggregateId:   articleID,
					DedupeKey:     dedupeKey,
					Payload:       payload,
				},
			}))
			return err
		})
	assert.NoError(t, err)
}
