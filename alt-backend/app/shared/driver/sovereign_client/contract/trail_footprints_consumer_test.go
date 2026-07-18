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
				// contactCount / firstOccurredAt carry the D24 collapse: repeated
				// contacts with one article arrive as one footprint with a count,
				// never one row per day. A provider-side drop silently regresses
				// the spine to the duplicate display, so both are pinned.
				"footprints": matchers.EachLike(matchers.MapMatcher{
					"footprintKey":    matchers.Like("open:article:1"),
					"verb":            matchers.Like("read"),
					"itemKey":         matchers.Like("article:1"),
					"occurredAt":      matchers.Like("2026-06-10T09:12:00Z"),
					"contactCount":    matchers.Like(2),
					"firstOccurredAt": matchers.Like("2026-06-01T08:00:00Z"),
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
				// episodes are the spine's default display unit (D24/D30, Wave 8):
				// footprints folded by same-article identity and cleaned-tag
				// chaining. A provider-side drop of episode_key/wear/footprints
				// would silently regress the FE back to the legacy flat spine,
				// so pin all three.
				"episodes": matchers.EachLike(matchers.MapMatcher{
					"episodeKey": matchers.Like("ep:open:article:1"),
					"wear":       matchers.Like("worn"),
					"footprints": matchers.EachLike(matchers.MapMatcher{
						"footprintKey": matchers.Like("open:article:1"),
						"verb":         matchers.Like("read"),
						"itemKey":      matchers.Like("article:1"),
						"occurredAt":   matchers.Like("2026-06-10T09:12:00Z"),
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
			assert.GreaterOrEqual(t, resp.Msg.Footprints[0].ContactCount, int32(1),
				"footprint.contact_count must be present (collapsed contacts, D24)")
			assert.NotNil(t, resp.Msg.Footprints[0].FirstOccurredAt,
				"footprint.first_occurred_at must be present")
			require.NotEmpty(t, resp.Msg.Branches, "provider must return the open branches")
			b := resp.Msg.Branches[0]
			assert.NotEmpty(t, b.RelationKind, "branch.relation_kind must be present")
			assert.NotEmpty(t, b.Why, "branch.why must be present")
			assert.NotEmpty(t, b.Confidence, "branch.confidence must be present")
			assert.NotEmpty(t, b.EvidenceRefs, "branch.evidence_refs must be present")
			require.NotEmpty(t, resp.Msg.Episodes, "provider must return the derived episode spine (D24/D30, Wave 8)")
			ep := resp.Msg.Episodes[0]
			assert.NotEmpty(t, ep.EpisodeKey, "episode.episode_key must be present")
			assert.NotEmpty(t, ep.Wear, "episode.wear must be present")
			require.NotEmpty(t, ep.Footprints, "episode.footprints must be present")
			assert.NotEmpty(t, ep.Footprints[0].Verb, "episode footprint.verb must be present")
			return nil
		})
	require.NoError(t, err)
}

// TestGetTrailFootprintsNarrowedToMatchingItems pins the trail search
// narrowing contract (Wave 9, D25): alt-backend's search_trail_usecase calls
// GetTrailFootprints with filter_item_keys set (no cursor, a large limit) to
// narrow the spine to episodes containing a search hit. A provider-side drop
// of filter_item_keys support would silently widen every search result back
// to the full spine, so this consumer pact pins both the request shape and
// that the response narrows accordingly.
func TestGetTrailFootprintsNarrowedToMatchingItems(t *testing.T) {
	mockProvider := newSovereignPact(t)

	const userID = "22222222-2222-2222-2222-222222222222"

	err := mockProvider.
		AddInteraction().
		Given("a user with footprints across two articles, one matching the search filter").
		UponReceiving("a GetTrailFootprints request narrowed to matching items").
		WithCompleteRequest(consumer.Request{
			Method: "POST",
			Path:   matchers.String("/services.sovereign.v1.KnowledgeSovereignService/GetTrailFootprints"),
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				"userId":         matchers.Like(userID),
				"limit":          matchers.Like(500),
				"filterItemKeys": matchers.EachLike("article:1", 1),
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status: 200,
			Headers: matchers.MapMatcher{
				"Content-Type": matchers.String("application/json"),
			},
			Body: matchers.MapMatcher{
				// Only the episode containing the matching item comes back —
				// the non-matching article:2 episode must not leak through.
				"episodes": matchers.EachLike(matchers.MapMatcher{
					"episodeKey": matchers.Like("ep:open:article:1"),
					"wear":       matchers.Like("worn"),
					"footprints": matchers.EachLike(matchers.MapMatcher{
						"footprintKey": matchers.Like("open:article:1"),
						"verb":         matchers.Like("read"),
						"itemKey":      matchers.Like("article:1"),
						"occurredAt":   matchers.Like("2026-06-10T09:12:00Z"),
					}, 1),
				}, 1),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			client := newSovereignClient(config)
			resp, err := client.GetTrailFootprints(context.Background(), connect.NewRequest(&sovereignv1.GetTrailFootprintsRequest{
				UserId:         userID,
				Limit:          500,
				FilterItemKeys: []string{"article:1"},
			}))
			if err != nil {
				return fmt.Errorf("GetTrailFootprints failed: %w", err)
			}
			require.NotEmpty(t, resp.Msg.Episodes, "provider must return episodes narrowed to filter_item_keys")
			ep := resp.Msg.Episodes[0]
			assert.NotEmpty(t, ep.EpisodeKey)
			require.NotEmpty(t, ep.Footprints)
			assert.Equal(t, "article:1", ep.Footprints[0].ItemKey, "the narrowed response must contain the matching item")
			return nil
		})
	require.NoError(t, err)
}
