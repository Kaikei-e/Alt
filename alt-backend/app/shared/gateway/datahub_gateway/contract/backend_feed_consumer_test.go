//go:build contract

// Pact CDC: alt-backend and alt-harvester → alt-data-hub, feed and feed-link
// capabilities (ADR-000954 Wave 3 batch 3, capability catalog §2.F / §2.G /
// §2.H).
//
// Two consumers appear in one file because the feed path is the one place the
// two binaries touch the same tables from opposite ends: alt-harvester writes
// what polling produced, alt-backend reads it. They still publish to separate
// pacts — the file is an organising convenience, not a shared pacticipant.
package contract

import (
	"context"
	"fmt"
	"net/url"
	"testing"
	"time"

	"alt/orchestrator/driver/models"
	"alt/shared/gateway/datahub_gateway"

	"github.com/google/uuid"
	"github.com/pact-foundation/pact-go/v2/consumer"
	"github.com/pact-foundation/pact-go/v2/matchers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testFeedLinkID  = "a1b2c3d4-1111-4111-8111-111111111111"
	testFeedRowID   = "b2c3d4e5-2222-4222-8222-222222222222"
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

// ---------------------------------------------------------------------------
// §2.H Feeds
// ---------------------------------------------------------------------------

// TestRegisterFeedsContract pins the collector's write. `created` distinguishes
// an insert from an update, and the registration writes no articles row
// (ADR-000953) — which is why there is no article id in the result.
func TestRegisterFeedsContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerHarvester)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub accepts feed registrations").
		UponReceiving("a RegisterFeeds request from alt-harvester").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/services.datahub.v1.DataHubService/RegisterFeeds"),
			Headers: jsonHeaders(),
			Body: map[string]interface{}{
				"feeds": []map[string]interface{}{{
					"title":       "Example Post",
					"description": "body",
					"websiteUrl":  "https://example.com/post",
					"pubDate":     "2026-07-31T00:00:00Z",
					"createdAt":   "2026-07-31T09:00:00Z",
					"updatedAt":   "2026-07-31T09:00:00Z",
					"feedLinkId":  testFeedLinkID,
				}},
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body: matchers.MapMatcher{
				"results": matchers.EachLike(map[string]interface{}{
					"feedId":  matchers.Regex(testFeedRowID, uuidLikePattern),
					"created": matchers.Like(true),
				}, 1),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewFeedGateway(newDataHubServiceClient(config))
			results, err := gw.RegisterMultipleFeedsWithState(context.Background(), feedRegistrationFixture())
			if err != nil {
				return fmt.Errorf("RegisterMultipleFeedsWithState failed: %w", err)
			}
			require.Len(t, results, 1)
			assert.Equal(t, testFeedRowID, results[0].FeedID)
			assert.True(t, results[0].Created)
			return nil
		})
	require.NoError(t, err)
}

// TestListFeedsCursorUnreadContract pins the tenant field.
//
// The driver read the user from the Go request context. Nothing carries a
// context over Connect, so the scope travels as an explicit userId — and a
// consumer that forgot it would previously have listed somebody else's feeds
// rather than failing. Both sides now verify the field is there.
func TestListFeedsCursorUnreadContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has unread feeds for the user").
		UponReceiving("a ListFeedsCursor request from alt-backend for the unread scope").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/services.datahub.v1.DataHubService/ListFeedsCursor"),
			Headers: jsonHeaders(),
			Body: map[string]interface{}{
				"scope":  "FEED_SCOPE_UNREAD",
				"userId": testUserID,
				"limit":  20,
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body: matchers.MapMatcher{
				"feeds": matchers.EachLike(map[string]interface{}{
					"id":          matchers.Regex(testFeedRowID, uuidLikePattern),
					"title":       matchers.Like("Example Post"),
					"description": matchers.Like("body"),
					"websiteUrl":  matchers.Like("https://example.com/post"),
					"pubDate":     matchers.Like("2026-07-31T00:00:00Z"),
					"createdAt":   matchers.Like("2026-07-31T09:00:00Z"),
					"updatedAt":   matchers.Like("2026-07-31T09:00:00Z"),
					"articleId":   matchers.Regex(testArticleID, uuidLikePattern),
					"ogImageUrl":  matchers.Like("https://cdn.example.com/og.png"),
				}, 1),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewFeedGateway(newDataHubServiceClient(config))
			feeds, err := gw.FetchUnreadFeedsListCursor(authedContext(testUserID), nil, 20, nil)
			if err != nil {
				return fmt.Errorf("FetchUnreadFeedsListCursor failed: %w", err)
			}
			require.Len(t, feeds, 1)
			require.NotNil(t, feeds[0].ArticleID)
			assert.Equal(t, testArticleID, *feeds[0].ArticleID)
			return nil
		})
	require.NoError(t, err)
}

