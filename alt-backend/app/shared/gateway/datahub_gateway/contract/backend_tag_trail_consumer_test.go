//go:build contract

// Pact CDC: alt-backend → alt-data-hub, the last two capabilities.
//
// ADR-000954 Wave 3 batch 6, capability catalog §2.J (Tag Trail paging) and
// §2.C (the recall rail's article fallback). After these two, alt-backend has
// no database pool at all — which is why the two properties pinned here are
// the ones a compiler cannot see:
//
//   - the cursor. Both Tag Trail procedures page by an exclusive published_at
//     bound, and a provider that ignored it would return the first page for
//     every request. Every test in both processes would stay green and the
//     Trail would loop on its first screen forever.
//   - `found`. GetArticleTitleAndLink answers a deleted article with
//     found=false and HTTP 200. proto3 gives an unset string and an empty
//     string the same bytes, so if the provider stopped sending the flag the
//     consumer would read every miss as an article titled "", and the rail
//     renders those as real items.
//
// Same pacticipant and same pact file as the other alt-backend consumer tests
// — one consumer, one pact — split only so the batches read apart.
package contract

import (
	"context"
	"fmt"
	"testing"
	"time"

	"alt/shared/gateway/datahub_gateway"

	"github.com/pact-foundation/pact-go/v2/consumer"
	"github.com/pact-foundation/pact-go/v2/matchers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testTrailTagID    = "8c9d0e1f-2a3b-4c5d-8e9f-0a1b2c3d4e5f"
	testTrailCursorAt = "2026-07-31T00:00:00Z"
)

func trailCursor(t *testing.T) *time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, testTrailCursorAt)
	require.NoError(t, err)
	return &parsed
}

// ---------------------------------------------------------------------------
// §2.J Tag Trail paging
// ---------------------------------------------------------------------------

// TestListArticlesByTagIDContract pins the paged read the Tag Trail makes.
//
// The cursor is in the request body as a literal rather than a matcher: this
// interaction exists to record that the consumer *sends* one. A Like matcher
// would let a consumer that dropped the field verify green, which is precisely
// the regression that would pin the Trail to its first page.
func TestListArticlesByTagIDContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has articles carrying the feed tag").
		UponReceiving("a ListArticlesByTagID request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/services.datahub.v1.DataHubService/ListArticlesByTagID"),
			Headers: jsonHeaders(),
			Body: map[string]interface{}{
				"tagId":  testTrailTagID,
				"cursor": testTrailCursorAt,
				"limit":  20,
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body: matchers.MapMatcher{
				"articles": matchers.EachLike(map[string]interface{}{
					"id":          matchers.Regex(testArticleID, uuidLikePattern),
					"title":       matchers.Like("Tagged article"),
					"url":         matchers.Like("https://example.com/tagged"),
					"publishedAt": matchers.Like("2026-07-30T08:00:00Z"),
					"feedId":      matchers.Regex(testFeedID, uuidLikePattern),
					"feedTitle":   matchers.Like("Example Feed"),
				}, 1),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewTagGateway(newDataHubServiceClient(config))
			articles, err := gw.FetchArticlesByTag(context.Background(), testTrailTagID, trailCursor(t), 20)
			if err != nil {
				return fmt.Errorf("FetchArticlesByTag failed: %w", err)
			}
			require.Len(t, articles, 1)
			assert.Equal(t, testArticleID, articles[0].ID)
			assert.Equal(t, "https://example.com/tagged", articles[0].Link)
			// feed_title travels. The Trail names the source beside every row,
			// and resolving it consumer-side would be a second round trip per
			// article.
			assert.Equal(t, "Example Feed", articles[0].FeedTitle)
			return nil
		})
	require.NoError(t, err)
}

// TestListArticlesByTagIDFirstPageContract records the first page: no cursor
// key at all, because protojson omits an unset timestamp rather than emitting
// null. A provider that required the field would reject every Trail opening.
func TestListArticlesByTagIDFirstPageContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has articles carrying the feed tag").
		UponReceiving("a ListArticlesByTagID request from alt-backend for the first page").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/services.datahub.v1.DataHubService/ListArticlesByTagID"),
			Headers: jsonHeaders(),
			Body: map[string]interface{}{
				"tagId": testTrailTagID,
				"limit": 20,
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body:    matchers.MapMatcher{},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewTagGateway(newDataHubServiceClient(config))
			articles, err := gw.FetchArticlesByTag(context.Background(), testTrailTagID, nil, 20)
			if err != nil {
				return fmt.Errorf("FetchArticlesByTag failed: %w", err)
			}
			assert.Empty(t, articles, "an empty page is an empty slice, not an error")
			return nil
		})
	require.NoError(t, err)
}

