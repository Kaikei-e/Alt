//go:build contract

// Pact CDC: alt-backend → alt-data-hub, on-demand OG image resolution
// (feed_og_images).
//
// This capability replaces the batch og-image-backfill work list. The contract
// that matters here is not the happy path but the encoding of "already settled":
// resolution runs when a reader scrolls a card into view, so anything the
// consumer misreads as "nothing known yet" becomes another request to someone
// else's origin on every scroll.
package contract

import (
	"context"
	"fmt"
	"testing"
	"time"

	"alt/domain"
	"alt/shared/gateway/datahub_gateway"

	"github.com/pact-foundation/pact-go/v2/consumer"
	"github.com/pact-foundation/pact-go/v2/matchers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testOgFeedIDFetchable  = "c3d4e5f6-3333-4333-8333-333333333333"
	testOgFeedIDSuppressed = "d4e5f6a7-4444-4444-8444-444444444444"
	testOgPageURL          = "https://example.com/posts/on-demand-og"
	testOgImageURL         = "https://cdn.example.com/on-demand-og.png"
)

// TestGetFeedOgImageTargetsContract pins the two encodings the resolver branches
// on.
//
// A feed still worth fetching answers with page_url and neither og_image_url
// nor suppressed — and under protojson both a false bool and an empty string
// are absent keys, so the wire form of "go ahead and fetch" is a body carrying
// only feed_id and page_url. A consumer that treated those absences as unknown
// would either never fetch or fetch every time.
func TestGetFeedOgImageTargetsContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub holds one unresolved feed and one whose origin already refused").
		UponReceiving("a GetFeedOgImageTargets request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/services.datahub.v1.DataHubService/GetFeedOgImageTargets"),
			Headers: jsonHeaders(),
			Body: map[string]interface{}{
				"feedIds": []string{testOgFeedIDFetchable, testOgFeedIDSuppressed},
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body: map[string]interface{}{
				"targets": []map[string]interface{}{
					{
						"feedId":  testOgFeedIDFetchable,
						"pageUrl": testOgPageURL,
					},
					{
						"feedId":     testOgFeedIDSuppressed,
						"pageUrl":    testOgPageURL,
						"suppressed": true,
					},
				},
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewOgImageGateway(newDataHubServiceClient(config))
			targets, err := gw.FetchFeedOgImageTargets(context.Background(),
				[]string{testOgFeedIDFetchable, testOgFeedIDSuppressed})
			if err != nil {
				return fmt.Errorf("FetchFeedOgImageTargets failed: %w", err)
			}

			require.Len(t, targets, 2)

			assert.Equal(t, testOgFeedIDFetchable, targets[0].FeedID)
			assert.Equal(t, testOgPageURL, targets[0].PageURL)
			assert.Empty(t, targets[0].OgImageURL)
			assert.False(t, targets[0].Suppressed)
			assert.True(t, targets[0].NeedsFetch(),
				"a feed with no image and no standing refusal is the one case that warrants an origin request")

			assert.True(t, targets[1].Suppressed)
			assert.False(t, targets[1].NeedsFetch(),
				"a standing refusal must suppress the fetch, or every scroll past the card re-requests the origin")

			return nil
		})

	require.NoError(t, err)
}

// TestGetFeedOgImageTargetsAbsentContract pins that a feed with no row is absent
// from the response rather than present-and-blank.
//
// The distinction is the whole negative cache: absent means "never asked, go
// ahead", and the resolver must not confuse it with a row that says "asked,
// refused".
func TestGetFeedOgImageTargetsAbsentContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub holds no feed_og_images row for the requested feed").
		UponReceiving("a GetFeedOgImageTargets request from alt-backend for an unknown feed").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/services.datahub.v1.DataHubService/GetFeedOgImageTargets"),
			Headers: jsonHeaders(),
			Body: map[string]interface{}{
				"feedIds": []string{"00000000-0000-4000-8000-000000000000"},
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			// An empty repeated field is an absent key under protojson.
			Body: matchers.MapMatcher{},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewOgImageGateway(newDataHubServiceClient(config))
			targets, err := gw.FetchFeedOgImageTargets(context.Background(),
				[]string{"00000000-0000-4000-8000-000000000000"})
			if err != nil {
				return fmt.Errorf("FetchFeedOgImageTargets failed: %w", err)
			}
			assert.Empty(t, targets)
			return nil
		})

	require.NoError(t, err)
}

// TestSaveFeedOgImageResolvedContract pins the success write.
func TestSaveFeedOgImageResolvedContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub accepts feed og image resolutions").
		UponReceiving("a SaveFeedOgImage request from alt-backend carrying a resolved URL").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/services.datahub.v1.DataHubService/SaveFeedOgImage"),
			Headers: jsonHeaders(),
			Body: map[string]interface{}{
				"feedId":     testOgFeedIDFetchable,
				"ogImageUrl": testOgImageURL,
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body:    matchers.MapMatcher{},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewOgImageGateway(newDataHubServiceClient(config))
			if err := gw.SaveFeedOgImage(context.Background(), testOgFeedIDFetchable, testOgImageURL, ""); err != nil {
				return fmt.Errorf("SaveFeedOgImage failed: %w", err)
			}
			return nil
		})

	require.NoError(t, err)
}

// TestSaveFeedOgImageRefusalContract pins the refusal write, and specifically
// that retry_after_seconds is absent for a robots.txt disallow.
//
// Zero is an absent key under protojson, and here zero carries meaning: "not
// within this retention window". A provider that read the absence as "retry
// immediately" would turn a site's stated policy into a request on every
// scroll — the exact behaviour that got the batch job blocked.
func TestSaveFeedOgImageRefusalContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub accepts feed og image refusals").
		UponReceiving("a SaveFeedOgImage request from alt-backend recording a robots.txt disallow").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/services.datahub.v1.DataHubService/SaveFeedOgImage"),
			Headers: jsonHeaders(),
			Body: map[string]interface{}{
				"feedId": testOgFeedIDSuppressed,
				"reason": string(domain.OgImageRefusedByRobots),
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body:    matchers.MapMatcher{},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewOgImageGateway(newDataHubServiceClient(config))
			err := gw.SaveFeedOgImage(context.Background(), testOgFeedIDSuppressed, "", domain.OgImageRefusedByRobots)
			if err != nil {
				return fmt.Errorf("SaveFeedOgImage failed: %w", err)
			}
			return nil
		})

	require.NoError(t, err)
}

// TestPurgeExpiredFeedOgImagesContract pins the retention sweep.
//
// feed_og_images holds scraped third-party artifacts, so it is inside the same
// copyright window as article_heads and image_proxy_cache. Without this call
// the on-demand resolver would accumulate them indefinitely, which is the one
// outcome that would make resolving on demand worse than not resolving at all.
func TestPurgeExpiredFeedOgImagesContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub holds feed og images past the retention window").
		UponReceiving("a PurgeExpiredFeedOgImages request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/services.datahub.v1.DataHubService/PurgeExpiredFeedOgImages"),
			Headers: jsonHeaders(),
			Body: map[string]interface{}{
				"ttlSeconds": "604800",
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body: map[string]interface{}{
				"purgedCount": "12",
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewOgImageGateway(newDataHubServiceClient(config))
			purged, err := gw.CleanupExpiredFeedOgImages(context.Background(), 7*24*time.Hour)
			if err != nil {
				return fmt.Errorf("CleanupExpiredFeedOgImages failed: %w", err)
			}
			assert.Equal(t, int64(12), purged)
			return nil
		})

	require.NoError(t, err)
}
