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

const pactDir = "../../../../../pacts"

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
		// ApplyProjectionMutationResponse declares success / error_message, but
		// the handler never populates them on failure: a repository error
		// becomes connect.NewError(CodeInternal), which is HTTP 500 with the
		// Connect error envelope. The previous expectation (200 + success=false)
		// described a soft-failure path the provider does not have, and the
		// hand-written stub was the only thing that ever produced it.
		WithCompleteResponse(consumer.Response{
			Status: 500,
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"code":    matchers.String("internal"),
				"message": matchers.Like("projection version mismatch"),
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
			if err == nil {
				return fmt.Errorf("ApplyProjectionMutation succeeded (%v); a rejected mutation must surface as a Connect error", resp)
			}
			assert.Equal(t, connect.CodeInternal, connect.CodeOf(err))
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

// dismiss_curation is the only mutation_type ApplyCurationMutation dispatches;
// every other value returns InvalidArgument. This interaction previously said
// create_lens, which the provider has never accepted — lens creation has its
// own CreateLens RPC. Note that no production code calls ApplyCurationMutation
// today; this pins the client method's wire shape, not a live call site.
func TestApplyCurationMutationDismissCuration(t *testing.T) {
	mockProvider := newSovereignPact(t)

	err := mockProvider.
		AddInteraction().
		Given("the curation mutation dismiss_curation is accepted").
		UponReceiving("an ApplyCurationMutation request of kind dismiss_curation").
		WithCompleteRequest(consumer.Request{
			Method: "POST",
			Path:   matchers.String("/services.sovereign.v1.KnowledgeSovereignService/ApplyCurationMutation"),
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"mutationType":   matchers.String("dismiss_curation"),
				"entityId":       matchers.Like("user-1:article-2"),
				"payload":        matchers.Like("eyJsZW5zX2lkIjoibGVucy0xIn0="),
				"idempotencyKey": matchers.Like("dismiss_curation:user-1:article-2"),
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
				MutationType:   "dismiss_curation",
				EntityId:       "user-1:article-2",
				Payload:        []byte(`{"lens_id":"lens-1"}`),
				IdempotencyKey: "dismiss_curation:user-1:article-2",
			}))
			if err != nil {
				return fmt.Errorf("ApplyCurationMutation failed: %w", err)
			}
			assert.True(t, resp.Msg.Success)
			return nil
		})
	require.NoError(t, err)
}
