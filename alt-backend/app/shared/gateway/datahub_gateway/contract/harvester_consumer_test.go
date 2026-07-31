//go:build contract

// Pact CDC: alt-harvester → alt-data-hub (alt.datahub.v1.DataHubService).
//
// ADR-000954 Wave 3, capability catalog §2.A / §2.D / §2.E / §2.L. These are
// the procedures the eight scheduled jobs call after their direct alt_db
// access is removed.
//
// What these interactions pin, beyond "the procedure exists":
//
//   - The Connect-RPC path carries the alt.datahub.v1 package name.
//   - Content-Type: application/json — Connect over HTTP/1.1 with the
//     protoJSON codec (ADR-000764). The gateways go through
//     datahub_client.NewServiceClient rather than the generated constructor
//     so the recorded bytes are the bytes production sends.
//   - protoJSON's int64 encoding. olderThanSeconds, ttlSeconds, prunedCount
//     and the rest are JSON *strings*, not numbers, because that is what
//     protojson emits for 64-bit integers. A consumer written against a
//     hand-drawn number would parse fine locally and fail on the wire.
//   - The outbox status enum travels as its name, not its number, and the
//     four transitions are four procedures rather than one procedure with a
//     status argument.
package contract

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"alt/domain"
	"alt/gen/proto/alt/datahub/v1/datahubv1connect"
	"alt/shared/driver/datahub_client"
	"alt/shared/gateway/datahub_gateway"

	"github.com/pact-foundation/pact-go/v2/consumer"
	"github.com/pact-foundation/pact-go/v2/matchers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const pactDir = "../../../../../pacts"

const (
	consumerHarvester = "alt-harvester"
	consumerBackend   = "alt-backend"
	providerDataHub   = "alt-data-hub"
)

const uuidLikePattern = `^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`

func newDataHubPact(t *testing.T, consumerName string) *consumer.V3HTTPMockProvider {
	t.Helper()
	mockProvider, err := consumer.NewV3Pact(consumer.MockHTTPProviderConfig{
		Consumer: consumerName,
		Provider: providerDataHub,
		PactDir:  filepath.Join(pactDir),
	})
	require.NoError(t, err)
	return mockProvider
}

// newDataHubServiceClient goes through the production constructor, not
// datahubv1connect directly, so the codec these pacts record is the codec the
// composition root wires.
func newDataHubServiceClient(config consumer.MockServerConfig) datahubv1connect.DataHubServiceClient {
	return datahub_client.NewServiceClient(
		http.DefaultClient,
		fmt.Sprintf("http://%s:%d", config.Host, config.Port),
	)
}

func jsonHeaders() matchers.MapMatcher {
	return matchers.MapMatcher{"Content-Type": matchers.String("application/json")}
}

// ---------------------------------------------------------------------------
// §2.A Outbox
// ---------------------------------------------------------------------------

// TestClaimOutboxBatchContract pins the capability the whole outbox worker
// rests on. The response events are already PROCESSING: the claim and the
// status change happen in one provider-side transaction, so a consumer must
// never see PENDING here.
func TestClaimOutboxBatchContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerHarvester)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has pending outbox events").
		UponReceiving("a ClaimOutboxBatch request from alt-harvester").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/alt.datahub.v1.DataHubService/ClaimOutboxBatch"),
			Headers: jsonHeaders(),
			Body:    map[string]interface{}{"limit": 10},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body: matchers.MapMatcher{
				"events": matchers.EachLike(map[string]interface{}{
					"id":        matchers.Regex("8f14e45f-ceea-467a-9d0c-1a2b3c4d5e6f", uuidLikePattern),
					"eventType": matchers.Like("ARTICLE_UPSERT"),
					// bytes over protoJSON is base64. This value decodes to
					// {"article_id":"a1"} — the worker unmarshals it, so the
					// encoding is part of the contract.
					"payload": matchers.Like("eyJhcnRpY2xlX2lkIjoiYTEifQ=="),
					// Claimed, therefore PROCESSING. Matched exactly rather
					// than Like(): the whole point of the capability is that
					// the caller cannot observe these rows as PENDING.
					"status":    matchers.String("OUTBOX_EVENT_STATUS_PROCESSING"),
					"createdAt": matchers.Like("2026-07-31T00:00:00Z"),
				}, 1),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewOutboxGateway(newDataHubServiceClient(config))

			events, err := gw.ClaimBatch(context.Background(), 10)
			if err != nil {
				return fmt.Errorf("ClaimBatch failed: %w", err)
			}
			require.Len(t, events, 1)
			assert.Equal(t, "ARTICLE_UPSERT", events[0].EventType)
			assert.Equal(t, domain.OutboxProcessing, events[0].Status)
			assert.JSONEq(t, `{"article_id":"a1"}`, string(events[0].Payload),
				"the base64 payload must survive the wire as the bytes the producer wrote")
			assert.False(t, events[0].CreatedAt.IsZero())
			return nil
		})
	require.NoError(t, err)
}

func TestMarkOutboxProcessedContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerHarvester)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has a claimed outbox event").
		UponReceiving("a MarkOutboxProcessed request from alt-harvester recording success").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/alt.datahub.v1.DataHubService/MarkOutboxProcessed"),
			Headers: jsonHeaders(),
			Body: map[string]interface{}{
				"id": "8f14e45f-ceea-467a-9d0c-1a2b3c4d5e6f",
				// The enum name, not its number: protojson emits names, and a
				// consumer that sent 3 would be silently rejected.
				"status": "OUTBOX_EVENT_STATUS_PROCESSED",
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body:    matchers.MapMatcher{},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewOutboxGateway(newDataHubServiceClient(config))
			return gw.MarkProcessed(context.Background(),
				"8f14e45f-ceea-467a-9d0c-1a2b3c4d5e6f", domain.OutboxProcessed, "")
		})
	require.NoError(t, err)
}

func TestMarkOutboxFailedCarriesErrorMessageContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerHarvester)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has a claimed outbox event").
		UponReceiving("a MarkOutboxProcessed request from alt-harvester recording failure").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/alt.datahub.v1.DataHubService/MarkOutboxProcessed"),
			Headers: jsonHeaders(),
			Body: map[string]interface{}{
				"id":     "8f14e45f-ceea-467a-9d0c-1a2b3c4d5e6f",
				"status": "OUTBOX_EVENT_STATUS_FAILED",
				// The reason is the only forensic record of why delivery gave
				// up; it must reach the row, not just the harvester's log.
				"errorMessage": "rag upsert refused: 503",
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body:    matchers.MapMatcher{},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewOutboxGateway(newDataHubServiceClient(config))
			return gw.MarkProcessed(context.Background(),
				"8f14e45f-ceea-467a-9d0c-1a2b3c4d5e6f", domain.OutboxFailed, "rag upsert refused: 503")
		})
	require.NoError(t, err)
}

// TestReleaseOutboxEventContract pins the transition that has no other route
// back: ClaimOutboxBatch reads PENDING only, so a PROCESSING row nobody
// releases is invisible to every future claim.
func TestReleaseOutboxEventContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerHarvester)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has a claimed outbox event").
		UponReceiving("a ReleaseOutboxEvent request from alt-harvester after mid-batch cancellation").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/alt.datahub.v1.DataHubService/ReleaseOutboxEvent"),
			Headers: jsonHeaders(),
			Body:    map[string]interface{}{"id": "8f14e45f-ceea-467a-9d0c-1a2b3c4d5e6f"},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body:    matchers.MapMatcher{},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewOutboxGateway(newDataHubServiceClient(config))
			return gw.Release(context.Background(), "8f14e45f-ceea-467a-9d0c-1a2b3c4d5e6f")
		})
	require.NoError(t, err)
}

func TestPruneOutboxEventsContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerHarvester)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has processed outbox events past retention").
		UponReceiving("a PruneOutboxEvents request from alt-harvester").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/alt.datahub.v1.DataHubService/PruneOutboxEvents"),
			Headers: jsonHeaders(),
			// int64 over protoJSON is a string. 604800 = 7 days, the
			// retention window the pruning job owns.
			Body: map[string]interface{}{"olderThanSeconds": "604800"},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body:    matchers.MapMatcher{"prunedCount": matchers.Like("12")},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewOutboxGateway(newDataHubServiceClient(config))
			pruned, err := gw.Prune(context.Background(), 7*24*time.Hour)
			if err != nil {
				return fmt.Errorf("Prune failed: %w", err)
			}
			assert.Equal(t, int64(12), pruned)
			return nil
		})
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// §2.D OG image pipeline (harvester half)
// ---------------------------------------------------------------------------

func TestListFeedsMissingOgImageContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerHarvester)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has recent articles with no og image").
		UponReceiving("a ListFeedsMissingOgImage request from alt-harvester").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/alt.datahub.v1.DataHubService/ListFeedsMissingOgImage"),
			Headers: jsonHeaders(),
			Body:    map[string]interface{}{"limit": 50},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body: matchers.MapMatcher{
				"candidates": matchers.EachLike(map[string]interface{}{
					"articleId": matchers.Regex("6f1a2f7e-1f1e-4c2a-9a3e-5b6c7d8e9f01", uuidLikePattern),
					"url":       matchers.Like("https://example.com/post"),
				}, 1),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewOgImageGateway(newDataHubServiceClient(config))
			candidates, err := gw.FetchFeedsMissingOgImage(context.Background(), 50)
			if err != nil {
				return fmt.Errorf("FetchFeedsMissingOgImage failed: %w", err)
			}
			require.Len(t, candidates, 1)
			assert.Equal(t, "https://example.com/post", candidates[0].URL)
			assert.NotEmpty(t, candidates[0].ArticleID)
			return nil
		})
	require.NoError(t, err)
}

func TestListUnwarmedOgImageURLsContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerHarvester)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has feeds with uncached og images").
		UponReceiving("a ListUnwarmedOgImageURLs request from alt-harvester").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/alt.datahub.v1.DataHubService/ListUnwarmedOgImageURLs"),
			Headers: jsonHeaders(),
			Body:    map[string]interface{}{"limit": 100},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body: matchers.MapMatcher{
				"urls": matchers.EachLike("https://cdn.example.com/og.png", 1),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewOgImageGateway(newDataHubServiceClient(config))
			urls, err := gw.FetchUnwarmedOgImageURLs(context.Background(), 100)
			if err != nil {
				return fmt.Errorf("FetchUnwarmedOgImageURLs failed: %w", err)
			}
			require.Len(t, urls, 1)
			assert.Equal(t, "https://cdn.example.com/og.png", urls[0])
			return nil
		})
	require.NoError(t, err)
}

// TestPurgeExpiredArticleHeadsContract covers a copyright-compliance delete.
// The window is transmitted rather than assumed by the provider: it is the
// retention job's policy, and a provider-side default would make changing it
// a data-hub deployment.
func TestPurgeExpiredArticleHeadsContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerHarvester)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has article heads past retention").
		UponReceiving("a PurgeExpiredArticleHeads request from alt-harvester").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/alt.datahub.v1.DataHubService/PurgeExpiredArticleHeads"),
			Headers: jsonHeaders(),
			Body:    map[string]interface{}{"ttlSeconds": "604800"},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body:    matchers.MapMatcher{"purgedCount": matchers.Like("7")},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewOgImageGateway(newDataHubServiceClient(config))
			purged, err := gw.CleanupExpiredArticleHeads(context.Background(), 7*24*time.Hour)
			if err != nil {
				return fmt.Errorf("CleanupExpiredArticleHeads failed: %w", err)
			}
			assert.Equal(t, int64(7), purged)
			return nil
		})
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// §2.E Image proxy cache (harvester retention half)
// ---------------------------------------------------------------------------

func TestPurgeImageProxyCacheOlderThanContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerHarvester)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has cached images past retention").
		UponReceiving("a PurgeImageProxyCacheOlderThan request from alt-harvester").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/alt.datahub.v1.DataHubService/PurgeImageProxyCacheOlderThan"),
			Headers: jsonHeaders(),
			Body:    map[string]interface{}{"ttlSeconds": "604800"},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body:    matchers.MapMatcher{"purgedCount": matchers.Like("3")},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewImageProxyCacheGateway(newDataHubServiceClient(config))
			purged, err := gw.CleanupImageProxyCacheOlderThan(context.Background(), 7*24*time.Hour)
			if err != nil {
				return fmt.Errorf("CleanupImageProxyCacheOlderThan failed: %w", err)
			}
			assert.Equal(t, int64(3), purged)
			return nil
		})
	require.NoError(t, err)
}

func TestEvictExpiredImageProxyCacheContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerHarvester)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has expired cached images").
		UponReceiving("an EvictExpiredImageProxyCache request from alt-harvester").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/alt.datahub.v1.DataHubService/EvictExpiredImageProxyCache"),
			Headers: jsonHeaders(),
			Body:    map[string]interface{}{},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body:    matchers.MapMatcher{"evictedCount": matchers.Like("5")},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewImageProxyCacheGateway(newDataHubServiceClient(config))
			evicted, err := gw.CleanupExpiredImageProxyCache(context.Background())
			if err != nil {
				return fmt.Errorf("CleanupExpiredImageProxyCache failed: %w", err)
			}
			assert.Equal(t, int64(5), evicted)
			return nil
		})
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// §2.L Scraping policy (harvester write half)
// ---------------------------------------------------------------------------

