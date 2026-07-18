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

// TestGetTrailBranchesForAnchorReturnsAnchoredBranches pins the Wave 10 (D26)
// patch-exit read contract: alt-backend requests branches anchored on one
// item (the article the user just finished reading), and sovereign narrows
// its response to that anchor. The branch four-tuple (relation_kind / why /
// evidence_refs / confidence) is the same untyped-branch guard as
// GetTrailFootprints' branch surface, so it is pinned again here — a
// provider-side drop would silently regress the patch-exit surface to an
// empty inbox.
func TestGetTrailBranchesForAnchorReturnsAnchoredBranches(t *testing.T) {
	mockProvider := newSovereignPact(t)

	const (
		userID        = "22222222-2222-2222-2222-222222222222"
		anchorItemKey = "article:1"
	)

	err := mockProvider.
		AddInteraction().
		Given("a user has an open branch anchored on the just-read item").
		UponReceiving("a GetTrailBranchesForAnchor request").
		WithCompleteRequest(consumer.Request{
			Method: "POST",
			Path:   matchers.String("/services.sovereign.v1.KnowledgeSovereignService/GetTrailBranchesForAnchor"),
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"userId":        matchers.Like(userID),
				"anchorItemKey": matchers.Like(anchorItemKey),
				"limit":         matchers.Like(2),
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status: 200,
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"branches": matchers.EachLike(matchers.MapMatcher{
					"branchKey":     matchers.Like("cluster:u:article:z"),
					"anchorItemKey": matchers.Like(anchorItemKey),
					"relationKind":  matchers.Like("cluster"),
					"why":           matchers.Like(`Because you read "US military courts in the UK" — joins rust`),
					"confidence":    matchers.Like("plausible"),
					"targetItemKey": matchers.Like("article:z"),
					"targetTitle":   matchers.Like("Async Rust"),
					"evidenceRefs": matchers.EachLike(matchers.MapMatcher{
						"refId": matchers.Like("rust"),
						"kind":  matchers.Like("tag"),
					}, 1),
				}, 1),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			client := newSovereignClient(config)
			resp, err := client.GetTrailBranchesForAnchor(context.Background(), connect.NewRequest(&sovereignv1.GetTrailBranchesForAnchorRequest{
				UserId:        userID,
				AnchorItemKey: anchorItemKey,
				Limit:         2,
			}))
			if err != nil {
				return fmt.Errorf("GetTrailBranchesForAnchor failed: %w", err)
			}
			require.NotEmpty(t, resp.Msg.Branches, "provider must return the anchored branch")
			b := resp.Msg.Branches[0]
			assert.Equal(t, anchorItemKey, b.AnchorItemKey, "the branch must be scoped to the requested anchor")
			assert.NotEmpty(t, b.RelationKind, "branch.relation_kind must be present")
			assert.NotEmpty(t, b.Why, "branch.why must be present")
			assert.NotEmpty(t, b.Confidence, "branch.confidence must be present")
			assert.NotEmpty(t, b.EvidenceRefs, "branch.evidence_refs must be present")
			return nil
		})
	require.NoError(t, err)
}
