//go:build contract

// Pact CDC: alt-backend → alt-data-hub, article capabilities.
//
// ADR-000954 Wave 3 batch 2, capability catalog §2.B / §2.C / §2.N — the
// article archive, the reads every article-serving surface makes, and the
// alt_db half of the knowledge backfill jobs.
//
// The thing these interactions exist to pin, over and above "the procedure
// answers", is `userId` on the wire. In-process the driver read the signed-in
// user out of the Go context; across this boundary the peer certificate says
// "alt-backend" and nothing about whose article this is, so the field is the
// only carrier of tenancy. A provider that stopped reading it, or a consumer
// that stopped sending it, would keep every test in both processes green and
// serve one user another user's articles.
//
// Same file, same pacticipant as backend_consumer_test.go — one consumer, one
// pact — split only so the two batches are readable apart.
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

const (
	testUserID    = "11111111-2222-3333-4444-555555555555"
	testArticleID = "6f1a2f7e-1f1e-4c2a-9a3e-5b6c7d8e9f01"
	testFeedID    = "33333333-4444-5555-6666-777777777777"
)

// authedContext carries a signed-in user, which is what the gateways read to
// fill `userId`. Without it they either send an empty scope or refuse, and
// both are behaviours these tests need to distinguish.
func authedContext(userID string) context.Context {
	return domain.SetUserContext(context.Background(), &domain.UserContext{
		UserID:    uuid.MustParse(userID),
		Email:     "contract@example.com",
		Role:      domain.UserRoleUser,
		TenantID:  uuid.MustParse(userID),
		LoginAt:   time.Now(),
		ExpiresAt: time.Now().Add(time.Hour),
	})
}

// ---------------------------------------------------------------------------
// §2.B Article writes
// ---------------------------------------------------------------------------

// TestArchiveArticleContract pins the one write in this batch.
//
// `created` is matched because the caller uses it to tell a first sighting
// from a re-fetch, and it comes from the upsert's `xmax = 0` — a value only
// the provider's transaction can produce.
func TestArchiveArticleContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub accepts article archives").
		UponReceiving("an ArchiveArticle request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/alt.datahub.v1.DataHubService/ArchiveArticle"),
			Headers: jsonHeaders(),
			Body: map[string]interface{}{
				"url":     "https://example.com/post",
				"title":   "Example",
				"content": "body text",
				// The owner is a field, not an ambient value. See the package
				// comment: this is the only thing carrying tenancy here.
				"userId": testUserID,
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body: matchers.MapMatcher{
				"articleId": matchers.Regex(testArticleID, uuidLikePattern),
				"created":   matchers.Like(true),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewArticleStoreGateway(newDataHubServiceClient(config))
			articleID, err := gw.SaveArticle(authedContext(testUserID), "https://example.com/post", "Example", "body text")
			if err != nil {
				return fmt.Errorf("SaveArticle failed: %w", err)
			}
			assert.Equal(t, testArticleID, articleID)
			return nil
		})
	require.NoError(t, err)
}

// TestArchiveArticleWithoutUserContract records that the gateway refuses
// before it reaches the network.
//
// There is no interaction here on purpose: an archive with no owner is not a
// request the provider should ever have to reject, because articles is keyed
// by (url, user_id) and a write with the zero owner lands on a row no
// tenant-scoped query can read back.
func TestArchiveArticleWithoutUserContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub accepts article archives").
		UponReceiving("a GetArticleByURL request from alt-backend with no signed-in user").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/alt.datahub.v1.DataHubService/GetArticleByURL"),
			Headers: jsonHeaders(),
			// userId omitted: the unscoped lookup is a different query, and
			// the provider treats the empty field as the caller asking for it.
			Body: map[string]interface{}{"url": "https://example.com/anon"},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body:    matchers.MapMatcher{},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			client := newDataHubServiceClient(config)

			store := datahub_gateway.NewArticleStoreGateway(client)
			if _, err := store.SaveArticle(context.Background(), "https://example.com/post", "Example", "body"); err == nil {
				return fmt.Errorf("SaveArticle with no user context must fail before the request is sent")
			}

			article, err := store.FetchArticleByURL(context.Background(), "https://example.com/anon")
			if err != nil {
				return fmt.Errorf("FetchArticleByURL failed: %w", err)
			}
			assert.Nil(t, article)
			return nil
		})
	require.NoError(t, err)
}

func TestSaveArticleHeadContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub accepts article head writes").
		UponReceiving("a SaveArticleHead request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/alt.datahub.v1.DataHubService/SaveArticleHead"),
			Headers: jsonHeaders(),
			Body: map[string]interface{}{
				"articleId": testArticleID,
				// head_html is NOT NULL; the placeholder is what a caller with
				// an image but no markup worth keeping sends.
				"headHtml":   "<head></head>",
				"ogImageUrl": "https://cdn.example.com/og.png",
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body:    matchers.MapMatcher{},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewOgImageGateway(newDataHubServiceClient(config))
			if err := gw.SaveArticleHead(context.Background(), testArticleID, "<head></head>", "https://cdn.example.com/og.png"); err != nil {
				return fmt.Errorf("SaveArticleHead failed: %w", err)
			}
			return nil
		})
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// §2.C Article reads
// ---------------------------------------------------------------------------

func TestGetArticleByURLContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has the article for the user").
		UponReceiving("a GetArticleByURL request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/alt.datahub.v1.DataHubService/GetArticleByURL"),
			Headers: jsonHeaders(),
			Body: map[string]interface{}{
				"url":    "https://example.com/post",
				"userId": testUserID,
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body: matchers.MapMatcher{
				"article": matchers.Like(map[string]interface{}{
					"id":      matchers.Regex(testArticleID, uuidLikePattern),
					"title":   matchers.Like("Example"),
					"content": matchers.Like("body text"),
					"url":     matchers.Like("https://example.com/post"),
					"feedId":  matchers.Regex(testFeedID, uuidLikePattern),
				}),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewArticleStoreGateway(newDataHubServiceClient(config))
			article, err := gw.FetchArticleByURL(authedContext(testUserID), "https://example.com/post")
			if err != nil {
				return fmt.Errorf("FetchArticleByURL failed: %w", err)
			}
			require.NotNil(t, article)
			assert.Equal(t, testArticleID, article.ID)
			return nil
		})
	require.NoError(t, err)
}

// TestGetArticleByURLMissContract pins the absent-field encoding of "not
// archived". The fetch usecase reads that absence as "go get the page", so an
// empty ArticleContent here would make it stop fetching.
func TestGetArticleByURLMissContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has no article for the url").
		UponReceiving("a GetArticleByURL request from alt-backend for an unarchived url").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/alt.datahub.v1.DataHubService/GetArticleByURL"),
			Headers: jsonHeaders(),
			Body: map[string]interface{}{
				"url":    "https://example.com/never-fetched",
				"userId": testUserID,
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			// No "article" key: protojson omits an unset message rather than
			// emitting null.
			Body: matchers.MapMatcher{},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewArticleStoreGateway(newDataHubServiceClient(config))
			article, err := gw.FetchArticleByURL(authedContext(testUserID), "https://example.com/never-fetched")
			if err != nil {
				return fmt.Errorf("FetchArticleByURL failed: %w", err)
			}
			assert.Nil(t, article, "an unarchived url must be nil-without-error, not an empty struct")
			return nil
		})
	require.NoError(t, err)
}

func TestBatchGetArticlesByURLsContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has the article for the user").
		UponReceiving("a BatchGetArticlesByURLs request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/alt.datahub.v1.DataHubService/BatchGetArticlesByURLs"),
			Headers: jsonHeaders(),
			Body: map[string]interface{}{
				"urls":   []string{"https://example.com/post"},
				"userId": testUserID,
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body: matchers.MapMatcher{
				// Keyed by URL. A requested URL with no archived article is
				// absent from this map, which is how the caller decides what
				// still needs fetching.
				"articles": matchers.Like(map[string]interface{}{
					"https://example.com/post": matchers.Like(map[string]interface{}{
						"id":      matchers.Regex(testArticleID, uuidLikePattern),
						"title":   matchers.Like("Example"),
						"content": matchers.Like("body text"),
						"url":     matchers.Like("https://example.com/post"),
					}),
				}),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewArticleStoreGateway(newDataHubServiceClient(config))
			articles, err := gw.FetchArticlesByURLs(authedContext(testUserID), []string{"https://example.com/post"})
			if err != nil {
				return fmt.Errorf("FetchArticlesByURLs failed: %w", err)
			}
			require.Len(t, articles, 1)
			assert.Equal(t, testArticleID, articles["https://example.com/post"].ID)
			return nil
		})
	require.NoError(t, err)
}

func TestGetArticleContentByIDContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has the article for the user").
		UponReceiving("a GetArticleContentByID request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/alt.datahub.v1.DataHubService/GetArticleContentByID"),
			Headers: jsonHeaders(),
			Body:    map[string]interface{}{"articleId": testArticleID},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body: matchers.MapMatcher{
				"article": matchers.Like(map[string]interface{}{
					"id":      matchers.Regex(testArticleID, uuidLikePattern),
					"title":   matchers.Like("Example"),
					"content": matchers.Like("body text"),
					// The URL is why this is not GetArticleByID: that
					// procedure returns tags and timestamps and no url.
					"url": matchers.Like("https://example.com/post"),
				}),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewArticleStoreGateway(newDataHubServiceClient(config))
			article, err := gw.FetchArticleByID(context.Background(), testArticleID)
			if err != nil {
				return fmt.Errorf("FetchArticleByID failed: %w", err)
			}
			require.NotNil(t, article)
			assert.Equal(t, "https://example.com/post", article.URL)
			return nil
		})
	require.NoError(t, err)
}

func TestListArticlesCursorContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has articles for the user").
		UponReceiving("a ListArticlesCursor request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/alt.datahub.v1.DataHubService/ListArticlesCursor"),
			Headers: jsonHeaders(),
			Body: map[string]interface{}{
				"userId": testUserID,
				"limit":  20,
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body: matchers.MapMatcher{
				"articles": matchers.EachLike(map[string]interface{}{
					"id":          matchers.Regex(testArticleID, uuidLikePattern),
					"feedId":      matchers.Regex(testFeedID, uuidLikePattern),
					"title":       matchers.Like("Example"),
					"content":     matchers.Like("body text"),
					"url":         matchers.Like("https://example.com/post"),
					"tags":        matchers.EachLike("go", 1),
					"publishedAt": matchers.Like("2026-07-31T00:00:00Z"),
					"createdAt":   matchers.Like("2026-07-31T00:00:00Z"),
				}, 1),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewArticleCursorGateway(newDataHubServiceClient(config))
			articles, err := gw.FetchArticlesWithCursor(authedContext(testUserID), nil, 20)
			if err != nil {
				return fmt.Errorf("FetchArticlesWithCursor failed: %w", err)
			}
			require.Len(t, articles, 1)
			assert.Equal(t, []string{"go"}, articles[0].Tags)
			return nil
		})
	require.NoError(t, err)
}

func TestListArticleIDsCursorContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has articles for the user").
		UponReceiving("a ListArticleIDsCursor request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/alt.datahub.v1.DataHubService/ListArticleIDsCursor"),
			Headers: jsonHeaders(),
			Body: map[string]interface{}{
				"userId": testUserID,
				"cursor": "2026-07-31T00:00:00Z",
				"limit":  20,
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body: matchers.MapMatcher{
				"articleIds": matchers.EachLike(matchers.Regex(testArticleID, uuidLikePattern), 1),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			cursor := time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC)
			gw := datahub_gateway.NewArticleCursorGateway(newDataHubServiceClient(config))
			ids, err := gw.FetchArticleIDsWithCursor(authedContext(testUserID), &cursor, 20)
			if err != nil {
				return fmt.Errorf("FetchArticleIDsWithCursor failed: %w", err)
			}
			require.Len(t, ids, 1)
			return nil
		})
	require.NoError(t, err)
}

func TestBatchGetArticlesByIDsContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has articles for the user").
		UponReceiving("a BatchGetArticlesByIDs request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/alt.datahub.v1.DataHubService/BatchGetArticlesByIDs"),
			Headers: jsonHeaders(),
			Body:    map[string]interface{}{"articleIds": []string{testArticleID}},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body: matchers.MapMatcher{
				"articles": matchers.EachLike(map[string]interface{}{
					"id":        matchers.Regex(testArticleID, uuidLikePattern),
					"feedId":    matchers.Regex(testFeedID, uuidLikePattern),
					"title":     matchers.Like("Example"),
					"content":   matchers.Like("body text"),
					"url":       matchers.Like("https://example.com/post"),
					"createdAt": matchers.Like("2026-07-31T00:00:00Z"),
				}, 1),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewArticleBatchGateway(newDataHubServiceClient(config))
			articles, err := gw.FetchArticlesByIDs(context.Background(), []uuid.UUID{uuid.MustParse(testArticleID)})
			if err != nil {
				return fmt.Errorf("FetchArticlesByIDs failed: %w", err)
			}
			require.Len(t, articles, 1)
			assert.Equal(t, uuid.MustParse(testArticleID), articles[0].ID)
			return nil
		})
	require.NoError(t, err)
}

func TestGetLatestArticleByFeedIDContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has articles for the feed").
		UponReceiving("a GetLatestArticleByFeedID request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/alt.datahub.v1.DataHubService/GetLatestArticleByFeedID"),
			Headers: jsonHeaders(),
			Body:    map[string]interface{}{"feedId": testFeedID},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body: matchers.MapMatcher{
				"article": matchers.Like(map[string]interface{}{
					"id":      matchers.Regex(testArticleID, uuidLikePattern),
					"title":   matchers.Like("Example"),
					"content": matchers.Like("body text"),
					"url":     matchers.Like("https://example.com/post"),
				}),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewLatestArticleGateway(newDataHubServiceClient(config))
			article, err := gw.FetchLatestArticleByFeedID(context.Background(), uuid.MustParse(testFeedID))
			if err != nil {
				return fmt.Errorf("FetchLatestArticleByFeedID failed: %w", err)
			}
			require.NotNil(t, article)
			return nil
		})
	require.NoError(t, err)
}

// TestLookupArticleURLContract pins the tenant-scoped URL resolution the
// Knowledge Trail's Open affordance depends on.
func TestLookupArticleURLContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has the article for the user").
		UponReceiving("a LookupArticleURL request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/alt.datahub.v1.DataHubService/LookupArticleURL"),
			Headers: jsonHeaders(),
			Body: map[string]interface{}{
				"articleId": testArticleID,
				"userId":    testUserID,
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body: matchers.MapMatcher{
				// An article outside the tenant answers "" rather than a
				// NotFound code, so that the response cannot be used to
				// confirm another tenant's article exists.
				"url": matchers.Like("https://example.com/post"),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewArticleURLLookupGateway(newDataHubServiceClient(config))
			url, err := gw.LookupArticleURL(context.Background(), testArticleID, uuid.MustParse(testUserID))
			if err != nil {
				return fmt.Errorf("LookupArticleURL failed: %w", err)
			}
			assert.Equal(t, "https://example.com/post", url)
			return nil
		})
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// §2.N Knowledge backfill
// ---------------------------------------------------------------------------

func TestCountBackfillArticlesContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has historic articles to replay").
		UponReceiving("a CountBackfillArticles request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/alt.datahub.v1.DataHubService/CountBackfillArticles"),
			Headers: jsonHeaders(),
			Body:    map[string]interface{}{},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			// int32, so a JSON number. The int64 counts elsewhere on this
			// service are strings; the difference is protojson's, not a
			// choice made per procedure.
			Body: matchers.MapMatcher{"count": matchers.Like(4242)},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewKnowledgeBackfillGateway(newDataHubServiceClient(config))
			count, err := gw.CountBackfillArticles(context.Background())
			if err != nil {
				return fmt.Errorf("CountBackfillArticles failed: %w", err)
			}
			assert.Equal(t, 4242, count)
			return nil
		})
	require.NoError(t, err)
}