// TestListArticlesByTagNameContract is the cross-feed half. It is a separate
// procedure rather than a flag on the one above, and this interaction is what
// keeps that true across a deploy.
func TestListArticlesByTagNameContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has articles carrying the tag name across feeds").
		UponReceiving("a ListArticlesByTagName request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/services.datahub.v1.DataHubService/ListArticlesByTagName"),
			Headers: jsonHeaders(),
			Body: map[string]interface{}{
				"tagName": "golang",
				"cursor":  testTrailCursorAt,
				"limit":   20,
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body: matchers.MapMatcher{
				"articles": matchers.EachLike(map[string]interface{}{
					"id":          matchers.Regex(testArticleID, uuidLikePattern),
					"title":       matchers.Like("Cross-feed article"),
					"url":         matchers.Like("https://example.com/cross"),
					"publishedAt": matchers.Like("2026-07-30T08:00:00Z"),
				}, 1),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewTagGateway(newDataHubServiceClient(config))
			articles, err := gw.FetchArticlesByTagName(context.Background(), "golang", trailCursor(t), 20)
			if err != nil {
				return fmt.Errorf("FetchArticlesByTagName failed: %w", err)
			}
			require.Len(t, articles, 1)
			assert.Equal(t, "Cross-feed article", articles[0].Title)
			// No feed on this row and that is legal: the cross-feed query does
			// not always resolve a feed title, and the absent fields must not
			// become an error.
			assert.Empty(t, articles[0].FeedTitle)
			return nil
		})
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// §2.C Article reference for the recall rail
// ---------------------------------------------------------------------------

func TestGetArticleTitleAndLinkContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has the article").
		UponReceiving("a GetArticleTitleAndLink request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/services.datahub.v1.DataHubService/GetArticleTitleAndLink"),
			Headers: jsonHeaders(),
			Body:    map[string]interface{}{"articleId": testArticleID},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body: matchers.MapMatcher{
				"found":       matchers.Like(true),
				"title":       matchers.Like("Recalled article"),
				"url":         matchers.Like("https://example.com/recalled"),
				"publishedAt": matchers.Like("2026-06-01T07:30:00Z"),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewArticleRefGateway(newDataHubServiceClient(config))
			title, link, publishedAt, err := gw.GetArticleTitleAndLink(context.Background(), testArticleID)
			if err != nil {
				return fmt.Errorf("GetArticleTitleAndLink failed: %w", err)
			}
			assert.Equal(t, "Recalled article", title)
			assert.Equal(t, "https://example.com/recalled", link)
			require.NotNil(t, publishedAt)
			return nil
		})
	require.NoError(t, err)
}

// TestGetArticleTitleAndLinkMissContract is the interaction the `found` field
// exists for.
//
// The response carries no title, no url and no found key — protojson omits a
// false bool — and the consumer must read that as "nothing to render" rather
// than as an article with an empty name. The rail skips items with no title,
// so getting this wrong shows up as blank rows rather than as an error.
func TestGetArticleTitleAndLinkMissContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has no article with that id").
		UponReceiving("a GetArticleTitleAndLink request from alt-backend for a deleted article").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/services.datahub.v1.DataHubService/GetArticleTitleAndLink"),
			Headers: jsonHeaders(),
			Body:    map[string]interface{}{"articleId": "00000000-0000-4000-8000-000000000000"},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body:    matchers.MapMatcher{},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewArticleRefGateway(newDataHubServiceClient(config))
			title, link, publishedAt, err := gw.GetArticleTitleAndLink(context.Background(), "00000000-0000-4000-8000-000000000000")
			if err != nil {
				return fmt.Errorf("GetArticleTitleAndLink failed: %w", err)
			}
			assert.Empty(t, title, "a miss must be an empty title and no error")
			assert.Empty(t, link)
			assert.Nil(t, publishedAt)
			return nil
		})
	require.NoError(t, err)
}
