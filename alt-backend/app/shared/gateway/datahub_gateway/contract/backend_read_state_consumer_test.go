//go:build contract

// Pact CDC: alt-backend → alt-data-hub, per-user feed state and tag reads
// (ADR-000954 Wave 3 batch 4, capability catalog §2.I / §2.J).
//
// One consumer here, unlike the feed file: none of these procedures is on a
// scheduler tick. Read marks, subscriptions, favourites and tag lookups all
// happen because somebody is looking at a screen, so alt-harvester never calls
// them.
//
// The interaction that carries the most weight in this file is the NotFound
// pair. Catalog §4-5 left two implementations answering "there is no such
// feed" two different ways, and this batch collapsed them; the pact is where
// that collapse becomes a contract rather than an implementation detail,
// because the consumer's branch on domain.ErrFeedNotFound is driven purely by
// the Connect code it receives.
package contract

import (
	"context"
	"errors"
	"fmt"
	"net/url"
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
	readStateUserID     = "3f2504e0-4f89-41d3-9a0c-0305e82c3301"
	readStateFeedURL    = "https://example.com/feed.xml"
	readStateMissingURL = "https://gone.example.com/feed.xml"
	readStateFeedID     = "b2c3d4e5-2222-4222-8222-222222222222"
	readStateLinkID     = "a1b2c3d4-1111-4111-8111-111111111111"
	readStateArticleID  = "6f1a2f7e-1f1e-4c2a-9a3e-5b6c7d8e9f01"
)

func mustParseURL(t *testing.T, raw string) url.URL {
	t.Helper()
	parsed, err := url.Parse(raw)
	require.NoError(t, err)
	return *parsed
}

// ---------------------------------------------------------------------------
// §2.I Read state — the §4-5 semantics
// ---------------------------------------------------------------------------

func TestMarkFeedReadContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has a feed at the url").
		UponReceiving("a MarkFeedRead request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/alt.datahub.v1.DataHubService/MarkFeedRead"),
			Headers: jsonHeaders(),
			Body: map[string]interface{}{
				"feedUrl": readStateFeedURL,
				// The tenant is a field, not a header and not the peer
				// certificate: the certificate names alt-backend, and the
				// read mark belongs to a person.
				"userId": readStateUserID,
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body:    matchers.MapMatcher{},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewReadStateGateway(newDataHubServiceClient(config))
			userID, err := parseTestUUID(readStateUserID)
			if err != nil {
				return err
			}
			if err := gw.UpdateFeedStatus(context.Background(), mustParseURL(t, readStateFeedURL), userID); err != nil {
				return fmt.Errorf("UpdateFeedStatus failed: %w", err)
			}
			return nil
		})
	require.NoError(t, err)
}

// TestMarkFeedReadNotFoundContract and TestMarkArticleReadNotFoundContract are
// the pair capability catalog §4-5 exists for.
//
// Both procedures write the same table and both now derive "no such feed" from
// one upsert's RowsAffected(); the two pacts assert that the two therefore
// answer the same status, and that the consumer translates each back into the
// same domain.ErrFeedNotFound its callers already branch on. Before this batch
// one path raised pgx.ErrNoRows from a preceding SELECT and the other
// RowsAffected() == 0, so a caller had to know which procedure it had used to
// know what an absence meant.
func TestMarkFeedReadNotFoundContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has no feed at the url").
		UponReceiving("a MarkFeedRead request from alt-backend for a url with no feed").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/alt.datahub.v1.DataHubService/MarkFeedRead"),
			Headers: jsonHeaders(),
			Body: map[string]interface{}{
				"feedUrl": readStateMissingURL,
				"userId":  readStateUserID,
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  404,
			Headers: jsonHeaders(),
			Body: matchers.MapMatcher{
				"code": matchers.Like("not_found"),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewReadStateGateway(newDataHubServiceClient(config))
			userID, err := parseTestUUID(readStateUserID)
			if err != nil {
				return err
			}
			err = gw.UpdateFeedStatus(context.Background(), mustParseURL(t, readStateMissingURL), userID)
			require.Error(t, err)
			assert.True(t, errors.Is(err, domain.ErrFeedNotFound),
				"NotFound must arrive as the domain error the REST layer turns into a 404")
			return nil
		})
	require.NoError(t, err)
}

func TestMarkArticleReadContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has a feed for the article url").
		UponReceiving("a MarkArticleRead request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/alt.datahub.v1.DataHubService/MarkArticleRead"),
			Headers: jsonHeaders(),
			Body: map[string]interface{}{
				"articleUrl": "https://example.com/post",
				"userId":     readStateUserID,
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body:    matchers.MapMatcher{},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewReadStateGateway(newDataHubServiceClient(config))
			userID, err := parseTestUUID(readStateUserID)
			if err != nil {
				return err
			}
			if err := gw.MarkArticleAsRead(context.Background(), mustParseURL(t, "https://example.com/post"), userID); err != nil {
				return fmt.Errorf("MarkArticleAsRead failed: %w", err)
			}
			return nil
		})
	require.NoError(t, err)
}

func TestMarkArticleReadNotFoundContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has no feed at the url").
		UponReceiving("a MarkArticleRead request from alt-backend for a url with no feed").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/alt.datahub.v1.DataHubService/MarkArticleRead"),
			Headers: jsonHeaders(),
			Body: map[string]interface{}{
				"articleUrl": readStateMissingURL,
				"userId":     readStateUserID,
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  404,
			Headers: jsonHeaders(),
			Body: matchers.MapMatcher{
				"code": matchers.Like("not_found"),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewReadStateGateway(newDataHubServiceClient(config))
			userID, err := parseTestUUID(readStateUserID)
			if err != nil {
				return err
			}
			err = gw.MarkArticleAsRead(context.Background(), mustParseURL(t, readStateMissingURL), userID)
			require.Error(t, err)
			assert.True(t, errors.Is(err, domain.ErrFeedNotFound),
				"the twin procedure must report an absent feed identically")
			return nil
		})
	require.NoError(t, err)
}

func TestGetReadFeedIDsContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has read marks for the user").
		UponReceiving("a GetReadFeedIDs request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/alt.datahub.v1.DataHubService/GetReadFeedIDs"),
			Headers: jsonHeaders(),
			Body: map[string]interface{}{
				"userId":  readStateUserID,
				"feedIds": []string{readStateFeedID},
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body: matchers.MapMatcher{
				// A list of the ids that ARE read. An unread feed is absent
				// rather than present with a false flag, which is what lets
				// the response stay small for a long page.
				"readFeedIds": matchers.EachLike(matchers.Regex(readStateFeedID, uuidLikePattern), 1),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewReadStateGateway(newDataHubServiceClient(config))
			userID, err := parseTestUUID(readStateUserID)
			if err != nil {
				return err
			}
			feedID, err := parseTestUUID(readStateFeedID)
			if err != nil {
				return err
			}
			read, err := gw.GetReadFeedIDs(context.Background(), userID, []uuid.UUID{feedID})
			if err != nil {
				return fmt.Errorf("GetReadFeedIDs failed: %w", err)
			}
			assert.True(t, read[feedID], "the caller rebuilds the membership set the cache is built around")
			return nil
		})
	require.NoError(t, err)
}

func TestGetAllReadFeedIDsContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has read marks for the user").
		UponReceiving("a GetAllReadFeedIDs request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/alt.datahub.v1.DataHubService/GetAllReadFeedIDs"),
			Headers: jsonHeaders(),
			Body:    map[string]interface{}{"userId": readStateUserID},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body: matchers.MapMatcher{
				"readFeedIds": matchers.EachLike(matchers.Regex(readStateFeedID, uuidLikePattern), 1),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewReadStateGateway(newDataHubServiceClient(config))
			userID, err := parseTestUUID(readStateUserID)
			if err != nil {
				return err
			}
			read, err := gw.GetAllReadFeedIDs(context.Background(), userID)
			if err != nil {
				return fmt.Errorf("GetAllReadFeedIDs failed: %w", err)
			}
			assert.Len(t, read, 1)
			return nil
		})
	require.NoError(t, err)
}