// TestListFeedsCursorFavoriteNoOgImageContract pins the absent og:image.
//
// A feed past the 7-day copyright retention window returns no image, and the
// frontend renders a placeholder for it. Encoding the absence as "" would make
// the proxy fetch an empty URL on every card of every older feed.
func TestListFeedsCursorFavoriteNoOgImageContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has favorite feeds past the og image retention window").
		UponReceiving("a ListFeedsCursor request from alt-backend for the favorite scope").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/services.datahub.v1.DataHubService/ListFeedsCursor"),
			Headers: jsonHeaders(),
			Body: map[string]interface{}{
				"scope":  "FEED_SCOPE_FAVORITE",
				"userId": testUserID,
				"limit":  20,
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body: matchers.MapMatcher{
				"feeds": matchers.EachLike(map[string]interface{}{
					"id":         matchers.Regex(testFeedRowID, uuidLikePattern),
					"title":      matchers.Like("Older Post"),
					"websiteUrl": matchers.Like("https://example.com/older"),
					"createdAt":  matchers.Like("2026-06-01T09:00:00Z"),
				}, 1),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewFeedGateway(newDataHubServiceClient(config))
			feeds, err := gw.FetchFavoriteFeedsListCursor(authedContext(testUserID), nil, 20)
			if err != nil {
				return fmt.Errorf("FetchFavoriteFeedsListCursor failed: %w", err)
			}
			require.Len(t, feeds, 1)
			assert.Nil(t, feeds[0].OgImageURL, "a retired og image must be absent, not empty")
			assert.Nil(t, feeds[0].ArticleID)
			return nil
		})
	require.NoError(t, err)
}

func TestListFeedsLimitContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has feeds").
		UponReceiving("a ListFeedsLimit request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/services.datahub.v1.DataHubService/ListFeedsLimit"),
			Headers: jsonHeaders(),
			Body:    map[string]interface{}{"limit": 10},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body: matchers.MapMatcher{
				"feeds": matchers.EachLike(map[string]interface{}{
					"id":         matchers.Regex(testFeedRowID, uuidLikePattern),
					"title":      matchers.Like("Example Post"),
					"websiteUrl": matchers.Like("https://example.com/post"),
					"createdAt":  matchers.Like("2026-07-31T09:00:00Z"),
				}, 1),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewFeedGateway(newDataHubServiceClient(config))
			feeds, err := gw.FetchFeedsListLimit(context.Background(), 10)
			if err != nil {
				return fmt.Errorf("FetchFeedsListLimit failed: %w", err)
			}
			require.Len(t, feeds, 1)
			assert.Equal(t, "Example Post", feeds[0].Title)
			return nil
		})
	require.NoError(t, err)
}

// TestGetSingleFeedEmptyContract pins "there are no feeds at all" as an unset
// field. A fresh install has none, and answering an error for it would make an
// empty database look like a broken one.
func TestGetSingleFeedEmptyContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has no feeds").
		UponReceiving("a GetSingleFeed request from alt-backend against an empty table").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/services.datahub.v1.DataHubService/GetSingleFeed"),
			Headers: jsonHeaders(),
			Body:    map[string]interface{}{},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body:    matchers.MapMatcher{},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewFeedGateway(newDataHubServiceClient(config))
			feed, err := gw.GetSingleFeed(context.Background())
			if err != nil {
				return fmt.Errorf("GetSingleFeed failed: %w", err)
			}
			assert.Nil(t, feed, "an empty feeds table is nil-without-error")
			return nil
		})
	require.NoError(t, err)
}

func TestListFeedsByFeedLinkIDContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has feeds for the feed link").
		UponReceiving("a ListFeedsByFeedLinkID request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/services.datahub.v1.DataHubService/ListFeedsByFeedLinkID"),
			Headers: jsonHeaders(),
			Body:    map[string]interface{}{"feedLinkId": testFeedLinkID},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body: matchers.MapMatcher{
				"feeds": matchers.EachLike(map[string]interface{}{
					"id":         matchers.Regex(testFeedRowID, uuidLikePattern),
					"title":      matchers.Like("Example Post"),
					"websiteUrl": matchers.Like("https://example.com/post"),
					"createdAt":  matchers.Like("2026-07-31T09:00:00Z"),
				}, 1),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewFeedGateway(newDataHubServiceClient(config))
			id, err := parseTestUUID(testFeedLinkID)
			if err != nil {
				return err
			}
			rows, err := gw.FetchFeedsByFeedLinkID(context.Background(), id)
			if err != nil {
				return fmt.Errorf("FetchFeedsByFeedLinkID failed: %w", err)
			}
			require.Len(t, rows, 1)
			assert.Equal(t, testFeedRowID, rows[0].ID)
			return nil
		})
	require.NoError(t, err)
}

