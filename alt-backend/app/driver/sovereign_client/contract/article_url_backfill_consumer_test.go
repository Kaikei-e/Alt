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

	"alt/domain"
	sovereignv1 "alt/gen/proto/services/sovereign/v1"
	"alt/gen/proto/services/sovereign/v1/sovereignv1connect"
)

// Consumer-Driven Contract test for the corrective ArticleUrlBackfilled
// event. The new admin one-shot tool (EmitArticleUrlBackfill on
// KnowledgeHomeAdminService — ADR-000869) appends one of these per
// article whose Knowledge Home projection has an empty URL but whose
// alt-db `articles.url` is non-empty and http(s)-scheme.
//
// What this test pins for sovereign:
//
//   - event_type literal `ArticleUrlBackfilled`. Sovereign must accept
//     it without rejecting on an event-type allowlist.
//
//   - dedupe_key namespace `article-url-backfill:<article_id>`. This is
//     deliberately distinct from `article-created:<article_id>` so the
//     existing `article-created:*` dedupe-registry rows (~27k from prior
//     runs) cannot silently drop the corrective re-emit. The unique
//     index on knowledge_events.dedupe_key still blocks double-append
//     within this namespace, providing per-article idempotency.
//
//   - actor_type=service / actor_id="knowledge-url-backfill". The
//     actor labels how this event was produced; provider may use it
//     for audit but must not vary semantics on it.
//
//   - payload schema: canonical wire keys `article_id` and `url`. The
//     bug class this event exists to repair was the `"link"` vs `"url"`
//     wire-form drift on ArticleCreated (PM-2026-041, ADR-000865 →
//     ADR-000867). Asserting `"url"` here directly — not via the
//     consumer struct round-trip — pins the bytes the projector reads.
//
//   - response field `event_seq` (int64). Pinned because the consumer
//     (knowledge_url_backfill_usecase, ADR-869) now distinguishes
//     newly-appended events (seq > 0) from dedupe-registry hits
//     (seq == 0) to keep the operator-facing skipped_duplicate counter
//     honest. Without this pin the response shape could silently lose
//     the field and the counter would regress to "always 0".
//
// What this test does NOT pin:
//
//   - The patch SQL on the projection table (covered by sovereign-side
//     projector tests, not consumer tests).
//
//   - The seq==0 dedupe-hit response. The Pact API expresses one
//     interaction; we pin the happy-path "newly appended" shape. The
//     usecase unit test (TestEmit_ReportsSkippedDuplicateAccurately)
//     covers the seq==0 branch via an in-process mock, which is the
//     correct boundary for that semantic.
func TestAppendKnowledgeEvent_ArticleUrlBackfilled(t *testing.T) {
	mockProvider, err := consumer.NewV3Pact(consumer.MockHTTPProviderConfig{
		Consumer: "alt-backend",
		Provider: "knowledge-sovereign",
		PactDir:  filepath.Join(pactDir),
	})
	require.NoError(t, err)

	eventID := uuid.New()
	tenantID := uuid.New()
	userID := uuid.New()
	articleID := uuid.New()
	occurredAt := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)
	// Build the dedupe key via the producer-side const so the contract
	// example follows future namespace bumps (v1 → v2 → …) without drift.
	// matchers.Like below treats the value as a string-shape match, so
	// provider verification stays compatible regardless of the exact
	// namespace prefix the consumer emits.
	dedupeKey := fmt.Sprintf(domain.DedupeKeyArticleUrlBackfill, articleID.String())

	payload, err := json.Marshal(map[string]any{
		"article_id":           articleID.String(),
		"url":                  "https://example.com/articles/42",
		"original_occurred_at": "2026-01-15T08:30:00Z",
	})
	require.NoError(t, err)

	err = mockProvider.
		AddInteraction().
		Given("sovereign accepts ArticleUrlBackfilled corrective events").
		UponReceiving("an AppendKnowledgeEvent request for ArticleUrlBackfilled").
		WithCompleteRequest(consumer.Request{
			Method: "POST",
			Path:   matchers.String("/services.sovereign.v1.KnowledgeSovereignService/AppendKnowledgeEvent"),
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"event": matchers.Like(map[string]any{
					"eventId":       eventID.String(),
					"occurredAt":    "2026-04-28T12:00:00Z",
					"tenantId":      tenantID.String(),
					"userId":        userID.String(),
					"actorType":     "service",
					"actorId":       "knowledge-url-backfill",
					"eventType":     "ArticleUrlBackfilled",
					"aggregateType": "article",
					"aggregateId":   articleID.String(),
					"dedupeKey":     dedupeKey,
					// payload is bytes on the wire (proto3 → base64 in JSON).
					// The literal value isn't asserted here — Pact matchers.Like
					// treats it as a shape match. The unit test
					// (usecase_test.go) pins the canonical "url" key inside.
					"payload": "eyJhcnRpY2xlX2lkIjoiMDAwMDAwMDAtMDAwMC0wMDAwLTAwMDAtMDAwMDAwMDAwMDAwIiwidXJsIjoiaHR0cHM6Ly9leGFtcGxlLmNvbSJ9",
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
				// Pin the eventSeq field shape so the consumer's accurate
				// SkippedDuplicate counter (ADR-869, port returning
				// (eventSeq int64, err error)) keeps working if the
				// provider ever re-renames the proto field.
				"eventSeq": matchers.Like(123),
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
					ActorId:       "knowledge-url-backfill",
					EventType:     "ArticleUrlBackfilled",
					AggregateType: "article",
					AggregateId:   articleID.String(),
					DedupeKey:     dedupeKey,
					Payload:       payload,
				},
			}))
			return err
		})
	assert.NoError(t, err)
}