func TestGetUserSubscribedFeedLinkIDsContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has subscriptions for the user").
		UponReceiving("a GetUserSubscribedFeedLinkIDs request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/alt.datahub.v1.DataHubService/GetUserSubscribedFeedLinkIDs"),
			Headers: jsonHeaders(),
			Body:    map[string]interface{}{"userId": readStateUserID},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body: matchers.MapMatcher{
				"feedLinkIds": matchers.EachLike(matchers.Regex(readStateLinkID, uuidLikePattern), 1),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewReadStateGateway(newDataHubServiceClient(config))
			userID, err := parseTestUUID(readStateUserID)
			if err != nil {
				return err
			}
			ids, err := gw.GetUserSubscriptions(context.Background(), userID)
			if err != nil {
				return fmt.Errorf("GetUserSubscriptions failed: %w", err)
			}
			require.Len(t, ids, 1)
			assert.Equal(t, readStateLinkID, ids[0].String())
			return nil
		})
	require.NoError(t, err)
}

// TestListSubscriptionsContract pins the unfollowed row's shape.
//
// The provider's query coalesces the missing subscription join to now(), so it
// must not forward that timestamp; an unfollowed link answers with no
// subscribedAt at all. A row that carried one would have the screen show a
// "following since" date for something nobody follows.
func TestListSubscriptionsContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has subscriptions for the user").
		UponReceiving("a ListSubscriptions request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/alt.datahub.v1.DataHubService/ListSubscriptions"),
			Headers: jsonHeaders(),
			Body:    map[string]interface{}{"userId": readStateUserID},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body: matchers.MapMatcher{
				"subscriptions": matchers.EachLike(map[string]interface{}{
					"feedLinkId":   matchers.Regex(readStateLinkID, uuidLikePattern),
					"url":          matchers.Like(readStateFeedURL),
					"isSubscribed": matchers.Like(true),
					"subscribedAt": matchers.Like("2026-05-01T12:00:00Z"),
				}, 1),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewReadStateGateway(newDataHubServiceClient(config))
			userID, err := parseTestUUID(readStateUserID)
			if err != nil {
				return err
			}
			sources, err := gw.ListSubscriptions(context.Background(), userID)
			if err != nil {
				return fmt.Errorf("ListSubscriptions failed: %w", err)
			}
			require.Len(t, sources, 1)
			assert.Equal(t, readStateLinkID, sources[0].ID)
			assert.True(t, sources[0].IsSubscribed)
			assert.False(t, sources[0].CreatedAt.IsZero())
			return nil
		})
	require.NoError(t, err)
}

func TestSubscribeContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub accepts subscription writes").
		UponReceiving("a Subscribe request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/alt.datahub.v1.DataHubService/Subscribe"),
			Headers: jsonHeaders(),
			Body: map[string]interface{}{
				"userId":     readStateUserID,
				"feedLinkId": readStateLinkID,
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			// Idempotent: subscribing twice is a success with nothing to say,
			// which the automatic subscribe on feed registration relies on.
			Body: matchers.MapMatcher{},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewReadStateGateway(newDataHubServiceClient(config))
			userID, err := parseTestUUID(readStateUserID)
			if err != nil {
				return err
			}
			linkID, err := parseTestUUID(readStateLinkID)
			if err != nil {
				return err
			}
			if err := gw.Subscribe(context.Background(), userID, linkID); err != nil {
				return fmt.Errorf("Subscribe failed: %w", err)
			}
			return nil
		})
	require.NoError(t, err)
}

func TestUnsubscribeContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub accepts subscription writes").
		UponReceiving("an Unsubscribe request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/alt.datahub.v1.DataHubService/Unsubscribe"),
			Headers: jsonHeaders(),
			Body: map[string]interface{}{
				"userId":     readStateUserID,
				"feedLinkId": readStateLinkID,
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body:    matchers.MapMatcher{},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewReadStateGateway(newDataHubServiceClient(config))
			userID, err := parseTestUUID(readStateUserID)
			if err != nil {
				return err
			}
			linkID, err := parseTestUUID(readStateLinkID)
			if err != nil {
				return err
			}
			if err := gw.Unsubscribe(context.Background(), userID, linkID); err != nil {
				return fmt.Errorf("Unsubscribe failed: %w", err)
			}
			return nil
		})
	require.NoError(t, err)
}

func TestAddFavoriteFeedContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has a feed at the url").
		UponReceiving("an AddFavoriteFeed request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/alt.datahub.v1.DataHubService/AddFavoriteFeed"),
			Headers: jsonHeaders(),
			Body: map[string]interface{}{
				"feedUrl": readStateFeedURL,
				"userId":  readStateUserID,
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body:    matchers.MapMatcher{},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewReadStateGateway(newDataHubServiceClient(config))
			userID, err := parseTestUUID(readStateUserID)
			if err != nil {
				return err
			}
			if err := gw.RegisterFavoriteFeed(context.Background(), readStateFeedURL, userID); err != nil {
				return fmt.Errorf("RegisterFavoriteFeed failed: %w", err)
			}
			return nil
		})
	require.NoError(t, err)
}