// TestSaveScrapingDomainContract pins the response shape that replaces
// in-place struct mutation. The driver assigned id/created_at/updated_at by
// writing into the caller's pointer; across a process boundary the provider is
// the only side that knows them, so they come back in the response.
func TestSaveScrapingDomainContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerHarvester)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub accepts scraping domain writes").
		UponReceiving("a SaveScrapingDomain request from alt-harvester").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/alt.datahub.v1.DataHubService/SaveScrapingDomain"),
			Headers: jsonHeaders(),
			Body: matchers.MapMatcher{
				"scrapingDomain": matchers.Like(map[string]interface{}{
					"id":                  matchers.Regex("00000000-0000-0000-0000-000000000000", uuidLikePattern),
					"domain":              matchers.Like("example.com"),
					"scheme":              matchers.Like("https"),
					"allowFetchBody":      matchers.Like(true),
					"robotsDisallowPaths": matchers.EachLike("/private", 1),
				}),
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body: matchers.MapMatcher{
				"scrapingDomain": matchers.Like(map[string]interface{}{
					"id":                  matchers.Regex("2b1c3d4e-5f60-4711-8899-aabbccddeeff", uuidLikePattern),
					"domain":              matchers.Like("example.com"),
					"scheme":              matchers.Like("https"),
					"allowFetchBody":      matchers.Like(true),
					"robotsDisallowPaths": matchers.EachLike("/private", 1),
					"createdAt":           matchers.Like("2026-07-31T00:00:00Z"),
					"updatedAt":           matchers.Like("2026-07-31T00:00:00Z"),
				}),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewScrapingDomainGateway(newDataHubServiceClient(config))

			sd := &domain.ScrapingDomain{
				Domain:              "example.com",
				Scheme:              "https",
				AllowFetchBody:      true,
				RobotsDisallowPaths: []string{"/private"},
			}
			if err := gw.Save(context.Background(), sd); err != nil {
				return fmt.Errorf("Save failed: %w", err)
			}
			assert.NotEqual(t, "00000000-0000-0000-0000-000000000000", sd.ID.String(),
				"the provider-assigned id must be written back into the caller's struct")
			assert.False(t, sd.CreatedAt.IsZero())
			return nil
		})
	require.NoError(t, err)
}

func TestListScrapingDomainsContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerHarvester)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has scraping domains").
		UponReceiving("a ListScrapingDomains request from alt-harvester").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/alt.datahub.v1.DataHubService/ListScrapingDomains"),
			Headers: jsonHeaders(),
			Body:    map[string]interface{}{"limit": 100},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body: matchers.MapMatcher{
				"scrapingDomains": matchers.EachLike(map[string]interface{}{
					"id":     matchers.Regex("2b1c3d4e-5f60-4711-8899-aabbccddeeff", uuidLikePattern),
					"domain": matchers.Like("example.com"),
					"scheme": matchers.Like("https"),
				}, 1),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewScrapingDomainGateway(newDataHubServiceClient(config))
			domains, err := gw.List(context.Background(), 0, 100)
			if err != nil {
				return fmt.Errorf("List failed: %w", err)
			}
			require.Len(t, domains, 1)
			assert.Equal(t, "example.com", domains[0].Domain)
			return nil
		})
	require.NoError(t, err)
}

// TestUpdateScrapingDomainPolicyContract pins the partial-update semantics: an
// omitted field leaves the stored value alone. Sending every field with a zero
// default would silently reset the three the caller did not mean to touch.
func TestUpdateScrapingDomainPolicyContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerHarvester)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has a scraping domain").
		UponReceiving("an UpdateScrapingDomainPolicy request from alt-harvester setting one field").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/alt.datahub.v1.DataHubService/UpdateScrapingDomainPolicy"),
			Headers: jsonHeaders(),
			Body: map[string]interface{}{
				"id": "2b1c3d4e-5f60-4711-8899-aabbccddeeff",
				// Exactly one key. The other three policy fields are absent,
				// which the provider maps to COALESCE(..., existing).
				"update": map[string]interface{}{"allowFetchBody": false},
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body:    matchers.MapMatcher{},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewScrapingDomainGateway(newDataHubServiceClient(config))

			id, err := parseTestUUID("2b1c3d4e-5f60-4711-8899-aabbccddeeff")
			if err != nil {
				return err
			}
			allow := false
			return gw.UpdatePolicy(context.Background(), id, &domain.ScrapingPolicyUpdate{AllowFetchBody: &allow})
		})
	require.NoError(t, err)
}
