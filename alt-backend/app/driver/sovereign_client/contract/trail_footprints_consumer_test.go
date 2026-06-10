//go:build contract

package contract

import (
	"context"
	"fmt"
	"testing"

	"connectrpc.com/connect"
	"github.com/pact-foundation/pact-go/v2/consumer"
	"github.com/pact-foundation/pact-go/v2/matchers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sovereignv1 "alt/gen/proto/services/sovereign/v1"
)

// TestGetTrailFootprintsReturnsSpine pins the Knowledge Trail read contract:
// sovereign returns footprints carrying verb / item_key / occurred_at. A
// provider-side drop of `verb` or `occurredAt` would empty the spine, so this
// consumer pact forces the provider to keep emitting them.
func TestGetTrailFootprintsReturnsSpine(t *testing.T) {
	mockProvider := newSovereignPact(t)

	const (
		userID = "22222222-2222-2222-2222-222222222222"
	)

	err := mockProvider.
		AddInteraction().
		Given("a user with at least one footprint exists").
		UponReceiving("a GetTrailFootprints request for the user").
		WithCompleteRequest(consumer.Request{
			Method: "POST",
			Path:   matchers.String("/services.sovereign.v1.KnowledgeSovereignService/GetTrailFootprints"),
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"userId": matchers.Like(userID),
				"limit":  matchers.Like(20),
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status: 200,
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"footprints": matchers.EachLike(matchers.MapMatcher{
					"footprintKey": matchers.Like("open:article:1"),
					"verb":         matchers.Like("read"),
					"itemKey":      matchers.Like("article:1"),
					"occurredAt":   matchers.Like("2026-06-10T09:12:00Z"),
				}, 1),
				// The branch four-tuple is the contract: a provider-side drop of
				// relation_kind / why / evidence_refs / confidence empties the
				// branch surface, so pin all four.
				"branches": matchers.EachLike(matchers.MapMatcher{
					"branchKey":    matchers.Like("cluster:u:article:z"),
					"relationKind": matchers.Like("cluster"),
					"why":          matchers.Like("Joins a topic you follow."),
					"confidence":   matchers.Like("plausible"),
					"evidenceRefs": matchers.EachLike(matchers.MapMatcher{
						"refId": matchers.Like("rust"),
						"kind":  matchers.Like("tag"),
					}, 1),
				}, 1),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			client := newSovereignClient(config)
			resp, err := client.GetTrailFootprints(context.Background(), connect.NewRequest(&sovereignv1.GetTrailFootprintsRequest{
				UserId: userID,
				Limit:  20,
			}))
			if err != nil {
				return fmt.Errorf("GetTrailFootprints failed: %w", err)
			}
			require.NotEmpty(t, resp.Msg.Footprints, "provider must return at least one footprint")
			assert.NotEmpty(t, resp.Msg.Footprints[0].Verb, "footprint.verb must be present")
			assert.NotNil(t, resp.Msg.Footprints[0].OccurredAt, "footprint.occurred_at must be present")
			require.NotEmpty(t, resp.Msg.Branches, "provider must return the open branches")
			b := resp.Msg.Branches[0]
			assert.NotEmpty(t, b.RelationKind, "branch.relation_kind must be present")
			assert.NotEmpty(t, b.Why, "branch.why must be present")
			assert.NotEmpty(t, b.Confidence, "branch.confidence must be present")
			assert.NotEmpty(t, b.EvidenceRefs, "branch.evidence_refs must be present")
			return nil
		})
	require.NoError(t, err)
}