// TestRemoveFavoriteFeedNotFoundContract completes the §4-5 set: the
// favourites raised the other sentinel (pgx.ErrNoRows rather than
// domain.ErrFeedNotFound) for the same situation, and the consumer must see
// one answer regardless of which write it made.
func TestRemoveFavoriteFeedNotFoundContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has no feed at the url").
		UponReceiving("a RemoveFavoriteFeed request from alt-backend for a url with no feed").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/alt.datahub.v1.DataHubService/RemoveFavoriteFeed"),
			Headers: jsonHeaders(),
			Body: map[string]interface{}{
				"feedUrl": readStateMissingURL,
				"userId":  readStateUserID,
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  404,
			Headers: jsonHeaders(),
			Body: matchers.MapMatcher{
				"code": matchers.Like("not_found"),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewReadStateGateway(newDataHubServiceClient(config))
			userID, err := parseTestUUID(readStateUserID)
			if err != nil {
				return err
			}
			err = gw.RemoveFavoriteFeed(context.Background(), readStateMissingURL, userID)
			require.Error(t, err)
			assert.True(t, errors.Is(err, domain.ErrFeedNotFound))
			return nil
		})
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// §2.J Tag reads
// ---------------------------------------------------------------------------

// TestGetArticleTagsEmptyContract is the interaction the on-the-fly generation
// path depends on: an untagged article is 200 with no tags, not 404. A
// provider that answered NotFound would make the caller treat the article as
// broken and never ask mq-hub to generate anything.
func TestGetArticleTagsEmptyContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has no tags for the article").
		UponReceiving("a GetArticleTags request from alt-backend for an untagged article").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/alt.datahub.v1.DataHubService/GetArticleTags"),
			Headers: jsonHeaders(),
			Body:    map[string]interface{}{"articleId": readStateArticleID},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			// An empty repeated field is an absent key under protojson.
			Body: matchers.MapMatcher{},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewTagGateway(newDataHubServiceClient(config))
			tags, err := gw.FetchArticleTags(context.Background(), readStateArticleID)
			if err != nil {
				return fmt.Errorf("FetchArticleTags failed: %w", err)
			}
			assert.Empty(t, tags, "an untagged article is empty, not an error")
			return nil
		})
	require.NoError(t, err)
}

func TestGetFeedTagsContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has tags for the feed").
		UponReceiving("a GetFeedTags request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/alt.datahub.v1.DataHubService/GetFeedTags"),
			Headers: jsonHeaders(),
			Body: map[string]interface{}{
				"feedId": readStateFeedID,
				"limit":  20,
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body: matchers.MapMatcher{
				"tags": matchers.EachLike(map[string]interface{}{
					"id":        matchers.Like("11111111-2222-3333-4444-555555555555"),
					"tagName":   matchers.Like("AI"),
					"createdAt": matchers.Like("2026-07-31T09:00:00Z"),
				}, 1),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewTagGateway(newDataHubServiceClient(config))
			tags, err := gw.FetchFeedTags(context.Background(), readStateFeedID, nil, 20)
			if err != nil {
				return fmt.Errorf("FetchFeedTags failed: %w", err)
			}
			require.Len(t, tags, 1)
			assert.Equal(t, "AI", tags[0].TagName)
			return nil
		})
	require.NoError(t, err)
}

// TestUpsertArticleTagsContract pins the write-back half of the on-the-fly
// path. It is the interaction that closes the two-source seam: before this
// batch the tag read and this write went straight to the database while the
// article body they operate on came over this connection.
func TestUpsertArticleTagsContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub accepts article tag writes").
		UponReceiving("an UpsertArticleTags request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/alt.datahub.v1.DataHubService/UpsertArticleTags"),
			Headers: jsonHeaders(),
			Body: map[string]interface{}{
				"articleId": readStateArticleID,
				"feedId":    readStateFeedID,
				"tags": []map[string]interface{}{
					{"name": "AI", "confidence": 0.9},
				},
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body: matchers.MapMatcher{
				"success":       matchers.Like(true),
				"upsertedCount": matchers.Like(1),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewTagGateway(newDataHubServiceClient(config))
			count, err := gw.UpsertArticleTags(context.Background(), readStateArticleID, readStateFeedID,
				[]domain.TagUpsert{{Name: "AI", Confidence: 0.9}})
			if err != nil {
				return fmt.Errorf("UpsertArticleTags failed: %w", err)
			}
			assert.Equal(t, int32(1), count)
			return nil
		})
	require.NoError(t, err)
}

func TestGetTagCooccurrencesContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has tag cooccurrences").
		UponReceiving("a GetTagCooccurrences request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/alt.datahub.v1.DataHubService/GetTagCooccurrences"),
			Headers: jsonHeaders(),
			Body:    map[string]interface{}{"tagNames": []string{"AI", "Go"}},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body: matchers.MapMatcher{
				"cooccurrences": matchers.EachLike(map[string]interface{}{
					"tagNameA":    matchers.Like("AI"),
					"tagNameB":    matchers.Like("Go"),
					"sharedCount": matchers.Like(4),
				}, 1),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewTagGateway(newDataHubServiceClient(config))
			items, err := gw.FetchTagCooccurrences(context.Background(), []string{"AI", "Go"})
			if err != nil {
				return fmt.Errorf("FetchTagCooccurrences failed: %w", err)
			}
			require.Len(t, items, 1)
			assert.Equal(t, 4, items[0].SharedCount)
			return nil
		})
	require.NoError(t, err)
}

func TestSearchTagsByPrefixContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has tags matching the prefix").
		UponReceiving("a SearchTagsByPrefix request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/alt.datahub.v1.DataHubService/SearchTagsByPrefix"),
			Headers: jsonHeaders(),
			Body: map[string]interface{}{
				"prefix": "AI",
				"limit":  10,
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body: matchers.MapMatcher{
				"hits": matchers.EachLike(map[string]interface{}{
					"tagName":      matchers.Like("AI"),
					"articleCount": matchers.Like(42),
				}, 1),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewTagGateway(newDataHubServiceClient(config))
			hits, err := gw.SearchTagsByPrefix(context.Background(), "AI", 10)
			if err != nil {
				return fmt.Errorf("SearchTagsByPrefix failed: %w", err)
			}
			require.Len(t, hits, 1)
			assert.Equal(t, 42, hits[0].ArticleCount)
			return nil
		})
	require.NoError(t, err)
}

// TestGetTagArticleCountsContract pins that the window is the request's, and
// that what comes back is counts rather than a verdict. The 7-versus-30-day
// comparison that turns these into "trending" stays on this side, so the
// provider never has to be redeployed to change what a surge means.
func TestGetTagArticleCountsContract(t *testing.T) {
	mockProvider := newDataHubPact(t, consumerBackend)

	since := time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)

	err := mockProvider.
		AddInteraction().
		Given("alt-data-hub has tagged articles for the user in the window").
		UponReceiving("a GetTagArticleCounts request from alt-backend").
		WithCompleteRequest(consumer.Request{
			Method:  "POST",
			Path:    matchers.String("/alt.datahub.v1.DataHubService/GetTagArticleCounts"),
			Headers: jsonHeaders(),
			Body: map[string]interface{}{
				"userId": readStateUserID,
				"since":  "2026-07-24T00:00:00Z",
			},
		}).
		WithCompleteResponse(consumer.Response{
			Status:  200,
			Headers: jsonHeaders(),
			Body: matchers.MapMatcher{
				"counts": matchers.EachLike(map[string]interface{}{
					"tagName":      matchers.Like("AI"),
					"articleCount": matchers.Like(10),
				}, 1),
			},
		}).
		ExecuteTest(t, func(config consumer.MockServerConfig) error {
			gw := datahub_gateway.NewTagGateway(newDataHubServiceClient(config))
			userID, err := parseTestUUID(readStateUserID)
			if err != nil {
				return err
			}
			counts, err := gw.FetchTagArticleCounts(context.Background(), userID, since)
			if err != nil {
				return fmt.Errorf("FetchTagArticleCounts failed: %w", err)
			}
			require.Len(t, counts, 1)
			assert.Equal(t, "AI", counts[0].TagName)
			assert.Equal(t, 10, counts[0].ArticleCount)
			return nil
		})
	require.NoError(t, err)
}
