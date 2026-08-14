//go:build contract

package contract

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	"connectrpc.com/connect"
	"github.com/pact-foundation/pact-go/v2/consumer"
	"github.com/pact-foundation/pact-go/v2/matchers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"alt/domain"
	datahubv1 "alt/gen/proto/services/datahub/v1"
	sovereignv1 "alt/gen/proto/services/sovereign/v1"
	"alt/gen/proto/services/sovereign/v1/sovereignv1connect"
)

// This file pins the *second* ArticleCreated producer: the
// services.datahub.v1.DataHubService/CreateArticle procedure pre-processor
// calls for every ingested article. The first one — the harvester's outbox
// worker — is pinned by article_created_consumer_test.go in this package.
//
// It drives the real handler rather than hand-building the request, because
// the fact under contract is not a field but a call: alt-data-hub appends
// ArticleCreated for an article whose row already existed. Only the producer
// itself can demonstrate that, and hand-building the request would pin a
// message the handler is free to never send. This is the same shape as the
// datahub_gateway consumer pacts, which drive real gateways.

// The handler under contract, the article writer standing in for alt-db and
// the two unused constructor arguments live in create_article_handler_test.go,
// which carries no build tag: the wiring they encode has to be checked where
// libpact_ffi is not installed too.

// connectAppender is the handler's knowledge event port, speaking to the Pact
// mock server over the generated Connect client.
//
// It is not sovereign_client.Client because that constructor runs a startup
// health probe (GetActiveProjectionVersion) as soon as it is enabled, and the
// mock server would record that probe as an unexpected request and fail the
// interaction. The mapping below is the same one read_client.go performs; what
// this test pins is what the handler put into the domain event, and Payload
// travels through untouched.
type connectAppender struct {
	client sovereignv1connect.KnowledgeSovereignServiceClient
	last   domain.KnowledgeEvent
}

func (a *connectAppender) AppendKnowledgeEvent(ctx context.Context, event domain.KnowledgeEvent) (int64, error) {
	a.last = event

	var userID string
	if event.UserID != nil {
		userID = event.UserID.String()
	}
	resp, err := a.client.AppendKnowledgeEvent(ctx, connect.NewRequest(&sovereignv1.AppendKnowledgeEventRequest{
		Event: &sovereignv1.KnowledgeEvent{
			EventId:       event.EventID.String(),
			OccurredAt:    timestamppb.New(event.OccurredAt),
			TenantId:      event.TenantID.String(),
			UserId:        userID,
			ActorType:     event.ActorType,
			ActorId:       event.ActorID,
			EventType:     event.EventType,
			AggregateType: event.AggregateType,
			AggregateId:   event.AggregateID,
			DedupeKey:     event.DedupeKey,
			Payload:       event.Payload,
		},
	}))
	if err != nil {
		return 0, err
	}
	return resp.Msg.GetEventSeq(), nil
}

// TestCreateArticleAppendsArticleCreatedForAnExistingArticle pins that
// CreateArticle appends ArticleCreated on the upsert branch too.
//
// `created == false` does not mean sovereign has already seen the article. It
// means alt-db already had the (url, user_id) row — which is exactly the state
// every retry of a failed CreateArticle arrives in, and the state pre-processor
// re-sends an unchanged article in on every crawl. Skipping the append there
// makes the miss permanent: no later call ever reports created=true again, so
// the Home row keeps whatever SummaryVersionCreated gave it, which is a blank
// title and no url. The append is idempotent on the provider side via
// dedupeKey, so the repeat costs a dedupe lookup and repairs the gap.
func TestCreateArticleAppendsArticleCreatedForAnExistingArticle(t *testing.T) {
	mockProvider, err := consumer.NewV3Pact(consumer.MockHTTPProviderConfig{
		Consumer: "alt-backend",
		Provider: "knowledge-sovereign",
		PactDir:  filepath.Join(pactDir),
	})
	require.NoError(t, err)

	// Fixed literals for the same reason article_created_consumer_test.go uses
	// them: matchers.Like records the example verbatim, and a pact whose
	// content changes between runs of the same commit cannot be republished.
	const (
		exampleEventID = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
		tenantID       = "eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee"
		articleID      = "ffffffff-ffff-ffff-ffff-ffffffffffff"
		articleTitle   = "Knowledge Home must not show a blank title"
		articleURL     = "https://example.com/articles/blank-title"
	)
	dedupeKey := fmt.Sprintf(domain.DedupeKeyArticleCreated, articleID)

	examplePayload, err := json.Marshal(map[string]any{
		"article_id":   articleID,
		"title":        articleTitle,
		"published_at": "2026-08-11T00:00:00Z",
		"tenant_id":    tenantID,
		"url":          articleURL,
	})
	require.NoError(t, err)

	appender := &connectAppender{}

	err = mockProvider.
		AddInteraction().
		Given("sovereign accepts ArticleCreated events carrying title and url").
		UponReceiving("an AppendKnowledgeEvent request for ArticleCreated from the CreateArticle procedure").
		WithCompleteRequest(consumer.Request{
			Method: "POST",
			Path:   matchers.String("/services.sovereign.v1.KnowledgeSovereignService/AppendKnowledgeEvent"),
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"event": matchers.Like(map[string]any{
					"eventId":    exampleEventID,
					"occurredAt": "2026-08-11T00:00:00Z",
					"tenantId":   tenantID,
					"userId":     tenantID,
					"actorType":  "service",
					// The data plane attributes the append to the caller whose
					// ingest produced the article, which is what distinguishes
					// this producer from the outbox worker's.
					"actorId":       "pre-processor",
					"eventType":     "ArticleCreated",
					"aggregateType": "article",
					"aggregateId":   articleID,
					"dedupeKey":     dedupeKey,
					"payload":       base64.StdEncoding.EncodeToString(examplePayload),
				}),
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status: 200,
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				// A dedupe hit answers 0 and is not an error — see
				// knowledge_event_port.AppendKnowledgeEventPort. protojson
				// renders int64 as a JSON string.
				"eventSeq": matchers.Like("0"),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			appender.client = newSovereignClient(config)

			h := newCreateArticleHandler(appender,
				articleUpsertWriter{articleID: articleID, created: false})

			_, err := h.CreateArticle(context.Background(), connect.NewRequest(&datahubv1.CreateArticleRequest{
				Title:  articleTitle,
				Url:    articleURL,
				FeedId: "33333333-4444-5555-6666-777777777777",
				UserId: tenantID,
			}))
			return err
		})
	assert.NoError(t, err)

	// The pact body matcher above is a type matcher, so it cannot catch a
	// payload whose keys regressed to the legacy "link" (PM-2026-041). Assert
	// the canonical keys on what the producer actually built.
	var payload map[string]any
	require.NoError(t, json.Unmarshal(appender.last.Payload, &payload))
	assert.Equal(t, articleURL, payload["url"], "canonical url key")
	assert.Equal(t, articleTitle, payload["title"])
	assert.Equal(t, articleID, payload["article_id"])
	assert.Equal(t, tenantID, payload["tenant_id"])
	assert.Equal(t, dedupeKey, appender.last.DedupeKey)
}
