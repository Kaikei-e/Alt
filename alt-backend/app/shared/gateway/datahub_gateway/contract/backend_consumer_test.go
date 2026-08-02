//go:build contract

// Pact CDC: alt-backend → alt-data-hub (services.datahub.v1.DataHubService).
//
// ADR-000954 Wave 3, capability catalog §2.D / §2.E / §2.L / §2.O — the half
// of the batch that sits on the user-facing request path rather than on a
// scheduler tick. Separate pact file from alt-harvester's on purpose: the two
// binaries exercise disjoint procedures, and one shared pacticipant would let
// a harvester-only break verify green because the backend never calls it.
package contract

import (
	"context"
	"fmt"
	"testing"
	"time"

	"alt/domain"

	"alt/shared/gateway/datahub_gateway"

	"github.com/google/uuid"
	"github.com/pact-foundation/pact-go/v2/consumer"
	"github.com/pact-foundation/pact-go/v2/matchers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func parseTestUUID(s string) (uuid.UUID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, fmt.Errorf("parse test uuid: %w", err)
	}
	return id, nil
}

// ---------------------------------------------------------------------------
// §2.D OG image reads
// ---------------------------------------------------------------------------

func TestGetArticleHeadContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has a scraped article head").
		UponReceiving("a GetArticleHead request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/services.datahub.v1.DataHubService/GetArticleHead"),
			Headers: jsonHeaders(),
			Body:    map[string]interface{}{"articleId": "6f1a2f7e-1f1e-4c2a-9a3e-5b6c7d8e9f01"},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body: matchers.MapMatcher{
				"head": matchers.Like(map[string]interface{}{
					"id":         matchers.Regex("11111111-2222-3333-4444-555555555555", uuidLikePattern),
					"articleId":  matchers.Regex("6f1a2f7e-1f1e-4c2a-9a3e-5b6c7d8e9f01", uuidLikePattern),
					"headHtml":   matchers.Like("<head><title>x</title></head>"),
					"ogImageUrl": matchers.Like("https://cdn.example.com/og.png"),
				}),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewOgImageGateway(newDataHubServiceClient(config))
			head, err := gw.FetchArticleHeadByArticleID(context.Background(), "6f1a2f7e-1f1e-4c2a-9a3e-5b6c7d8e9f01")
			if err != nil {
				return fmt.Errorf("FetchArticleHeadByArticleID failed: %w", err)
			}
			require.NotNil(t, head)
			assert.Equal(t, "https://cdn.example.com/og.png", head.OgImageURL)
			return nil
		})
	require.NoError(t, err)
}

// TestGetArticleHeadMissContract pins the absent-field encoding of "never
// scraped". fetch_article_usecase reads the nil to decide whether to scrape,
// so an empty ArticleHead object here would make it stop re-scraping articles
// it has never seen.
func TestGetArticleHeadMissContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has no article head for the article").
		UponReceiving("a GetArticleHead request from alt-backend for an unscraped article").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/services.datahub.v1.DataHubService/GetArticleHead"),
			Headers: jsonHeaders(),
			Body:    map[string]interface{}{"articleId": "00000000-0000-4000-8000-000000000000"},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			// No "head" key at all — protojson omits an unset optional
			// message rather than emitting null.
			Body: matchers.MapMatcher{},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewOgImageGateway(newDataHubServiceClient(config))
			head, err := gw.FetchArticleHeadByArticleID(context.Background(), "00000000-0000-4000-8000-000000000000")
			if err != nil {
				return fmt.Errorf("FetchArticleHeadByArticleID failed: %w", err)
			}
			assert.Nil(t, head, "a missing head must be nil-without-error, not an empty struct")
			return nil
		})
	require.NoError(t, err)
}

func TestBatchGetOgImageURLsContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has og images for the articles").
		UponReceiving("a BatchGetOgImageURLs request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/services.datahub.v1.DataHubService/BatchGetOgImageURLs"),
			Headers: jsonHeaders(),
			Body: map[string]interface{}{
				"articleIds": []string{"6f1a2f7e-1f1e-4c2a-9a3e-5b6c7d8e9f01"},
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body: matchers.MapMatcher{
				// A protoJSON map is a plain object keyed by the article id.
				"ogImageUrls": matchers.Like(map[string]interface{}{
					"6f1a2f7e-1f1e-4c2a-9a3e-5b6c7d8e9f01": matchers.Like("https://cdn.example.com/og.png"),
				}),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewOgImageGateway(newDataHubServiceClient(config))
			urls, err := gw.FetchOgImageURLsByArticleIDs(context.Background(),
				[]string{"6f1a2f7e-1f1e-4c2a-9a3e-5b6c7d8e9f01"})
			if err != nil {
				return fmt.Errorf("FetchOgImageURLsByArticleIDs failed: %w", err)
			}
			assert.Equal(t, "https://cdn.example.com/og.png", urls["6f1a2f7e-1f1e-4c2a-9a3e-5b6c7d8e9f01"])
			return nil
		})
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// §2.E Image proxy cache (serving path)
// ---------------------------------------------------------------------------

// TestGetImageProxyCacheContract pins the two things a JSON codec makes easy
// to get wrong for this message: the image bytes are base64, and sizeBytes is
// a 64-bit integer, so protoJSON writes it as a string.
func TestGetImageProxyCacheContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has a live image proxy cache entry").
		UponReceiving("a GetImageProxyCache request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/services.datahub.v1.DataHubService/GetImageProxyCache"),
			Headers: jsonHeaders(),
			Body:    map[string]interface{}{"urlHash": "abc123"},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body: matchers.MapMatcher{
				"entry": matchers.Like(map[string]interface{}{
					"urlHash":     matchers.Like("abc123"),
					"originalUrl": matchers.Like("https://cdn.example.com/og.png"),
					// base64 of the stored bytes.
					"data":        matchers.Like("UklGRg=="),
					"contentType": matchers.Like("image/webp"),
					"width":       matchers.Like(600),
					"height":      matchers.Like(315),
					"sizeBytes":   matchers.Like("4"),
					"expiresAt":   matchers.Like("2026-08-07T00:00:00Z"),
				}),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewImageProxyCacheGateway(newDataHubServiceClient(config))
			entry, err := gw.GetCachedImage(context.Background(), "abc123")
			if err != nil {
				return fmt.Errorf("GetCachedImage failed: %w", err)
			}
			require.NotNil(t, entry)
			assert.Equal(t, "image/webp", entry.ContentType)
			assert.NotEmpty(t, entry.Data, "the cached bytes must survive base64 transport")
			assert.Equal(t, 600, entry.Width)
			return nil
		})
	require.NoError(t, err)
}

// TestGetImageProxyCacheMissContract: a miss and an expired entry are the same
// answer, and both are the absence of the field.
func TestGetImageProxyCacheMissContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has no live image proxy cache entry").
		UponReceiving("a GetImageProxyCache request from alt-backend that misses").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/services.datahub.v1.DataHubService/GetImageProxyCache"),
			Headers: jsonHeaders(),
			Body:    map[string]interface{}{"urlHash": "missing"},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body:    matchers.MapMatcher{},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewImageProxyCacheGateway(newDataHubServiceClient(config))
			entry, err := gw.GetCachedImage(context.Background(), "missing")
			if err != nil {
				return fmt.Errorf("GetCachedImage failed: %w", err)
			}
			assert.Nil(t, entry)
			return nil
		})
	require.NoError(t, err)
}

func TestPutImageProxyCacheContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub accepts image proxy cache writes").
		UponReceiving("a PutImageProxyCache request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/services.datahub.v1.DataHubService/PutImageProxyCache"),
			Headers: jsonHeaders(),
			Body: matchers.MapMatcher{
				"entry": matchers.Like(map[string]interface{}{
					"urlHash":     matchers.Like("abc123"),
					"originalUrl": matchers.Like("https://cdn.example.com/og.png"),
					"data":        matchers.Like("UklGRg=="),
					"contentType": matchers.Like("image/webp"),
					"sizeBytes":   matchers.Like("4"),
					// The TTL the writer computed. Sent, not derived by the
					// provider: the cache window is the image proxy's policy,
					// and a provider-side default would make every entry
					// expire on alt-data-hub's schedule instead.
					"expiresAt": matchers.Like("2026-08-07T00:00:00Z"),
				}),
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body:    matchers.MapMatcher{},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewImageProxyCacheGateway(newDataHubServiceClient(config))
			return gw.SaveCachedImage(context.Background(), &domain.ImageProxyCacheEntry{
				URLHash:     "abc123",
				OriginalURL: "https://cdn.example.com/og.png",
				Data:        []byte{0x52, 0x49, 0x46, 0x46},
				ContentType: "image/webp",
				SizeBytes:   4,
				ExpiresAt:   time.Date(2026, 8, 7, 0, 0, 0, 0, time.UTC),
			})
		})
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// §2.L Scraping policy (backend read half) + declined domains
// ---------------------------------------------------------------------------

func TestGetScrapingDomainByDomainContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has a scraping domain").
		UponReceiving("a GetScrapingDomainByDomain request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/services.datahub.v1.DataHubService/GetScrapingDomainByDomain"),
			Headers: jsonHeaders(),
			Body:    map[string]interface{}{"domain": "example.com"},
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
					"forceRespectRobots":  matchers.Like(true),
					"robotsCrawlDelaySec": matchers.Like(5),
					"robotsDisallowPaths": matchers.EachLike("/private", 1),
				}),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewScrapingDomainGateway(newDataHubServiceClient(config))
			sd, err := gw.GetByDomain(context.Background(), "example.com")
			if err != nil {
				return fmt.Errorf("GetByDomain failed: %w", err)
			}
			require.NotNil(t, sd)
			assert.True(t, sd.AllowFetchBody)
			require.NotNil(t, sd.RobotsCrawlDelaySec)
			assert.Equal(t, 5, *sd.RobotsCrawlDelaySec)
			assert.Equal(t, []string{"/private"}, sd.RobotsDisallowPaths)
			return nil
		})
	require.NoError(t, err)
}

// TestGetScrapingDomainByDomainUnknownContract: "never seen" must stay
// distinguishable from "recorded as permissive". The compliance check falls
// back to a live robots.txt fetch on nil and would skip that fallback if the
// provider synthesised a default row.
func TestGetScrapingDomainByDomainUnknownContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has no scraping domain for the host").
		UponReceiving("a GetScrapingDomainByDomain request from alt-backend for an unknown host").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/services.datahub.v1.DataHubService/GetScrapingDomainByDomain"),
			Headers: jsonHeaders(),
			Body:    map[string]interface{}{"domain": "unknown.example"},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body:    matchers.MapMatcher{},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewScrapingDomainGateway(newDataHubServiceClient(config))
			sd, err := gw.GetByDomain(context.Background(), "unknown.example")
			if err != nil {
				return fmt.Errorf("GetByDomain failed: %w", err)
			}
			assert.Nil(t, sd)
			return nil
		})
	require.NoError(t, err)
}

func TestGetScrapingDomainByIDContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has a scraping domain").
		UponReceiving("a GetScrapingDomainByID request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/services.datahub.v1.DataHubService/GetScrapingDomainByID"),
			Headers: jsonHeaders(),
			Body:    map[string]interface{}{"id": "2b1c3d4e-5f60-4711-8899-aabbccddeeff"},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body: matchers.MapMatcher{
				"scrapingDomain": matchers.Like(map[string]interface{}{
					"id":     matchers.Regex("2b1c3d4e-5f60-4711-8899-aabbccddeeff", uuidLikePattern),
					"domain": matchers.Like("example.com"),
					"scheme": matchers.Like("https"),
				}),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewScrapingDomainGateway(newDataHubServiceClient(config))
			id, err := parseTestUUID("2b1c3d4e-5f60-4711-8899-aabbccddeeff")
			if err != nil {
				return err
			}
			sd, err := gw.GetByID(context.Background(), id)
			if err != nil {
				return fmt.Errorf("GetByID failed: %w", err)
			}
			require.NotNil(t, sd)
			assert.Equal(t, "example.com", sd.Domain)
			return nil
		})
	require.NoError(t, err)
}

func TestSaveDeclinedDomainContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub accepts declined domain writes").
		UponReceiving("a SaveDeclinedDomain request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/services.datahub.v1.DataHubService/SaveDeclinedDomain"),
			Headers: jsonHeaders(),
			Body: map[string]interface{}{
				"userId": "11111111-2222-3333-4444-555555555555",
				"domain": "paywalled.example",
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body:    matchers.MapMatcher{},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewDeclinedDomainGateway(newDataHubServiceClient(config))
			return gw.SaveDeclinedDomain(context.Background(),
				"11111111-2222-3333-4444-555555555555", "paywalled.example")
		})
	require.NoError(t, err)
}

func TestIsDomainDeclinedContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has a declined domain for the user").
		UponReceiving("an IsDomainDeclined request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/services.datahub.v1.DataHubService/IsDomainDeclined"),
			Headers: jsonHeaders(),
			Body: map[string]interface{}{
				"userId": "11111111-2222-3333-4444-555555555555",
				"domain": "paywalled.example",
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body:    matchers.MapMatcher{"declined": matchers.Like(true)},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewDeclinedDomainGateway(newDataHubServiceClient(config))
			declined, err := gw.IsDomainDeclined(context.Background(),
				"11111111-2222-3333-4444-555555555555", "paywalled.example")
			if err != nil {
				return fmt.Errorf("IsDomainDeclined failed: %w", err)
			}
			assert.True(t, declined)
			return nil
		})
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// §2.O Automatic full-text fetch groundwork
// ---------------------------------------------------------------------------

func TestListSubscribedUserIDsByFeedLinkIDContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has subscribers for the feed link").
		UponReceiving("a ListSubscribedUserIDsByFeedLinkID request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/services.datahub.v1.DataHubService/ListSubscribedUserIDsByFeedLinkID"),
			Headers: jsonHeaders(),
			Body:    map[string]interface{}{"feedLinkId": "33333333-4444-5555-6666-777777777777"},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body: matchers.MapMatcher{
				"userIds": matchers.EachLike("11111111-2222-3333-4444-555555555555", 1),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewAutoFulltextGateway(newDataHubServiceClient(config))
			ids, err := gw.ListSubscribedUserIDsByFeedLinkID(context.Background(),
				"33333333-4444-5555-6666-777777777777")
			if err != nil {
				return fmt.Errorf("ListSubscribedUserIDsByFeedLinkID failed: %w", err)
			}
			require.Len(t, ids, 1)
			return nil
		})
	require.NoError(t, err)
}

func TestCheckArticleExistsByURLForUserContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has the article for the user").
		UponReceiving("a CheckArticleExistsByURLForUser request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/services.datahub.v1.DataHubService/CheckArticleExistsByURLForUser"),
			Headers: jsonHeaders(),
			Body: map[string]interface{}{
				"url":    "https://example.com/post",
				"userId": "11111111-2222-3333-4444-555555555555",
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body: matchers.MapMatcher{
				"exists":    matchers.Like(true),
				"articleId": matchers.Regex("6f1a2f7e-1f1e-4c2a-9a3e-5b6c7d8e9f01", uuidLikePattern),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewAutoFulltextGateway(newDataHubServiceClient(config))
			exists, articleID, err := gw.CheckArticleExistsByURLForUser(context.Background(),
				"https://example.com/post", "11111111-2222-3333-4444-555555555555")
			if err != nil {
				return fmt.Errorf("CheckArticleExistsByURLForUser failed: %w", err)
			}
			assert.True(t, exists)
			assert.NotEmpty(t, articleID)
			return nil
		})
	require.NoError(t, err)
}
