//go:build contract

// Pact CDC: alt-backend and alt-harvester → alt-data-hub, feed-link
// capabilities (ADR-000954 Wave 3 batch 3, capability catalog §2.F / §2.G).
package contract

import (
	"context"
	"fmt"
	"testing"

	"alt/shared/gateway/datahub_gateway"

	"github.com/pact-foundation/pact-go/v2/consumer"
	"github.com/pact-foundation/pact-go/v2/matchers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testFeedLinkID  = "a1b2c3d4-1111-4111-8111-111111111111"
	testFeedLinkURL = "https://example.com/feed.xml"
)

// ---------------------------------------------------------------------------
// §2.F Feed links
// ---------------------------------------------------------------------------

// TestRegisterFeedLinkContract pins the idempotent registration. The response
// carries alreadyExisted rather than an error, because subscribing twice is a
// normal outcome the registration flow reports to the user.
func TestRegisterFeedLinkContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub accepts feed link registrations").
		UponReceiving("a RegisterFeedLink request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/services.datahub.v1.DataHubService/RegisterFeedLink"),
			Headers: jsonHeaders(),
			Body:    map[string]interface{}{"url": testFeedLinkURL},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			// A false bool is an absent key under protojson, so the successful
			// first registration answers {}. Pinning that is the point: a
			// consumer that read a missing key as "unknown" rather than "newly
			// added" would report every registration as a duplicate.
			Body: matchers.MapMatcher{},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewFeedLinkGateway(newDataHubServiceClient(config))
			if err := gw.RegisterRSSFeedLink(context.Background(), testFeedLinkURL); err != nil {
				return fmt.Errorf("RegisterRSSFeedLink failed: %w", err)
			}
			return nil
		})
	require.NoError(t, err)
}

// TestBulkRegisterFeedLinksContract pins the OPML import's partial-success
// shape. failedUrls names the outlines that did not land, so one bad entry in
// a large file does not discard the rest.
func TestBulkRegisterFeedLinksContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub accepts bulk feed link registrations").
		UponReceiving("a BulkRegisterFeedLinks request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/services.datahub.v1.DataHubService/BulkRegisterFeedLinks"),
			Headers: jsonHeaders(),
			Body: map[string]interface{}{
				"urls": []string{"https://a.example.com/feed.xml", "https://b.example.com/feed.xml"},
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body: matchers.MapMatcher{
				"registered": matchers.Like(1),
				"skipped":    matchers.Like(1),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewFeedLinkGateway(newDataHubServiceClient(config))
			result, err := gw.RegisterFeedLinkBulk(context.Background(),
				[]string{"https://a.example.com/feed.xml", "https://b.example.com/feed.xml"})
			if err != nil {
				return fmt.Errorf("RegisterFeedLinkBulk failed: %w", err)
			}
			require.NotNil(t, result)
			assert.Equal(t, 2, result.Total)
			assert.Equal(t, 1, result.Imported)
			assert.Equal(t, 1, result.Skipped)
			assert.Zero(t, result.Failed, "an absent failedUrls key means nothing failed")
			return nil
		})
	require.NoError(t, err)
}

func TestListFeedLinksContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has feed links").
		UponReceiving("a ListFeedLinks request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/services.datahub.v1.DataHubService/ListFeedLinks"),
			Headers: jsonHeaders(),
			Body:    map[string]interface{}{},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body: matchers.MapMatcher{
				"feedLinks": matchers.EachLike(map[string]interface{}{
					"id":  matchers.Regex(testFeedLinkID, uuidLikePattern),
					"url": matchers.Like(testFeedLinkURL),
				}, 1),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewFeedLinkGateway(newDataHubServiceClient(config))
			links, err := gw.FetchFeedLinks(context.Background())
			if err != nil {
				return fmt.Errorf("FetchFeedLinks failed: %w", err)
			}
			require.Len(t, links, 1)
			assert.Equal(t, testFeedLinkURL, links[0].URL)
			return nil
		})
	require.NoError(t, err)
}

// TestListFeedLinksWithHealthNeverPolledContract pins the absence that the
// admin screen reads as "never checked".
//
// A link with no availability row must arrive with no availability message —
// not a zero-valued one. GetHealthStatus maps nil to Unknown and a
// zero-failure row to Healthy, so encoding the absence as a zero would show a
// feed nobody has ever polled as green.
func TestListFeedLinksWithHealthNeverPolledContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has a feed link that has never been polled").
		UponReceiving("a ListFeedLinksWithHealth request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/services.datahub.v1.DataHubService/ListFeedLinksWithHealth"),
			Headers: jsonHeaders(),
			Body:    map[string]interface{}{},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body: matchers.MapMatcher{
				"feedLinks": matchers.EachLike(map[string]interface{}{
					"feedLink": matchers.Like(map[string]interface{}{
						"id":  matchers.Regex(testFeedLinkID, uuidLikePattern),
						"url": matchers.Like(testFeedLinkURL),
					}),
				}, 1),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewFeedLinkGateway(newDataHubServiceClient(config))
			links, err := gw.FetchFeedLinksWithAvailability(context.Background())
			if err != nil {
				return fmt.Errorf("FetchFeedLinksWithAvailability failed: %w", err)
			}
			require.Len(t, links, 1)
			assert.Nil(t, links[0].Availability,
				"a never-polled link must stay nil, or the admin screen calls it healthy")
			return nil
		})
	require.NoError(t, err)
}

func TestDeleteFeedLinkContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has feed links").
		UponReceiving("a DeleteFeedLink request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/services.datahub.v1.DataHubService/DeleteFeedLink"),
			Headers: jsonHeaders(),
			Body:    map[string]interface{}{"id": testFeedLinkID},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body:    matchers.MapMatcher{},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewFeedLinkGateway(newDataHubServiceClient(config))
			id, err := parseTestUUID(testFeedLinkID)
			if err != nil {
				return err
			}
			if err := gw.DeleteFeedLink(context.Background(), id); err != nil {
				return fmt.Errorf("DeleteFeedLink failed: %w", err)
			}
			return nil
		})
	require.NoError(t, err)
}

// TestResolveFeedLinkIDByURLMissContract pins the nil-without-error the
// registration flow depends on: an unsubscribed URL is how a new subscription
// is recognised, not a failure.
func TestResolveFeedLinkIDByURLMissContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has no feed link for the url").
		UponReceiving("a ResolveFeedLinkIDByURL request from alt-backend for an unsubscribed url").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/services.datahub.v1.DataHubService/ResolveFeedLinkIDByURL"),
			Headers: jsonHeaders(),
			Body:    map[string]interface{}{"feedUrl": "https://unknown.example.com/feed.xml"},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body:    matchers.MapMatcher{},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewFeedLinkGateway(newDataHubServiceClient(config))
			id, err := gw.FetchFeedLinkIDByURL(context.Background(), "https://unknown.example.com/feed.xml")
			if err != nil {
				return fmt.Errorf("FetchFeedLinkIDByURL failed: %w", err)
			}
			assert.Nil(t, id, "an unsubscribed url is nil-without-error, not an error")
			return nil
		})
	require.NoError(t, err)
}

// TestListFeedLinkDomainsContract — alt-harvester's daily scraping policy job
// seeds itself from this list.
func TestListFeedLinkDomainsContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerHarvester)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has feed links").
		UponReceiving("a ListFeedLinkDomains request from alt-harvester").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/services.datahub.v1.DataHubService/ListFeedLinkDomains"),
			Headers: jsonHeaders(),
			Body:    map[string]interface{}{},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body: matchers.MapMatcher{
				"domains": matchers.EachLike(map[string]interface{}{
					"domain": matchers.Like("example.com"),
					"scheme": matchers.Like("https"),
				}, 1),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewFeedLinkGateway(newDataHubServiceClient(config))
			domains, err := gw.ListFeedLinkDomains(context.Background())
			if err != nil {
				return fmt.Errorf("ListFeedLinkDomains failed: %w", err)
			}
			require.Len(t, domains, 1)
			assert.Equal(t, "example.com", domains[0].Domain)
			return nil
		})
	require.NoError(t, err)
}

// TestListRSSFeedURLsContract is the collector's input. Only active or
// never-assessed links appear, which is the mechanism by which a link disabled
// by RecordFeedLinkFailure stops being polled.
func TestListRSSFeedURLsContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerHarvester)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has pollable feed links").
		UponReceiving("a ListRSSFeedURLs request from alt-harvester").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/services.datahub.v1.DataHubService/ListRSSFeedURLs"),
			Headers: jsonHeaders(),
			Body:    map[string]interface{}{},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body: matchers.MapMatcher{
				"feedLinks": matchers.EachLike(map[string]interface{}{
					"id":  matchers.Regex(testFeedLinkID, uuidLikePattern),
					"url": matchers.Like(testFeedLinkURL),
				}, 1),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewFeedLinkGateway(newDataHubServiceClient(config))
			links, err := gw.FetchRSSFeedURLs(context.Background())
			if err != nil {
				return fmt.Errorf("FetchRSSFeedURLs failed: %w", err)
			}
			require.Len(t, links, 1)
			assert.Equal(t, testFeedLinkURL, links[0].URL)
			return nil
		})
	require.NoError(t, err)
}

