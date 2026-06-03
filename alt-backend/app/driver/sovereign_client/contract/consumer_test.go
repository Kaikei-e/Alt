//go:build contract

// Package contract contains Consumer-Driven Contract tests for
// alt-backend → knowledge-sovereign.
//
// alt-backend's knowledge projector writes every mutation through the
// sovereign_client Connect-RPC driver. These tests pin the wire format
// of the three mutation dispatch RPCs the projector depends on
// (ApplyProjectionMutation / ApplyRecallMutation / ApplyCurationMutation)
// so that knowledge-sovereign cannot regress the contract without
// failing provider verification.
//
// Invariants under test:
//   - Append-first event log — mutation_type enum stays stable; consumer
//     pins the literal string values from knowledge_sovereign_port.
//   - Single emission — idempotency_key is forwarded verbatim; provider
//     may echo success=false on dedupe rejection with a non-empty
//     errorMessage, which the consumer surfaces as an error.
package contract

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"testing"

	"connectrpc.com/connect"
	"github.com/pact-foundation/pact-go/v2/consumer"
	"github.com/pact-foundation/pact-go/v2/matchers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sovereignv1 "alt/gen/proto/services/sovereign/v1"
	"alt/gen/proto/services/sovereign/v1/sovereignv1connect"
)

const pactDir = "../../../../pacts"

func newSovereignPact(t *testing.T) *consumer.V3HTTPMockProvider {
	t.Helper()
	mockProvider, err := consumer.NewV3Pact(consumer.MockHTTPProviderConfig{
		Consumer: "alt-backend",
		Provider: "knowledge-sovereign",
		PactDir:  filepath.Join(pactDir),
	})
	require.NoError(t, err)
	return mockProvider
}

func newSovereignClient(config consumer.MockServerConfig) sovereignv1connect.KnowledgeSovereignServiceClient {
	return sovereignv1connect.NewKnowledgeSovereignServiceClient(
		http.DefaultClient,
		fmt.Sprintf("http://%s:%d", config.Host, config.Port),
		connect.WithProtoJSON(),
	)
}

func TestApplyProjectionMutationUpsertHomeItem(t *testing.T) {
	mockProvider := newSovereignPact(t)

	err := mockProvider.
		AddInteraction().
		Given("the projection mutation upsert_home_item is accepted").
		UponReceiving("an ApplyProjectionMutation request of kind upsert_home_item").
		WithCompleteRequest(consumer.Request{
			Method: "POST",
			Path:   matchers.String("/services.sovereign.v1.KnowledgeSovereignService/ApplyProjectionMutation"),
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"mutationType":   matchers.String("upsert_home_item"),
				"entityId":       matchers.Like("user-1:article-1"),
				"payload":        matchers.Like("eyJ1c2VyX2lkIjoidXNlci0xIn0="),
				"idempotencyKey": matchers.Like("upsert_home_item:user-1:article-1"),
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
			client := newSovereignClient(config)
			resp, err := client.ApplyProjectionMutation(context.Background(), connect.NewRequest(&sovereignv1.ApplyProjectionMutationRequest{
				MutationType:   "upsert_home_item",
				EntityId:       "user-1:article-1",
				Payload:        []byte(`{"user_id":"user-1"}`),
				IdempotencyKey: "upsert_home_item:user-1:article-1",
			}))
			if err != nil {
				return fmt.Errorf("ApplyProjectionMutation failed: %w", err)
			}
			assert.True(t, resp.Msg.Success)
			return nil
		})
	require.NoError(t, err)
}

func TestApplyProjectionMutationReturnsErrorMessageOnRejection(t *testing.T) {
	mockProvider := newSovereignPact(t)

	err := mockProvider.
		AddInteraction().
		Given("the projection mutation is rejected with an error").
		UponReceiving("an ApplyProjectionMutation request that the provider refuses to apply").
		WithCompleteRequest(consumer.Request{
			Method: "POST",
			Path:   matchers.String("/services.sovereign.v1.KnowledgeSovereignService/ApplyProjectionMutation"),
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"mutationType":   matchers.String("dismiss_home_item"),
				"entityId":       matchers.Like("user-1:article-2"),
				"payload":        matchers.Like("eyJ1c2VyX2lkIjoidXNlci0xIn0="),
				"idempotencyKey": matchers.Like("dismiss_home_item:user-1:article-2"),
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status: 200,
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"success":      matchers.Like(false),
				"errorMessage": matchers.Like("projection version mismatch"),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			client := newSovereignClient(config)
			resp, err := client.ApplyProjectionMutation(context.Background(), connect.NewRequest(&sovereignv1.ApplyProjectionMutationRequest{
				MutationType:   "dismiss_home_item",
				EntityId:       "user-1:article-2",
				Payload:        []byte(`{"user_id":"user-1"}`),
				IdempotencyKey: "dismiss_home_item:user-1:article-2",
			}))
			if err != nil {
				return fmt.Errorf("ApplyProjectionMutation unexpected transport error: %w", err)
			}
			assert.False(t, resp.Msg.Success)
			assert.NotEmpty(t, resp.Msg.ErrorMessage)
			return nil
		})
	require.NoError(t, err)
}