// TestGetArticleSummaryByArticleIDMissContract pins the answer the summarise
// path reads as "generate one".
//
// The driver raised pgx.ErrNoRows for an unsummarised article and the caller
// treated the error as a cache miss. Over an RPC an error is a fault, so the
// miss has to be an unset field — otherwise StreamSummarize would report a
// data plane failure for every article it has not summarised yet.
func TestGetArticleSummaryByArticleIDMissContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has no summary for the article").
		UponReceiving("a GetArticleSummaryByArticleID request from alt-backend for an unsummarised article").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/services.datahub.v1.DataHubService/GetArticleSummaryByArticleID"),
			Headers: jsonHeaders(),
			Body: map[string]interface{}{
				"articleId": testArticleID,
				"userId":    testUserID,
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body:    matchers.MapMatcher{},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewFeedGateway(newDataHubServiceClient(config))
			summary, err := gw.FetchArticleSummaryByArticleID(authedContext(testUserID), testArticleID)
			if err != nil {
				return fmt.Errorf("FetchArticleSummaryByArticleID failed: %w", err)
			}
			assert.Nil(t, summary, "an unsummarised article is nil-without-error, not a fault")
			return nil
		})
	require.NoError(t, err)
}

func TestGetFeedSummaryContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has a summary for the article url").
		UponReceiving("a GetFeedSummary request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/services.datahub.v1.DataHubService/GetFeedSummary"),
			Headers: jsonHeaders(),
			Body: map[string]interface{}{
				"feedUrl": "https://example.com/post",
				"userId":  testUserID,
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body: matchers.MapMatcher{
				"summary": matchers.Like(map[string]interface{}{
					"summary": matchers.Like("要約テキスト"),
				}),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewFeedGateway(newDataHubServiceClient(config))
			parsed, parseErr := parseTestURL("https://example.com/post")
			if parseErr != nil {
				return parseErr
			}
			summary, err := gw.FetchFeedSummary(authedContext(testUserID), parsed)
			if err != nil {
				return fmt.Errorf("FetchFeedSummary failed: %w", err)
			}
			require.NotNil(t, summary)
			assert.Equal(t, "要約テキスト", summary.Summary)
			return nil
		})
	require.NoError(t, err)
}

func TestSearchFeedsByTitleContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has feeds matching the title query for the user").
		UponReceiving("a SearchFeedsByTitle request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/services.datahub.v1.DataHubService/SearchFeedsByTitle"),
			Headers: jsonHeaders(),
			Body: map[string]interface{}{
				"query":  "example",
				"userId": testUserID,
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body: matchers.MapMatcher{
				"feeds": matchers.EachLike(map[string]interface{}{
					"title":      matchers.Like("Example Post"),
					"websiteUrl": matchers.Like("https://example.com/post"),
					"pubDate":    matchers.Like("2026-07-31T00:00:00Z"),
				}, 1),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewFeedGateway(newDataHubServiceClient(config))
			feeds, err := gw.SearchFeedsByTitle(context.Background(), "example", testUserID)
			if err != nil {
				return fmt.Errorf("SearchFeedsByTitle failed: %w", err)
			}
			require.Len(t, feeds, 1)
			assert.Equal(t, "2026-07-31T00:00:00Z", feeds[0].Published)
			return nil
		})
	require.NoError(t, err)
}

// TestGetRandomFeedEmptyContract — nothing tagged yet is a state the Tag Trail
// entry point renders, not an error it reports.
func TestGetRandomFeedEmptyContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has no tagged feeds").
		UponReceiving("a GetRandomFeed request from alt-backend with nothing tagged").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/services.datahub.v1.DataHubService/GetRandomFeed"),
			Headers: jsonHeaders(),
			Body:    map[string]interface{}{},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body:    matchers.MapMatcher{},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewFeedGateway(newDataHubServiceClient(config))
			feed, err := gw.FetchRandomFeed(context.Background())
			if err != nil {
				return fmt.Errorf("FetchRandomFeed failed: %w", err)
			}
			assert.Nil(t, feed)
			return nil
		})
	require.NoError(t, err)
}

func TestGetFeedURLsByArticleIDsContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has feeds for the articles").
		UponReceiving("a GetFeedURLsByArticleIDs request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/services.datahub.v1.DataHubService/GetFeedURLsByArticleIDs"),
			Headers: jsonHeaders(),
			Body:    map[string]interface{}{"articleIds": []string{testArticleID}},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body: matchers.MapMatcher{
				"pairs": matchers.EachLike(map[string]interface{}{
					"feedId":       matchers.Regex(testFeedRowID, uuidLikePattern),
					"articleId":    matchers.Regex(testArticleID, uuidLikePattern),
					"url":          matchers.Like("https://example.com/post"),
					"feedTitle":    matchers.Like("Example Blog"),
					"articleTitle": matchers.Like("Example Post"),
				}, 1),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewFeedGateway(newDataHubServiceClient(config))
			pairs, err := gw.GetFeedURLsByArticleIDs(context.Background(), []string{testArticleID})
			if err != nil {
				return fmt.Errorf("GetFeedURLsByArticleIDs failed: %w", err)
			}
			require.Len(t, pairs, 1)
			assert.Equal(t, "Example Blog", pairs[0].FeedTitle)
			return nil
		})
	require.NoError(t, err)
}

// TestBatchGetFeedTitlesByIDsContract pins the omission of unknown ids. The
// Morning Letter enrichment defaults the byline itself; an entry mapping an
// unknown id to "" would claim the feed exists with no title.
func TestBatchGetFeedTitlesByIDsContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has feeds").
		UponReceiving("a BatchGetFeedTitlesByIDs request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/services.datahub.v1.DataHubService/BatchGetFeedTitlesByIDs"),
			Headers: jsonHeaders(),
			Body:    map[string]interface{}{"feedIds": []string{testFeedRowID}},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body: matchers.MapMatcher{
				"titles": matchers.Like(map[string]interface{}{
					testFeedRowID: matchers.Like("Example Blog"),
				}),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewFeedGateway(newDataHubServiceClient(config))
			id, err := parseTestUUID(testFeedRowID)
			if err != nil {
				return err
			}
			titles, err := gw.FetchFeedTitlesByIDs(context.Background(), []uuid.UUID{id})
			if err != nil {
				return fmt.Errorf("FetchFeedTitlesByIDs failed: %w", err)
			}
			assert.Equal(t, "Example Blog", titles[id])
			return nil
		})
	require.NoError(t, err)
}

func TestGetInoreaderSummariesByURLsContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has imported inoreader articles for the urls").
		UponReceiving("a GetInoreaderSummariesByURLs request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/services.datahub.v1.DataHubService/GetInoreaderSummariesByURLs"),
			Headers: jsonHeaders(),
			Body:    map[string]interface{}{"urls": []string{"https://example.com/post"}},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body: matchers.MapMatcher{
				"summaries": matchers.EachLike(map[string]interface{}{
					"articleUrl":  matchers.Like("https://example.com/post"),
					"title":       matchers.Like("Example Post"),
					"content":     matchers.Like("<p>body</p>"),
					"contentType": matchers.Like("html"),
					"publishedAt": matchers.Like("2026-07-31T00:00:00Z"),
					"fetchedAt":   matchers.Like("2026-07-31T01:00:00Z"),
					"inoreaderId": matchers.Like("tag:google.com,2005:reader/item/0001"),
				}, 1),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewFeedGateway(newDataHubServiceClient(config))
			summaries, err := gw.FetchInoreaderSummariesByURLs(context.Background(), []string{"https://example.com/post"})
			if err != nil {
				return fmt.Errorf("FetchInoreaderSummariesByURLs failed: %w", err)
			}
			require.Len(t, summaries, 1)
			assert.Equal(t, "Example Post", summaries[0].Title)
			assert.Nil(t, summaries[0].Author, "an absent author stays nil rather than an empty string")
			return nil
		})
	require.NoError(t, err)
}

// parseTestURL keeps the url.Parse error out of the interaction bodies.
func parseTestURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("parse test url %q: %w", raw, err)
	}
	return parsed, nil
}

// feedRegistrationFixture is the one collected item RegisterFeeds sends.
//
// The timestamps are the caller's, stamped once per poll rather than defaulted
// server-side, which is why they appear in the request body at all.
func feedRegistrationFixture() []models.Feed {
	feedLinkID := testFeedLinkID
	return []models.Feed{{
		Title:       "Example Post",
		Description: "body",
		WebsiteURL:  "https://example.com/post",
		PubDate:     time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC),
		CreatedAt:   time.Date(2026, 7, 31, 9, 0, 0, 0, time.UTC),
		UpdatedAt:   time.Date(2026, 7, 31, 9, 0, 0, 0, time.UTC),
		FeedLinkID:  &feedLinkID,
	}}
}