// TestListFeedLinksForExportContract covers the query that used to be raw SQL
// issued through AltDBRepository.GetPool() from a gateway (catalog §4-7).
//
// The empty title is deliberate and pinned: a link whose feed has never been
// collected has no title, and the hostname the OPML file shows instead is
// substituted by the renderer on this side.
func TestListFeedLinksForExportContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has feed links").
		UponReceiving("a ListFeedLinksForExport request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/services.datahub.v1.DataHubService/ListFeedLinksForExport"),
			Headers: jsonHeaders(),
			Body:    map[string]interface{}{},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body: matchers.MapMatcher{
				"entries": matchers.EachLike(map[string]interface{}{
					"url":   matchers.Like(testFeedLinkURL),
					"title": matchers.Like("Example Blog"),
				}, 1),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewFeedLinkGateway(newDataHubServiceClient(config))
			entries, err := gw.FetchFeedLinksForExport(context.Background())
			if err != nil {
				return fmt.Errorf("FetchFeedLinksForExport failed: %w", err)
			}
			require.Len(t, entries, 1)
			assert.Equal(t, "Example Blog", entries[0].Title)
			return nil
		})
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// §2.G Feed link availability
// ---------------------------------------------------------------------------

// TestRecordFeedLinkFailureBelowThresholdContract pins the merged capability
// from catalog §4-4.
//
// The consumer sends its threshold and receives the post-increment row. It
// never receives a count it is expected to compare and act on with a second
// call — that read-modify-write is what the merge removed, and a response
// shape that reintroduced it (no disabledNow, say) would let the racy sequence
// come back.
func TestRecordFeedLinkFailureBelowThresholdContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerHarvester)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has a feed link with failures below the threshold").
		UponReceiving("a RecordFeedLinkFailure request from alt-harvester").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/services.datahub.v1.DataHubService/RecordFeedLinkFailure"),
			Headers: jsonHeaders(),
			Body: map[string]interface{}{
				"feedUrl":              testFeedLinkURL,
				"reason":               "403 Forbidden",
				"disableAfterFailures": 5,
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body: matchers.MapMatcher{
				"availability": matchers.Like(map[string]interface{}{
					"feedLinkId":          matchers.Regex(testFeedLinkID, uuidLikePattern),
					"isActive":            matchers.Like(true),
					"consecutiveFailures": matchers.Like(3),
					"lastFailureAt":       matchers.Like("2026-07-31T10:00:00Z"),
					"lastFailureReason":   matchers.Like("403 Forbidden"),
				}),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewFeedLinkAvailabilityGateway(newDataHubServiceClient(config), 5)
			availability, disabledNow, err := gw.RecordFeedLinkFailure(context.Background(), testFeedLinkURL, "403 Forbidden")
			if err != nil {
				return fmt.Errorf("RecordFeedLinkFailure failed: %w", err)
			}
			require.NotNil(t, availability)
			assert.Equal(t, 3, availability.ConsecutiveFailures)
			assert.True(t, availability.IsActive)
			assert.False(t, disabledNow, "an absent disabledNow means no transition happened")
			return nil
		})
	require.NoError(t, err)
}

// TestRecordFeedLinkFailureAtThresholdContract pins the other half: crossing
// the threshold reports both the disabled row and the transition, in one
// answer, so the caller neither re-reads nor re-decides.
func TestRecordFeedLinkFailureAtThresholdContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerHarvester)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has a feed link at the failure threshold").
		UponReceiving("a RecordFeedLinkFailure request from alt-harvester that crosses the threshold").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/services.datahub.v1.DataHubService/RecordFeedLinkFailure"),
			Headers: jsonHeaders(),
			Body: map[string]interface{}{
				"feedUrl":              "https://dead.example.com/feed.xml",
				"reason":               "404 Not Found",
				"disableAfterFailures": 5,
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body: matchers.MapMatcher{
				"availability": matchers.Like(map[string]interface{}{
					"feedLinkId":          matchers.Regex(testFeedLinkID, uuidLikePattern),
					"consecutiveFailures": matchers.Like(5),
					"lastFailureReason":   matchers.Like("404 Not Found"),
				}),
				"disabledNow": matchers.Like(true),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewFeedLinkAvailabilityGateway(newDataHubServiceClient(config), 5)
			availability, disabledNow, err := gw.RecordFeedLinkFailure(context.Background(),
				"https://dead.example.com/feed.xml", "404 Not Found")
			if err != nil {
				return fmt.Errorf("RecordFeedLinkFailure failed: %w", err)
			}
			require.NotNil(t, availability)
			assert.True(t, disabledNow)
			assert.False(t, availability.IsActive,
				"an absent isActive is false: the row must describe itself after the disable")
			return nil
		})
	require.NoError(t, err)
}

func TestResetFeedLinkFailuresContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerHarvester)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has a feed link with failures below the threshold").
		UponReceiving("a ResetFeedLinkFailures request from alt-harvester").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/services.datahub.v1.DataHubService/ResetFeedLinkFailures"),
			Headers: jsonHeaders(),
			Body:    map[string]interface{}{"feedUrl": testFeedLinkURL},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body:    matchers.MapMatcher{},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewFeedLinkAvailabilityGateway(newDataHubServiceClient(config), 5)
			if err := gw.ResetFeedLinkFailures(context.Background(), testFeedLinkURL); err != nil {
				return fmt.Errorf("ResetFeedLinkFailures failed: %w", err)
			}
			return nil
		})
	require.NoError(t, err)
}