// TestListBackfillArticlesContract sends a full keyset cursor, which is the
// only shape the provider accepts past the first page: half a cursor restarts
// the walk and re-emits every knowledge event the job already emitted.
func TestListBackfillArticlesContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has historic articles to replay").
		UponReceiving("a ListBackfillArticles request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/alt.datahub.v1.DataHubService/ListBackfillArticles"),
			Headers: jsonHeaders(),
			Body: map[string]interface{}{
				"lastCreatedAt": "2026-01-02T03:04:05Z",
				"lastArticleId": testArticleID,
				"limit":         100,
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body: matchers.MapMatcher{
				"articles": matchers.EachLike(map[string]interface{}{
					"articleId":   matchers.Regex(testArticleID, uuidLikePattern),
					"userId":      matchers.Regex(testUserID, uuidLikePattern),
					"createdAt":   matchers.Like("2026-01-02T03:04:05Z"),
					"publishedAt": matchers.Like("2026-01-02T03:04:05Z"),
					"title":       matchers.Like("Example"),
					"url":         matchers.Like("https://example.com/post"),
				}, 1),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			cursor := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
			articleID := uuid.MustParse(testArticleID)
			gw := datahub_gateway.NewKnowledgeBackfillGateway(newDataHubServiceClient(config))
			articles, err := gw.ListBackfillArticles(context.Background(), &cursor, &articleID, 100)
			if err != nil {
				return fmt.Errorf("ListBackfillArticles failed: %w", err)
			}
			require.Len(t, articles, 1)
			assert.Equal(t, articleID, articles[0].ArticleID)
			return nil
		})
	require.NoError(t, err)
}

func TestCountBackfillSummaryTitlesContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has summary versions to replay").
		UponReceiving("a CountBackfillSummaryTitles request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/alt.datahub.v1.DataHubService/CountBackfillSummaryTitles"),
			Headers: jsonHeaders(),
			Body:    map[string]interface{}{},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body:    matchers.MapMatcher{"count": matchers.Like(7)},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewKnowledgeBackfillGateway(newDataHubServiceClient(config))
			count, err := gw.CountBackfillSummaryTitles(context.Background())
			if err != nil {
				return fmt.Errorf("CountBackfillSummaryTitles failed: %w", err)
			}
			assert.Equal(t, 7, count)
			return nil
		})
	require.NoError(t, err)
}

func TestListBackfillSummaryTitlesContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has summary versions to replay").
		UponReceiving("a ListBackfillSummaryTitles request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/alt.datahub.v1.DataHubService/ListBackfillSummaryTitles"),
			Headers: jsonHeaders(),
			Body: map[string]interface{}{
				"lastGeneratedAt":      "2026-03-04T05:06:07Z",
				"lastSummaryVersionId": "22222222-3333-4444-5555-666666666666",
				"limit":                100,
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body: matchers.MapMatcher{
				"entries": matchers.EachLike(map[string]interface{}{
					"summaryVersionId": matchers.Regex("22222222-3333-4444-5555-666666666666", uuidLikePattern),
					"articleId":        matchers.Regex(testArticleID, uuidLikePattern),
					"userId":           matchers.Regex(testUserID, uuidLikePattern),
					// A separate column from userId even though the query
					// produces the same value today, so it is a separate field
					// rather than an alias the consumer has to know about.
					"tenantId":    matchers.Regex(testUserID, uuidLikePattern),
					"title":       matchers.Like("Example"),
					"generatedAt": matchers.Like("2026-03-04T05:06:07Z"),
				}, 1),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			cursor := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
			versionID := uuid.MustParse("22222222-3333-4444-5555-666666666666")
			gw := datahub_gateway.NewKnowledgeBackfillGateway(newDataHubServiceClient(config))
			entries, err := gw.ListBackfillSummaryTitles(context.Background(), &cursor, &versionID, 100)
			if err != nil {
				return fmt.Errorf("ListBackfillSummaryTitles failed: %w", err)
			}
			require.Len(t, entries, 1)
			assert.Equal(t, versionID, entries[0].SummaryVersionID)
			return nil
		})
	require.NoError(t, err)
}