// TestGetKnowledgeLoopEntriesReturnsRelations pins the ADR-000937 relation-set
// on the GetKnowledgeLoopEntries read path. alt-backend's sovereign_client maps
// `entries[].relations` (a JSONB-opaque bytes field) into the domain entry the
// BFF decodes into structured loopv1.Relation. This consumer pact is the CDC
// RED that forces knowledge-sovereign to keep emitting the field; without it a
// provider-side drop of `relations` would silently empty the Orient surface
// (the PM-2026-045 / ADR-000928 silent-fallback failure mode).
func TestGetKnowledgeLoopEntriesReturnsRelations(t *testing.T) {
	mockProvider := newSovereignPact(t)

	const (
		userID   = "22222222-2222-2222-2222-222222222222"
		tenantID = "11111111-1111-1111-1111-111111111111"
		// base64 of a one-element relation-set: [{"kind":"continuation","state":"open"}]
		relationsB64 = "W3sia2luZCI6ImNvbnRpbnVhdGlvbiIsInN0YXRlIjoib3BlbiJ9XQ=="
	)

	err := mockProvider.
		AddInteraction().
		Given("a knowledge loop entry with a continuation relation exists").
		UponReceiving("a GetKnowledgeLoopEntries request for the user's lens").
		WithCompleteRequest(consumer.Request{
			Method: "POST",
			Path:   matchers.String("/services.sovereign.v1.KnowledgeSovereignService/GetKnowledgeLoopEntries"),
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"tenantId":   matchers.Like(tenantID),
				"userId":     matchers.Like(userID),
				"lensModeId": matchers.Like("default"),
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status: 200,
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"entries": matchers.EachLike(matchers.MapMatcher{
					"userId":   matchers.Like(userID),
					"tenantId": matchers.Like(tenantID),
					"entryKey": matchers.Like("entry:article-1"),
					// `relations` is a proto bytes field → base64 string on the
					// wire. Its presence is the contract (ADR-000937).
					"relations": matchers.Like(relationsB64),
				}, 1),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			client := newSovereignClient(config)
			resp, err := client.GetKnowledgeLoopEntries(context.Background(), connect.NewRequest(&sovereignv1.GetKnowledgeLoopEntriesRequest{
				TenantId:   tenantID,
				UserId:     userID,
				LensModeId: "default",
			}))
			if err != nil {
				return fmt.Errorf("GetKnowledgeLoopEntries failed: %w", err)
			}
			require.NotEmpty(t, resp.Msg.Entries, "provider must return at least one entry")
			assert.NotEmpty(t, resp.Msg.Entries[0].Relations,
				"entry.relations must be present (ADR-000937 relation-set)")
			return nil
		})
	require.NoError(t, err)
}

func TestApplyRecallMutationSnoozeCandidate(t *testing.T) {
	mockProvider := newSovereignPact(t)

	err := mockProvider.
		AddInteraction().
		Given("the recall mutation snooze_candidate is accepted").
		UponReceiving("an ApplyRecallMutation request of kind snooze_candidate").
		WithCompleteRequest(consumer.Request{
			Method: "POST",
			Path:   matchers.String("/services.sovereign.v1.KnowledgeSovereignService/ApplyRecallMutation"),
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"mutationType":   matchers.String("snooze_candidate"),
				"entityId":       matchers.Like("user-1:article-3"),
				"payload":        matchers.Like("eyJ1c2VyX2lkIjoidXNlci0xIn0="),
				"idempotencyKey": matchers.Like("snooze_candidate:user-1:article-3"),
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
			client := newSovereignClient(config)
			resp, err := client.ApplyRecallMutation(context.Background(), connect.NewRequest(&sovereignv1.ApplyRecallMutationRequest{
				MutationType:   "snooze_candidate",
				EntityId:       "user-1:article-3",
				Payload:        []byte(`{"user_id":"user-1"}`),
				IdempotencyKey: "snooze_candidate:user-1:article-3",
			}))
			if err != nil {
				return fmt.Errorf("ApplyRecallMutation failed: %w", err)
			}
			assert.True(t, resp.Msg.Success)
			return nil
		})
	require.NoError(t, err)
}

func TestApplyCurationMutationCreateLens(t *testing.T) {
	mockProvider := newSovereignPact(t)

	err := mockProvider.
		AddInteraction().
		Given("the curation mutation create_lens is accepted").
		UponReceiving("an ApplyCurationMutation request of kind create_lens").
		WithCompleteRequest(consumer.Request{
			Method: "POST",
			Path:   matchers.String("/services.sovereign.v1.KnowledgeSovereignService/ApplyCurationMutation"),
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"mutationType":   matchers.String("create_lens"),
				"entityId":       matchers.Like("lens-1"),
				"payload":        matchers.Like("eyJsZW5zX2lkIjoibGVucy0xIn0="),
				"idempotencyKey": matchers.Like("create_lens:lens-1"),
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
			client := newSovereignClient(config)
			resp, err := client.ApplyCurationMutation(context.Background(), connect.NewRequest(&sovereignv1.ApplyCurationMutationRequest{
				MutationType:   "create_lens",
				EntityId:       "lens-1",
				Payload:        []byte(`{"lens_id":"lens-1"}`),
				IdempotencyKey: "create_lens:lens-1",
			}))
			if err != nil {
				return fmt.Errorf("ApplyCurationMutation failed: %w", err)
			}
			assert.True(t, resp.Msg.Success)
			return nil
		})
	require.NoError(t, err)
}
